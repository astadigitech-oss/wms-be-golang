package controllers

import (
	"errors"
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"strconv"

	// "time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//B2B
func GetBulkyDocuments(c *gin.Context) {
	q := c.Query("q")
	
	limit := 30
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	offset := (page - 1) * limit

	var bulky_documents []models.BulkyDocument
	var totalData int64
	baseQuery := config.DB.Model(&models.BulkyDocument{})

	if q != "" {
		query := "%" + q + "%"
		baseQuery = baseQuery.Where("(code_document LIKE ? OR name_document LIKE ?)", query, query)
	}

	if err := baseQuery.Session(&gorm.Session{}).Count(&totalData).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal menghitung total data", "error": err.Error()})
		return
	}

	if err := baseQuery.Order("created_at DESC").Limit(limit).
		Offset(offset).Scan(&bulky_documents).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal mengambil data bulky document", "error": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"success": false,
		"message": "List Document Bulky",
		"resources": gin.H{
			"data": bulky_documents,
			"current_page": page,
			"links": links,
			"per_page": limit,
			"total_data": totalData,
			"from": offset + 1,
			"to": offset + int(totalData),
		},
	})
}

func DetailBulkyDocument(c *gin.Context) {
	db := config.DB
	documentID := c.Param("bulky_doc_id")

	// =========================
	// Ambil bulky document
	// =========================
	var bulkyDocument models.BulkyDocument
	if err := db.First(&bulkyDocument, documentID).Error; err != nil {
		c.JSON(404, gin.H{
			"success": false,
			"message": "Data bulky document tidak ditemukan",
		})
		return
	}

	// =========================
	// Ambil semua bag product
	// =========================
	var bagProducts []models.BagProduct
	if err := db.
		Where("bulky_document_id = ?", bulkyDocument.ID).
		Find(&bagProducts).Error; err != nil {

		c.JSON(422, gin.H{
			"success": false,
			"message": "Gagal mengambil bag product",
		})
		return
	}

	if len(bagProducts) == 0 {
		c.JSON(200, gin.H{
			"success": true,
			"message": "Data document bulky",
			"data": gin.H{
				"bulky_document": bulkyDocument,
				"total_bag":      0,
				"bag_products":   []interface{}{},
			},
		})
		return
	}

	// =========================
	// Ambil SUM after_price per bag
	// =========================
	var bagIDs []uint64
	for _, bag := range bagProducts {
		bagIDs = append(bagIDs, bag.ID)
	}

	type bagPriceSum struct {
		BagProductID uint64
		AfterPrice   float64
	}

	var priceSums []bagPriceSum
	db.Model(&models.BulkySale{}).
		Select("bag_product_id, SUM(after_price_bulky_sale) as after_price").
		Where("bag_product_id IN ?", bagIDs).
		Group("bag_product_id").
		Scan(&priceSums)

	// =========================
	// Mapping hasil sum
	// =========================
	priceMap := map[uint64]float64{}
	for _, p := range priceSums {
		priceMap[p.BagProductID] = p.AfterPrice
	}

	type bagProductResponse struct {
		ID           uint64  `json:"id"`
		BarcodeBag       string  `json:"barcode_bag"`
		NameBag       string  `json:"name_bag"`
		Status       string  `json:"status"`
		TotalProduct int     `json:"total_product"`
		Price        float64 `json:"price"`
	}

	var bag_response []bagProductResponse

	for i := range bagProducts {
		bag_response = append(bag_response, bagProductResponse{
			ID: bagProducts[i].ID,
			BarcodeBag: bagProducts[i].BarcodeBag,
			NameBag: bagProducts[i].NameBag,
			Status: bagProducts[i].Status,
			TotalProduct: int(bagProducts[i].TotalProduct),
			Price: priceMap[bagProducts[i].ID],
		})
	}

	// =========================
	// Response
	// =========================
	c.JSON(200, gin.H{
		"success": true,
		"message": "Data document bulky",
		"data": gin.H{
			"bulky_document": bulkyDocument,
			"total_bag":      len(bagProducts),
			"bag_products":   bag_response,
		},
	})
}


func BagByUser(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	// =========================
	// Query params
	// =========================
	// q := c.Query("q")
	bagID := c.Query("bag_id")
	docID := c.Param("doc_id")

	// perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	// page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	// if perPage <= 0 {
	// 	perPage = 10
	// }

	// offset := (page - 1) * perPage

	// =========================
	// Ambil Bulky Document
	// =========================
	var bulkyDoc models.BulkyDocument
	if err := config.DB.First(&bulkyDoc, docID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Bulky document tidak ditemukan",
		})
		return
	}

	// =========================
	// Ambil semua bag (ringan)
	// =========================
	type bagProductListRes struct {
		ID           uint64 `json:"id"`
		BarcodeBag   string `json:"barcode_bag"`
		NameBag      string `json:"name_bag"`
		TotalProduct int    `json:"total_product"`
		Status		string 	`json:"status"`
	}

	var bags []bagProductListRes
	config.DB.
		Table("bag_products").
		Select("id, barcode_bag, name_bag, total_product, status").
		Where("bulky_document_id = ? AND user_id = ?", bulkyDoc.ID, user.ID).
		Order("created_at DESC").
		Scan(&bags)

	// =========================
	// Tentukan bag aktif
	// =========================
	var bag models.BagProduct
	bagQuery := config.DB.
		Preload("BulkySales", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Where("bulky_document_id = ? AND user_id = ?", bulkyDoc.ID, user.ID)

	if bagID != "" {
		bagQuery = bagQuery.Where("id = ?", bagID)
	} else {
		bagQuery = bagQuery.Where("(status IS NULL OR status = ?)", "proses")
	}

	err := bagQuery.Order("created_at DESC").First(&bag).Error

	// =========================
	// Jika tidak ada bag aktif
	// =========================
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "List of bag products",
				"data": gin.H{
					"ids":            bags,
					"bulky_document": bulkyDoc,
					"bag_product":    nil,
				},
			})
			return
		}

		// error lain (DB error, dll)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal mengambil data bag",
			"error": err.Error(),
		})
		return
	}

	// =========================
	// Response
	// =========================
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Detail bag product with paginated items",
		"data": gin.H{
			"ids":            bags,
			"bulky_document": bulkyDoc,
			"bag_product":    bag,
		},
	})
}

func ShowBagProductDetail(c *gin.Context) {
	db := config.DB
	// =========================
	// Params
	// =========================
	bagID := c.Param("bag_id")
	query := c.Query("q")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "15"))

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 15
	}

	offset := (page - 1) * limit

	// =========================
	// Ambil Bag Product
	// =========================
	var bagProduct models.BagProduct
	if err := db.First(&bagProduct, bagID).Error; err != nil {
		c.JSON(404, gin.H{
			"success": false,
			"message": "Bag product tidak ditemukan",
		})
		return
	}

	// =========================
	// Hitung total price
	// =========================
	var totalPrice float64
	db.Model(&models.BulkySale{}).
		Where("bag_product_id = ?", bagProduct.ID).
		Select("COALESCE(SUM(after_price_bulky_sale),0)").
		Scan(&totalPrice)

	type bagProductResponse struct {
		ID           uint64  `json:"id"`
		BarcodeBag       string  `json:"barcode_bag"`
		NameBag       string  `json:"name_bag"`
		Status       string  `json:"status"`
		TotalProduct int     `json:"total_product"`
		Price        float64 `json:"price"`
	}
	
	bag_response := bagProductResponse{
		ID: bagProduct.ID,
		BarcodeBag: bagProduct.BarcodeBag,
		NameBag: bagProduct.NameBag,
		Status: bagProduct.Status,
		TotalProduct: int(bagProduct.TotalProduct),
		Price: totalPrice,
	}

	// =========================
	// Query bulky sales (base)
	// =========================
	baseQuery := db.Model(&models.BulkySale{}).
		Where("bag_product_id = ?", bagProduct.ID)

	if query != "" {
		baseQuery = baseQuery.Where(
			"product_barcode LIKE ? OR bundle_barcode LIKE ? OR product_name LIKE ?",
			"%"+query+"%",
			"%"+query+"%",
			"%"+query+"%",
		)
	}

	// =========================
	// Total data (untuk pagination)
	// =========================
	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		c.JSON(404, gin.H{
			"success": false,
			"message": "Gagal menghitung total bulky sales",
			"error": err.Error(),
		})
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// =========================
	// Ambil data bulky sales
	// =========================
	var bulkySales []models.BulkySale
	baseQuery.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&bulkySales)

	// =========================
	// Category count (GROUP BY)
	// =========================
	type categoryCount struct {
		Category string `json:"category"`
		Count    int64  `json:"count"`
	}

	var categoryCounts []categoryCount
	if err := baseQuery.Session(&gorm.Session{}).
		Select("product_category as category, COUNT(*) as count").
		Group("product_category").Scan(&categoryCounts).Error; err != nil {

		c.JSON(404, gin.H{
			"success": false,
			"message": "Gagal menghitung category",
			"error": err.Error(),
		})
		return
	}

	// =========================
	// Response
	// =========================
	c.JSON(200, gin.H{
		"success": true,
		"message": "Detail Bag Product",
		"data": gin.H{
			"bag_product": bag_response,
			"category_counts": categoryCounts,
			"bulky_sales": gin.H{
				"data": bulkySales,
				"current_page": page, 
				"per_page": limit,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"to":             offset + len(bulkySales),
			},
		},
	})
}

func CreateBulkyDocument(c *gin.Context) {
	type payloadRequest struct {
		DiscountBulky float64 `json:"discount_bulky"`
		BuyerID       *uint64 `json:"buyer_id"`
		NameDocument  string  `json:"name_document" binding:"required"`
	}

	user := c.MustGet("auth_user").(models.User)

	var req payloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success":  false,
			"message": "Input tidak valid!",
			"error": err.Error(),
		})
		return
	}

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction", "error": tx.Error.Error()})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
				"success": false, 
				"message": "Internal server error",
				"error": fmt.Sprintf("%v", r),
			})
        }
    }()

	// =========================
	// Ambil buyer (opsional)
	// =========================
	var buyer *models.Buyer
	if req.BuyerID != nil {
		var b models.Buyer
		if err := tx.First(&b, *req.BuyerID).Error; err != nil {
			tx.Rollback()
			if errors.Is(err, gorm.ErrRecordNotFound) {			
				c.JSON(http.StatusNotFound, gin.H{
					"success": false, 
					"message": "Buyer tidak ditemukan",
				})
			}else {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false, 
					"message": "Gagal mengambil data buyer",
					"error": err.Error(),
				})
			}
			return
		}
		buyer = &b
	}

	// =========================
	// Ambil dokumen terakhir (FOR UPDATE)
	// =========================
	var lastDoc models.BulkyDocument
	err := tx.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Order("id DESC").
		First(&lastDoc).Error

	nextNumber := 1
	if err == nil {
		re := regexp.MustCompile(`^(\d+)[\.\-]`)
		matches := re.FindStringSubmatch(lastDoc.NameDocument)
		if len(matches) > 1 {
			if num, err := strconv.Atoi(matches[1]); err == nil {
				nextNumber = num + 1
			}
		}
	}

	finalName := fmt.Sprintf("%d-%s", nextNumber, req.NameDocument)

	// =========================
	// Cek duplikasi nama
	// =========================
	var count int64
	tx.Model(&models.BulkyDocument{}).
		Where("name_document = ?", finalName).
		Count(&count)

	if count > 0 {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{
			"success": false, 
			"message": "Nama dokumen sudah digunakan, silakan coba lagi.",
			"error": err.Error(),
		})
		return
	}

	// =========================
	// Simpan bulky document
	// =========================
	code_doc, err := helpers.GenerateBulkyCode(tx)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{
			"success": false, 
			"message": "Gagal membuat generate code document",
			"error": err.Error(),
		})
		return
	}

	userID := uint64(user.ID)
	bulky := models.BulkyDocument{
		UserID:              userID,
		NameUser:            user.Name,
		CodeDocument: 		code_doc,	
		TotalProduct:   	0,
		TotalOldPrice:  	0,
		DiscountBulky:       req.DiscountBulky,
		AfterPriceBulky:     0,
		CategoryBulky:       nil,
		StatusBulky:         "proses",
		NameDocument:        finalName,
	}

	if buyer != nil {
		bulky.BuyerID = &buyer.ID
		bulky.NameBuyer = &buyer.NameBuyer
	}

	if err := tx.Create(&bulky).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false, 
			"message": "Gagall membuat document bulky.",
			"error": err.Error(),
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(http.StatusOK, gin.H{
		"success": true, 
		"message": "data start b2b berhasil di buat!.",
	})

}

func UpdateBulkyDocument(c *gin.Context) {
	idParam := c.Param("bulky_doc_id")
	id, err := strconv.ParseUint(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "ID tidak valid",
		})
		return
	}

	type payloadRequest struct {
		DiscountBulky *float64 `json:"discount_bulky"`
		BuyerID       *uint64  `json:"buyer_id"`
		CategoryBulky *string  `json:"category_bulky"`
		NameDocument  string   `json:"name_document" binding:"required"`
	}

	// =========================
	// Ambil data bulky document
	// =========================
	var bulky models.BulkyDocument
	if err := config.DB.First(&bulky, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Data tidak ditemukan",
		})
		return
	}

	// =========================
	// Bind request
	// =========================
	var req payloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Input tidak valid!",
			"errors":  err.Error(),
		})
		return
	}

	// =========================
	// Validasi unique name_document (kecuali ID sendiri)
	// =========================
	var count int64
	config.DB.Model(&models.BulkyDocument{}).
		Where("name_document = ?", req.NameDocument).
		Where("id <> ?", bulky.ID).
		Count(&count)

	if count > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Nama dokumen sudah digunakan",
		})
		return
	}

	// =========================
	// Ambil buyer (opsional)
	// =========================
	var buyer *models.Buyer
	if req.BuyerID != nil {
		var b models.Buyer
		if err := config.DB.First(&b, *req.BuyerID).Error; err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"message": "Buyer tidak ditemukan",
			})
			return
		}
		buyer = &b
	}

	// =========================
	// Prepare update data
	// =========================
	updateData := map[string]interface{}{
		"name_document": req.NameDocument,
	}

	if req.DiscountBulky != nil {
		if *req.DiscountBulky > 100 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"message": "Diskon tidak boleh lebih dari 100",
			})
			return
		}
		updateData["discount_bulky"] = *req.DiscountBulky
	} else {
		updateData["discount_bulky"] = 0
	}

	if buyer != nil {
		updateData["buyer_id"] = buyer.ID
		updateData["name_buyer"] = buyer.NameBuyer
	}

	// =========================
	// Update
	// =========================
	if err := config.DB.Model(&bulky).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal mengupdate data",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "berhasil mengupdate data",
		"data":    bulky,
	})
}

func BulkyDocumentFinish(c *gin.Context) {
	doc_id := c.Param("bulky_doc_id")

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction", "error": tx.Error.Error()})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
				"success": false, 
				"message": "Internal server error",
				"error": fmt.Sprintf("%v", r),
			})
        }
    }()

	// =========================
	// Ambil bulky document (harus proses)
	// =========================
	var bulkyDocument models.BulkyDocument
	if err := tx.
		Where("id = ? AND status_bulky = ?", doc_id, "proses").
		First(&bulkyDocument).Error; err != nil {

		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": "Data bulky belum dibuat atau sudah selesai",
		})
		return
	}

	// =========================
	// Hitung total product & old price
	// =========================
	var result struct {
		TotalProduct int64
		OldPrice     float64
	}

	tx.Model(&models.BulkySale{}).
		Select(
			"COUNT(*) as total_product, COALESCE(SUM(product_old_price),0) as old_price",
		).
		Where("bulky_document_id = ?", bulkyDocument.ID).
		Scan(&result)

	if result.TotalProduct == 0 {
		tx.Rollback()
		c.JSON(400, gin.H{
			"success": false,
			"message": "Tidak ada data bulky sale",
		})
		return
	}

	// =========================
	// Hitung after price
	// =========================
	discount := bulkyDocument.DiscountBulky
	totalAfterPrice := result.OldPrice - (result.OldPrice * discount / 100)

	// =========================
	// Update bulky document
	// =========================
	if err := tx.Model(&bulkyDocument).Updates(map[string]interface{}{
		"status_bulky":        "selesai",
		"total_product": result.TotalProduct,
		"after_price_bulky":   totalAfterPrice,
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal update bulky document",
			"error": err.Error(),
		})
		return
	}
	// =========================
	// Update bag product
	// =========================
	if err := tx.Model(&models.BagProduct{}).Where("bulky_document_id = ? AND status = ?", bulkyDocument.ID, "proses").
		Update("status", "done").Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal update bag product",
			"error": err.Error(),
		})
		return

	}

	// Update rack_id product menjadi nil
	if err := tx.Exec(`
		UPDATE products p
		JOIN bulky_sales bs ON bs.product_barcode = p.barcode
		SET p.rack_id = NULL
		WHERE bs.bulky_document_id = ?
		AND bs.product_barcode IS NOT NULL
	`, bulkyDocument.ID).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal reset rack product",
			"error": err.Error(),
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	// =========================
	// Response
	// =========================
	c.JSON(200, gin.H{
		"success": true,
		"message": "Data bulky berhasil disimpan",
		"data": gin.H{
			"total_product": result.TotalProduct,
			"total_price":   totalAfterPrice,
		},
	})
}

func ExportBulkyDocument(c *gin.Context) {
	doc_id := c.Param("doc_id")

	var bulkyDoc models.BulkyDocument
	if err := config.DB.First(&bulkyDoc, doc_id).Error; err != nil {
		c.JSON(404, gin.H{
			"success": false,
			"message": "Bulky Document tidak ditemukan",
			"error": err.Error(),
		})
		return
	}

	var bags []models.BagProduct
	if err := config.DB.
		Preload("BulkySales").
		Where("bulky_document_id = ?", bulkyDoc.ID).
		Find(&bags).Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal ambil data",
			"error": err.Error(),
		})
		return
	}

	file := excelize.NewFile()

	allSales, grandTotal := collectSalesData(bags)

	createListSheet(file, allSales, grandTotal)
	createSummaryCategorySheet(file, allSales)
	createSummaryBagSheet(file, bags)

	file.DeleteSheet("Sheet1")

	fileName := fmt.Sprintf("bulky-%s-%s.xlsx", bulkyDoc.NameDocument, time.Now().Format("2006-01-02"))
	path := "./public/exports/bulky-documents/" + fileName

	os.MkdirAll("./public/exports/bulky-documents/", 0755)
	if err := file.SaveAs(path); err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/bulky-documents/%s", os.Getenv("APP_URL"), fileName)
	c.JSON(200, gin.H{
		"success": true,
		"message": "File berhasil diunduh",
		"data":    downloadURL,
	})
}


func StoreBagBulkyDocument(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
	doc_id := c.Param("doc_id")

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction", "error": tx.Error.Error()})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
				"success": false, 
				"message": "Internal server error",
				"error": fmt.Sprintf("%v", r),
			})
        }
    }()

	// =========================
	// Ambil Bulky Document
	// =========================
	var bulkyDoc models.BulkyDocument
	if err := tx.
		Where("id = ? AND status_bulky = ?", doc_id, "proses").
		First(&bulkyDoc).Error; err != nil {

		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Bulky document tidak ditemukan atau sudah done",
		})
		return
	}

	// =========================
	// Ambil username prefix
	// =========================
	username := strings.ToLower(user.Username)
	if len(username) > 3 {
		username = username[:3]
	}

	// =========================
	// Cari bag aktif
	// =========================
	var activeBag models.BagProduct
	err := tx.
		Where("bulky_document_id = ?", bulkyDoc.ID).
		Where("user_id = ?", user.ID).
		Where("status = ?", "proses").
		Where("name_bag LIKE ?", username+"-%").
		Order("id DESC").
		First(&activeBag).Error

	barcode := fmt.Sprintf("bag-%d-%s", user.ID, helpers.RandomString(7))
	var nextNumber int

	// =========================
	// Jika ada bag aktif → tutup
	// =========================
	if err == nil {
		if err := tx.Model(&activeBag).
			Update("status", "done").Error; err != nil {

			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Gagal update bag lama",
				"error": err.Error(),
			})
			return
		}

		re := regexp.MustCompile(`^` + username + `-(\d+)$`)
		if match := re.FindStringSubmatch(activeBag.NameBag); len(match) == 2 {
			nextNumber, _ = strconv.Atoi(match[1])
			nextNumber++
		} else {
			nextNumber = 1
		}

	} else {
		// =========================
		// Cari bag terakhir
		// =========================
		var lastBag models.BagProduct
		if err := tx.
			Where("bulky_document_id = ?", bulkyDoc.ID).
			Where("user_id = ?", user.ID).
			Where("name_bag LIKE ?", username+"-%").
			Order("id DESC").
			First(&lastBag).Error; err == nil {

			re := regexp.MustCompile(`^` + username + `-(\d+)$`)
			if match := re.FindStringSubmatch(lastBag.NameBag); len(match) == 2 {
				nextNumber, _ = strconv.Atoi(match[1])
				nextNumber++
			} else {
				nextNumber = 1
			}
		} else {
			nextNumber = 1
		}
	}

	// =========================
	// Buat bag baru
	// =========================
	newBag := models.BagProduct{
		UserID:           uint64(user.ID),
		BulkyDocumentID:  bulkyDoc.ID,
		TotalProduct:     0,
		Status:           "proses",
		NameBag:          fmt.Sprintf("%s-%d", username, nextNumber),
		BarcodeBag:       barcode,
	}

	if err := tx.Create(&newBag).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal membuat karung product",
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal commit transaksi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Berhasil membuat karung baru",
		"data":    newBag,
	})
}

func DestroyBagBulkyDocument(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
	bagID := c.Param("bag_id")

	var bag models.BagProduct
	if err := config.DB.
		Where("id = ? AND user_id = ?", bagID, user.ID).
		First(&bag).Error; err != nil {

		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Bag product tidak ditemukan",
		})
		return
	}

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction", "error": tx.Error.Error()})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
				"success": false, 
				"message": "Internal server error",
				"error": fmt.Sprintf("%v", r),
			})
        }
    }()

	// =========================
	// Cek Bulky Document
	// =========================
	var bulkyDoc models.BulkyDocument
	if err := tx.
		Where("id = ? AND status_bulky = ?", bag.BulkyDocumentID, "proses").
		First(&bulkyDoc).Error; err != nil {

		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Bulky document tidak ditemukan atau sudah selesai",
		})
		return
	}

	// =========================
	// Ambil semua bag user
	// =========================
	var bags []models.BagProduct
	tx.
		Select("id, status").
		Where("bulky_document_id = ? AND user_id = ?", bag.BulkyDocumentID, user.ID).
		Order("id DESC").
		Find(&bags)

	// =========================
	// Jika bag terakhir dihapus
	// =========================
	if len(bags) > 1 && bags[0].ID == bag.ID {
		prevBag := bags[1]
		tx.Model(&models.BagProduct{}).
			Where("id = ?", prevBag.ID).
			Update("status", "proses")
	}

	// =========================
	// Ambil bulky sales
	// =========================
	var sales []models.BulkySale
	tx.
		Where("bag_product_id = ?", bag.ID).
		Find(&sales)

	if len(sales) >= 1 {
		var totalOldPrice float64
		var totalAfterPrice float64
		totalProduct := len(sales)
	
		for _, s := range sales {
			totalOldPrice += s.ProductOldPrice
			totalAfterPrice += s.AfterPriceBulkySale
		}
	
		// =========================
		// Update bulky document
		// =========================
		bulkyDoc.TotalOldPrice -= totalOldPrice
		bulkyDoc.AfterPriceBulky -= totalAfterPrice
		bulkyDoc.TotalProduct -= int64(totalProduct)
	
		if err := tx.Save(&bulkyDoc).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Gagal update bulky document",
			})
			return
		}
	
		// =========================
		// Kembalikan status product / bundle
		// =========================
		for _, sale := range sales {
	
			if sale.ProductBarcode != nil {
				tx.Model(&models.Product{}).
					Where("barcode = ?", *sale.ProductBarcode).
					Update("status", sale.ProductStatusBefore)
	
			} else if sale.BundleBarcode != nil {
				tx.Model(&models.Bundle{}).
					Where("barcode = ?", *sale.BundleBarcode).
					Update("status", sale.ProductStatusBefore)
			}
		}
	}

	// =========================
	// Hapus bag product
	// =========================
	if err := tx.Delete(&bag).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal menghapus bag product",
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal commit transaksi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Berhasil menghapus bag product",
		"data":    bag,
	})
}

func StoreBulkySale(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	type payloadRequest struct {
		BulkyDocumentID uint   `json:"bulky_document_id" binding:"required"`
		BarcodeProduct         string `json:"barcode_product" binding:"required"`
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
			case "BulkyDocumentID":
				errorsMap["bulky_document_id"] = "Bulky document ID wajib diisi"
			case "BarcodeProduct":
				errorsMap["barcode_product"] = "Barcode product wajib diisi"

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

	var bulkyDoc models.BulkyDocument
	if err := config.DB.First(&bulkyDoc, req.BulkyDocumentID).Error; err != nil {
		c.JSON(404, gin.H{"success": false, "message": "Bulky document tidak ditemukan"})
		return
	}

	if bulkyDoc.StatusBulky != "proses" {
		c.JSON(400, gin.H{"success": false, "message": "Bulky sudah selesai"})
		return
	}

	var bag models.BagProduct
	if err := config.DB.Where("user_id = ? AND bulky_document_id = ?", user.ID, bulkyDoc.ID).
		Where("status", "proses").Last(&bag).Error; err != nil {
		c.JSON(404, gin.H{"success": false, "message": "Bag product dengan status proses tidak ditemukan"})
		return
	}

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction", "error": tx.Error.Error()})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
				"success": false, 
				"message": "Internal server error",
				"error": fmt.Sprintf("%v", r),
			})
        }
    }()

	// Cegah barcode double input
	var exists models.BulkySale
	if err := tx.Where("barcode = ?", req.BarcodeProduct).First(&exists).Error; err == nil {
		tx.Rollback()
		c.JSON(422, gin.H{"success": false, "message": "Barcode sudah diinput"})
		return
	}

	var (
		product       models.Product
		bundle        models.Bundle
		isBundle	  bool

		oldPrice	float64
		afterPrice	float64
	)

	isBundle = false
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("Category").
		Where("barcode = ?", req.BarcodeProduct).
		First(&product).Error

	if err != nil {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Category").
			Where("barcode = ?", req.BarcodeProduct).
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
			"message": "Product / bundle sudah dijual",
		})
		return
	}

	bulkySale := models.BulkySale{
		BulkyDocumentID: bulkyDoc.ID,
		BagProductID:    bag.ID,
	}

	if isBundle {
		oldPrice = bundle.TotalPrice
		afterPrice = oldPrice - (oldPrice * bulkyDoc.DiscountBulky / 100.0)
		
		bulkySale.BundleBarcode	= &req.BarcodeProduct
		bulkySale.ProductName =     	bundle.NameBundle
		bulkySale.ProductCategory =     bundle.Category.NameCategory
		bulkySale.ProductOldPrice =     oldPrice
		bulkySale.ProductPrice =     bundle.TotalPriceCustom
		bulkySale.ProductStatusBefore =     bundle.Status
		bulkySale.ProductQuantity =     bundle.TotalProduct
		bulkySale.AfterPriceBulkySale =     afterPrice

		if err := tx.Model(&bundle).Update("status", "sale").Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"success": false, "message": "Gagal update status Bundle", "error": err.Error()})
			return
		}
	}else {
		oldPrice = product.OldPriceProduct
		afterPrice = oldPrice - (oldPrice * bulkyDoc.DiscountBulky / 100.0)
			
		bulkySale.ProductBarcode = 		&req.BarcodeProduct
		bulkySale.ProductName =     	product.Name
		bulkySale.ProductCategory =     product.Category.NameCategory
		bulkySale.ProductOldPrice =     oldPrice
		bulkySale.ProductPrice =     product.Price
		bulkySale.ProductStatusBefore =     product.Status
		bulkySale.ProductQuantity =     product.Quantity
		bulkySale.AfterPriceBulkySale =     afterPrice

		if err := tx.Model(&product).Update("status", "sale").Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"success": false, "message": "Gagal update status product", "error": err.Error()})
			return
		}

		//recalculate rack
		if product.RackID != nil {
			rackID := *product.RackID
			if err := helpers.RecalculateRack(tx, rackID); err != nil {
				tx.Rollback()
				c.JSON(500, gin.H{"success": false, "message": "Gagal update rak", "error": err.Error()})
				return
			}
		}
	}

	if err := tx.Create(&bulkySale).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal simpan bulky sale", "error": err.Error()})
		return
	}

	// Update bag
	if err := tx.Model(&bag).
		Update("total_product", gorm.Expr("total_product + ?", 1)).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal update bag", "error": err.Error()})
		return
	}

	if err := tx.Model(&bulkyDoc).Updates(map[string]interface{}{
		"total_product":		gorm.Expr("total_product + ?", 1),
		"total_old_price":		gorm.Expr("total_old_price + ?", oldPrice),
		"after_price_bulky":		gorm.Expr("after_price_bulky + ?", afterPrice),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal update bulky document", "error": err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(200, gin.H{
		"success": true,
		"message": "Data berhasil disimpan",
		"data":    bulkySale,
	})
}

func ImportFileBulkySale(c *gin.Context) {
	db := config.DB
	user := c.MustGet("auth_user").(models.User)

	bulkyDocumentID := c.PostForm("bulky_document_id")

	var bulkyDocument models.BulkyDocument
	if err := db.First(&bulkyDocument, bulkyDocumentID).Error; err != nil {
		c.JSON(404, gin.H{"success": false, "message": "Data bulky tidak ditemukan"})
		return
	}

	if bulkyDocument.StatusBulky != "proses" {
		c.JSON(400, gin.H{"success": false, "message": "Data bulky sudah selesai"})
		return
	}

	// =========================
	// Ambil bag product
	// =========================
	var bagProduct models.BagProduct
	if err := db.
		Where("user_id = ? AND bulky_document_id = ? AND status = ?", user.ID, bulkyDocument.ID, "proses").
		Order("id DESC").
		First(&bagProduct).Error; err != nil {

		c.JSON(404, gin.H{"success": false, "message": "Bag produk dengan status proses tidak ditemukan"})
		return
	}

	file, err := c.FormFile("file_import")
	if err != nil {
		c.JSON(422, gin.H{"success": false, "message": "File import wajib diisi"})
		return
	}

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction", "error": tx.Error.Error()})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
				"success": false, 
				"message": "Internal server error",
				"error": fmt.Sprintf("%v", r),
			})
        }
    }()

	result, err := importBulkyExcel(
		tx,
		file,
		bulkyDocument,
		bagProduct.ID,
	)

	if err != nil {
		tx.Rollback()
		c.JSON(422, gin.H{"success": false, "message": err.Error()})
		return
	}

	if result.TotalFound == 0 {
		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": "Tidak ada data valid, semua barcode tidak ditemukan",
			"data":    result,
		})
		return
	}

	// update bulky_document
	if err := tx.Model(&bulkyDocument).Updates(map[string]interface{}{
		"total_product": 				gorm.Expr("total_product + ?", result.TotalFound),
		"total_old_price": 				gorm.Expr("total_old_price + ?", result.TotalOldPrice),
		"after_price_bulky": 				gorm.Expr("after_price_bulky + ?", result.TotalAfterPrice),
	}).Error; err!= nil {

		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal update data bulky document",
			"error":    err.Error(),
		})
		return
	}

	// update bag product
	if err := tx.Model(&bagProduct).
		Update("total_product", gorm.Expr("total_product + ?", result.TotalFound)).Error; err!= nil {

		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal update data bag product",
			"error":    err.Error(),
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(200, gin.H{
		"success": true,
		"message": "Data berhasil ditambahkan",
		"data": gin.H{
			"import":                   true,
			"total_barcode_found":      result.TotalFound,
			"total_barcode_not_found":  len(result.NotFoundBarcodes),
			"data_barcode_not_found":   result.NotFoundBarcodes,
			"data_barcode_duplicate":   result.DuplicateBarcodes,
		},
	})
}

func DeleteBulkySale(c *gin.Context) {
	db := config.DB
	bulky_sale_id := c.Param("bulky_sale_id")

	var bulkySale models.BulkySale
	if err := db.First(&bulkySale, bulky_sale_id).Error; err != nil {
		c.JSON(404, gin.H{
			"success": false,
			"message": "Bulky sale tidak ditemukan",
		})
		return
	}

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction", "error": tx.Error.Error()})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
				"success": false, 
				"message": "Internal server error",
				"error": fmt.Sprintf("%v", r),
			})
        }
    }()

	// =========================
	// Ambil bulky document (harus status proses)
	// =========================
	var bulkyDocument models.BulkyDocument
	if err := tx.
		Where("id = ? AND status_bulky = ?", bulkySale.BulkyDocumentID, "proses").
		First(&bulkyDocument).Error; err != nil {

		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": "Document Bulky tidak ditemukan atau status done",
		})
		return
	}

	// =========================
	// Kembalikan status product / bundle
	// =========================
	if bulkySale.ProductBarcode != nil {
		var product models.Product
		if err := tx.
			Where("barcode = ?", bulkySale.ProductBarcode).
			First(&product).Error; err != nil {

			tx.Rollback()
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(404, gin.H{
					"success": false,
					"message": fmt.Sprintf("Product dengan barcode %s tidak ditemukan", *bulkySale.ProductBarcode),
				})
			}else {
				c.JSON(500, gin.H{
					"success": false,
					"message": "Gagal mencari product",
					"error": err.Error(),
				})
			}

			return
		}

		// product ketemu
		if err := tx.Model(&product).
			Update("status", bulkySale.ProductStatusBefore). Error; err != nil {
			
			tx.Rollback()
			c.JSON(500, gin.H{
				"success": false,
				"message": "Gagal update status product",
				"error": err.Error(),
			})
			return
		}

		//recalculate rack
		if product.RackID != nil {
			rackID := *product.RackID
			if err := helpers.RecalculateRack(tx, rackID); err != nil {
				tx.Rollback()
				c.JSON(500, gin.H{"success": false, "message": "Gagal update rak", "error": err.Error()})
				return
			}
		}
	} else {
		// kalau bukan product, cek bundle
		var bundle models.Bundle
		if err := tx.
			Where("barcode = ?", bulkySale.BundleBarcode).
			First(&bundle).Error; err != nil {

			tx.Rollback()
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(404, gin.H{
					"success": false,
					"message": fmt.Sprintf("Bundle dengan barcode %s tidak ditemukan", *bulkySale.BundleBarcode),
				})
			}else {
				c.JSON(500, gin.H{
					"success": false,
					"message": "Gagal mencari bundle",
					"error": err.Error(),
				})
			}

			return
		}

		// bundle ketemu
		if err := tx.Model(&bundle).
			Update("status", bulkySale.ProductStatusBefore). Error; err != nil {
			
			tx.Rollback()
			c.JSON(500, gin.H{
				"success": false,
				"message": "Gagal update status bundle",
				"error": err.Error(),
			})
			return
		}
	}

	// =========================
	// Update bulky document
	// =========================
	total_product := bulkyDocument.TotalProduct - 1
	if total_product < 0 {
		total_product = 0
	}
	total_old_price := bulkyDocument.TotalOldPrice - bulkySale.ProductOldPrice
	if total_old_price < 0 {
		total_old_price = 0
	}
	after_price_bulky := bulkyDocument.AfterPriceBulky - bulkySale.AfterPriceBulkySale
	if after_price_bulky < 0 {
		after_price_bulky = 0
	}

	if err := tx.Model(&bulkyDocument).
		Updates(map[string]interface{}{
			"total_product":     total_product,
			"total_old_price":   total_old_price,
			"after_price_bulky":       after_price_bulky,
		}).Error; err != nil {
		
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal update bulky document",
			"error": err.Error(),
		})
		return
	}

	// =========================
	// Kurangi bag product
	// =========================
	if err := tx.Model(&models.BagProduct{}).
		Where("id = ?", bulkySale.BagProductID).
		Update("total_product", gorm.Expr("total_product - ?", 1)).Error; err != nil {
		
		tx.Rollback()
		c.JSON(422, gin.H{
			"success": false,
			"message": "Gagal update bag product",
			"error" : err.Error(),
		})
		return
	}

	// =========================
	// Hapus bulky sale
	// =========================
	if err := tx.Delete(&bulkySale).Error; err != nil {
		tx.Rollback()
		c.JSON(422, gin.H{
			"success": false,
			"message": "Gagal menghapus bulky sale",
			"error" : err.Error(),
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(200, gin.H{
		"success": true,
		"message": "Data berhasil dihapus",
	})
}

// ====================== Import Product
type importResult struct {
	TotalFound        int
	NotFoundBarcodes  []string
	DuplicateBarcodes []string
	TotalOldPrice		float64
	TotalAfterPrice		float64
}

func importBulkyExcel(
	tx *gorm.DB,
	file *multipart.FileHeader,
	bulkyDocument models.BulkyDocument,
	bagProductID uint64,
) (*importResult, error) {

	f, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	excel, err := excelize.OpenReader(f)
	if err != nil {
		return nil, err
	}

	sheets := excel.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("sheet excel tidak ditemukan")
	}
	sheetName := sheets[0]

	rows, err := excel.GetRows(sheetName)
	if err != nil {
		return nil, err
	}

	result := &importResult{}
	processed := map[string]bool{}
	var item models.BulkySale

	var bulkySales []models.BulkySale

	for i, row := range rows {
		if i == 0 {
			continue // skip header
		}

		if len(row) == 0 {
			continue
		}

		barcode := strings.TrimSpace(row[0])
		if barcode == "" {
			continue
		}

		if processed[barcode] {
			result.DuplicateBarcodes = append(result.DuplicateBarcodes, barcode)
			continue
		}

		processed[barcode] = true

		// =========================
		// Cek product
		// =========================
		var product models.Product
		found := false

		if err := tx.Preload("Category").
			Where("barcode = ?", barcode).First(&product).Error; err == nil {

			if product.Status == "sale" {
				continue
			}

			item = buildBulkySaleFromProduct(product,bulkyDocument,bagProductID)
			bulkySales = append(bulkySales, item)

			tx.Model(&product).Update("status", "sale")

			if product.RackID != nil {
				rackID := *product.RackID
				if err := helpers.RecalculateRack(tx, rackID); err != nil {
					return nil, fmt.Errorf("gagal update rak: %w", err)
				}
			}

			found = true
		} else {
			// =========================
			// Cek bundle
			// =========================
			var bundle models.Bundle
			if err := tx.Where("barcode = ?", barcode).First(&bundle).Error; err == nil {

				if bundle.Status == "sale" {
					continue
				}

				item = buildBulkySaleFromBundle(bundle,bulkyDocument,bagProductID)
				bulkySales = append(bulkySales, item)

				tx.Model(&bundle).Update("status", "sale")
				found = true
			}
		}

		if found {
			result.TotalFound++
			result.TotalOldPrice += item.ProductOldPrice
			result.TotalAfterPrice += item.AfterPriceBulkySale
		} else {
			result.NotFoundBarcodes = append(result.NotFoundBarcodes, barcode)
		}
	}

	if len(bulkySales) > 0 {
		if err := tx.Create(&bulkySales).Error; err != nil {
			return nil, err
		}
	}

	return result, nil
}

func buildBulkySaleFromProduct(
	p models.Product,
	doc models.BulkyDocument,
	bagID uint64,
) models.BulkySale {
	oldPrice := p.OldPriceProduct
	afterPrice := oldPrice - (oldPrice * doc.DiscountBulky / 100.0)

	return models.BulkySale{
		BulkyDocumentID:        doc.ID,
		BagProductID:           bagID,
		ProductBarcode:       	&p.Barcode,
		ProductCategory:        p.Category.NameCategory,
		ProductName:            p.Name,
		ProductStatusBefore:    p.Status,
		ProductOldPrice:      oldPrice,
		ProductPrice: p.Price,
		AfterPriceBulkySale:    afterPrice,
		ProductQuantity:        p.Quantity,
	}
}

func buildBulkySaleFromBundle(
	b models.Bundle,
	doc models.BulkyDocument,
	bagID uint64,
) models.BulkySale {
	oldPrice := b.TotalPrice
	afterPrice := oldPrice - (oldPrice * doc.DiscountBulky / 100.0)

	return models.BulkySale{
		BulkyDocumentID:     doc.ID,
		BagProductID:        bagID,
		BundleBarcode:    &b.Barcode,
		ProductCategory:     b.Category.NameCategory,
		ProductName:         b.NameBundle,
		ProductStatusBefore: b.Status,
		ProductOldPrice:   		oldPrice,
		ProductPrice: b.TotalPriceCustom,
		AfterPriceBulkySale: afterPrice,
		ProductQuantity:     b.TotalProduct,
	}
}

// ================= Export Bulky Document
type saleRow struct {
	Barcode  string
	Name     string
	Qty      int
	Price    float64
	Category string
}

func collectSalesData(bags []models.BagProduct) ([]saleRow, float64) {
	var rows []saleRow
	var grandTotal float64

	for i, bag := range bags {
		log.Printf(
			"Bag %d (%s) bulkySales: %d",
			i, bag.NameBag, len(bag.BulkySales),
		)
	}

	for _, bag := range bags {
		for _, s := range bag.BulkySales {
			price := s.ProductOldPrice
			grandTotal += price

			rowItem := saleRow{
				Name:     s.ProductName,
				Qty:      int(s.ProductQuantity),
				Price:    price,
				Category: s.ProductCategory,
			}

			if s.ProductBarcode != nil {
				rowItem.Barcode = *s.ProductBarcode
			}else {
				rowItem.Barcode = *s.BundleBarcode
			}

			rows = append(rows, rowItem)
		}
	}
	return rows, grandTotal
}

func createListSheet(f *excelize.File, data []saleRow, grandTotal float64) {
	sheet := "List Products"
	f.NewSheet(sheet)

	row := 1
	f.SetSheetRow(sheet, "A1", &[]interface{}{len(data), "", "", grandTotal})
	row++

	f.SetSheetRow(sheet, "A2", &[]interface{}{
		"Barcode Bulky Sale",
		"Name Product Bulky Sale",
		"QTY",
		"Old Price Bulky Sale",
	})
	row++

	for _, d := range data {
		f.SetSheetRow(sheet, fmt.Sprintf("A%d", row), &[]interface{}{
			d.Barcode,  d.Name, d.Qty, d.Price,
		})
		row++
	}

	lastRow := len(data) + 2 // header + total

	greenHeader := styleGreenHeader(f)
	blueHeader  := styleBlueHeader(f)
	borderOnly  := styleBorderOnly(f)
	// numberStyle := styleNumber(f)

	// A1:D1 → total row
	f.SetCellStyle(sheet, "A1", "D1", greenHeader)

	// A2:D2 → header kolom
	f.SetCellStyle(sheet, "A2", "D2", blueHeader)

	// A3:D{lastRow} → body
	if lastRow > 2 {
		f.SetCellStyle(sheet, "A3", fmt.Sprintf("D%d", lastRow), borderOnly)
	}

	//set lebar kolom nama product
	f.SetColWidth(sheet, "A", "A", 20)
	f.SetColWidth(sheet, "B", "B", 60)
	f.SetColWidth(sheet, "C", "C", 6)
	f.SetColWidth(sheet, "D", "D", 20)

}

func createSummaryCategorySheet(f *excelize.File, data []saleRow) {
	sheet := "Summary Category"
	f.NewSheet(sheet)

	blueHeader  := styleBlueHeader(f)
	borderOnly  := styleBorderOnly(f)

	f.SetSheetRow(sheet, "A1", &[]interface{}{
		"CATEGORY",
		"Count of Barcode Bulky Sale",
		"Sum of Old Price Bulky Sale",
	})

	f.SetCellStyle(sheet, "A1", "C1", blueHeader)
	f.SetColWidth(sheet, "A", "C", 35)

	type summary struct {
		Count int
		Sum   float64
	}

	group := map[string]*summary{}

	for _, d := range data {
		if group[d.Category] == nil {
			group[d.Category] = &summary{}
		}
		group[d.Category].Count++
		group[d.Category].Sum += d.Price
	}

	row := 2
	var totalCount int
	var totalSum float64

	for cat, v := range group {
		f.SetSheetRow(sheet, fmt.Sprintf("A%d", row), &[]interface{}{
			cat, v.Count, v.Sum,
		})
		totalCount += v.Count
		totalSum += v.Sum
		row++
	}

	f.SetCellStyle(sheet, "A2", fmt.Sprintf("C%d", row-1), borderOnly)

	f.SetSheetRow(sheet, fmt.Sprintf("A%d", row), &[]interface{}{
		"Grand Total", totalCount, totalSum,
	})

	footerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
			Size: 12,
		},
		NumFmt: 3,
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#BDD7EE"}, // FFC6EFCE
			Pattern: 1,
		},
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
	})

	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), footerStyle)
}

func createSummaryBagSheet(f *excelize.File, bags []models.BagProduct) {
	sheet := "Summary Bag"
	f.NewSheet(sheet)

	blueHeader := styleBlueHeader(f)
	borderOnly := styleBorderOnly(f)

	f.SetSheetRow(sheet, "A1", &[]interface{}{
		"NO", "Barcode", "Name Bag",
		"Count of Barcode Bulky Sale",
		"Sum of Old Price Bulky Sale",
	})
	f.SetCellStyle(sheet, "A1", "E1", blueHeader)
	f.SetColWidth(sheet, "A", "A", 5)
	f.SetColWidth(sheet, "B", "B", 17)
	f.SetColWidth(sheet, "C", "C", 15)
	f.SetColWidth(sheet, "D", "E", 35)

	row := 2
	no := 1
	var totalCount int
	var totalSum float64

	for _, bag := range bags {
		count := len(bag.BulkySales)
		var sum float64

		for _, s := range bag.BulkySales {
			sum += s.ProductOldPrice
		}

		totalCount += count
		totalSum += sum

		f.SetSheetRow(sheet, fmt.Sprintf("A%d", row), &[]interface{}{
			no, bag.BarcodeBag, bag.NameBag, count, sum,
		})

		no++
		row++
	}

	f.SetCellStyle(sheet, "A2", fmt.Sprintf("E%d", row-1), borderOnly)

	f.SetSheetRow(sheet, fmt.Sprintf("A%d", row), &[]interface{}{
		"Grand Total", "", "", totalCount, totalSum,
	})

	footerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
			Size: 12,
		},
		NumFmt: 3,
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#BDD7EE"}, // FFC6EFCE
			Pattern: 1,
		},
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
	})

	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("E%d", row), footerStyle)

	f.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row))
}

func styleGreenHeader(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
			Size: 12,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
		},
		NumFmt: 3,
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#C6EFCE"}, // FFC6EFCE
			Pattern: 1,
		},
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
	})
	return style
}

func styleBlueHeader(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#BDD7EE"}, // FFBDD7EE
			Pattern: 1,
		},
		NumFmt: 3,
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
	})
	return style
}

func styleBorderOnly(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{
			WrapText: true,
		},
		NumFmt: 3,
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
	})
	return style
}

// func styleNumber(f *excelize.File) int {
// 	style, _ := f.NewStyle(&excelize.Style{
// 		NumFmt: 3, // #,##0
// 	})
// 	return style
// }










