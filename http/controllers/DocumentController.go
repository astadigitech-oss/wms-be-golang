package controllers

import (
	"context"
	"errors"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"liquid8/wms/services"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	// "time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// ==================== Manifest Inbound ====================
func IndexDocuments(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	status := strings.TrimSpace(c.Query("f"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	var documents []models.Document
	var total int64

	db := config.DB.Model(&models.Document{}).
		Where("document_product_type = ?", "reguler")

	// SEARCH: code_document OR base_document
	if q != "" {
		db = db.Where(
			"(code LIKE ? OR name_document LIKE ?)",
			"%"+q+"%", "%"+q+"%",
		)
	}

	// FILTER STATUS
	if status != "" {
		db = db.Where("(status_document LIKE ?)", "%"+status+"%")
	}

	// TOTAL COUNT (for pagination info)
	if err := db.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": err})
		return
	}

	// GET DATA
	if err := db.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&documents).Error; err != nil {

		c.JSON(500, gin.H{"success": false, "message": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Documents",
			"resource": gin.H{
				"current_page":   page,
				"data":           documents,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + len(documents),
				"total":          total,
			},
		},
	})
}

func DetailDocument(c *gin.Context) {
	query := c.Query("q")
	code_document := c.Param("code")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	limit := 50
	offset := (page - 1) * limit

	var productOlds []models.ProductOld
	var total int64

	db := config.DB.Model(&models.ProductOld{}).
		Where("code_document = ?", code_document)
		
	if query != "" {
		db = db.Where(
			"(old_barcode_product LIKE ? OR old_name_product LIKE ?)", "%"+query+"%", "%"+query+"%",
		)
	}

	// TOTAL COUNT (for pagination info)
	if err := db.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": err})
		return
	}

	// Query ProductOld
	err := db.
		Limit(limit).
		Offset(offset).
		Find(&productOlds).Error


	if err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// Ambil document berdasarkan code_document
	var document models.Document
	err = config.DB.Where("code = ?", code_document).First(&document).Error

	if err != nil {
		c.JSON(404, gin.H{
			"success": false,
			"message": "code document tidak ada",
		})
		return
	}

	// Response
	c.JSON(200, gin.H{
		"success": true,
		"message": "Data Document products",
		"data": gin.H{
			"document_name":  document.NameDocument,
			"status":         document.StatusDocument,
			"total_columns": document.TotalColumnDocument,
			"custom_barcode": document.CustomBarcode,
			"code_document":  document.Code,
			"resource": gin.H{
				"current_page":  page,
				"data":  productOlds,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + len(productOlds),
				"total":          total,
			},

		},
	})
}

func SearchProductOld(c *gin.Context)  {
	barcode := c.Param("barcode")
	code_document := c.Param("code")

	var response struct {
		Produk models.ProductOld `json:"produk"`
		ColorTags []models.ColorTag `json:"color_tags,omitempty"`
	}

	var productOld models.ProductOld

	// Query ProductOld
	err := config.DB.Where("code_document = ?", code_document).
		Where("old_barcode_product = ?", barcode).First(&productOld).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{
				"success": false,
				"message": "Product old not found",
			})
			return
		}else {
			c.JSON(500, gin.H{
				"success": false,
				"message": "server error: " + err.Error(),
			})
		}

		return
	}

	response.Produk = productOld

	if productOld.OldPriceProduct < 99999 {
		err := config.DB.Where("min_price_color <= ?", productOld.OldPriceProduct).Where("max_price_color >= ?", productOld.OldPriceProduct).Find(&response.ColorTags).Error
		if err != nil {
			c.JSON(500, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}

	// Response
	c.JSON(200, gin.H{
		"success": true,
		"message": "Produk Ditemukan",
		"resource": response,
	})
}

func GetUserScanWeb(c *gin.Context) {
	codeDocument := c.Param("code")
	
	type ScanSummary struct {
		TotalScansAll   int64 `json:"total_scans_all"`
		CountUser       int64 `json:"count_user"`
		TotalScansToday int64 `json:"total_scans_today"`
	}

	var wg sync.WaitGroup
	// Error channel dengan kapasitas 4 (untuk 4 Go routine)
	errChan := make(chan error, 4) 
	
	// Tempat penyimpanan hasil
	var usw []models.UserScanWeb
	summary := ScanSummary{}

	today := helpers.GetToday()

	// --- 1. Go Routine: Ambil Data Detail (usw) ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := config.DB.
			Model(&models.UserScanWeb{}).
			Preload("User").
			Where("code_document = ?", codeDocument).
			Find(&usw).Error
		
		if err != nil {
			errChan <- fmt.Errorf("error finding detail data: %w", err)
		}
	}()

    // Hitung TotalScansAll (MENGGUNAKAN SUM)
    wg.Add(1)
    go func() {
        defer wg.Done()
        
        // Kueri SUM(total_scan)
        var totalSum sql.NullInt64
        err := config.DB.
            Model(&models.UserScanWeb{}).
            Where("code_document = ?", codeDocument).
            Select("SUM(total_scans)").
            Row().
            Scan(&totalSum)

        if err != nil && err != sql.ErrNoRows { // Pastikan tidak mengabaikan error kecuali ErrNoRows (jika DB spesifik)
            errChan <- fmt.Errorf("error calculating total scans all: %w", err)
            return
        }
        if totalSum.Valid {
			summary.TotalScansAll = totalSum.Int64
		} else {
			summary.TotalScansAll = 0
		}
    }()
    
	// --- 3. Go Routine: Hitung User Unik (CountUser) ---
	wg.Add(1)
	go func() {
		defer wg.Done()
        // ... (Kueri sama seperti sebelumnya: COUNT(DISTINCT user_id))
		err := config.DB.
			Model(&models.UserScanWeb{}).
			Where("code_document = ?", codeDocument).
			Distinct("user_id").
			Count(&summary.CountUser).Error 

		if err != nil {
			errChan <- fmt.Errorf("error counting distinct users: %w", err)
		}
	}()
    
	// Hitung Scan Hari Ini (TotalScansToday)
	wg.Add(1)
	go func() {
		defer wg.Done()
        // ... (Kueri sama seperti sebelumnya: COUNT WHERE scanned_at >= startOfDay)
		var totalScanToday sql.NullInt64
		err := config.DB.
			Model(&models.UserScanWeb{}).
			Where("code_document = ?", codeDocument).
			Where("scan_date >= ?", today).
			Select("SUM(total_scans)").
            Row().
            Scan(&totalScanToday)

		if err != nil && err != sql.ErrNoRows { // Pastikan tidak mengabaikan error kecuali ErrNoRows (jika DB spesifik)
            errChan <- fmt.Errorf("error counting today's scans: %w", err)
            return
        }

        if totalScanToday.Valid {
			summary.TotalScansToday = totalScanToday.Int64
		} else {
			summary.TotalScansToday = 0
		}
	}()

	// Tunggu dan Cek Error
	wg.Wait()
	close(errChan) 

	for err := range errChan {
		if err != nil {
			c.JSON(500, gin.H{"success": false, "message": err.Error()})
			return
		}
	}
    
	// --- Response Final ---
	c.JSON(200, gin.H{
		"success":  true,
		"message":  "Detail Data with Summary (Parallel)",
		"resource": gin.H{
			"summary": summary,
			"data":    usw,
		},
	})
}

func ChangeCustomBarcode(c *gin.Context) {

	var request struct {
		CodeDocument string `json:"code_document" binding:"required"`
		CustomBarcode string `json:"custom_barcode" binding:"required"`
	}

	// ✅ Validasi body
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{
			"success": false,
			"message": "custom_barcode wajib diisi",
		})
		return
	}

	// ✅ TRANSACTION
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"status": false, "message": "Gagal memulai transaksi"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"status": false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	// ✅ Update langsung (tanpa SELECT dulu → cepat)
	result := tx.
		Model(&models.Document{}).
		Where("code = ?", request.CodeDocument).
		Where("document_product_type = ?", "reguler").
		Update("custom_barcode", request.CustomBarcode)

	// ✅ Cek error
	if result.Error != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal update barcode",
			"error":   result.Error.Error(),
		})
		return
	}

	// ✅ Commit
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Commit gagal",
		})
		return
	}

	// ✅ Response success
	c.JSON(200, gin.H{
		"success": true,
		"message": "Barcode berhasil diupdate",
		"data": gin.H{
			"code_document":  request.CodeDocument,
			"custom_barcode": request.CustomBarcode,
		},
	})
}

func DestroyDocument(c *gin.Context) {
	code_document := c.Param("code")

    tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"status": false, "message": "Gagal memulai transaksi"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"status": false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

    // HAPUS product_olds YANG TIDAK DIPAKAI
    deleteUnusedProductOld := `
        DELETE FROM product_olds
        WHERE code_document = ?
    `

	if err := tx.Exec(deleteUnusedProductOld, code_document).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal hapus product_old", "error": err})
        return
    }

    // Hapus document
    result := tx.Where("code = ?", code_document).
        Delete(&models.Document{})

    if result.Error != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal menghapus document", "error": result.Error})
        return
    }

    // COMMIT
    if err := tx.Commit().Error; err != nil {
        c.JSON(500, gin.H{"status": false, "message": "Gagal commit transaksi", "error": err})
        return
    }

    c.JSON(200, gin.H{
        "status":  true,
        "message": "Document berhasil di hapus",
    })
}

func DestroyProductOld(c *gin.Context)  {
	id_product_old := c.Param("id")

	// delete ProductOld
	query := `
        DELETE FROM product_olds
        WHERE id = ?
    `
	err := config.DB.Exec(query, id_product_old).Error

	if err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Response
	c.JSON(200, gin.H{
		"success": true,
		"message": "Berhasil dihapus",
		"resource": nil,
	})
}

// ===================== Riwayat Check ====================
func CheckHistories(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	var riwayatChecks []models.RiwayatCheck
	var total int64

	db := config.DB.Model(&models.RiwayatCheck{})

	// SEARCH: code_document OR base_document
	if q != "" {
		db = db.Where(
			"(code_document LIKE ? OR name_document LIKE ?)",
			"%"+q+"%", "%"+q+"%",
		)
	}

	// TOTAL COUNT (for pagination info)
	if err := db.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": err})
		return
	}

	// GET DATA
	if err := db.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&riwayatChecks).Error; err != nil {

		c.JSON(500, gin.H{"success": false, "message": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)
	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Documents",
			"resource": gin.H{
				"current_page":   page,
				"data":           riwayatChecks,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + len(riwayatChecks),
				"total":          total,
			},
		},
	})
}

func DetailHistory(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	var riwayatChecks []models.RiwayatCheck
	var total int64

	db := config.DB.Model(&models.RiwayatCheck{})

	// SEARCH: code_document OR base_document
	if q != "" {
		db = db.Where(
			"(code_document LIKE ? OR name_document LIKE ?)",
			"%"+q+"%", "%"+q+"%",
		)
	}

	// TOTAL COUNT (for pagination info)
	if err := db.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": err})
		return
	}

	// GET DATA
	if err := db.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&riwayatChecks).Error; err != nil {

		c.JSON(500, gin.H{"success": false, "message": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Documents",
			"resource": gin.H{
				"current_page":   page,
				"data":           riwayatChecks,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + len(riwayatChecks),
				"total":          total,
			},
		},
	})
}

// ===================== Slow Moving Product BKL ====================
func ListBKLDocuments(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 10
	offset := (page - 1) * limit

	var bklDocuments []models.BklDocument
	query := config.DB.Model(&models.BklDocument{})

	if q != "" {
		query = query.Where("code_bkl LIKE ?", "%"+q+"%")
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"message": "Terjadi kesalahan",
			"error":   err.Error(),
		})
		return
	}

	if err := query.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&bklDocuments).Error; err != nil {
		
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"message": "Terjadi kesalahan",
			"error":   err.Error(),
		})
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Documents",
			"resource": gin.H{
				"current_page":   page,
				"data":           bklDocuments,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + int(total),
				"total":          total,
			},
		},
	})
}
func GenerateBKLCode(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	// AMBIL USER 
	user := c.MustGet("auth_user").(models.User)

	// AMBIL DOKUMEN TERAKHIR
	var lastDoc models.BklDocument

	err := config.DB.
		Order("id DESC").
		First(&lastDoc).Error

	nextSequence := 1

	if err == nil {
		parts := strings.Split(lastDoc.CodeBkl, "-")
		if len(parts) > 0 {
			if lastNumber, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
				nextSequence = lastNumber + 1
			}
		}
	}

	// GENERATE CODE
	generatedCode := fmt.Sprintf("%d-BKL-%06d", user.ID, nextSequence)

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "Berhasil generate code",
		"data": gin.H{
			"code_document_bkl": generatedCode,
		},
	})
}
func DetailBKL(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Format ID Summary SO tidak valid"})
        return
    }

	var bklDocument models.BklDocument
	if err := config.DB.Preload("BklItem.ColorTag").First(&bklDocument, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "BKL Document tidak ditemukan"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "Detail BKL Document",
		"resource": bklDocument,
	})
}
func ProcessOlseraOutgoing(c *gin.Context) {
	type payloadRequest struct {
		DestinationID uint64 `json:"destination_id" binding:"required"`
		OlseraDocumentID string `json:"olsera_document_id" binding:"required"`
		OlseraDocumentCode string `json:"olsera_document_code" binding:"required"`
		DamageQty    *int   `json:"damage_qty" binding:"omitempty,min=0"`
		Colors       []struct {
			ColorTagID uint64 `json:"color_tag_id" binding:"required"`
			Qty        int    `json:"qty" binding:"required,min=1"`
		} `json:"colors" binding:"min=1,dive"`
	}

	var payload payloadRequest
	if err := c.ShouldBindJSON(&payload); err != nil {

		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": false,
				"message": "Format JSON tidak valid",
			})
			return
		}

		errorsMap := make(map[string]string)
		for _, e := range ve {
			field := e.Field()
			structField := e.StructField()
			namespace := e.Namespace()

			// ===== VALIDASI COLORS ARRAY =====
			if field == "Colors" {
				if e.Tag() == "min" {
					errorsMap["colors"] = "Daftar warna tidak boleh kosong"
				}
				continue
			}

			// ===== VALIDASI ITEM DALAM COLORS =====
			if strings.Contains(namespace, ".Colors[") {
				switch structField {
				case "ColorTagID":
					errorsMap["color_tag_id"] = "Color tag wajib diisi"
				case "Qty":
					errorsMap["qty"] = "Quantity minimal 1"
				default:
					errorsMap[strings.ToLower(structField)] =
						"Validasi gagal pada field " + structField
				}
				continue
			}

			// ===== FIELD LAIN =====
			switch field {
			case "DestinationID":
				errorsMap["destination_id"] = "Destination ID wajib diisi"
			case "OlseraDocumentID":
				errorsMap["olsera_document_id"] = "Olsera document ID wajib diisi"
			case "OlseraDocumentCode":
				errorsMap["olsera_document_code"] = "Code document olsera wajib diisi"
			case "DamageQty":
				errorsMap["damage_qty"] = "Damage qty minimal 1"
			default:
				errorsMap[strings.ToLower(field)] =
					"Validasi gagal pada field " + field
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validasi gagal",
			"errors": errorsMap,
		})
		return
	}

	// TRANSACTION
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal memulai transaksi"})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"success": false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	var destination models.MigrateColorDestination
	if err := tx.First(&destination, payload.DestinationID).Error; err != nil {
		tx.Rollback()
		helpers.ErrorResponse(c, 404, "Toko tidak ditemukan, pastikan Destination ID sudah benar", err)
		return
	}

	log := helpers.NewLogger("./logs/app.log")
	olseraService := services.NewOlseraService(&destination, log)

	resp, err := olseraService.GetDetailOutgoingStock(c.Request.Context(), map[string]string{"id": payload.OlseraDocumentID})
	if err != nil {
		tx.Rollback()
		helpers.ErrorResponse(c, 400, "Gagal menarik detail olsera", err)
		return
	}

	dataRes,_ := resp.Data.(map[string]interface{})
	dataRes2,_ := dataRes["data"].(map[string]interface{})
	status,_ := dataRes2["status"].(string)
	status_desc,_ := dataRes2["status_desc"].(string)
	if status != "D" {
		tx.Rollback()
		helpers.ErrorResponse(c, 500, fmt.Sprintf("Validasi Gagal: status olsera %s (%s)", status_desc, status), nil)
		return
	}

	var colorTagIDs []uint64
	for _, c := range payload.Colors {
		colorTagIDs = append(colorTagIDs, c.ColorTagID)
	}

	// ===============================
	// Hitung Qty Olsera
	// ===============================
	olseraQty := map[string]int{"24": 0, "12": 0}
	productName := map[string]string{}

	items,_ := dataRes2["items"].([]interface{})
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			continue			
		}

		oName,_ := item["product_name"].(string) 
		qty,_ := item["qty"].(float64) 

		// name := strings.ToUpper(oName)
		if strings.Contains(oName, "24") {
			olseraQty["24"] += int(qty)
			productName["24"] = oName
		}
		if strings.Contains(oName, "12") {
			olseraQty["12"] += int(qty)
			productName["12"] = oName
		}
	}
	// ===============================
	// Ambil Semua ColorTags Sekali Query
	// ===============================
	var colorTags []models.ColorTag
	colorIDs := make([]uint64, 0)
	for _, c := range payload.Colors {
		colorIDs = append(colorIDs, c.ColorTagID)
	}

	if len(colorIDs) > 0 {
		if err := tx.Where("id IN ?", colorIDs).Find(&colorTags).Error; err != nil {
			tx.Rollback()
			helpers.ErrorResponse(c, 500, "Gagal mengambil color tag", err)
			return
		}
	}

	colorMap := make(map[uint64]string)
	for _, ct := range colorTags {
		colorMap[ct.ID] = strings.ToLower(ct.NameColor)
	}

	valid24 := map[string]bool{"merah": true, "biru": true, "big": true}
	valid12 := map[string]bool{"kuning": true, "hijau": true, "small": true}

	inputQty := map[string]int{"24": 0, "12": 0}
	mapped := map[string][]struct{
		ColorID uint64
		Tag string
		Qty int
	}{"24": {}, "12": {}}

	for _, col := range payload.Colors {
		name := colorMap[col.ColorTagID]
		name, exists := colorMap[col.ColorTagID]
		if !exists {
			tx.Rollback()
			helpers.ErrorResponse(c, 422, fmt.Sprintf("Color tag tidak ditemukan | Color ID : %d", col.ColorTagID), nil)
			return
		}

		if valid24[name] {
			inputQty["24"] += col.Qty
			mapped["24"] = append(mapped["24"], struct{
				ColorID uint64
				Tag string
				Qty int
			}{col.ColorTagID, name, col.Qty})
		}
		if valid12[name] {
			inputQty["12"] += col.Qty
			mapped["12"] = append(mapped["12"], struct{
				ColorID uint64
				Tag string
				Qty int
			}{col.ColorTagID, name, col.Qty})
		}
	}

	unaccounted24 := olseraQty["24"] - inputQty["24"]
	unaccounted12 := olseraQty["12"] - inputQty["12"]

	if unaccounted24 < 0 || unaccounted12 < 0 {
		tx.Rollback()
		helpers.ErrorResponse(c, 422, "QC yang diinputkan melebihi stok olsera", err)
		return
	}

	totalUnaccounted := unaccounted24 + unaccounted12
	damagedQTY := 0
	if payload.DamageQty != nil {
		damagedQTY = *payload.DamageQty
	}
	if damagedQTY > totalUnaccounted {
		tx.Rollback()
		helpers.ErrorResponse(c, 422, fmt.Sprintf("Damage QTY melebihi sisa barang yang belum di QC (%d)", totalUnaccounted), nil)
		return
	}

	totalLost := totalUnaccounted - damagedQTY
	allocatedDamage := map[string]int{"24": 0, "12":0}
	remainingDamage := damagedQTY
	for _,cat := range []string{"24", "12"} {
		take := unaccounted24
		if cat == "12" {
			take = unaccounted12
		}

		if remainingDamage < take {
			take = remainingDamage
		}
		allocatedDamage[cat] = take
		remainingDamage -= take
	}
	// ===============================
	// Create Document
	// ===============================
	user := c.MustGet("auth_user").(models.User)
	document := models.BklDocument{
		CodeBkl: payload.OlseraDocumentCode,
		Status: "done",
		UserID: uint64(user.ID),
	}

	if err := tx.
		Where(models.BklDocument{CodeBkl: payload.OlseraDocumentCode}).
		FirstOrCreate(&document, models.BklDocument{
			CodeBkl: payload.OlseraDocumentCode,
			Status:  "done",
			UserID:  uint64(user.ID),
		}).Error; err != nil {
		tx.Rollback()
		helpers.ErrorResponse(c, 500, "Gagal membuat document", err)
		return
	}
	// ===============================
	// Create History BKL Item
	// ===============================
	//catat history color
	if len(payload.Colors) > 0 {
		for _,col := range payload.Colors {
			bkl_item := models.BklItem{
				BklDocumentID: document.ID,
				Type: "in",
				Qty: col.Qty,
				TagColorID: &col.ColorTagID,
				IsDamaged: false,
				IsLost: false,
			}

			if err := tx.Create(&bkl_item).Error; err != nil {
				tx.Rollback()		
				helpers.ErrorResponse(c, 500, "Gagal membuat history bkl item color", err)		
				return
			}
		}
	}
	//catat history damaged
	if damagedQTY > 0 {
		bkl_item := models.BklItem{
			BklDocumentID: document.ID,
			Type: "in",
			Qty: damagedQTY,
			IsDamaged: true,
			IsLost: false,
		}

		if err := tx.Create(&bkl_item).Error; err != nil {
			tx.Rollback()		
			helpers.ErrorResponse(c, 500, "Gagal membuat history bkl item type damage", err)		
			return
		}
	}
	//catat history lost
	if totalLost > 0 {
		bkl_item := models.BklItem{
			BklDocumentID: document.ID,
			Type: "in",
			Qty: totalLost,
			IsDamaged: false,
			IsLost: true,
		}

		if err := tx.Create(&bkl_item).Error; err != nil {
			tx.Rollback()		
			helpers.ErrorResponse(c, 500, "Gagal membuat history bkl item type lost", err)		
			return
		}
	}
	// ===============================
	// Batch Insert BklProduct
	// ===============================
	var products []models.BklProduct
	// dateIn := helpers.GetCurentTime()
	for cat, list := range mapped {
		if olseraQty[cat] == 0 {
			continue
		}

		price := float64(12000)
		if cat == "24" {
			price = float64(24000)
		}

		for _, entry := range list {
			for i := 0; i < entry.Qty; i++ {
				generateBarcode := "BKL-" + helpers.RandomString(10)
				products = append(products, models.BklProduct{
					CodeDocument: payload.OlseraDocumentCode,
					OldBarcodeProduct: &generateBarcode,
					OldQuantityProduct: 1,
					OldNameProduct: productName[cat],
					OldPriceProduct: price,
					Barcode: generateBarcode,
					Name: productName[cat],
					Quantity: 1,
					Status: "display",
					TagColorID: &entry.ColorID,
					Price: price,
					DisplayPrice: price,
					Quality: "lolos",
					ActualQuality: "lolos",
					ActualOldPrice: price,
				})
			}
		}

		qualty_text := "damaged"
		for i := 0; i < allocatedDamage[cat]; i++ {
			generateBarcode := "BKL-" + helpers.RandomString(10)
			products = append(products, models.BklProduct{
				CodeDocument: payload.OlseraDocumentCode,
				OldBarcodeProduct: &generateBarcode,
				OldQuantityProduct: 1,
				OldNameProduct: productName[cat],
				OldPriceProduct: price,
				Barcode: generateBarcode,
				Name: productName[cat],
				Quantity: 1,
				Status: "display",
				Price: price,
				DisplayPrice: price,
				Quality: "damaged",
				QualityText: &qualty_text,
				ActualQuality: "damaged",
				ActualOldPrice: price,
			})
		}
	}

	if len(products) > 0 {
		if err := tx.CreateInBatches(products, 500).Error; err != nil {
			tx.Rollback()
			helpers.ErrorResponse(c, 500, "Gagal membuat Bkl Product", err)
			return
		}
	}
	// ===============================
	// Create User Log Action
	// ===============================
	meta := map[string]interface{}{}
	if err := helpers.LogUserAction(user.ID, user.Name, fmt.Sprintf("Memproses QC olsera outgoing dengan code document %s", payload.OlseraDocumentCode), "bkl/create/list-return-olsera", meta); err != nil {
		tx.Rollback()
		helpers.ErrorResponse(c, 500, "Gagal mencatat log user action", err)
		return
	}
	// ===============================
	// Update Status Olsera
	// ===============================
	if _, err := olseraService.UpdateStatusStockInOut(c.Request.Context(), map[string]string{
		"pk": payload.OlseraDocumentID,
		"status": "P",
	}); err != nil {
		tx.Rollback()
		helpers.ErrorResponse(c, 500, "Gagal publish olsera", err)
		return
	}

	// Commit
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Commit failed"})
		return
	}

	c.JSON(200, gin.H{
		"status": true,
		"message": "QC Selesai & Batch Insert berhasil",
	})
}
// func CreateBKL(c *gin.Context) {
// 	type payloadRequest struct {
// 		NameDocument string `json:"name_document" binding:"required"`
// 		Type         string `json:"type" binding:"required,oneof=in out"`
// 		DamageQty    *int   `json:"damage_qty" binding:"omitempty,min=1"`
// 		Colors       []struct {
// 			ColorTagID uint64 `json:"color_tag_id" binding:"required"`
// 			Qty        int    `json:"qty" binding:"required,min=1"`
// 		} `json:"colors" binding:"min=1,dive"`
// 	}

// 	var payload payloadRequest
// 	if err := c.ShouldBindJSON(&payload); err != nil {

// 		ve, ok := err.(validator.ValidationErrors)
// 		if !ok {
// 			c.JSON(http.StatusBadRequest, gin.H{
// 				"status": false,
// 				"message": "Format JSON tidak valid",
// 			})
// 			return
// 		}

// 		errorsMap := make(map[string]string)

// 		for _, e := range ve {
// 			field := e.Field()
// 			structField := e.StructField()
// 			namespace := e.Namespace()

// 			// ===== VALIDASI COLORS ARRAY =====
// 			if field == "Colors" {
// 				if e.Tag() == "min" {
// 					errorsMap["colors"] = "Daftar warna tidak boleh kosong"
// 				}
// 				continue
// 			}

// 			// ===== VALIDASI ITEM DALAM COLORS =====
// 			if strings.Contains(namespace, ".Colors[") {
// 				switch structField {
// 				case "ColorTagID":
// 					errorsMap["color_tag_id"] = "Color tag wajib diisi"
// 				case "Qty":
// 					errorsMap["qty"] = "Quantity minimal 1"
// 				default:
// 					errorsMap[strings.ToLower(structField)] =
// 						"Validasi gagal pada field " + structField
// 				}
// 				continue
// 			}

// 			// ===== FIELD LAIN =====
// 			switch field {
// 			case "NameDocument":
// 				errorsMap["name_document"] = "Nama dokumen wajib diisi"
// 			case "Type":
// 				errorsMap["type"] = "Type harus bernilai in atau out"
// 			case "DamageQty":
// 				errorsMap["damage_qty"] = "Damage qty minimal 1"
// 			default:
// 				errorsMap[strings.ToLower(field)] =
// 					"Validasi gagal pada field " + field
// 			}
// 		}

// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"status": false,
// 			"message": "Validasi gagal",
// 			"errors": errorsMap,
// 		})
// 		return
// 	}

// 	var colorTagIDs []uint64
// 	for _, c := range payload.Colors {
// 		colorTagIDs = append(colorTagIDs, c.ColorTagID)
// 	}

// 	var existingIDs []uint64
// 	err := config.DB.
// 		Model(&models.ColorTag{}).
// 		Where("id IN ?", colorTagIDs).
// 		Pluck("id", &existingIDs).Error

// 	if err != nil {
// 		c.JSON(500, gin.H{"status": false, "message": "Gagal validasi color tag"})
// 		return
// 	}

// 	if len(existingIDs) != len(colorTagIDs) {
// 		c.JSON(http.StatusUnprocessableEntity, gin.H{
// 			"status": false,
// 			"message": "Salah satu color tag tidak ditemukan",
// 		})
// 		return
// 	}

// 	// AMBIL USER 
// 	user := c.MustGet("auth_user").(models.User)

// 	// TRANSACTION
// 	tx := config.DB.WithContext(c.Request.Context()).Begin()
// 	if tx.Error != nil {
// 		c.JSON(500, gin.H{"status": false, "message": "Gagal memulai transaksi"})
// 		return
// 	}

// 	defer func() {
// 		if r := recover(); r != nil {
// 			tx.Rollback()
// 			c.JSON(500, gin.H{
// 				"status": false,
// 				"message": "Terjadi kesalahan internal",
// 				"error": fmt.Sprintf("%v", r),
// 			})
// 		}
// 	}()

// 	// INSERT BKL DOCUMENT
// 	document := models.BklDocument{
// 		CodeBkl: payload.NameDocument,
// 		Status:  "done",
// 		UserID:  uint64(user.ID),
// 	}

// 	if err := tx.Create(&document).Error; err != nil {
// 		tx.Rollback()
// 		c.JSON(500, gin.H{
// 			"status": false,
// 			"message": "Gagal menyimpan dokumen",
// 			"error": err.Error(),
// 		})
// 		return
// 	}

// 	// SIMPAN ITEMS
// 	if payload.DamageQty != nil {
// 		item := models.BklItem{
// 			BklDocumentID: document.ID,
// 			Qty:           *payload.DamageQty,
// 			Type:          payload.Type,
// 			IsDamaged:     true,
// 		}
// 		if err := tx.Create(&item).Error; err != nil {
// 			tx.Rollback()
// 			c.JSON(500, gin.H{"status": false, "message": "Gagal menyimpan item", "error": err.Error()})
// 			return
// 		}
// 	}

// 	for _, item := range payload.Colors {
// 		item := models.BklItem{
// 			BklDocumentID: document.ID,
// 			TagColorID:    &item.ColorTagID,
// 			Qty:           item.Qty,
// 			Type:          payload.Type,
// 			IsDamaged:     false,
// 		}
// 		if err := tx.Create(&item).Error; err != nil {
// 			tx.Rollback()
// 			c.JSON(500, gin.H{"status": false, "message": "Gagal menyimpan item"})
// 			return
// 		}
// 	}

	// // Commit
	// if err := tx.Commit().Error; err != nil {
	// 	c.JSON(500, gin.H{"success": false, "message": "Commit failed"})
	// 	return
	// }

// 	c.JSON(http.StatusOK, gin.H{
// 		"status": true,
// 		"message": "BKL Berhasil Dibuat",
// 	})
// }
func ToEditBKL(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Format ID Summary SO tidak valid"})
        return
    }

	var bklDocument models.BklDocument
	if err := config.DB.First(&bklDocument, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "BKL tidak ditemukan"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal mengambil data BKL", "error": err.Error()})
		}

		return
	}

	if err := config.DB.Model(&bklDocument).Update("status", "process").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal memperbarui data BKL", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true, 
		"message": "Mode Edit Aktif",
		"resource": bklDocument,
	})
}
func UpdateBKL(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Format ID Summary SO tidak valid"})
        return
    }

	type payloadRequest struct {
		NameDocument string `json:"name_document" binding:"required"`
		Type         string `json:"type" binding:"required,oneof=in out"`
		DamageQty    *int   `json:"damage_qty" binding:"omitempty,min=1"`
		Colors       []struct {
			ColorTagID uint64 `json:"color_tag_id" binding:"required"`
			Qty        int    `json:"qty" binding:"required,min=1"`
		} `json:"colors" binding:"min=1,dive"`
	}

	var payload payloadRequest
	if err := c.ShouldBindJSON(&payload); err != nil {

		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": false,
				"message": "Format JSON tidak valid",
			})
			return
		}

		errorsMap := make(map[string]string)

		for _, e := range ve {
			field := e.Field()
			structField := e.StructField()
			namespace := e.Namespace()

			// ===== VALIDASI COLORS ARRAY =====
			if field == "Colors" {
				if e.Tag() == "min" {
					errorsMap["colors"] = "Daftar warna tidak boleh kosong"
				}
				continue
			}

			// ===== VALIDASI ITEM DALAM COLORS =====
			if strings.Contains(namespace, ".Colors[") {
				switch structField {
				case "ColorTagID":
					errorsMap["color_tag_id"] = "Color tag wajib diisi"
				case "Qty":
					errorsMap["qty"] = "Quantity minimal 1"
				default:
					errorsMap[strings.ToLower(structField)] =
						"Validasi gagal pada field " + structField
				}
				continue
			}

			// ===== FIELD LAIN =====
			switch field {
			case "NameDocument":
				errorsMap["name_document"] = "Nama dokumen wajib diisi"
			case "Type":
				errorsMap["type"] = "Type harus bernilai in atau out"
			case "DamageQty":
				errorsMap["damage_qty"] = "Damage qty minimal 1"
			default:
				errorsMap[strings.ToLower(field)] =
					"Validasi gagal pada field " + field
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"message": "Validasi gagal",
			"errors": errorsMap,
		})
		return
	}

	// TRANSACTION
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"status": false, "message": "Gagal memulai transaksi"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"status": false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	var bklDocument models.BklDocument
	if err := tx.First(&bklDocument, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "BKL tidak ditemukan"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal mengambil data BKL", "error": err.Error()})
		}

		return
	}

	// Delete Item
	if err := tx.
		Where("bkl_document_id = ?", bklDocument.ID).
		Delete(&models.BklItem{}).Error; err != nil {

		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Terjadi keslahan",
			"error": err.Error(),
		})
		return
	}

	//save item
	if payload.DamageQty != nil {
		item := models.BklItem{
			BklDocumentID: bklDocument.ID,
			Qty:           *payload.DamageQty,
			Type:          payload.Type,
			IsDamaged:     true,
		}
		if err := tx.Create(&item).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"status": false, "message": "Gagal menyimpan item", "error": err.Error()})
			return
		}
	}

	for _, item := range payload.Colors {
		item := models.BklItem{
			BklDocumentID: bklDocument.ID,
			TagColorID:    &item.ColorTagID,
			Qty:           item.Qty,
			Type:          payload.Type,
			IsDamaged:     false,
		}
		if err := tx.Create(&item).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"status": false, "message": "Gagal menyimpan item"})
			return
		}
	}


	if err := tx.Model(&bklDocument).Update("status", "done").Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal mengupdate status BKL",
			"error": err.Error(),
		})
		tx.Rollback()
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Terjadi Kesalahan", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true, 
		"message": "BKL Berhasil Diupdate",
		"resource": bklDocument,
	})
}
func ListOlseraOutgoing(c *gin.Context) {
	type outgoingItem struct {
		ID            uint    `json:"id"`
		TransNo       string    `json:"trans_no"`
		Note            string    `json:"note"`
		Status        string    `json:"status"`
		Date          string `json:"date"`
		DestinationID uint      `json:"destination_id"`
		ShopName      string    `json:"shop_name"`
	}

	type paginatedResponse struct {
		Page       int            `json:"current_page"`
		Data       []outgoingItem `json:"data"`
		From      int            `json:"from"`
		To      int            `json:"to"`
		Total      int            `json:"total"`
		PerPage    int            `json:"per_page"`
		Link	   []gin.H			`json:"links"`
		LastPage int            `json:"last_page"`
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	searchQuery := strings.ToLower(c.Query("q"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage := 10

	var destinations []models.MigrateColorDestination
	if err := config.DB.Where("is_olsera_integreted = ?", true).Find(&destinations).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mengambil data destination", err)
		return
	}

	sem := make(chan struct{}, 1)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allDrafts []outgoingItem
	log := helpers.NewLogger("./logs/app.log")
	for _, dest := range destinations {
		wg.Add(1)
		
		go func(destination models.MigrateColorDestination) {
			defer wg.Done()

			//Worker Pool (Limit concurrency)
			sem <- struct{}{}
			defer func() { <-sem }()

			olseraService := services.NewOlseraService(&destination, log)

			resp, err := olseraService.GetOutgoingStockList(ctx, map[string]string{
				"search_column[]": "status",
				"search_text[]": "D",
			})
			if err != nil {
				// helpers.ErrorResponse(c, 500, "Gagal melakukan fetching data ke olsera", err)
				log.WithError(err).Error("Fetching failed")
				// return
			}

			if resp.Success && resp.StatusCode == 200 {
				dataRes, _ := resp.Data.(map[string]interface{})
				dataRes2, _ := dataRes["data"].([]interface{})
	
				for _, rawItem := range dataRes2 {

					item, ok := rawItem.(map[string]interface{})
					if !ok {
						log.Error("item gagal parse")
						continue
					}

					// SAFE parsing
					idFloat, _ := item["id"].(float64)
					id := uint(idFloat)

					transNo, _ := item["trans_no"].(string)
					note, _ := item["note"].(string)
					status, _ := item["status"].(string)
					dateStr, _ := item["date"].(string)

					if searchQuery != "" {
						if !strings.Contains(strings.ToLower(destination.ShopName), searchQuery) &&
							!strings.Contains(strings.ToLower(transNo), searchQuery) {
							continue
						}
					}

					out := outgoingItem{
						ID:            id,
						TransNo:       transNo,
						Note:          note,
						Status:        status,
						Date:          dateStr,
						DestinationID: uint(destination.ID),
						ShopName:      destination.ShopName,
					}

					mu.Lock()
					allDrafts = append(allDrafts, out)
					mu.Unlock()
				}
			}
		}(dest)
	}

	wg.Wait()

	// Sorting
	sort.Slice(allDrafts, func(i, j int) bool {
		parsedDatei, _ := helpers.ParseFlexibleDate(allDrafts[i].Date)
		parsedDatej, _ := helpers.ParseFlexibleDate(allDrafts[j].Date)

		return parsedDatei.After(parsedDatej)
	})

	// pagination
	total := len(allDrafts)
	start := (page - 1) * perPage
	end := start + perPage

	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	
	// pagination links
	lasPage := int(math.Ceil(float64(total) / float64(perPage)))
	links := helpers.BuildPaginationLinks(c, page, lasPage)
	paginated := allDrafts[start:end]

	response := paginatedResponse{
		Page:       page,
		Data:       paginated,
		From: start + 1,
		To: start + total,
		Total:      total,
		PerPage:    perPage,
		Link: links,
		LastPage: lasPage,
	}

	c.JSON(200, gin.H{
		"success":  true,
		"message": "List Antrean Retur Olsera",
		"resource": response,
	})
}
func DetailOlseraOutgoing(c *gin.Context) {
	type summaryExpectedQty struct {
		TotalQty24K int `json:"total_qty_24K"`
		TotalQty12K int `json:"total_qty_12K"`
		TotalQty    int `json:"total_qty"`
	}

	id := c.Param("id")

	destinationID := c.Query("destination_id")
	if destinationID == "" {
		helpers.ErrorResponse(c, 400, "destination ID wajib ada", nil)
		return
	}

	var destination models.MigrateColorDestination
	if err := config.DB.First(&destination, destinationID).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Toko tidak ditemukan atau sudah terhapus", nil)
		return
	}

	log := helpers.NewLogger("./logs/app.log")
	olseraService := services.NewOlseraService(&destination, log)

	resp, err := olseraService.GetDetailOutgoingStock(c.Request.Context(), map[string]string{"id": id})
	if err != nil {
		helpers.ErrorResponse(c, 400, "Gagal menarik detail olsera", err)
		return
	}

	dataRes,_ := resp.Data.(map[string]interface{})
	dataRes2,_ := dataRes["data"].(map[string]interface{})
	itemList, _ := dataRes2["items"].([]interface{})

	var total24K, total12K int

	for _, rawItem := range itemList {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}

		name,_ := item["product_name"].(string)
		qty, _ := item["qty"].(float64)

		switch {
		case strings.Contains(name, "24"):
			total24K += int(qty)
		case strings.Contains(name, "12"):
			total12K += int(qty)
		}
	}

	summary := summaryExpectedQty{
		TotalQty24K: total24K,
		TotalQty12K: total12K,
		TotalQty:    total24K + total12K,
	}

	dataRes2["destination_id"] = destination.ID
	dataRes2["summary_expected_qty"] = summary

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message": "Detail Return Olsera",
		"resource":  dataRes2,
	})
}

// ===================== Migrate To Repair ====================
// Migrate to repair
func ListMigrateRepairDocs(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 15
	offset := (page - 1) * limit

	var migrate_repair_docs []models.MigrateRepairDocument
	query := config.DB.Model(&models.MigrateRepairDocument{})

	if q != "" {
		searchQuery := "%" + q + "%"
		query = query.Where("(code LIKE ? OR name_user LIKE ?)", searchQuery, searchQuery)
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"message": "Terjadi kesalahan",
			"error":   err.Error(),
		})
		return
	}

	if err := query.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&migrate_repair_docs).Error; err != nil {
		
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"message": "Terjadi kesalahan",
			"error":   err.Error(),
		})
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Document Migrate Repair",
			"resource": gin.H{
				"current_page":   page,
				"data":           migrate_repair_docs,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + int(total),
				"total":          total,
			},
		},
	})
}

func GetMigrateRepairIndex(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	q := c.Query("q")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))

	offset := (page - 1) * limit

	defer func() {
		if r := recover(); r != nil {
			c.JSON(500, gin.H{
				"status": false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	var repair models.MigrateRepairDocument

	// Cari repair yang status proses
	err := config.DB.
		Where("user_id = ? AND status = ?", user.ID, "process").
		Order("created_at DESC").
		First(&repair).Error

	// Kalau tidak ada, buat baru
	if errors.Is(err, gorm.ErrRecordNotFound) {

		code, err := helpers.GenerateCodeMigrateRepair(config.DB)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal menghasilkan kode", "error": err.Error()})
			return
		}

		repair = models.MigrateRepairDocument{
			UserID:   uint64(user.ID),
			NameUser: user.Name,
			Status:   "process",
			Code:     code,
		}

		links := helpers.BuildPaginationLinks(c, page, 1)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "List Migrate Repair Items",
			"resource":  gin.H{
				"migrate_document": repair,
				"migrate_bulky_product": gin.H{
					"current_page": page,
					"data": []models.MigrateRepairItem{},
					"links": links,
					"per_page": limit,
					"total_data": 0,
				},
			},
		})
	}

	// Ambil items berdasarkan repair ID
	type productsData struct {
        ID          uint64  `json:"id"`
        OldBarcode  string  `json:"old_barcode"`
        NewBarcode     string  `json:"new_barcode"`
        Name        string  `json:"name"`
        Price       float64 `json:"price"`
        OldPrice       float64 `json:"old_price"`
        Status      string  `json:"status"`
        Category    string  `json:"category"`
		IsSO		string  `json:"is_so"`
    }

	var items []productsData
	query := config.DB.Table("migrate_repair_items").
		Select(`
			migrate_repair_items.id AS id,
			products.old_barcode_product AS old_barcode,
			products.barcode AS new_barcode,
			products.name AS name,
			products.price AS price,
			products.old_price_product AS old_price,
			products.status AS status,
			categories.name_category AS category,
			products.is_so AS is_so
		`).
		Joins("JOIN products ON products.id = migrate_repair_items.product_id").
		Joins("JOIN categories ON categories.id = products.category_id").
		Where("repair_document_id = ?", repair.ID)

	if q != "" {
		query = query.Where("(products.name LIKE ? OR products.barcode LIKE ? OR products.old_barcode_product LIKE ?)", "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}

	var totalData int64
	if err := query.Session(&gorm.Session{}).Count(&totalData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if err := query.
		Order("migrate_repair_items.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&items).Error; err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	//build pagination
	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))

	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// Tambah status readable
	for i := range items {
		if items[i].IsSO == "done" {
			items[i].IsSO = "Sudah SO"
		} else {
			items[i].IsSO = "Belum SO"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "List Migrate Repair Items",
		"resource":  gin.H{
			"migrate_document": repair,
			"migrate_bulky_product": gin.H{
				"current_page": page,
				"data": items,
				"links": links,
				"per_page": limit,
				"total_data": totalData,
			},
		},
	})
}

func DeleteMigrateRepairItem(c *gin.Context) {
	item_id := c.Param("item_id")

	//Start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"status": false, "message": "Gagal memulai transaksi"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"status": false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	//mbil repair item
	var repair_item models.MigrateRepairItem
	if err := tx.First(&repair_item, item_id).Error;  err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Repair item not found"})
		}else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal mengambil data repair item", "error": err.Error()})
		}

		tx.Rollback()
		return
	}

	//ambil repair doc
	var migrate_doc models.MigrateRepairDocument
	if err := tx.
		Preload("MigrateRepairItem").
		Where("id = ?", repair_item.RepairDocumentID).First(&migrate_doc).Error;  err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Repair document not found"})
		}else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal mengambil data repair document", "error": err.Error()})
		}

		tx.Rollback()
		return
	}

	// update quality product
	result := tx.Model(&models.Product{}).Where("id = ?", repair_item.ProductID).
		Updates(map[string]interface{}{"status": "display", "quality": "lolos", "quality_text": nil})
	
	if result.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "gagal update quality product", "error": result.Error.Error()})
		return
	}

	if result.RowsAffected == 0 {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Product not found or quality not updated"})
		return
	}

	//hapus repair item
	if err := tx.Delete(&repair_item).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal menghapus repair item", "error": err.Error()})
		return
	}

	//cek jika sisa product 0 maka hapus document
	remainingItems := len(migrate_doc.MigrateRepairItem) - 1
	if remainingItems == 0 {
		if err := tx.Delete(&migrate_doc).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal menghapus repair document", "error": err.Error()})
			return
		}
	}

	//commit
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Commit gagal",
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Repair Item berhasil di hapus",
	})
}

func DetailMigrateRepairDocs(c *gin.Context) {
	id := c.Param("id")

	var migrate_repair_doc models.MigrateRepairDocument
	if err := config.DB.Preload("MigrateRepairItem", func(db *gorm.DB) *gorm.DB {
		return db.
            Joins("JOIN products ON products.id = migrate_repair_items.product_id").
            Where("products.status NOT IN ?", []string{"dump", "scrap_qcd"}).
            Preload("Product")
	}).First(&migrate_repair_doc, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Migrate repair dokumen tidak ditemukan",
			})
		}else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Terjadi kesalahan",
				"error":   err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Detail Document Migrate Repair",
		"resource": migrate_repair_doc,
	})
}

func ListMigrateProducts(c *gin.Context) {
    q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	//inisialisasi query
	baseQuery := config.DB.Model(&models.Product{}).
        Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
        Where("products.status NOT IN ?", []string{"dump", "migrate", "scrap_qcd", "sale", "repair"}).
        Where("products.category_id IS NOT NULL").
        Where("products.tag_color_id IS NULL").
        Where("products.quality = ?", "lolos").
		Where("NOT EXISTS (SELECT 1 FROM migrate_repair_items mri WHERE mri.product_id = products.id)").
        Where("(categories.name_category LIKE ? OR categories.name_category ?)", "%"+ "ELEKTRONIK" +"%", "%"+ "REFURBISHED" +"%")

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(products.barcode LIKE ? OR "+
            "products.old_barcode_product LIKE ? OR " + 
            "products.name LIKE ?)", searchPattern, searchPattern, searchPattern)
	}

    // Paginate Data
    type productsData struct {
        ID          uint64  `json:"id"`
        OldBarcode  string  `json:"old_barcode"`
        NewBarcode     string  `json:"new_barcode"`
        Name        string  `json:"name"`
        Price       float64 `json:"price"`
        OldPrice       float64 `json:"old_price"`
        Status      string  `json:"status"`
        Category   *string  `json:"category"`
    }

    var products []productsData
	var totalData int64

    baseQuery.Session(&gorm.Session{}).Count(&totalData)

    // Ambil data detail
    err := baseQuery.Session(&gorm.Session{}).
        Select(`
            products.id, 
            products.old_barcode_product AS old_barcode, 
            products.barcode AS new_barcode, 
            products.name AS name, 
            products.price AS price, 
            products.old_price_product AS old_price, 
            products.status AS status, 
            COALESCE(color_tags.name_color, categories.name_category) AS category
        `).
        Order("products.created_at DESC").
        Limit(limit).Offset(offset).
        Find(&products).Error

    if err != nil {
        c.JSON(500, gin.H{"success": false, "message": "error", "error": err.Error()})
        return
    }

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))
	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Migrate Product",
			"resource": gin.H{
                "total_data":           totalData,
                "data":                 products,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + int(totalData),
				"total":          totalData,
			},
		},
	})
}

func AddMigrateProduct(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	type payloadRequest struct {
		Barcode     string `json:"barcode" binding:"required"`
		Description string `json:"description" binding:"required,min=3"`
	}

	var payload payloadRequest
	if err := c.ShouldBindJSON(&payload); err != nil {

		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": false,
				"message": "Format JSON tidak valid",
			})
			return
		}

		errorsMap := make(map[string]string)

		for _, e := range ve {
			field := e.Field()
			// direct fields mapping
			switch field {
			case "Description":
				if e.Tag() == "required" {
					errorsMap["description"] = "Deskripsi wajib diisi"
				} else if e.Tag() == "min" {
					errorsMap["description"] = "Deskripsi minimal 3 karakter"
				}
			case "Barcode":
				if e.Tag() == "required" {
					errorsMap["barcode"] = "Barcode wajib diisi"
				}
			default:
				errorsMap[strings.ToLower(field)] =
					"Validasi gagal pada field " + field
			}

		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"message": "Validasi gagal",
			"errors": errorsMap,
		})
		return
	}

	// TRANSACTION
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"status": false, "message": "Gagal memulai transaksi"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"status": false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	var product models.Product
	if err := tx.Preload("Category").
		Where("category_id IS NOT NULL").
		Where("barcode = ?", payload.Barcode).
		Where("quality = ?", "lolos").
		Where("status NOT IN ?", []string{"dump", "migrate", "scrap_qcd", "sale", "repair"}).
		First(&product).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  false,
				"message": "Product not found",
			})
		}else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Internal server error",
				"error":   err.Error(),
			})
		}
		tx.Rollback()
		return
	}

	if !strings.Contains(strings.ToUpper(product.Category.NameCategory), "ELEKTRONIK") {
		tx.Rollback()
		c.JSON(422, gin.H{
			"errors": gin.H{
				"barcode": []string{
					"Scan Gagal! Bukan kategori ELEKTRONIK.",
				},
			},
		})
		return
	}

	var migrateRepair models.MigrateRepairDocument

	err := tx.
		Where("user_id = ? AND status = ?", user.ID, "process").
		First(&migrateRepair).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// data belum ada → create
			code, err := helpers.GenerateCodeMigrateRepair(tx)
			if err != nil {
				tx.Rollback()
				c.JSON(500, gin.H{
					"status": false,
					"message": "Gagal membuat kode migrate repair",
					"error": err.Error(),
				})
				return
			}

			migrateRepair = models.MigrateRepairDocument{
				UserID:        uint64(user.ID),
				NameUser:      user.Name,
				Status:        "process",
				Code:         code,
			}

			if err := tx.Create(&migrateRepair).Error; err != nil {
				tx.Rollback()
				c.JSON(500, gin.H{
					"success": false,
					"message": "Gagal membuat data migrate repair",
					"error": err.Error(),
				})
				return
			}
		} else {
			// error lain (DB error)
			tx.Rollback()
			c.JSON(500, gin.H{
				"success": false,
				"message": "Gagal mengambil data migrate repair",
				"error": err.Error(),
			})
			return
		}
	}

	migrate_item := models.MigrateRepairItem{
		RepairDocumentID: migrateRepair.ID,
		ProductID:       product.ID,
	}

	if err := tx.Create(&migrate_item).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal membuat data migrate repair item",
			"error": err.Error(),
		})
		return
	}

	rackID := product.RackID
	if err := tx.Model(&product).Updates(map[string]interface{}{
		"status": "migrate",
		"quality": "migrate",
		"rack_id": nil,
		"quality_text": payload.Description,
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal memperbarui status produk",
			"error": err.Error(),
		})
		return
	}

	if rackID != nil {
		if err := helpers.RecalculateRack(tx, *rackID); err != nil {
			tx.Rollback()
			c.JSON(400, gin.H{
				"success": false,
				"message": "Gagal recalculate rack",
			})
			return
		}
	}

	// ✅ Commit
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Commit gagal",
			"error": err.Error(),
		})
		return
	}

	c.JSON(201, gin.H{"success": true, "message": "Product migrated successfully"})
}

func MigrateRepairDone(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	// TRANSACTION
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"status": false, "message": "Gagal memulai transaksi"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"status": false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()


	var migrateRepair models.MigrateRepairDocument
	if err := tx.
		Where("user_id = ? AND status = ?", user.ID, "process").
		First(&migrateRepair).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			tx.Rollback()
			c.JSON(404, gin.H{
				"success": false,
				"message": "Tidak ada dokumen aktif untuk diselesaikan!",
			})

			return
		} else {
			// error lain (DB error)
			tx.Rollback()
			c.JSON(500, gin.H{
				"success": false,
				"message": "Gagal mengambil data migrate repair",
				"error": err.Error(),
			})
			return
		}
	}

	if err := tx.Model(&migrateRepair).Update("status", "added").Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal memperbarui status migrate repair",
			"error": err.Error(),
		})
		return
	}

	// ✅ Commit
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Commit gagal",
			"error": err.Error(),
		})
		return
	}

	c.JSON(201, gin.H{"success": true, "message": "Migrasi selesai!"})
}

func MigrateProductUpdate(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
    item_id := c.Param("item_id")

    type payloadRequest struct {
        NewNameProduct     string  `json:"new_name_product" binding:"required"`
        NewQuantityProduct int     `json:"new_quantity_product" binding:"required,gt=0"`
        NewPriceProduct    float64 `json:"new_price_product" binding:"required,gt=0"`
        CategoryID         *uint64  `json:"category_id"`
        TagColorID         *uint64  `json:"tag_color_id"`
        OldPriceProduct    float64 `json:"old_price_product" binding:"required,gt=0"`
    }

    var payload payloadRequest
    if err := c.ShouldBindJSON(&payload); err != nil {
        ve, ok := err.(validator.ValidationErrors)
        if !ok {
            c.JSON(400, gin.H{"status": false, "message": "Format JSON tidak valid"})
            return
        }

        errors := make(map[string]string)
        for _, e := range ve {
            field := strings.ToLower(e.Field())

            switch field {
                case "newnameproduct":
                    errors["new_name_product"] = "Nama produk baru wajib diisi"
                case "newquantityproduct":
                    if e.Tag() == "required" {
                        errors["new_quantity_product"] = "Jumlah produk wajib diisi"
                    } else {
                        errors["new_quantity_product"] = "Jumlah harus lebih besar dari 0"
                    }
                case "newpriceproduct":
                    if e.Tag() == "required" {
                        errors["new_price_product"] = "Harga baru wajib diisi"
                    } else {
                        errors["new_price_product"] = "Harga harus lebih besar dari 0"
                    }
                case "oldpriceproduct":
                    if e.Tag() == "required" {
                        errors["old_price_product"] = "Harga lama wajib diisi"
                    } else {
                        errors["old_price_product"] = "Harga lama harus lebih besar dari 0"
                    }
            }
        }

        c.JSON(http.StatusBadRequest, gin.H{
            "status": false,
            "message": "Validasi gagal",
            "errors": errors,
        })
        return
    }

	if payload.OldPriceProduct < 100000 {
		c.JSON(400, gin.H{"success": false, "message": "old price product tidak boleh kurang dari 100k"})
		return
	}

    tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}
    
	// Pastikan Rollback dipanggil jika ada panic atau error di tengah proses
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error occurred and transaction rolled back"})
            return
		}
	}()

    //load data item
    var repair_item models.MigrateRepairItem
    if err := tx.First(&repair_item, item_id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Migrate repair item tidak ditemukan"})
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query migrate repair item", "error": err.Error()})
        }

        tx.Rollback()
        return
    }

	//load data product
	var product models.Product
	if err := tx.First(&product, repair_item.ProductID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"success":false, "message": "data product tidak ditemukan pada item ini"})
		}else {
			c.JSON(404, gin.H{"success":false, "message": "failed to query product", "error": err.Error()})
		}

		return
	}

    // create update data
    updateData := map[string]interface{}{
        "name": payload.NewNameProduct,
        "quantity": payload.NewQuantityProduct,
    }

    var discount float64
	if payload.CategoryID == nil {
		tx.Rollback()
		c.JSON(400, gin.H{"status": false, "message": "Total price >= 100rb, wajib pilih kategori"})
		return
	}

	var category models.Category
	if err := tx.First(&category, payload.CategoryID).Error; err != nil {
		tx.Rollback()
		c.JSON(404, gin.H{"status": false, "message": "Category tidak ditemukan", "error": err.Error()})
		return
	}

	discount = payload.OldPriceProduct * (float64(category.DiscountCategory)/100.0)
	discount = math.Round(discount)
	if discount > category.MaxPriceCategory {
		discount = category.MaxPriceCategory
	} 

	calculatedPrice := payload.OldPriceProduct - discount
	if math.Round(calculatedPrice) != math.Round(payload.NewPriceProduct) {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Harga setelah diskon kategori tidak sesuai. Harap periksa kembali."})
		return
	}

	updateData["price"] = calculatedPrice
	updateData["category_id"] = category.ID
	updateData["tag_color_id"] = nil
	updateData["display_price"] = calculatedPrice

	metadata := map[string]interface{}{
		"before_edit": map[string]interface{}{
			"name_product": product.Name,
			"price_product": product.Price,
			"old_price_product": product.OldPriceProduct,
			"category_id": product.CategoryID,
		},
		"after_edit": map[string]interface{}{
			"name_product": payload.NewNameProduct,
			"price_product": payload.NewPriceProduct,
			"old_price_product": payload.OldPriceProduct,
			"category_id": payload.CategoryID,
		},
	}

    if err := tx.Model(&product).Updates(updateData).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal update data product", "error": err.Error()})
        return
    }

    // if err := tx.Model(&models.ProductOld{}).
    //     Where("id = ?", product.ProductOldID).
    //     Update("old_price_product", payload.OldPriceProduct).Error; err != nil {
    //     tx.Rollback()
    //     c.JSON(500, gin.H{"status": false, "message": "Gagal update data product old", "error": err.Error()})
    //     return
    // }

	if err := helpers.LogUserAction(user.ID, user.Name, fmt.Sprintf("Update data product (%s)", product.Barcode), "migrate-to-repair/product/update", metadata); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "gagal membuat log user action", "error": err.Error()})
		return
	}

    if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

    c.JSON(200, gin.H{
        "status": true,
        "message": "product berhasil diupdate",
    })

}

func MigrateProductToDisplay(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
    item_id := c.Param("item_id")

    tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}
    
	// Pastikan Rollback dipanggil jika ada panic atau error di tengah proses
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error occurred and transaction rolled back"})
            return
		}
	}()

	var repair_item models.MigrateRepairItem
	if err := tx.First(&repair_item, item_id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Migrate repair item tidak ditemukan"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query migrate repair item", "error": err.Error()})
		}

        tx.Rollback()
        return
    }

	//load data product
    var product models.Product
    if err := tx.Where("id = ?", repair_item.ProductID).
            Where("status = ?", "migrate").
            Where("quality = ?", "migrate").
            First(&product).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "product not found"})
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query product", "error": err.Error()})
        }

        tx.Rollback()
        return
    }

    // create update data
    updateData := map[string]interface{}{
        "location_type": "main",
        "status": "display",
        "quality": "lolos",
        "quality_text": nil,
    }

    if err := tx.Model(&product).Updates(updateData).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal update data product", "error": err.Error()})
        return
    }

	if err := tx.Delete(&repair_item).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to delete migrate repair item", "error": err.Error()})
		return
	}

	metadata := map[string]interface{}{}
	if err := helpers.LogUserAction(user.ID, user.Name, fmt.Sprintf("Migrasi product repair %s ke display", product.Barcode), "migrate-to-repair/to-display", metadata); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to log user action", "error": err.Error()})
		return
	}

    if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

    c.JSON(200, gin.H{
        "status": true,
        "message": "Product berhasil dipindahkan ke display",
    })

}

func MigrateProductToDump(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
    item_id := c.Param("item_id")

    tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}
    
	// Pastikan Rollback dipanggil jika ada panic atau error di tengah proses
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error occurred and transaction rolled back"})
            return
		}
	}()

	var repair_item models.MigrateRepairItem
	if err := tx.First(&repair_item, item_id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Migrate repair item tidak ditemukan"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query migrate repair item", "error": err.Error()})
		}

        tx.Rollback()
        return
    }

	//load data product
    var product models.Product
    if err := tx.Where("id = ?", repair_item.ProductID).
            Where("status = ?", "migrate").
            Where("quality = ?", "migrate").
            First(&product).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "product not found"})
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query product", "error": err.Error()})
        }

        tx.Rollback()
        return
    }

    if err := tx.Model(&product).Update("status", "dump").Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal update data product", "error": err.Error()})
        return
    }

	if err := tx.Delete(&repair_item).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to delete migrate repair item", "error": err.Error()})
		return
	}

	metadata := map[string]interface{}{}
	if err := helpers.LogUserAction(user.ID, user.Name, fmt.Sprintf("Migrasi product repair %s ke QCD", product.Barcode), "migrate-to-repair/to-qcd", metadata); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to log user action", "error": err.Error()})
		return
	}

    if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

    c.JSON(200, gin.H{
        "status": true,
        "message": "Product berhasil dipindahkan ke qcd",
    })

}
// ===================== OUTBOUND ====================
// QCD -> scrap
func GetScrapDocuments(c *gin.Context) {
	db := config.DB

	q := c.Query("q")
	status := c.Query("status")

	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	offset := (page - 1) * limit

	type scrapDocumentResponse struct {
        ID            uint64    `json:"id"`
        CodeDocument  string    `json:"code_document"`
        Status        string    `json:"status"`
        TotalProduct  int64     `json:"total_product"`
        TotalNewPrice float64     `json:"total_new_price"`
        TotalOldPrice float64     `json:"total_old_price"`
        CreatedAt     time.Time `json:"created_at"`
        UserID       uint64    `json:"user_id"`
        UserName     string    `json:"user_name"`
    }

	var results []scrapDocumentResponse
	var total int64

	baseQuery := db.Table("scrap_documents sd").
		Select(`
			sd.id AS id,
			sd.code_document AS code_document,
			sd.status AS status,
			sd.total_product AS total_product,
			sd.total_new_price AS total_new_price,
			sd.total_old_price AS total_old_price,
			sd.created_at AS created_at,
			u.id   AS user_id,
			u.name AS user_name
		`).
		Joins("LEFT JOIN users u ON u.id = sd.user_id")

	// ===== Filter q =====
	if q != "" {
		like := "%" + q + "%"

		baseQuery = baseQuery.Where(`
			sd.code_document_scrap LIKE ?
			OR u.name LIKE ?
			OR EXISTS (
				SELECT 1
				FROM scrap_items si
                JOIN products p ON p.id = si.product_id
                WHERE si.scrap_document_id = sd.id
                AND p.barcode LIKE ?
			)
		`,
			like, like, like,
		)
	}

	// ===== Filter status =====
	if status != "" {
		baseQuery = baseQuery.Where("sd.status = ?", status)
	}

	// ===== Count total =====
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// ===== Fetch data =====
	if err := baseQuery.
		Order("sd.created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&results).Error; err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// ===== Response =====
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "List Data Scrap Documents",
		"data": gin.H{
			"current_page": page,
			"per_page":     limit,
			"total":        total,
			"data":         results,
			"links" :		links,
		},
	})
}

func ExportScrapQcdDocument(c *gin.Context) {
	doc_id := c.Param("doc_id")

    var document models.ScrapDocument
    if err := config.DB.Preload("User").First(&document, doc_id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(404, gin.H{
                "success": false,
                "message": "Document tidak ditemukan",
            })
        }else {
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
        }
        return
    }

    f := excelize.NewFile()
    //mengubah nama sheet
    sheet1 := "Product List"
    f.SetSheetName("Sheet1", sheet1)

    if err := services.WriteProductScrapQcd(f, sheet1, document.ID); err != nil {
        c.JSON(500, gin.H{
            "success": false,
            "message": "Gagal memproses " + sheet1,
            "error": err.Error(),
        })
        return
    }

    sheet2 := "Document Summary"
    f.NewSheet(sheet2)

    if err := services.WriteScrapQcdSummaryDocument(f, sheet2, document); err != nil {
        c.JSON(500, gin.H{
            "success": false,
            "message": "Gagal memproses " + sheet2,
            "error": err.Error(),
        })
        return
    }

    //save file
    fileName := fmt.Sprintf("QCD_%s.xlsx", strings.ReplaceAll(document.CodeDocument, "/", "-"))
	dir := "./public/exports"
	os.MkdirAll(dir, 0755)

	fullPath := filepath.Join(dir, fileName)
	if err := f.SaveAs(fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

    downloadURL := fmt.Sprintf("%s/public/exports/%s", os.Getenv("APP_URL"), fileName)

    c.JSON(200, gin.H{
        "success": true,
        "file_name": "File berhasil diunduh",
        "download_url": downloadURL,
    })
}

func ExportSummaryScrapQcd(c *gin.Context) {
    f := excelize.NewFile()
    //mengubah nama sheet
    sheet1 := "All Products"
    f.SetSheetName("Sheet1", sheet1)

    if err := services.WriteAllProductScrapQcd(f, sheet1); err != nil {
        c.JSON(500, gin.H{
            "success": false,
            "message": "Gagal memproses " + sheet1,
            "error": err.Error(),
        })
        return
    }

    sheet2 := "Document Summary"
    f.NewSheet(sheet2)

    if err := services.WriteScrapQcdSummaryAllDocument(f, sheet2); err != nil {
        c.JSON(500, gin.H{
            "success": false,
            "message": "Gagal memproses " + sheet2,
            "error": err.Error(),
        })
        return
    }

    //save file
    fileName := fmt.Sprintf("All_Scrap_QCD_%s.xlsx",time.Now().Format("2006-01-02"))
	dir := "./public/exports"
	os.MkdirAll(dir, 0755)

	fullPath := filepath.Join(dir, fileName)
	if err := f.SaveAs(fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

    downloadURL := fmt.Sprintf("%s/public/exports/%s", os.Getenv("APP_URL"), fileName)

    c.JSON(200, gin.H{
        "success": true,
        "file_name": fileName,
        "download_url": downloadURL,
    })
}

func GetProductDumps(c *gin.Context) {
    q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 30
	offset := (page - 1) * limit

	//inisialisasi query
	baseQuery := config.DB.Model(&models.Product{}).
        Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
        Where("products.status = ?", "dump")

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(products.barcode LIKE ? OR "+
            "products.old_barcode_product LIKE ? OR " + 
            "products.name LIKE ? OR " + 
            "color_tags.name_color LIKE ? OR " + 
            "categories.name_category LIKE ?)", searchPattern, searchPattern, searchPattern, searchPattern, searchPattern)
	}

    // Paginate Data
    type productsData struct {
        ID          uint64  `json:"id"`
        OldBarcode  string  `json:"old_barcode"`
        NewBarcode     string  `json:"new_barcode"`
        Name        string  `json:"name"`
        Price       float64 `json:"price"`
        OldPrice       float64 `json:"old_price"`
        Status      string  `json:"status"`
        Source      string  `json:"source"`
        Category   *string  `json:"category"`
    }

    var products []productsData
	var totalData int64

    baseQuery.Session(&gorm.Session{}).Count(&totalData)

    // Ambil data detail
    err := baseQuery.Session(&gorm.Session{}).
        Select(`
            products.id, 
            products.old_barcode_product AS old_barcode, 
            products.barcode AS new_barcode, 
            products.name AS name, 
            products.price AS price, 
            products.old_price_product AS old_price, 
            products.status AS status, 
			CASE 
                WHEN products.quality = 'migrate' THEN 'migrate'
                WHEN products.location_type = 'main' THEN 'display'
                ELSE 'staging'
            END AS source, 
            COALESCE(color_tags.name_color, categories.name_category) AS category
        `).
        Order("products.created_at DESC").
        Limit(limit).Offset(offset).
        Find(&products).Error

    if err != nil {
        c.JSON(500, gin.H{"success": false, "message": "error", "error": err.Error()})
        return
    }

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))
	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Product Dump",
			"resource": gin.H{
                "current_page":           page,
                "data":                 products,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + int(totalData),
				"total":          totalData,
			},
		},
	})
}

func DetailScrapDocuments(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	q := c.Query("q")
	scrap_id := c.Param("scrap_id")	

	limit := 30
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	offset := (page - 1) * limit

	var doc models.ScrapDocument
	db := config.DB

	// 1. Cari scrap doc
	err := db.First(&doc, scrap_id).Error

	//Jika TIDAK ADA scrap document
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(404, gin.H{
			"success": false,
			"message": "Scrap document tidak ditemukan",
		})
		
		return
	}

	//Jika ADA sesi aktif
	// Ambil item lewat scrap_items → products
	type ItemResponse struct {
		ID              uint64    `json:"id"`
		NameProduct  string    `json:"name_product"`
		Barcode      string    `json:"barcode"`
		NewPrice        float64   `json:"new_price"`
		OldPrice        float64   `json:"old_price"`
		Status          string    `json:"status"`
		Source          string    `json:"source"`
		Category        string    `json:"category"`
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
	}

	var items []ItemResponse
	var total int64

	baseQuery := db.Table("scrap_items").
		Select(`
			products.id, 
            products.name AS name_product, 
            products.barcode AS barcode, 
            products.price AS new_price, 
            products.old_price_product AS old_price, 
            products.status AS status, 
			CASE 
                WHEN products.quality = 'migrate' THEN 'migrate'
                WHEN products.location_type = 'main' THEN 'display'
                ELSE 'staging'
            END AS source, 
            COALESCE(color_tags.name_color, categories.name_category) AS category,
			products.created_at,
			products.updated_at
		`).
		Joins("JOIN products ON products.id = scrap_items.product_id").
		Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
		Where("scrap_items.scrap_document_id = ?", doc.ID)
	
	if q != "" {
		keyword := "%" + q + "%"
		baseQuery = baseQuery.Where("(products.name LIKE ? OR products.barcode LIKE ?)", keyword, keyword)
	}

	baseQuery.Session(&gorm.Session{}).Count(&total)

	baseQuery.
		Order("products.updated_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&items)

	lastPage := int(math.Ceil(float64(total) / float64(limit)))
	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Sesi Scrap Aktif Ditemukan",
		"data": gin.H{
			"document": doc,
			"items": gin.H{
				"current_page":   page,
                "data":           items,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + len(items),
				"total":          total,
			},
		},
	})
}

func GetActiveSession(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	user := c.MustGet("auth_user").(models.User)

	limit := 15
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	offset := (page - 1) * limit

	var doc models.ScrapDocument
	db := config.DB

	// 1. Cari sesi aktif
	err := db.
		Where("user_id = ? AND status = ?", user.ID, "proses").
		First(&doc).Error

	//Jika TIDAK ADA sesi aktif → buat baru
	if errors.Is(err, gorm.ErrRecordNotFound) {
		now := time.Now()
		monthYear := fmt.Sprintf("%02d/%d", now.Month(), now.Year())

		var lastDoc models.ScrapDocument
		nextNumber := 1

		db.
			Where("code_document LIKE ?", "%/"+monthYear).
			Order("id DESC").
			First(&lastDoc)

		if lastDoc.ID != 0 {
			parts := strings.Split(lastDoc.CodeDocument, "/")
			if len(parts) > 0 {
				if n, err := strconv.Atoi(parts[0]); err == nil {
					nextNumber = n + 1
				}
			}

		}

		code := fmt.Sprintf("%04d/%s", nextNumber, monthYear)

		doc = models.ScrapDocument{
			CodeDocument: code,
			UserID:       uint64(user.ID),
			Status:       "proses",
		}

		if err := db.Create(&doc).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Gagal membuat sesi scrap",
			})
			return
		}

		links := helpers.BuildPaginationLinks(c, page, 1)

		c.JSON(http.StatusCreated, gin.H{
			"success": true,
			"message": "Sesi Scrap Baru Berhasil Dibuat",
			"data": gin.H{
				"document": doc,
				"items":    gin.H{
					"current_page": page,
					"data": []any{},
					"links": links,
				},
			},
		})
		return
	}

	//Jika ADA sesi aktif
	// Ambil item lewat scrap_items → products
	type ItemResponse struct {
		ID              uint64    `json:"id"`
		NameProduct  string    `json:"name_product"`
		Barcode      string    `json:"barcode"`
		NewPrice        float64   `json:"new_price"`
		OldPrice        float64   `json:"old_price"`
		Status          string    `json:"status"`
		Source          string    `json:"source"`
		Category        string    `json:"category"`
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
	}

	var items []ItemResponse
	var total int64

	baseQuery := db.Table("scrap_items").
		Select(`
			products.id, 
            products.name AS name_product, 
            products.barcode AS barcode, 
            products.price AS new_price, 
            products.old_price_product AS old_price, 
            products.status AS status, 
			CASE 
                WHEN products.quality = 'migrate' THEN 'migrate'
                WHEN products.location_type = 'main' THEN 'display'
                ELSE 'staging'
            END AS source, 
            COALESCE(color_tags.name_color, categories.name_category) AS category,
			products.created_at,
			products.updated_at
		`).
		Joins("JOIN products ON products.id = scrap_items.product_id").
		Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
		Where("scrap_items.scrap_document_id = ?", doc.ID)

	baseQuery.Session(&gorm.Session{}).Count(&total)

	baseQuery.
		Order("products.updated_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&items)

	lastPage := int(math.Ceil(float64(total) / float64(limit)))
	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Sesi Scrap Aktif Ditemukan",
		"data": gin.H{
			"document": doc,
			"items": gin.H{
				"current_page":   page,
                "data":           items,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + len(items),
				"total":          total,
			},
		},
	})
}

func AddProductToScrap(c *gin.Context) {
	scrap_id := c.Param("scrap_id")
	barcode := c.Param("barcode")

	// TRANSACTION
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"status": false, "message": "Gagal memulai transaksi"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"status": false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	// Ambil Scrap Document
	var doc models.ScrapDocument
	if err := tx.First(&doc, scrap_id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Dokumen scrap tidak ditemukan",
		})
		return
	}

	if doc.Status != "proses" {
		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Dokumen terkunci / sudah selesai",
		})
		return
	}

	// Ambil Produk
	var product models.Product
	if err := tx.Where("barcode = ?", barcode).
		First(&product).Error; err != nil {

		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Produk tidak ditemukan",
			"error": err.Error(),
		})

		return
	}

	if product.Status != "dump" {
		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Status produk harus dump",
		})
		return
	}

	// Cek apakah produk sedang discrap
	var count int64
	tx.Model(&models.ScrapItem{}).Where("product_id = ?", product.ID).
		Count(&count)

	if count > 0 {
		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Produk sudah masuk dalam scrap / sedang di scrap lain",
		})
		return
	}

	// Insert ke scrap_items
	item := models.ScrapItem{
		ScrapDocumentID: doc.ID,
		ProductID:       product.ID,
	}

	if err := tx.Create(&item).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Recalculate Total
	result := tx.Model(&doc).Updates(map[string]interface{}{
		"total_product":                    gorm.Expr("total_product + 1"),
		"total_new_price":      gorm.Expr("total_new_price + ?", product.Price),
		"total_old_price":      gorm.Expr("total_old_price + ?", product.OldPriceProduct),
	})

	if result.RowsAffected == 0 {
		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": "Scrap Document tidak ditemukan atau tidak berubah",
		})
		return
	}

	//update status product
	if err := tx.Model(&product).Update("status", "scrap_qcd").Error; err != nil {
		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": "Product gagal di update ke scrap qcd",
			"error": err.Error(),
		})

		return
	}

	// ✅ Commit
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Commit gagal",
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Produk masuk list scrap",
	})
}

func AddAllProductToScrap(c *gin.Context) {
	scrap_id := c.Param("scrap_id")

	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"status": false, "message": "Gagal memulai transaksi"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"status": false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	// Ambil scrap document aktif
	var doc models.ScrapDocument
	if err := tx.First(&doc, scrap_id).Error; err != nil {
		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": "Scrap document tidak ditemukan",
		})
		return
	}

	if doc.Status != "proses" {
		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": "Document terkunci / selesai",
		})
		return
	}

	var count int64
	err := tx.
		Model(&models.Product{}).
		Where("products.status = ?", "dump").
		Where("NOT EXISTS (SELECT 1 FROM scrap_items si WHERE si.product_id = products.id)").
		Count(&count).Error


	if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal menghitung total product"})
		return
	}

	if count <= 0 {
		tx.Rollback()
		c.JSON(200, gin.H{"success": true, "message": "Tidak ada product dump"})
		return
	}

	// insert scrap item
	err = tx.Exec(`
		INSERT INTO scrap_items (scrap_document_id, product_id, created_at, updated_at)
		SELECT ?, p.id, NOW(), NOW()
		FROM products p
		WHERE p.status = 'dump'
		AND NOT EXISTS (
			SELECT 1 FROM scrap_items si WHERE si.product_id = p.id
		)
	`, doc.ID).Error

	if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal insert scrap items",
			"error":   err.Error(),
		})
		return
	}

	// UPDATE STATUS PRODUCT → scrap_qcd
	err = tx.Exec(`
		UPDATE products p
		JOIN scrap_items si ON si.product_id = p.id
		SET p.status = 'scrap_qcd',
			p.updated_at = NOW()
		WHERE si.scrap_document_id = ?
	`, doc.ID).Error


	if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal update status product", "error": err.Error()})
		return
	}

	// menghitung total scrap document
	type Totals struct {
		TotalProduct  int64
		TotalNewPrice float64
		TotalOldPrice float64
	}

	var totals Totals
	err = tx.Raw(`
		SELECT
			COUNT(*) as total_product,
			SUM(p.price) as total_new_price,
			SUM(p.old_price_product) as total_old_price
		FROM scrap_items si
		JOIN products p ON p.id = si.product_id
		WHERE si.scrap_document_id = ?
	`, doc.ID).Scan(&totals).Error

	if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal menghitung total scrap document", "error": err.Error()})
		return
	}

	err = tx.
		Model(&models.ScrapDocument{}).
		Where("id = ?", doc.ID).
		Updates(map[string]interface{}{
			"total_product":   totals.TotalProduct,
			"total_new_price": totals.TotalNewPrice,
			"total_old_price": totals.TotalOldPrice,
		}).Error

	if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal update scrap document", "error": err.Error()})
		return
	}

	// Commit
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal commit transaksi",
			"error": err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "Semua product dump berhasil ditambahkan ke scrap",
		"data": gin.H{
			"total_product":   totals.TotalProduct,
			"total_new_price": totals.TotalNewPrice,
			"total_old_price": totals.TotalOldPrice,
		},
	})
}

func LockScrapDocument(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	scrap_id := c.Param("scrap_id")

	var doc models.ScrapDocument
	db := config.DB

	// 1. Cari scrap doc
	err := db.First(&doc, scrap_id).Error

	//Jika TIDAK ADA scrap document
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(404, gin.H{
			"success": false,
			"message": "Scrap document tidak ditemukan",
		})
		
		return
	}

	if doc.Status != "proses" {
		c.JSON(422, gin.H{
			"success": false,
			"message": "Scrap document sudah terkunci / selesai",
		})
		
		return
	}

	if doc.TotalProduct == 0 {
		c.JSON(422, gin.H{
			"success": false,
			"message": "List kosong! Masukan produk sebelum menyelesaikan input",
		})
		
		return
	}

	if err := db.Model(&doc).Update("status", "lock").Error; err!=nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Scrap document gagal diupdate",
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Scrap document berhasil terkunci",
	})
}

func FinishScrapDocument(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	scrap_id := c.Param("scrap_id")

	var doc models.ScrapDocument
	db := config.DB

	// 1. Cari scrap doc
	err := db.First(&doc, scrap_id).Error

	//Jika TIDAK ADA scrap document
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(404, gin.H{
			"success": false,
			"message": "Scrap document tidak ditemukan",
		})
		
		return
	}

	if doc.Status == "selesai" {
		c.JSON(422, gin.H{
			"success": false,
			"message": "Scrap document sudah terkunci / selesai",
		})
		
		return
	}

	if doc.TotalProduct == 0 {
		c.JSON(422, gin.H{
			"success": false,
			"message": "List kosong! Masukan produk sebelum menyelesaikan input",
		})
		
		return
	}

	if err := db.Model(&doc).Update("status", "selesai").Error; err!=nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Scrap document gagal diupdate",
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Scrap document berhasil selesai",
	})
}




