package controllers

import (
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"strings"

	// "fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"strconv"

	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	// "github.com/go-playground/validator/v10"
)

// ===================== OUTBOUND ====================
//Sale
func GetSaleDocuments(c *gin.Context) {
	q := c.Query("q")

	limit := 10
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	offset := (page - 1) * limit

	var sale_documents []models.SaleDocument
	var totalData int64

	baseQuery := config.DB.Model(&models.SaleDocument{}).Where("status = ?", "selesai")
	
	if q != "" {
		query := "%" + q + "%"
		baseQuery = baseQuery.Where("(code_document_sale LIKE ? OR buyer_name LIKE ?)", query, query)
	}

	if err := baseQuery.Session(&gorm.Session{}).Count(&totalData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if err := baseQuery.
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "name")
		}).
		Preload("Buyer", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "point_buyer")
		}).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&sale_documents).Error;  err != nil {

		c.JSON(500, gin.H{"success": false, "message": "Gagal mengambil data sale douments", "error": err.Error()})	
		return	
	}

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// ===== Response =====
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "List Data Sale Documents",
		"data": gin.H{
			"current_page": page,
			"per_page":     limit,
			"total":        totalData,
			"data":         sale_documents,
			"links" :		links,
		},
	})
}

func SaleIndex(c *gin.Context) {
	db := config.DB
	user := c.MustGet("auth_user").(models.User)

	limit := 50
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	offset := (page - 1) * limit

	// --- SALES ---
	type responseSale struct {
		ID				uint64 `json:"id"`
		Barcode			string `json:"barcode"`
		ProductName		string `json:"product_name"`
		Category		string `json:"category"`
		Qty				int64	`json:"qty"`
		Price			float64	`json:"price"`
	}

	var sales []responseSale
	var totalSale float64
	var totalDataSale int64

	baseQuery := db.Model(&models.Sale{}).
		Joins("LEFT JOIN products ON products.barcode = sales.barcode_item").
		Joins("LEFT JOIN bundles ON bundles.barcode = sales.barcode_item").
		Joins(`
			LEFT JOIN categories 
				ON categories.id = 
					CASE 
						WHEN products.id IS NOT NULL THEN products.category_id
						ELSE bundles.category_id
					END
		`).
		Where("sales.status_sale = ? AND sales.user_id = ?", "proses", user.ID)

	//hitung total sale
	if err := baseQuery.Session(&gorm.Session{}).
		Select("COALESCE(SUM(sales.product_price_sale),0)").
		Scan(&totalSale).Error; err != nil {

		c.JSON(500, gin.H{"success": false, "message": "gagal menghitung total sale", "error": err.Error()})
		return
	}

	//hitung total data sale
	if err := baseQuery.Session(&gorm.Session{}).Count(&totalDataSale).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "gagal menghitung total data sale", "error": err.Error()})
		return
	}

	if err := baseQuery.
		Select(`
			sales.id,
			sales.barcode_item AS barcode,
			COALESCE(products.name, bundles.name_bundle) AS product_name,
			categories.name_category AS category,
			COALESCE(products.quantity, bundles.total_product) AS qty,
			sales.product_price_sale AS price
		`).
		Order("sales.created_at DESC").
		Limit(limit).Offset(offset).
		Scan(&sales).Error; err != nil {

		c.JSON(500, gin.H{"success": false, "message": "gagal mengambil data sale", "error": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(totalDataSale) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// --- SALE DOCUMENT ---
	var saleDoc models.SaleDocument
	err := db.
		Where("status = ? AND user_id = ?", "proses", user.ID).
		First(&saleDoc).Error

	codeDocument := ""
	buyerName := ""
	address := ""
	phone := ""

	currentTransaction := 0
	monthlyPoint := 0
	monthlyRank := "-"
	buyerID := (*uint64)(nil)
	var nextRank models.LoyaltyRank
	var rank *models.LoyaltyRank

	var (
		nextRankName       interface{}
		transactionNext    int
		percentageDiscount float64
		rankName           interface{}
	)

	if errors.Is(err, gorm.ErrRecordNotFound) {
		code, err := helpers.GenerateCodeSaleDocument(db, uint64(user.ID))
		if err != nil {
			c.JSON(500, gin.H{"success": false, "message": "gagal generate code document", "error": err.Error()})
			return
		}

		codeDocument = code
	} else {
		codeDocument = saleDoc.CodeDocumentSale
		buyerName = saleDoc.BuyerName
		buyerID = &saleDoc.BuyerID
		address = saleDoc.BuyerAddress
		phone = saleDoc.BuyerPhone

		var buyer models.Buyer
		db.Preload("Rank").First(&buyer, *buyerID)
		
		currentTransaction = buyer.TransactionCount
		if buyer.LoyaltyRankID != nil {
			rank = buyer.Rank
		}

		// Next rank
		var errNextRank error
		if currentTransaction <= 1 {
			errNextRank = db.
				Where("min_transactions = ?", currentTransaction).
				Limit(1).
				First(&nextRank).Error
		}else {
			errNextRank = db.
				Where("min_transactions > ?", currentTransaction).
				Order("min_transactions ASC").
				Limit(1).
				First(&nextRank).Error
		}	
		
		if errNextRank == nil {
			nextRankName = nextRank.Rank
			transactionNext = max(1, nextRank.MinTransactions-currentTransaction)
		} else {
			nextRankName = nil
			transactionNext = 0
		}

		// Monthly point
		db.Model(&models.SaleDocument{}).
			Where(`
				buyer_id = ?
				AND status = 'selesai'
				AND MONTH(created_at) = MONTH(NOW())
				AND YEAR(created_at) = YEAR(NOW())
			`, *buyerID).
			Select("COALESCE(SUM(buyer_point),0)").
			Scan(&monthlyPoint)

		// Monthly rank
		var higherRankCount int64
		db.Raw(`
			SELECT COUNT(*) FROM (
				SELECT buyer_id
				FROM sale_documents
				WHERE status = 'selesai'
				AND MONTH(created_at) = MONTH(NOW())
				AND YEAR(created_at) = YEAR(NOW())
				GROUP BY buyer_id
				HAVING SUM(buyer_point) > ?
			) t
		`, monthlyPoint).Scan(&higherRankCount)

		monthlyRank = strconv.Itoa(int(higherRankCount + 1))
	}

	if rank != nil {
		rankName = rank.Rank
		percentageDiscount = rank.PercentageDiscount
	} else {
		rankName = nil
		percentageDiscount = 0
	}

	// --- RESPONSE ---
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "list data sale",
		"data": gin.H{
			"buyer_id": buyerID,
			"code_document_sale":     codeDocument,
			"buyer_address":          address,
			"buyer_phone":            phone,
			"sale_buyer_name":        buyerName,
			"total_sale":             totalSale,
			"rank":                   rankName,
			"next_rank":              nextRankName,
			"transaction_next": 	  transactionNext,
			"percentage_discount": percentageDiscount,
			"current_transaction":  currentTransaction,
			"monthly_point":        monthlyPoint,
			"monthly_rank_position": monthlyRank,
			"sales":                gin.H{
				"current_page": page,
				"per_page":     limit,
				"total":        totalDataSale,
				"data":         sales,
				"links" :		links,
			},
		},
	})
}

// func ShowSaleDocument(c *gin.Context) {
// 	id := c.Param("id")

// 	var sale models.SaleDocument
// 	if err := config.DB.
// 		Preload("Sales").
// 		Preload("User").
// 		Preload("Buyer.Rank").
// 		First(&sale, id).Error; err != nil {

// 		if errors.Is(err, gorm.ErrRecordNotFound) {
// 			c.JSON(http.StatusNotFound, gin.H{
// 				"success": false,
// 				"message": "Sale document tidak ditemukan",
// 			})
// 		}else {
// 			c.JSON(http.StatusNotFound, gin.H{
// 				"success": false,
// 				"message": "Gagal query sale document",
// 				"error": err.Error(),
// 			})
// 		}

// 		return
// 	}

// 	createdAt := sale.CreatedAt
// 	month := int(createdAt.Month())
// 	year := createdAt.Year()

// 	// Monthly Point
// 	var monthlyPoint int64
// 	if err := config.DB.
// 		Model(&models.SaleDocument{}).
// 		Where("buyer_id = ?", sale.BuyerID).
// 		Where("status = ?", "selesai").
// 		Where("MONTH(created_at) = ?", month).
// 		Where("YEAR(created_at) = ?", year).
// 		Select("COALESCE(SUM(buyer_point_document_sale),0)").
// 		Scan(&monthlyPoint).Error; err != nil {

// 		c.JSON(400, gin.H{
// 			"success": false,
// 			"message": "Gagal menghitung monthly point",
// 			"error": err.Error(),
// 		})
// 	}

// 	// Monthly Rank Position
// 	var higherRankCount int64
// 	if err := config.DB.
// 		Model(&models.SaleDocument{}).
// 		Select("buyer_id").
// 		Where("status = ?", "selesai").
// 		Where("MONTH(created_at) = ?", month).
// 		Where("YEAR(created_at) = ?", year).
// 		Group("buyer_id").
// 		Having("SUM(buyer_point) > ?", monthlyPoint).
// 		Count(&higherRankCount).Error; err != nil {

// 		c.JSON(400, gin.H{
// 			"success": false,
// 			"message": "Gagal menghitung ranking buyer",
// 			"error": err.Error(),
// 		})
// 	}

// 	monthlyRank := higherRankCount + 1

// 	// Loyalty Info saat transaksi
// 	loyalty, err := helpers.GetRankAtTransaction(
// 		sale.BuyerID,
// 		sale.CreatedAt,
// 	)

// 	// 4️⃣ Response
// 	c.JSON(http.StatusOK, responses.Success("data document sale", gin.H{
// 		"id":                 sale.ID,
// 		"code_document_sale": sale.CodeDocumentSale,
// 		"status":             sale.Status,
// 		"created_at":         sale.CreatedAt,
// 		"buyer": gin.H{
// 			"id":                     sale.Buyer.ID,
// 			"point_buyer":            sale.Buyer.PointBuyer,
// 			"rank":                   loyalty.Rank,
// 			"next_rank":              loyalty.NextRank,
// 			"transaction_next":       loyalty.TransactionNext,
// 			"percentage_discount":    loyalty.PercentageDiscount,
// 			"current_transaction":    loyalty.TransactionCount,
// 			"expire_date":            loyalty.ExpireDate,
// 			"monthly_point":          monthlyPoint,
// 			"monthly_rank_position":  monthlyRank,
// 		},
// 	}))
// }

func StoreProductToSale(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	type payloadRequest struct {
		SaleBarcode  string   `json:"sale_barcode" binding:"required"`
		BuyerID      uint64   `json:"buyer_id" binding:"required"`
		NewDiscount  *float64 `json:"new_discount" binding:"omitempty,gt=0"`
		TypeDiscount *string  `json:"type_discount" binding:"omitempty,oneof=new old"`
	}

	var req payloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {

		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Format JSON tidak valid",
			})
			return
		}

		errorsMap := make(map[string]string)

		for _, e := range ve {
			field := e.Field()

			switch field {
			case "SaleBarcode":
				errorsMap["sale_barcode"] = "Barcode wajib diisi"

			case "BuyerID":
				errorsMap["buyer_id"] = "Buyer wajib dipilih"

			case "NewDiscount":
				errorsMap["new_discount"] = "Diskon harus lebih dari 0"

			case "TypeDiscount":
				errorsMap["type_discount"] = "Type discount harus bernilai 'new' atau 'old'"

			default:
				errorsMap[strings.ToLower(field)] =
					"Validasi gagal pada field " + field
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Validasi gagal",
			"errors": errorsMap,
		})
		return
	}

	defer func() {
		if r := recover(); r != nil {

			stack := debug.Stack() // ← full stack trace

			// log ke file / stdout
			fmt.Printf("PANIC: %v\n%s\n", r, stack)

			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Terjadi kesalahan internal",
				// JANGAN kirim stack ke client di production
			})
		}
	}()

	// db := config.DB

	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start database transaction",
		})
		return
	}

	/* ===========================
	FIND PRODUCT OR BUNDLE
	=========================== */
	var (
		product       models.Product
		bundle        models.Bundle
		isBundle      bool
		barcodeItem   string
		newPrice      float64
		oldPrice      float64
		basePrice     float64
		discount      float64
		totalDiscount float64
	)

	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("ProductOld").
		Preload("Category").
		Where("barcode = ?", req.SaleBarcode).
		First(&product).Error

	if err != nil {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Category").
			Where("barcode = ?", req.SaleBarcode).
			First(&bundle).Error; err != nil {

			tx.Rollback()
			c.JSON(404, gin.H{
				"success": false,
				"message": "Produk / bundle tidak ditemukan",
			})
			return
		}
		isBundle = true
	}

	if (!isBundle && product.Status == "sale") || (isBundle && bundle.Status == "sale") {
		tx.Rollback()
		c.JSON(400, gin.H{
			"success": false,
			"message": "Product / bundle sudah dimasukkan ke penjualan",
		})
		return
	}

	/* ===========================
	BUYER
	=========================== */
	var buyer models.Buyer
	if err := tx.Preload("Rank").First(&buyer, req.BuyerID).Error; err != nil {
		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": "Buyer tidak ditemukan",
		})
		return
	}

	/* ===========================
	SALE DOCUMENT
	=========================== */
	var saleDoc models.SaleDocument
	err = tx.Where("user_id = ? AND status = 'proses'", user.ID).
		First(&saleDoc).Error

	if err != nil {
		if err != gorm.ErrRecordNotFound {
			tx.Rollback()
			c.JSON(500, gin.H{"success": false, "message": err.Error()})
			return
		}

		code, _ := helpers.GenerateCodeSaleDocument(tx, uint64(user.ID))
		saleDoc = models.SaleDocument{
			UserID:           uint64(user.ID),
			CodeDocumentSale: code,
			BuyerID:          buyer.ID,
			BuyerName:        buyer.NameBuyer,
			BuyerPhone:       buyer.PhoneBuyer,
			BuyerAddress:     buyer.AddressBuyer,
			Status:           "proses",
			NewDiscountSale:  req.NewDiscount,
			TypeDiscount:     req.TypeDiscount,
		}

		if err := tx.Create(&saleDoc).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"success": false, "message": "Gagal membuat sale document"})
			return
		}
	}

	/* ===========================
	PRICE SETUP
	=========================== */
	if isBundle {
		oldPrice = bundle.TotalPrice
		basePrice = bundle.TotalPriceCustom
		newPrice = basePrice
		barcodeItem = bundle.Barcode
		totalDiscount = 0.0

		discount := product.ProductOld.OldPriceProduct * (float64(product.Category.DiscountCategory)/100.0)
		discount = math.Round(discount)
		if discount > product.Category.MaxPriceCategory {
			discount = product.Category.MaxPriceCategory
		} 

		expectedPrice := product.ProductOld.OldPriceProduct - discount

		if bundle.TotalPriceCustom != expectedPrice {
			tx.Rollback()
			c.JSON(400, gin.H{
				"success": false,
				"message": "Harga bundle tidak sesuai",
				"barcode": barcodeItem,
				"price_now": bundle.TotalPriceCustom,
				"expected_price": expectedPrice,
			})
			return
		}
	} else {
		oldPrice = product.ProductOld.OldPriceProduct
		basePrice = product.DisplayPrice
		newPrice = product.Price
		barcodeItem = product.Barcode
		totalDiscount = product.Price - product.DisplayPrice

		if product.Discount != nil {
			discount = *product.Discount
		}

		discount := product.ProductOld.OldPriceProduct * (float64(product.Category.DiscountCategory)/100.0)
		discount = math.Round(discount)
		if discount > product.Category.MaxPriceCategory {
			discount = product.Category.MaxPriceCategory
		} 
		expectedPrice := product.ProductOld.OldPriceProduct - discount

		if product.Price != expectedPrice {
			tx.Rollback()
			c.JSON(400, gin.H{
				"success": false,
				"message": "Harga product tidak sesuai",
				"barcode": barcodeItem,
				"price_now": product.Price,
				"expected_price": expectedPrice,
				"discount": discount,
			})
			return
		}
	}

	productPriceSale := basePrice

	if req.NewDiscount != nil && *req.NewDiscount > 0 {
		discount = *req.NewDiscount

		if req.TypeDiscount != nil && *req.TypeDiscount == "new" {
			totalDiscount = newPrice * discount / 100
			productPriceSale = newPrice - totalDiscount
		} else {
			totalDiscount = oldPrice * discount / 100
			productPriceSale = oldPrice - totalDiscount
		}

		basePrice = productPriceSale
	}

	/* ===========================
	TOTAL SALE CHECK
	=========================== */
	var totalPriceSale float64
	if err := tx.Model(&models.Sale{}).
		Select("COALESCE(SUM(base_price),0)").
		Where("user_id = ? AND status_sale = 'proses'", user.ID).
		Scan(&totalPriceSale).Error; err != nil {

		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": err.Error()})
		return
	}

	newTotal := totalPriceSale + basePrice

	if newTotal >= 5000000 {
		var discountLoyalty float64

		if buyer.TransactionCount == 0 {
			discountLoyalty = 0
		} else {
			discountLoyalty = buyer.Rank.PercentageDiscount
		}

		if discountLoyalty > 0 {
			if err := tx.Model(&models.Sale{}).
				Where("sale_document_id = ?", saleDoc.ID).
				Update(
					"product_price_sale",
					gorm.Expr("base_price * (1 - ? / 100)", discountLoyalty),
				).Error; err != nil {

				tx.Rollback()
				c.JSON(500, gin.H{"success": false, "message": err.Error()})
				return
			}

			loyaltyDiscount := basePrice * (discountLoyalty / 100)
			totalDiscount += loyaltyDiscount
			productPriceSale = basePrice - loyaltyDiscount
		}
	}

	/* ===========================
	INSERT SALE
	=========================== */
	sale := models.Sale{
		UserID:            uint64(user.ID),
		SaleDocumentID:    saleDoc.ID,
		BarcodeItem:       barcodeItem,
		ProductPriceSale:  math.Ceil(productPriceSale),
		BasePrice:         math.Ceil(basePrice),
		TotalDiscountSale: math.Ceil(totalDiscount),
		DiscountSale:      discount,
		TypeDiscount:      req.TypeDiscount,
		StatusSale:        "proses",
	}

	if err := tx.Create(&sale).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal insert sale"})
		return
	}

	/* ===========================
	UPDATE STATUS
	=========================== */
	if !isBundle {
		if err := tx.Model(&product).Update("status", "sale").Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"success": false, "message": err.Error()})
			return
		}
	} else {
		if err := tx.Model(&bundle).Update("status", "sale").Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"success": false, "message": err.Error()})
			return
		}
	}

	/* ===========================
	COMMIT
	=========================== */
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal commit transaksi"})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "Item berhasil ditambahkan ke penjualan",
	})

}

func UpdatePriceSale(c *gin.Context) {
	var sale models.Sale

	// ===========================
	// Ambil sale (route param)
	// ===========================
	if err := config.DB.
		Where("id = ? AND status_sale = 'proses'", c.Param("sale_id")).
		First(&sale).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {		
			c.JSON(404, gin.H{
				"success": false,
				"message": "sale tidak ditemukan",
			})
		}else {
			c.JSON(404, gin.H{
				"success": false,
				"message": "gagal memuat data sale",
				"error": err.Error(),
			})
		}

		return
	}

	// ===========================
	// Validasi input
	// ===========================
	type payload struct {
		UpdatePriceSale float64 `json:"update_price_sale" binding:"required,numeric"`
	}

	var req payload
	if err := c.ShouldBindJSON(&req); err != nil {

		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Format JSON tidak valid",
			})
			return
		}

		errorsMap := make(map[string]string)

		for _, e := range ve {
			field := e.Field()

			switch field {
			case "UpdatePriceSale":
				errorsMap["update_price_sale"] = "Update price sale wajib diisi dan berupa angka"

			default:
				errorsMap[strings.ToLower(field)] =
					"Validasi gagal pada field " + field
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Validasi gagal",
			"errors": errorsMap,
		})
		return
	}

	// ===========================
	// Hitung selisih harga
	// ===========================
	// positif  -> harga naik
	// negatif  -> harga turun
	updateDiff := req.UpdatePriceSale - sale.ProductPriceSale

	// ===========================
	// 4. Update data sale
	// ===========================
	if err := config.DB.Model(&sale).Updates(map[string]interface{}{
		"product_update_price_sale": updateDiff,
		"product_price_sale":        req.UpdatePriceSale,
	}).Error; err != nil {

		c.JSON(500, gin.H{
			"success": false,
			"message": "gagal update data",
			"error":   err.Error(),
		})
		return
	}

	// ===========================
	// Response sukses
	// ===========================
	c.JSON(200, gin.H{
		"success": true,
		"message": "data berhasil di update",
		"data":    sale,
	})
}

func DestroySale(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
	saleID := c.Param("sale_id")

	db := config.DB

	err := db.Transaction(func(tx *gorm.DB) error {

		/* ===========================
		   GET SALE (LOCKED)
		=========================== */
		var sale models.Sale
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND status_sale = 'proses'", saleID, user.ID).
			First(&sale).Error; err != nil {

			return errors.New("sale tidak ditemukan")
		}

		sale_document_id := sale.SaleDocumentID

		/* ===========================
		   GET ALL SALE IN DOCUMENT
		=========================== */
		var sales []models.Sale
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("sale_document_id = ? AND user_id = ? AND status_sale = 'proses'",
				sale_document_id, user.ID).
			Find(&sales).Error; err != nil {
			return err
		}

		/* ===========================
		   HITUNG TOTAL
		=========================== */
		var totalBefore float64
		for _, s := range sales {
			totalBefore += s.ProductPriceSale
		}

		totalAfter := totalBefore - sale.ProductPriceSale

		/* ===========================
		   ROLLBACK DISCOUNT
		=========================== */
		if totalAfter < 5000000 {
			if err := tx.Model(&models.Sale{}).
				Where("sale_document_id = ? AND user_id = ? AND status_sale = 'proses'",
					sale_document_id, user.ID).
				Updates(map[string]interface{}{
					"product_price_sale": gorm.Expr("base_price"),
					"product_update_price_sale": nil,
					"total_discount_sale": gorm.Expr(
						"total_discount_sale - (base_price - product_price_sale)",
					),
				}).Error; err != nil {

				return err
			}
		}

		/* ===========================
		   RESET PRODUCT STATUS
		=========================== */
		res := tx.Model(&models.Product{}).
			Where("barcode = ?", sale.BarcodeItem).
			Update("status", "display")

		if res.Error != nil {
			return res.Error
		}

		if res.RowsAffected == 0 {
			res = tx.Model(&models.Bundle{}).
				Where("barcode = ?", sale.BarcodeItem).
				Update("status", "not sale")

			if res.Error != nil {
				return res.Error
			}

			if res.RowsAffected == 0 {
				return fmt.Errorf(
					"barcode %s tidak ditemukan di product maupun bundle",
					sale.BarcodeItem,
				)
			}
		}

		/* ===========================
		   DELETE SALE
		=========================== */
		if err := tx.Delete(&sale).Error; err != nil {
			return err
		}

		/* ===========================
		   DELETE SALE DOCUMENT
		=========================== */
		if len(sales) <= 1 {
			if err := tx.
				Where("id = ? AND user_id = ?",
					sale_document_id, user.ID).
				Delete(&models.SaleDocument{}).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Data berhasil dihapus",
	})
}

func SaleFinish(c *gin.Context) {
	type payloadRequest struct {
		Voucher            *float64 `json:"voucher"`
		CardboxQty         *int64   `json:"cardbox_qty" binding:"required_with=CardboxUnitPrice"`
		CardboxUnitPrice   *float64 `json:"cardbox_unit_price" binding:"required_with=CardboxQty"`
		Tax                *float64 `json:"tax" binding:"omitempty,min=0,max=50"`
		IsTax              *int     `json:"is_tax"`
	}

	// ===========================
	// PANIC RECOVER
	// ===========================
	defer func() {
		if r := recover(); r != nil {
			c.JSON(500, gin.H{
				"success": false,
				"message": "Terjadi kesalahan internal",
				"error":   fmt.Sprintf("%v", r),
			})
		}
	}()

	user := c.MustGet("auth_user").(models.User)

	var req payloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {

		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Format JSON tidak valid",
			})
			return
		}

		errorsMap := make(map[string]string)

		for _, e := range ve {
			field := e.Field()
			tag := e.Tag()

			switch field {

			case "CardboxQty":
				switch tag {
				case "required_with":
					errorsMap["cardbox_qty"] = "Cardbox qty wajib diisi jika cardbox unit price diisi"
				default:
					errorsMap["cardbox_qty"] = "Cardbox qty tidak valid"
				}

			case "CardboxUnitPrice":
				switch tag {
				case "required_with":
					errorsMap["cardbox_unit_price"] = "Cardbox unit price wajib diisi jika cardbox qty diisi"
				default:
					errorsMap["cardbox_unit_price"] = "Cardbox unit price tidak valid"
				}

			case "Tax":
				switch tag {
				case "min":
					errorsMap["tax"] = "Tax minimal 0%"
				case "max":
					errorsMap["tax"] = "Tax maksimal 50%"
				default:
					errorsMap["tax"] = "Tax tidak valid"
				}

			default:
				errorsMap[strings.ToLower(field)] =
					"Validasi gagal pada field " + field
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Validasi gagal",
			"errors":  errorsMap,
		})
		return
	}


	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}

	// ===========================
	// SALE DOCUMENT
	// ===========================
	var saleDocument models.SaleDocument
	if err := tx.
		Where("status = 'proses' AND user_id = ?", user.ID).
		First(&saleDocument).Error; err != nil {

		tx.Rollback()
		c.JSON(400, gin.H{"success": false, "message": "Data sale belum dibuat"})
		return
	}

	// ===========================
	// SALES
	// ===========================
	var sales []models.Sale
	if err := tx.
		Where("sale_document_id = ?", saleDocument.ID).
		Find(&sales).Error; err != nil || len(sales) == 0 {

		tx.Rollback()
		c.JSON(400, gin.H{"success": false, "message": "Tidak ada produk dalam sale"})
		return
	}

	// ===========================
	// APPROVAL LOGIC
	// ===========================
	approved := "0"

	for _, s := range sales {
		if s.GaborSale != nil || s.ProductUpdatePriceSale != nil {
			approved = "1"
			tx.Model(&models.Sale{}).
				Where("id = ?", s.ID).
				Update("approved", "1")
		} else {
			tx.Model(&models.Sale{}).
				Where("id = ?", s.ID).
				Update("approved", "0")
		}
	}

	if req.Voucher != nil && *req.Voucher > 0 {
		approved = "1"
	}

	if saleDocument.NewDiscountSale != nil && *saleDocument.NewDiscountSale > 0 {
		approved = "1"
	}

	// ===========================
	// NOTIFICATION
	// ===========================
	if approved == "1" {
		sale_doc_id := uint(saleDocument.ID)
		if err := tx.Create(&models.Notification{
			UserID:           user.ID,
			NotificationName: "approve discount sale",
			Status:           "sale",
			Role:             "Spv",
			ExternalID:       &sale_doc_id,
		}).Error; err != nil {

			tx.Rollback()
			c.JSON(500, gin.H{"success": false, "message": "Gagal membuat notifikasi"})
			return
		}

		// tx.Model(&saleDocument).Update("approved", "1")
	}

	// ===========================
	// TOTAL HITUNGAN
	// ===========================
	var totalDisplay, totalPrice, totalOld float64

	tx.Model(&models.Sale{}).
		Where("sale_document_id = ?", saleDocument.ID).
		Select("COALESCE(SUM(base_price),0)").Scan(&totalDisplay)

	tx.Model(&models.Sale{}).
		Where("sale_document_id = ?", saleDocument.ID).
		Select("COALESCE(SUM(product_price_sale),0)").Scan(&totalPrice)

	tx.Model(&models.Sale{}).
		Joins("JOIN products ON products.id = sales.product_id").
		Joins("JOIN product_olds ON product_olds.product_id = products.id").
		Where("sales.sale_document_id = ?", saleDocument.ID).
		Select("COALESCE(SUM(product_olds.old_price_product), 0)").
		Scan(&totalOld)

	if req.Voucher != nil {
		totalPrice -= *req.Voucher

		if totalPrice < 0 {
			totalPrice = 0
		}
	}

	earnPoint := int64(math.Floor(totalPrice / 1000))
	cardboxTotal := float64(0)
	if req.CardboxQty != nil && req.CardboxUnitPrice != nil {
		cardboxTotal = float64(*req.CardboxQty) * *req.CardboxUnitPrice
	}

	grandTotal := totalPrice + cardboxTotal
	
	// ===========================
	// TAX
	// ===========================
	taxPercent := float64(0)
	priceAfterTax := grandTotal

	if req.IsTax != nil && *req.IsTax == 1 {
		if req.Tax == nil {
			tx.Rollback()
			c.JSON(422, gin.H{"success": false, "message": "Tax wajib diisi"})
			return
		}
		taxPercent = *req.Tax
		priceAfterTax += grandTotal * (taxPercent / 100)
	}

	// ===========================
	// UPDATE STATUS SALE
	// ===========================
	tx.Model(&models.Sale{}).
		Where("sale_document_id = ?", saleDocument.ID).
		Update("status_sale", "selesai")

	// ===========================
	// BUYER
	// ===========================
	var buyer models.Buyer
	if err := tx.First(&buyer, saleDocument.BuyerID).Error; err != nil {
		tx.Rollback()
		c.JSON(404, gin.H{"success": false, "message": "Buyer tidak ditemukan"})
		return
	}
	// currentTransaction := buyer.TransactionCount + 1

	// ===========================
	// Loyalty rank
	// ===========================
	if err := helpers.ProcessLoyalty(tx, &buyer, totalDisplay); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal Proses Loyalty buyer", "error": err.Error()})
		return
	}
	// ===========================
	// Update Data Buyer
	// ===========================
	type BuyerStats struct {
		AvgPurchase    float64
		TotalTransaksi int64
	}

	var stats BuyerStats
	if err := tx.Model(&models.SaleDocument{}).
		Select(`
			COALESCE(AVG(total_price), 0) AS avg_purchase,
			COUNT(*) AS total_transaksi
		`).
		Where("buyer_id = ?", buyer.ID).
		Where("status = ?", "selesai").
		Scan(&stats).Error; err != nil {

		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	typeBuyer := "Biasa"
	if stats.TotalTransaksi == 2 || stats.TotalTransaksi == 3 {
		typeBuyer = "Repeat"
	} else if stats.TotalTransaksi > 3 {
		typeBuyer = "Reguler"
	}

	if err := tx.Model(&buyer).Updates(map[string]interface{}{
		"type_buyer":               typeBuyer,
		"amount_transaction_buyer": gorm.Expr("amount_transaction_buyer + 1"),
		"amount_purchase_buyer":    gorm.Expr("amount_purchase_buyer + ?", totalPrice),
		"avg_purchase_buyer":       stats.AvgPurchase,
		"point_buyer":              gorm.Expr("point_buyer + ?", earnPoint),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "gagal update data buyer", "error": err.Error()})
		return
	}

	// ===========================
	// UPDATE SALE DOCUMEN
	// ===========================
	errSaleDoc := tx.Model(&saleDocument).Updates(map[string]interface{}{
		"buyer_point":     			earnPoint,
		"total_product":   			len(sales),
		"total_old_price": 			totalOld,
		"total_price":     			totalPrice,
		"total_display":   			totalDisplay,
		"status":          			"selesai",
		"cardbox_qty":                   req.CardboxQty,
		"cardbox_unit_price":            req.CardboxUnitPrice,
		"cardbox_total_price":           cardboxTotal,
		"voucher":                       req.Voucher,
		"approved":                      approved,
		"is_tax":                        req.IsTax,
		"tax":                           taxPercent,
		"price_after_tax":               math.Ceil(priceAfterTax),
	}).Error

	if errSaleDoc != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal update sale document", "error": errSaleDoc.Error()})
		return
	}

	if err := helpers.LogUserAction(user.ID, user.Name, fmt.Sprintf("Melakukan sale. Code Sale Document: %s", saleDocument.CodeDocumentSale), "outbound/sale/kasir", map[string]interface{}{}); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal membuat user log action",
			"error": err.Error(),
		})		

		return
	}
	// ===========================
	// COMMIT
	// ===========================
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal commit transaksi"})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "Data berhasil disimpan",
		"data":    saleDocument,
	})
}



