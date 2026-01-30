package controllers

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	// "fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"strconv"

	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/xuri/excelize/v2"
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

func DetailSaleDocument(c *gin.Context) {
	id := c.Param("id")

	var sale models.SaleDocument
	if err := config.DB.
		Preload("Sales").
		Preload("User").
		Preload("Buyer").
		First(&sale, id).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Sale document tidak ditemukan",
			})
		}else {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Gagal query sale document",
				"error": err.Error(),
			})
		}

		return
	}

	createdAt := sale.CreatedAt
	month := int(createdAt.Month())
	year := createdAt.Year()

	// Monthly Point
	var monthlyPoint int64
	if err := config.DB.
		Model(&models.SaleDocument{}).
		Where("buyer_id = ?", sale.BuyerID).
		Where("status = ?", "selesai").
		Where("MONTH(created_at) = ?", month).
		Where("YEAR(created_at) = ?", year).
		Select("COALESCE(SUM(buyer_point),0)").
		Scan(&monthlyPoint).Error; err != nil {

		c.JSON(400, gin.H{
			"success": false,
			"message": "Gagal menghitung monthly point",
			"error": err.Error(),
		})
	}

	// Monthly Rank Position
	var higherRankCount int64
	if err := config.DB.
		Model(&models.SaleDocument{}).
		Select("buyer_id").
		Where("status = ?", "selesai").
		Where("MONTH(created_at) = ?", month).
		Where("YEAR(created_at) = ?", year).
		Group("buyer_id").
		Having("SUM(buyer_point) > ?", monthlyPoint).
		Count(&higherRankCount).Error; err != nil {

		c.JSON(400, gin.H{
			"success": false,
			"message": "Gagal menghitung ranking buyer",
			"error": err.Error(),
		})

		return
	}

	monthlyRank := higherRankCount + 1

	// Gunakan helper function untuk mendapatkan rank info SAMPAI transaksi ini
    // Passing created_at untuk mendapatkan state pada saat transaksi ini terjadi
	rankInfo, err := helpers.GetCurrentRankInfo(
		config.DB, 
		uint(sale.BuyerID),
		sale.CreatedAt, 
	)

	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal mengambil rank info", "error": err.Error()})
		return
	}

	//data transactionCount adalah data stelah transaksi ini
	transactionCountAfter := rankInfo.TransactionCount
	expiredDate := rankInfo.ExpireDate
	transactionCountBefore := max(0, transactionCountAfter - 1)

	//cari rank sebelum transaksi ini
	var rankAtTransaction models.LoyaltyRank
	if err := config.DB.Where("min_transactions <= ?", transactionCountBefore).
		Order("min_transactions DESC").First(&rankAtTransaction).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal mengambil rank transaksi", "error": err.Error()})
		return
	}

	var nextRankAtTransaction models.LoyaltyRank
	if err := config.DB.Where("min_transactions > ?", transactionCountBefore).
		Order("min_transactions ASC").First(&nextRankAtTransaction).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal mengambil next rank", "error": err.Error()})
		return
	}

	var formattedExpireDate interface{}
	if expiredDate != nil {
		formattedExpireDate = expiredDate.Format("2006-01-02 15:04:05")
	} else {
		formattedExpireDate = nil // atau "" jika ingin string kosong
	}

	buyerData := map[string]interface{}{
		"id":					sale.Buyer.ID,
		"point_buyer": 			sale.Buyer.PointBuyer,
		"rank":					rankAtTransaction.Rank,
		"next_rank":			nextRankAtTransaction.Rank,
		"transaction_next":		max(0, nextRankAtTransaction.MinTransactions - transactionCountAfter),
		"percentage_discount":	rankAtTransaction.PercentageDiscount,
		"current_transaction":	transactionCountAfter,
		"expire_date":			formattedExpireDate,
		"monthly_point":		monthlyPoint,
		"monthly_rank_position": monthlyRank,
	}

	type saleItemResponse struct {
		ID          uint64  `json:"id"`
		Barcode		string	`json:"barcode"`
		NameProduct string  `json:"name_product"`
		Category	string	`json:"category"`
		Qty         int64   `json:"qty"`
		ProductPriceSale       float64 `json:"product_price_sale"`
	}

	var sales []saleItemResponse
	for _, s := range sale.Sales {
		item := saleItemResponse{
			ProductPriceSale:  s.ProductPriceSale,
			NameProduct: s.ProductName,
			Category: s.ProductCategory,
			Qty: s.ProductQuantity,

		}

		if s.ProductBarcode != nil {
			item.Barcode = *s.ProductBarcode
		} else {
			item.Barcode = *s.BundleBarcode
		}

		sales = append(sales, item)
	}



	// 4️⃣ Response
	c.JSON(http.StatusOK, gin.H{
		"success":                 true,
		"message": "Detail sale document",
		"resource": gin.H{
			"id":                sale.ID,
			"user_id":           sale.UserID,
			"code_document_sale": sale.CodeDocumentSale,
			"buyer_id":      sale.BuyerID,
			"buyer_name":    sale.BuyerName,
			"buyer_phone":   sale.BuyerPhone,
			"buyer_address": sale.BuyerAddress,
			"buyer_point":   sale.BuyerPoint,
			"new_discount_sale": sale.NewDiscountSale,
			"type_discount":     sale.TypeDiscount,
			"total_product":     sale.TotalProduct,
			"total_old_price":   sale.TotalOldPrice,
			"total_price":       sale.TotalPrice,
			"total_display_price":     sale.TotalDisplayPrice,
			"status": sale.Status,
			"cardbox_qty":         sale.CardboxQty,
			"cardbox_unit_price": sale.CardboxUnitPrice,
			"cardbox_total_price": sale.CardboxTotalPrice,
			"voucher": sale.Voucher,
			"approved": sale.Approved,
			"is_tax": sale.IsTax,
			"tax":    sale.Tax,
			"price_after_tax": sale.PriceAfterTax,
			"grand_total_price":     sale.GrandTotalPrice,
			"created_at": sale.CreatedAt,
			"updated_at": sale.UpdatedAt,
			"sales": sales,
			"user":  sale.User,
			"buyer": buyerData, // hasil DTO / enrichment buyer
		},
	})
}

func AddProductToSaleDocument(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	type payloadRequest struct {
		SaleBarcode  string   `json:"sale_barcode" binding:"required"`
		SaleDocumentID      uint64   `json:"sale_document_id" binding:"required,numeric"`
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

			case "SaleDocumentID":
				errorsMap["sale_document_id"] = "Sale Document ID wajib diisi dan berupa numerik"

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
		itemStatusBefore   string

		productName string
		productCategory string
		productQuantity int64

		newPrice      float64
		oldPrice      float64
		basePrice     float64
		discount      float64
		totalDiscount float64
	)

	var saleDoc models.SaleDocument
	if err := tx.Where("id = ?", req.SaleDocumentID).Where("user_id = ?", user.ID).First(&saleDoc).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"success": false, "message": "sale document tidak ditemukan"})
		}else {
			c.JSON(500, gin.H{"success": false, "message": "gagal mengambil data sale document", "error": err.Error()})
		}

		return
	}

	if saleDoc.Status != "selesai" {
		c.JSON(400, gin.H{"success": false, "message": "sale document masih dalam proses"})
		return
	}

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
	if err := tx.Preload("Rank").First(&buyer, saleDoc.BuyerID).Error; err != nil {
		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": "Buyer tidak ditemukan",
		})
		return
	}

	/* ===========================
	PRICE SETUP / DEFAULT VALUE
	=========================== */
	if isBundle {
		productName = bundle.NameBundle
		productCategory = bundle.Category.NameCategory
		productQuantity = bundle.TotalProduct

		itemStatusBefore = bundle.Status
		oldPrice = bundle.TotalPrice
		basePrice = bundle.TotalPriceCustom
		newPrice = basePrice
		barcodeItem = bundle.Barcode
		totalDiscount = 0.0

		category_discount := bundle.TotalPrice * (float64(bundle.Category.DiscountCategory)/100.0)
		category_discount = math.Round(category_discount)
		if category_discount > bundle.Category.MaxPriceCategory {
			category_discount = bundle.Category.MaxPriceCategory
		} 

		expectedPrice := bundle.TotalPrice - category_discount

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
		productName = product.Name
		productCategory = product.Category.NameCategory
		productQuantity = product.Quantity

		itemStatusBefore = product.Status
		oldPrice = product.ProductOld.OldPriceProduct
		basePrice = product.DisplayPrice
		newPrice = product.Price
		barcodeItem = product.Barcode
		totalDiscount = product.Price - product.DisplayPrice

		if product.Discount != nil {
			discount = *product.Discount
		}

		category_discount := product.ProductOld.OldPriceProduct * (float64(product.Category.DiscountCategory)/100.0)
		category_discount = math.Round(category_discount)
		if category_discount > product.Category.MaxPriceCategory {
			category_discount = product.Category.MaxPriceCategory
		} 
		expectedPrice := product.ProductOld.OldPriceProduct - category_discount

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

	if *saleDoc.NewDiscountSale > 0 {
		discount = *saleDoc.NewDiscountSale

		if saleDoc.TypeDiscount != nil && *saleDoc.TypeDiscount == "new" {
			totalDiscount = newPrice * discount / 100
			productPriceSale = newPrice - totalDiscount
		} else {
			totalDiscount = oldPrice * discount / 100
			productPriceSale = oldPrice - totalDiscount
		}

		basePrice = productPriceSale
	}

	newTotalDisplayPrice := saleDoc.TotalDisplayPrice + basePrice

	if saleDoc.TotalDisplayPrice >= 5000000 {
		var rankBefore models.LoyaltyRank
		// abaikan discount loyalty jika buyer rank sebelumnya new buyer
		// transaction count = 2, artinya buyer baru naik rank bronze saat ini, sebelumnya new buyer 
		if  buyer.Rank != nil && buyer.Rank.Rank != "New Buyer" && buyer.TransactionCount > 2 {
			transaction_count_before := buyer.TransactionCount - 1

			if err := tx.
				Where("min_transactions <= ?", transaction_count_before).
				Order("min_transactions DESC").
				Limit(1).
				First(&rankBefore).Error; err != nil {
				c.JSON(400, gin.H{"success": false, "message": "Gagal mengambil rank buyer", "error": err.Error()})

				return
			}

			discountLoyalty := rankBefore.PercentageDiscount
			if discountLoyalty > 0 {
				loyaltyDiscount := basePrice * (discountLoyalty / 100)
				totalDiscount += loyaltyDiscount
				productPriceSale = basePrice - loyaltyDiscount
			}
		}

	}else {
		// jika total price Baru tembus 5jt setelah produk ini ditambahkan
		if newTotalDisplayPrice >= 5000000 {
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
	}

	/* ===========================
	INSERT SALE
	=========================== */
	sale := models.Sale{
		UserID:            uint64(user.ID),
		SaleDocumentID:    saleDoc.ID,
		ProductName: productName,
		ProductCategory: productCategory,
		ProductQuantity: productQuantity,
		ProductOldPrice: oldPrice,
		ProductPrice: newPrice,
		ProductPriceSale:  math.Ceil(productPriceSale),
		BasePrice:         math.Ceil(basePrice),
		TotalDiscountSale: math.Ceil(totalDiscount),
		DiscountSale:      discount,
		TypeDiscount:      saleDoc.TypeDiscount,
		StatusSale:        "selesai",	
		ProductStatusBefore: itemStatusBefore,
	}

	if isBundle {
		sale.BundleBarcode = &barcodeItem
		sale.ProductBarcode = nil
	}else {
		sale.BundleBarcode = nil
		sale.ProductBarcode = &barcodeItem
	}

	if err := tx.Create(&sale).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal insert sale", "error": err.Error()})
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

	newTotalPrice := saleDoc.TotalPrice + productPriceSale
	priceAfterTax := float64(0)

	//tamabah biaya karton box
	grandTotal := newTotalPrice + saleDoc.CardboxTotalPrice

	//Hitung Pajak jika pakek tax
	if saleDoc.IsTax == true && saleDoc.Tax != nil {
		tax := grandTotal * (*saleDoc.Tax / 100.0)
		priceAfterTax = grandTotal + tax
	}

	earnPoint := int64(math.Floor(newTotalPrice / 1000))

	// ===========================
	// Loyalty rank
	// ===========================
	if saleDoc.TotalPrice < 5000000 && newTotalDisplayPrice >= 5000000 {		
		if err := helpers.ProcessLoyalty(tx, &buyer, newTotalDisplayPrice); err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"success": false, "message": "Gagal Proses Loyalty buyer", "error": err.Error()})
			return
		}
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
		"amount_purchase_buyer":    gorm.Expr("amount_purchase_buyer + ?", newTotalPrice),
		"avg_purchase_buyer":       stats.AvgPurchase,
		"point_buyer":              gorm.Expr("point_buyer = ?", earnPoint),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "gagal update data buyer", "error": err.Error()})
		return
	}

	// ===========================
	// UPDATE SALE DOCUMEN
	// ===========================
	errSaleDoc := tx.Model(&saleDoc).Updates(map[string]interface{}{
		"buyer_point":     			earnPoint,
		"total_product":   			gorm.Expr("total_product + 1"),
		"total_old_price": 			gorm.Expr("total_old_price + ?", oldPrice),
		"total_price":     			newTotalPrice,
		"total_display_price":   	newTotalDisplayPrice,
		"grand_total_price":   		grandTotal,
		"price_after_tax":          math.Ceil(priceAfterTax),
	}).Error

	if errSaleDoc != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal update sale document", "error": errSaleDoc.Error()})
		return
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

func UpdateSaleDocument(c *gin.Context) {
	var saleDocument models.SaleDocument
	user := c.MustGet("auth_user").(models.User)

	// Ambil ID dari param
	id := c.Param("sale_doc_id")

	// Cari SaleDocument
	if err := config.DB.Where("id = ?", id).Where("user_id = ?", user.ID).First(&saleDocument).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"success": false, "message": "sale document tidak ditemukan"})
		}else {
			c.JSON(500, gin.H{"success": false, "message": "gagal mengambil data sale document", "error": err.Error()})
		}

		return
	}

	if saleDocument.Status != "selesai" {
		c.JSON(400, gin.H{"success": false, "message": "sale document masih dalam proses"})
	}

	// =========================
	// Request Validation
	// =========================
	type Request struct {
		CardboxQty       int `json:"cardbox_qty" binding:"required,numeric"`
		CardboxUnitPrice float64 `json:"cardbox_unit_price" binding:"required,numeric"`
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status":  false,
			"message": "Input tidak valid!",
			"errors":  err.Error(),
		})
		return
	}

	// =========================
	// Cek perubahan data
	// =========================
	if req.CardboxQty == saleDocument.CardboxQty &&
		req.CardboxUnitPrice == saleDocument.CardboxUnitPrice {

		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": "Data tidak ada yang berubah!",
			"data":    saleDocument,
		})
		return
	}

	// =========================
	// Hitung Cardbox
	// =========================
	newCardboxTotal := float64(req.CardboxQty) * req.CardboxUnitPrice

	// Grand total
	grandTotal := saleDocument.TotalPrice + newCardboxTotal

	// Price after tax
	priceAfterTax := grandTotal

	if saleDocument.IsTax == false && saleDocument.Tax != nil && *saleDocument.Tax > 0 {
		taxAmount := grandTotal * (*saleDocument.Tax / 100)
		priceAfterTax = grandTotal + taxAmount
	}

	// =========================
	// Update database
	// =========================
	err := config.DB.Model(&saleDocument).Updates(map[string]interface{}{
		"cardbox_qty":         req.CardboxQty,
		"cardbox_unit_price": req.CardboxUnitPrice,
		"cardbox_total_price": newCardboxTotal,
		"grand_total_price":    grandTotal,
		"price_after_tax":    priceAfterTax,
	}).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "Gagal menyimpan data",
			"error":   err.Error(),
		})
		return
	}

	// Reload relasi
	config.DB.Preload("Sales").Preload("User").First(&saleDocument, saleDocument.ID)

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "Data berhasil disimpan!",
		"data":    saleDocument,
	})
}

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

	if req.NewDiscount != nil && *req.NewDiscount > 100 {
		c.JSON(400, gin.H{
			"success": false,
			"message": "New Discount tidak boleh lebih dari 100",
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
		itemStatusBefore   string

		productName string
		productCategory string
		productQuantity int64

		newPrice      float64
		oldPrice      float64
		basePrice     float64
		discount      float64
		totalDiscount float64
	)

	isBundle = false
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
	PRICE SETUP / DEFAULT VALUE
	=========================== */
	if isBundle {
		productName = bundle.NameBundle
		productCategory = bundle.Category.NameCategory
		productQuantity = bundle.TotalProduct

		itemStatusBefore = bundle.Status
		oldPrice = bundle.TotalPrice
		basePrice = bundle.TotalPriceCustom
		newPrice = basePrice
		barcodeItem = bundle.Barcode
		totalDiscount = 0.0

		category_discount := bundle.TotalPrice * (float64(bundle.Category.DiscountCategory)/100.0)
		category_discount = math.Round(category_discount)
		if category_discount > bundle.Category.MaxPriceCategory {
			category_discount = bundle.Category.MaxPriceCategory
		} 

		expectedPrice := bundle.TotalPrice - category_discount

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
		productName = product.Name
		productCategory = product.Category.NameCategory
		productQuantity = product.Quantity

		itemStatusBefore = product.Status
		oldPrice = product.ProductOld.OldPriceProduct
		basePrice = product.DisplayPrice
		newPrice = product.Price
		barcodeItem = product.Barcode
		totalDiscount = product.Price - product.DisplayPrice

		if product.Discount != nil {
			discount = *product.Discount
		}

		category_discount := product.ProductOld.OldPriceProduct * (float64(product.Category.DiscountCategory)/100.0)
		category_discount = math.Round(category_discount)
		if category_discount > product.Category.MaxPriceCategory {
			category_discount = product.Category.MaxPriceCategory
		} 
		expectedPrice := product.ProductOld.OldPriceProduct - category_discount

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
		discountLoyalty := float64(0)
		now := time.Now().In(time.FixedZone("Asia/Jakarta", 7*3600))

		if buyer.LoyaltyRankID != nil && buyer.ExpireDate != nil && buyer.ExpireDate.After(now) {
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
		ProductName: productName,
		ProductCategory: productCategory,
		ProductQuantity: productQuantity,
		ProductOldPrice: oldPrice,
		ProductPrice: newPrice,
		ProductPriceSale:  math.Ceil(productPriceSale),
		BasePrice:         math.Ceil(basePrice),
		TotalDiscountSale: math.Ceil(totalDiscount),
		DiscountSale:      discount,
		TypeDiscount:      req.TypeDiscount,
		StatusSale:        "proses",	
		ProductStatusBefore: itemStatusBefore,
	}

	if isBundle {
		sale.BundleBarcode = &barcodeItem
		sale.ProductBarcode = nil
	}else {
		sale.ProductBarcode = &barcodeItem
		sale.BundleBarcode = nil
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
		First(&sale, c.Param("sale_id")).Error; err != nil {

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
		if sale.ProductBarcode != nil {
			res := tx.Model(&models.Product{}).Where("barcode = ?", sale.ProductBarcode).
				Update("status", sale.ProductStatusBefore)
			
			if res.Error != nil {
				return res.Error
			}else if res.RowsAffected == 0 {
				return fmt.Errorf("Product dengan barcode %s tidak ditemukan", *sale.ProductBarcode)
			}
		}else {
			res := tx.Model(&models.Bundle{}).
				Where("barcode = ?", sale.BundleBarcode).
				Update("status", sale.ProductStatusBefore)

			if res.Error != nil {
				return res.Error
			}else if res.RowsAffected == 0 {
				return fmt.Errorf("Bundle dengan barcode %s tidak ditemukan", *sale.BundleBarcode)
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
	var totalDisplay, totalPrice, totalOldProduct, totalOldBundle float64

	tx.Model(&models.Sale{}).
		Where("sale_document_id = ?", saleDocument.ID).
		Select("COALESCE(SUM(base_price),0)").Scan(&totalDisplay)

	tx.Model(&models.Sale{}).
		Where("sale_document_id = ?", saleDocument.ID).
		Select("COALESCE(SUM(product_price_sale),0)").Scan(&totalPrice)

	tx.Model(&models.Sale{}).
		Joins("JOIN products ON products.barcode = sales.barcode_item").
		Joins("JOIN product_olds ON product_olds.product_id = products.id").
		Where("sales.sale_document_id = ?", saleDocument.ID).
		Select("COALESCE(SUM(product_olds.old_price_product), 0)").
		Scan(&totalOldProduct)

	tx.Model(&models.Sale{}).
		Joins("JOIN bundles ON bundles.barcode = sales.barcode_item").
		Where("sales.sale_document_id = ?", saleDocument.ID).
		Select("COALESCE(SUM(bundles.total_price), 0)").
		Scan(&totalOldBundle)

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
		priceAfterTax += grandTotal * (taxPercent / 100.0)
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
		"total_old_price": 			totalOldProduct + totalOldBundle,
		"total_price":     			totalPrice,
		"total_display_price":   			totalDisplay,
		"status":          			"selesai",
		"cardbox_qty":                   req.CardboxQty,
		"cardbox_unit_price":            req.CardboxUnitPrice,
		"cardbox_total_price":           cardboxTotal,
		"voucher":                       req.Voucher,
		"approved":                      approved,
		"is_tax":                        req.IsTax,
		"tax":                           taxPercent,
		"grand_total_price":               grandTotal,
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

func ExportInvoiceSale(c *gin.Context) {
	sale_doc_id := c.Param("sale_doc_id")

	var saleDoc models.SaleDocument
	if err := config.DB.Preload("User").
		First(&saleDoc, sale_doc_id).Error; err != nil {
		
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"success": false, "message": "Sale document tidak ditemukan"})
		}else {
			c.JSON(500, gin.H{"success": false, "message": "Gagal mengambil data sale document", "error": err.Error()})
		}

		return
	}

	f := excelize.NewFile()
	sheet := "Invoice"
	f.SetSheetName("Sheet1", sheet)

	// HEADER SALE DOCUMENT
	// =========================
	headers := []string{
		"Username Cashier",
		"Kode Dokumen",
		"ID Pembeli",
		"Nama Pembeli",
		"Telepon Pembeli",
		"Alamat Pembeli",
		"Diskon Baru",
		"Total Produk",
		"Total Harga",
		"Harga Normal",
		"Status Penjualan",
		"Qty Kardus",
		"Harga Kardus",
		"Total Harga Kardus",
		"Tanggal Dibuat",
		"Voucher",
		"Pajak",
		"Harga Setelah Pajak",
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// DATA SALE DOCUMENT
	// =========================
	row := 2
	values := []interface{}{
		saleDoc.User.Username,
		saleDoc.CodeDocumentSale,
		saleDoc.BuyerID,
		saleDoc.BuyerName,
		saleDoc.BuyerPhone,
		saleDoc.BuyerAddress,
		saleDoc.NewDiscountSale,
		saleDoc.TotalProduct,
		saleDoc.TotalPrice,
		saleDoc.TotalDisplayPrice,
		saleDoc.Status,
		saleDoc.CardboxQty,
		saleDoc.CardboxUnitPrice,
		saleDoc.CardboxTotalPrice,
		saleDoc.CreatedAt.Format("2006-01-02 15:04:05"),
		saleDoc.Voucher,
		saleDoc.Tax,
		saleDoc.PriceAfterTax,
	}

	for i, v := range values {
		cell, _ := excelize.CoordinatesToCellName(i+1, row)
		f.SetCellValue(sheet, cell, v)
	}

	// =========================
	// HEADER SALES
	// =========================
	row += 2
	salesHeaders := []string{
		"Nama Produk",
		"Kategori Produk",
		"Barcode Produk",
		"Harga Produk",
		"Kuantitas Produk",
		"Status Penjualan",
		"Total Diskon",
		"Tanggal Dibuat",
		"Harga Normal",
	}

	for i, h := range salesHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, row)
		f.SetCellValue(sheet, cell, h)
	}

	row++
	for _, s := range saleDoc.Sales {
		var barcode  string

		// ITEM PRODUCT
		if s.ProductBarcode != nil {
			barcode = s.Product.Barcode
		// ITEM BUNDLE
		} else if s.BundleBarcode != nil {
			barcode = s.Bundle.Barcode

		// SAFETY FALLBACK
		} else {
			continue // data rusak → skip
		}

		createdAt := ""
		if s.CreatedAt != nil {
			createdAt = s.CreatedAt.Format("2006-01-02 15:04:05")
		}

		data := []interface{}{
			s.ProductName,
			s.ProductCategory,
			barcode,
			s.ProductPriceSale,
			s.ProductQuantity,
			s.StatusSale,
			s.TotalDiscountSale,
			createdAt,
			s.BasePrice,
		}

		for i, v := range data {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			f.SetCellValue(sheet, cell, v)
		}
		row++
	}

	// SAVE FILE
	// =========================
	fileName := fmt.Sprintf("invoice-sale-%s.xlsx", saleDoc.CodeDocumentSale)
	exportPath := "public/exports"

	if err := os.MkdirAll(exportPath, 0777); err != nil {
		c.JSON(500, gin.H{"message": "Gagal membuat folder export path", "error": err.Error()})
		return
	}

	filePath := filepath.Join(exportPath, fileName)
	if err := f.SaveAs(filePath); err != nil {
		c.JSON(500, gin.H{"message": "Gagal menyimpan file", "error": err.Error()})
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/%s", os.Getenv("APP_URL"), fileName)

	c.JSON(200, gin.H{
		"success": true,
		"message": "unduh",
		"data":    downloadURL,
	})

}


