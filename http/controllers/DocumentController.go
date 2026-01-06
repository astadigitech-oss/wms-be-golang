package controllers

import (
	"errors"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"net/http"

	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	// "time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
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

	db := config.DB.Model(&models.Document{})

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

	db := config.DB.Model(&models.ProductOld{})
	if query != "" {
		db = db.
			Where("code_document = ?", code_document).
			Where(
				"(old_barcode_product LIKE ? OR old_name_product LIKE ?)","%"+query+"%", "%"+query+"%",)
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
	err := config.DB.Where("code_document = ?", code_document).Where("old_barcode_product", barcode).First(&productOld).Error

	if err != nil {
		c.JSON(404, gin.H{
			"success": false,
			"message": err.Error(),
		})
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
	tx := config.DB.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// ✅ Update langsung (tanpa SELECT dulu → cepat)
	result := tx.
		Model(&models.Document{}).
		Where("code = ?", request.CodeDocument).
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

    tx := config.DB.Begin()
    if tx.Error != nil {
        c.JSON(500, gin.H{"status": false, "message": "Gagal memulai transaksi"})
        return
    }

    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(500, gin.H{"status": false, "message": "Terjadi kesalahan sistem", "error" : r})
        }
    }()

    // HAPUS product_olds YANG TIDAK DIPAKAI
    deleteUnusedProductOld := `
        DELETE FROM product_olds
        WHERE code_document = ?
        AND id NOT IN (
            SELECT DISTINCT product_old_id 
            FROM products 
            WHERE product_old_id IS NOT NULL
        )
    `

	if err := tx.Exec(deleteUnusedProductOld, code_document).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal hapus product_old tidak terpakai", "error": err})
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
        AND id NOT IN (
            SELECT DISTINCT product_old_id 
            FROM products 
            WHERE product_old_id IS NOT NULL
        )
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

	// AMBIL USER ID DARI CONTEXT
	userIDAny, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "Unauthorized",
		})
		return
	}

	userID := userIDAny.(uint)

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
	generatedCode := fmt.Sprintf("%d-BKL-%06d", userID, nextSequence)

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

func CreateBKL(c *gin.Context) {
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

	var colorTagIDs []uint64
	for _, c := range payload.Colors {
		colorTagIDs = append(colorTagIDs, c.ColorTagID)
	}

	var existingIDs []uint64
	err := config.DB.
		Model(&models.ColorTag{}).
		Where("id IN ?", colorTagIDs).
		Pluck("id", &existingIDs).Error

	if err != nil {
		c.JSON(500, gin.H{"status": false, "message": "Gagal validasi color tag"})
		return
	}

	if len(existingIDs) != len(colorTagIDs) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": false,
			"message": "Salah satu color tag tidak ditemukan",
		})
		return
	}

	// AMBIL USER ID
	userIDAny, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": false,
			"message": "Unauthorized",
		})
		return
	}

	userID := userIDAny.(uint)

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

	// INSERT BKL DOCUMENT
	document := models.BklDocument{
		CodeBkl: payload.NameDocument,
		Status:  "done",
		UserID:  uint64(userID),
	}

	if err := tx.Create(&document).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"status": false,
			"message": "Gagal menyimpan dokumen",
			"error": err.Error(),
		})
		return
	}

	// SIMPAN ITEMS
	if payload.DamageQty != nil {
		item := models.BklItem{
			BklDocumentID: document.ID,
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
			BklDocumentID: document.ID,
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

	// Commit
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Commit failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"message": "BKL Berhasil Dibuat",
	})
}

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

