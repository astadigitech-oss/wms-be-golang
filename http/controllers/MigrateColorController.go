package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"liquid8/wms/services"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

func GetMigrateDocuments(c *gin.Context) {
	q := c.Query("q")
	
	limit := 15
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	offset := (page - 1) * limit

	var migrate_documents []models.MigrateColorDocument
	baseQuery := config.DB.Model(&models.MigrateColorDocument{}).Where("status_document = ?", "selesai")

	if q != "" {
		query := "%" + q + "%"
		baseQuery = baseQuery.Where("(code_document LIKE ? OR created_at LIKE ?)", query, query)
	}

	var totalData int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&totalData).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal menghitung total data", "error": err.Error()})
		return
	}

	if err := baseQuery.Order("created_at DESC").
		Limit(limit).Offset(offset).Find(&migrate_documents).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal mengambil data migrate documents", "error": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"success": false,
		"message": "List Migrate Document",
		"resources": gin.H{
			"data": migrate_documents,
			"current_page": page,
			"links": links,
			"per_page": limit,
			"total_data": totalData,
			"from": offset + 1,
			"to": offset + len(migrate_documents),
		},
	})
}

func DetailMigrateDocument(c *gin.Context) {
	doc_id := c.Param("doc_id")

	var migrate_documents models.MigrateColorDocument
	if err := config.DB.Preload("Migrates").
		First(&migrate_documents, doc_id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{
				"success": false,
				"message": "Migrate document tidak ditemukan",
			})
		}else {
			c.JSON(500, gin.H{
				"success": false,
				"message": "Gagal mencari data Migrate document",
				"error":   err.Error(),
			})
		}

		return
	}

	c.JSON(200, gin.H{
		"success": false,
		"message": "Data document migrate",
		"resource": migrate_documents,
	})
}

//get migrate status = proses
func GetActiveMigrateDocument(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	var migrate_documents models.MigrateColorDocument
	err := config.DB.Preload("Migrates").
		Where("user_id = ?", user.ID).
		Where("status_document = ?", "proses").
		First(&migrate_documents).Error

	// ========== JIKA TIDAK ADA DATA ==========
	if errors.Is(err, gorm.ErrRecordNotFound) {

		c.JSON(200, gin.H{
			"success": true,
			"message": "data migrate document!",
			"resources": gin.H{
				"destionation": "aktif",
				"data":         []interface{}{},
			},
		})
		return
	}

	// ========== JIKA ERROR LAIN ==========
	if err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "gagal mengambil data migrate",
			"error":   err.Error(),
		})
		return
	}

	// ========== JIKA ADA DATA ==========
	c.JSON(200, gin.H{
		"success": true,
		"message": "data berhasil disimpan!",
		"resources": gin.H{
			"destionation": "disable",
			"data":         migrate_documents,
		},
	})
}

func GetColorDestination(c *gin.Context) {
	type colorCount struct {
		Color string `json:"color"`
		Total int64  `json:"total"`
		FixedPrice float64  `json:"fixed_price"`
	}

	var colors []colorCount
	// HITUNG BERDASARKAN JOIN COLOR TAG
	err := config.DB.Table("products as p").
		Select("ct.name_color as color, COUNT(*) as total, ct.fixed_price_color as fixed_price").
		Joins("JOIN color_tags ct ON ct.id = p.tag_color_id").
		Where("p.tag_color_id IS NOT NULL").
		Where("p.category_id IS NULL").
		Where("p.is_so IS NULL").
		Where("p.quality = ?", "lolos").
		Where("(p.warehouse_type IS NULL OR p.warehouse_type = ?)", "type1").
		Where("p.status IN ?", []string{"display","expired","slow_moving"}).
		Group("ct.name_color, ct.fixed_price_color").
		Scan(&colors).Error

	if err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "gagal mengambil data color",
			"error":   err.Error(),
		})
		return
	}

	if len(colors) < 1 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "tidak ada data data color",
			"data":    nil,
		})
		return
	}
		
	var migrates []struct {
		ProductColor string
		BookedTotal int64
	}
	if err := config.DB.
		Model(&models.MigrateColorItem{}).
		Select("product_color, SUM(product_total) as booked_total").
		Where("status = ?", "proses").
		Group("product_color").
		Scan(&migrates).Error; err != nil {

		helpers.ErrorResponse(c, 500, "Gagal mengambil data migrate", err)
		return
	}

	bookedColors := make(map[string]int64)
	for _, r := range migrates {
		bookedColors[r.ProductColor] = r.BookedTotal
	}

	var filteredColors []colorCount
	for _, color := range colors {
		booked_total := bookedColors[color.Color]
	
		remaining := color.Total - booked_total
		if remaining <= 0 {continue}

		color.Total = remaining
		filteredColors = append(filteredColors, color)
	}

	// Ambil destinations
	var destinations []models.MigrateColorDestination
	if err := config.DB.
		Order("created_at DESC").
		Find(&destinations).Error; err != nil {

		c.JSON(500, gin.H{
			"success": false,
			"message": "gagal mengambil destination",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "list data product by color",
		"data": gin.H{
			"color":        filteredColors,
			"destinations": destinations,
		},
	})
}

func StoreMigrateColor(c *gin.Context) {

	type payloadRequest struct {
		ProductColor            string `json:"product_color" binding:"required"`
		ProductTotal            int     `json:"product_total" binding:"required,numeric"`
		Destination  string  `json:"destination" binding:"required"`
	}

	user := c.MustGet("auth_user").(models.User) 

	var req payloadRequest

	// Validator
	if err := c.ShouldBindJSON(&req); err != nil {

		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "Format JSON tidak valid",
			})
			return
		}

		errors := make(map[string]string)
		for _, e := range ve {
			field := strings.ToLower(e.Field())

			switch field {

				case "productcolor":
					if e.Tag() == "required" {
						errors["product_color"] = "Warna produk wajib diisi"
					}

				case "producttotal":
					if e.Tag() == "required" {
						errors["product_total"] = "Total Produk wajib diisi"
					} else {
						errors["product_total"] = "Total Produk harus berupa angka / numeric"
					}

				case "destination":
					if e.Tag() == "required" {
						errors["destination"] = "Destination wajib diisi"
					}
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status":  false,
			"message": "Validasi gagal",
			"errors":  errors,
		})
		return
	}

	// ==========================
	// Ambil product by color
	// ==========================
	var products []models.Product

	err := config.DB.
		Table("products as p").
		Joins("JOIN color_tags ct ON ct.id = p.tag_color_id").
		Where("ct.name_color = ?", req.ProductColor).
		Where("p.status IN ?", []string{"display", "expired", "slow_moving"}).
		Order("p.created_at ASC").
		Find(&products).Error

	if err != nil || len(products) == 0 {
		c.JSON(422, gin.H{
			"success": false,
			"message": "Data tidak di temukan!",
		})
		return
	}

	// Cek total
	if req.ProductTotal > len(products) {
		c.JSON(422, gin.H{
			"success": false,
			"message": "anda menginputkan product total lebih dari total product color",
			"data":    gin.H{
				"product_total": req.ProductTotal,
				"total_product_color" : len(products), 
			},
		})
		return
	}

	// ==========================
	// TRANSACTION START
	// ==========================
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

	// ==========================
	// Cari document proses
	// ==========================
	var migrateDocument models.MigrateColorDocument

	err = tx.
		Where("user_id = ?", user.ID).
		Where("status_document = ?", "proses").
		First(&migrateDocument).Error

	// Jika belum ada → buat baru
	if err != nil {
		newCode, err := helpers.GenerateCodeMigrateColorDocument(tx)

		if err != nil {
			tx.Rollback()

			c.JSON(422, gin.H{
				"success": false,
				"message": "Gagal generate code documment migrate",
				"error":   err.Error(),
			})
			return
		}

		migrateDocument = models.MigrateColorDocument{
			CodeDocument:       newCode,
			DestinyDocument:    req.Destination,
			TotalProductDocument: 0,
			StatusDocument:     "proses",
			UserID:                    uint64(user.ID),
		}

		if err := tx.Create(&migrateDocument).Error; err != nil {
			tx.Rollback()

			c.JSON(422, gin.H{
				"success": false,
				"message": "Gagal membuat document migrate",
				"error":   err.Error(),
			})
			return
		}
	}

	// ==========================
	// CREATE MIGRATE
	// ==========================
	migrate := models.MigrateColorItem{
		CodeDocumentMigrate: migrateDocument.CodeDocument,
		ProductColor:        req.ProductColor, // sesuaikan tipe di struct
		ProductTotal:        req.ProductTotal,
		Status:       		 "proses",
		UserID:              uint64(user.ID),
	}

	if err := tx.Create(&migrate).Error; err != nil {
		tx.Rollback()

		c.JSON(422, gin.H{
			"success": false,
			"message": "Data gagal di simpan!",
			"error":   err.Error(),
		})
		return
	}

	// ====================================================
	// UPDATE PRODUCT SEJUMLAH product_total
	// ====================================================
	// Ambil ID sejumlah product_total saja
	// var selectedIDs []uint64

	// for i := 0; i < req.ProductTotal; i++ {
	// 	selectedIDs = append(selectedIDs, products[i].ID)
	// }

	// // Update status hanya yang terpilih
	// if err := tx.Model(&models.Product{}).
	// 	Where("id IN ?", selectedIDs).
	// 	Update("status", "migrate").Error; err != nil {

	// 	tx.Rollback()
	// 	c.JSON(422, gin.H{
	// 		"success": false,
	// 		"message": "Gagal update status product",
	// 		"error":   err.Error(),
	// 	})
	// 	return
	// }

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	// ==========================
	// RESPONSE
	// ==========================
	c.JSON(200, gin.H{
		"success": true,
		"message": "data berhasil disimpan!",
		"data":    migrate,
	})
}

func DestroyMigrateColor(c *gin.Context) {
	migrate_id := c.Param("migrate_id")

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

	var migrate models.MigrateColorItem
	if err := tx.First(&migrate, migrate_id).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{
				"success": false,
				"message": "Data tidak ditemukan",
			})
		}else {
			c.JSON(500, gin.H{
				"success": false,
				"message": "Gagal mengambil data migrate",
				"error": err.Error(),
			})
		}

		return
	}

	var totalData int64
	if err := tx.Model(&models.MigrateColorItem{}).Where("code_document_migrate = ?", migrate.CodeDocumentMigrate).
		Count(&totalData).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal menghitung data migrate",
			"error": err.Error(),
		})

		return
	}

	if err := tx.Delete(&migrate).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal menghapus data migrate",
			"error": err.Error(),
		})

		return
	}

	if totalData <= 1 {
		if err := tx.Where("code_document = ?", migrate.CodeDocumentMigrate).
			Delete(&models.MigrateColorDocument{}).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"success": false,
				"message": "Gagal menghapus data migrate document",
				"error": err.Error(),
			})

			return
		}
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(200, gin.H{
		"success": true,
		"message": "Data migrate berhasil dihapus",
	})
}

func MigrateDocumentFinish(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

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
				"message": "Internal server error (panic)",
				"error": fmt.Sprintf("%v", r),
			})
        }
    }()

	var migrateDocs []models.MigrateColorDocument
	err := tx.Preload("Migrates").
		Where("user_id = ? AND status_document = ?", user.ID, "proses").
		Find(&migrateDocs).Error

	if err != nil {
		tx.Rollback()
		helpers.ErrorResponse(c, 500, "Gagal mengambil data migrate document", err)
		return
	}

	if len(migrateDocs) == 0 {
		tx.Rollback()
		helpers.ErrorResponse(c, 404, "Tidak ada dokumen yang perlu diproses", nil)
		return
	}

	successCount := 0
	log := helpers.NewLogger("./logs/app.log")
	ctx := c.Request.Context()
	for _, doc := range migrateDocs {

		// ===== GET DESTINATION =====
		var destination models.MigrateColorDestination
		if err := tx.Where("shop_name = ?", doc.DestinyDocument).
			First(&destination).Error; err != nil {
			tx.Rollback()
			helpers.ErrorResponse(c, 404, "Destination tidak ditemukan", nil)
			return
		}

		// ===== UPDATE PRODUCT STATUS =====
		total := 0
		for _, m := range doc.Migrates {
			total += m.ProductTotal
			var ids []uint
			err := tx.Table("products p").
				Select("p.id").
				Joins("JOIN color_tags ct ON ct.id = p.tag_color_id").
				Where("ct.name_color = ? AND p.status IN ?",
					m.ProductColor,
					[]string{"display", "expired", "slow_moving"},
				).
				Limit(m.ProductTotal).
				Pluck("p.id", &ids).Error

			if err != nil {
				tx.Rollback()
				helpers.ErrorResponse(c, 500, "Gagal mengambil id product migrate", err)
				return
			}

			if len(ids) == 0 {
				tx.Rollback()
				helpers.ErrorResponse(c, 400, "Tidak ada product tag color yang bisa di kirim", nil)
				return
			}

			err = tx.Table("products").
				Where("id IN ?", ids).
				Update("status", "migrate").Error
			
			if err != nil {
				tx.Rollback()
				helpers.ErrorResponse(c, 500, "Gagal mengupdate product menjadi migrate", err)
				return
			}
		}

		var olseraPk uint
		var olseraResponseLog string

		// ===== IF OLSERA =====
		if destination.IsOlseraIntegreted {
			olseraService := services.NewOlseraService(&destination, log)

			// CREATE HEADER / DOCUMENT
			resCreate, err := olseraService.CreateStockInOut(ctx, map[string]interface{}{
				"date": time.Now().Format("2006-01-02"),
				"type": "I",
				"note": "Migrasi WMS: " + doc.CodeDocument,
			})

			if err != nil {
				tx.Rollback()
				helpers.ErrorResponse(c, 500, "Gagal create header stock in out olsera", err)
				return
			}
			
			jsonBytes, err := json.Marshal(resCreate.Data)
			if err != nil {
				tx.Rollback()
				helpers.ErrorResponse(c, 500, "Converting json failed", err)
				return
			}
			olseraResponseLog = string(jsonBytes)


			dataRes, _ := resCreate.Data.(map[string]interface{})
			dataRes2, _ := dataRes["data"].(map[string]interface{})

			idFloat, ok := dataRes2["id"].(float64)
			if !ok {
				tx.Rollback()
				helpers.ErrorResponse(c, 500, "Invalid type id", nil)
				return
			}

			olseraPk = uint(idFloat)
			if olseraPk == 0 {
				tx.Rollback()
				helpers.ErrorResponse(c, 500, "Gagal mendapatkan ID Transaksi (PK) dari Olsera", nil)
				return
			}

			// ===== GROUP BY COLOR =====
			grouped := map[string]int{}
			for _, m := range doc.Migrates {
				grouped[strings.ToLower(m.ProductColor)] += m.ProductTotal
			}

			// ===== GET MAPPING By Destination =====
			mappingMap, err := config.GetTagMapping(destination.ShopName)
			if err != nil {
				tx.Rollback()
				helpers.ErrorResponse(c, 500, "Gagal maping tag color", err)
				return
			}

			cart := make(map[string]int)
			for tag, qty := range grouped {
				olseraID, ok := mappingMap[tag]
				if !ok {
					helpers.ErrorResponse(c, 500, fmt.Sprintf("Mapping tidak ditemukan untuk tag %s",tag), nil)
					return
				}

				cart[olseraID] += qty
			}

			// ===== ADD ITEMS =====
			for olseraID, qty := range cart {
				_, err := olseraService.AddItemStockInOut(ctx, map[string]interface{}{
					"pk":          olseraPk,
					"product_ids": olseraID,
					"qty":         qty,
					"type":        "I",
				})

				if err != nil {
					tx.Rollback()
					helpers.ErrorResponse(c, 500, fmt.Sprintf("Gagal menambah group item (ID Olsera: %s)", olseraID), err)
					return
				}
			}

			// ===== PUBLISH =====
			_, err = olseraService.UpdateStatusStockInOut(ctx, map[string]interface{}{
				"pk":     olseraPk,
				"status": "P",
			})

			if err != nil {
				tx.Rollback()
				helpers.ErrorResponse(c, 500, "Gagal mem-posting (Publish) dokumen Stock In", err)
				return
			}

			// ===== UPDATE MIGRATE =====
			if err := tx.Model(&models.MigrateColorItem{}).
				Where("code_document_migrate = ?", doc.CodeDocument).
				Update("status", "selesai").Error; err != nil {
				tx.Rollback()
				helpers.ErrorResponse(c, 500, "Gagal update status migrate item", err)
				return
			}
	
			// ===== UPDATE DOCUMENT =====
			if err := tx.Model(&doc).Updates(map[string]interface{}{
				"total_product_document": total,
				"status_document":        "selesai",
				"olsera_purchase_id":     olseraPk,
				"olsera_response_log":     olseraResponseLog,
			}).Error; err != nil {
				tx.Rollback()
				helpers.ErrorResponse(c, 500, "Gagal update migrate document", err)
				return
			}
		}


		successCount++
	}

	if err := tx.Commit().Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed commit", err)
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": fmt.Sprintf("Berhasil memproses %d dokumen migrasi", successCount),
	})
}



//Destination
func GetMigrateDestinations(c *gin.Context) {
	q := c.Query("q")
	
	limit := 50
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	offset := (page - 1) * limit

	var destinations []models.MigrateColorDestination
	baseQuery := config.DB.Model(&models.MigrateColorDestination{})

	if q != "" {
		query := "%" + q + "%"
		baseQuery = baseQuery.Where("shop_name LIKE ?", query)
	}

	var totalData int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&totalData).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal menghitung total data", "error": err.Error()})
		return
	}

	if err := baseQuery.Order("created_at DESC").
		Limit(limit).Offset(offset).Find(&destinations).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal mengambil data destinations", "error": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"success": false,
		"message": "List Destinations",
		"resources": gin.H{
			"data": destinations,
			"current_page": page,
			"links": links,
			"per_page": limit,
			"total_data": totalData,
			"from": offset + 1,
			"to": offset + len(destinations),
		},
	})
}

func StoreMigrateDestination(c *gin.Context) {
	type payloadRequest struct {
		ShopName    string `json:"shop_name" binding:"required,max=255"`
		PhoneNumber string `json:"phone_number" binding:"required,max=15"`
		Address      string `json:"address" binding:"required"`
	}

	var req payloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {

		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "Format JSON tidak valid",
			})
			return
		}

		errors := make(map[string]string)
		for _, e := range ve {
			field := strings.ToLower(e.Field())

			switch field {

				case "shopname":
					if e.Tag() == "required" {
						errors["shop_name"] = "Nama toko wajib diisi"
					}else {
						errors["shop_name"] = "Tidak boleh lebih dari 255 karakter"

					}

				case "phonenumber":
					if e.Tag() == "required" {
						errors["phone_number"] = "Nomor hp wajib diisi"
					} else {
						errors["phone_number"] = "Nomor hp tidak boleh lebih dari 15 digit"
					}

				case "address":
					if e.Tag() == "required" {
						errors["address"] = "Alamat wajib diisi"
					}
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status":  false,
			"message": "Validasi gagal",
			"errors":  errors,
		})
		return
	}

	// Cek UNIQUE shop_name
	var exists int64
	config.DB.Model(&models.MigrateColorDestination{}).
		Where("shop_name = ?", req.ShopName).
		Count(&exists)

	if exists > 0 {
		c.JSON(400, gin.H{
			"success": false,
			"message": "Nama toko ini sudah digunakan",
		})
		return
	}

	destination := models.MigrateColorDestination{
		ShopName: req.ShopName,
		PhoneNumber: req.PhoneNumber,
		Address: req.Address,
	}

	if err := config.DB.Create(&destination).Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal menyimpan destination",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "Destination berhasil dibuat",
		"data":    destination,
	})

}

func UpdateMigrateDestination(c *gin.Context) {
	id := c.Param("destination_id")

	type payloadRequest struct {
		ShopName    string `json:"shop_name" binding:"required,max=255"`
		PhoneNumber string `json:"phone_number" binding:"required,max=15"`
		Address      string `json:"address" binding:"required"`
	}

	var req payloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {

		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "Format JSON tidak valid",
			})
			return
		}

		errors := make(map[string]string)
		for _, e := range ve {
			field := strings.ToLower(e.Field())

			switch field {

				case "shopname":
					if e.Tag() == "required" {
						errors["shop_name"] = "Nama toko wajib diisi"
					}else {
						errors["shop_name"] = "Tidak boleh lebih dari 255 karakter"

					}

				case "phonenumber":
					if e.Tag() == "required" {
						errors["phone_number"] = "Nomor hp wajib diisi"
					} else {
						errors["phone_number"] = "Nomor hp tidak boleh lebih dari 15 digit"
					}

				case "address":
					if e.Tag() == "required" {
						errors["address"] = "Alamat wajib diisi"
					}
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status":  false,
			"message": "Validasi gagal",
			"errors":  errors,
		})
		return
	}

	var destination models.MigrateColorDestination
	if err := config.DB.First(&destination, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{
				"success": false,
				"message": "Destination tidak ditemukan",
			})
		}else {
			c.JSON(500, gin.H{
				"success": false,
				"message": "Gagal mencari data destination",
				"error":   err.Error(),
			})
		}

		return
	}

	// Cek UNIQUE shop_name
	var exists int64
	config.DB.Model(&models.MigrateColorDestination{}).
		Where("shop_name = ?", req.ShopName).
		Where("id != ?", destination.ID).
		Count(&exists)

	if exists > 0 {
		c.JSON(400, gin.H{
			"success": false,
			"message": "Nama toko ini sudah digunakan",
		})
		return
	}

	data := map[string]interface{}{
		"shop_name": req.ShopName,
		"phone_number": req.PhoneNumber,
		"address": req.Address,
	}

	if err := config.DB.Model(&destination).Updates(data).Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal mengubah data destination",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "Destination berhasil diupdate",
		"data":    destination,
	})

}

func DestroyMigrateDestination(c *gin.Context) {
	id := c.Param("destination_id")

	var destination models.MigrateColorDestination
	if err := config.DB.First(&destination, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{
				"success": false,
				"message": "Destination tidak ditemukan",
			})
		}else {
			c.JSON(500, gin.H{
				"success": false,
				"message": "Gagal mencari data destination",
				"error":   err.Error(),
			})
		}

		return
	}

	if err := config.DB.Delete(&destination).Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal menghapus destination",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "Destination berhasil dihapus",
	})

}

func ColorStockStatistics(c *gin.Context) {
	type colorStatistic struct {
		Color	string	`json:"color"`
		Qty 	int64 	`json:"qty"`
		TotalValue float64	`json:"total_value"`
	}

	var (
		grandTotalStickerQty int64
		grandTotalStickerValue float64
		grandTotalOlseraQty int64
		grandTotalOlseraValue float64
	)

	var mainProduct []colorStatistic
	if err := config.DB.Table("products p").
		Select(`
			LOWER(ct.name_color) as color,
			COUNT(*) as qty,
			COALESCE(SUM(p.price), 0) as total_value
		`).
		Joins("JOIN color_tags ct ON ct.id = p.tag_color_id").
		Where("p.category_id IS NULL").
		Where("p.is_so IS NULL").
		Where("p.tag_color_id IS NOT NULL").
		Where("p.status IN ?", []string{"display", "expired", "slow_moving"}).
		Where("p.quality = ?", "lolos").
		Where("p.warehouse_type IS NULL OR p.warehouse_type IN ?", []string{"type1", "type2"}).
		Group("color").
		Scan(&mainProduct).Error; err != nil {
		
		helpers.ErrorResponse(c, 500, "Gagal query data color product", err)
		return
	}

	productSticker := make(map[string]map[string]float64)
	for _, item := range mainProduct {
		color := strings.ToLower(item.Color) // biar konsisten

		productSticker[color] = map[string]float64{
			"qty":         float64(item.Qty),
			"total_value": item.TotalValue,
		}
		grandTotalStickerQty += item.Qty
		grandTotalStickerValue += item.TotalValue
	}

	//Olsera stock
	log := helpers.NewLogger("./logs/app.log")
	var destinations []models.MigrateColorDestination
	if err := config.DB.Where("is_olsera_integreted", true).Find(&destinations).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mengambil data destinations", err)
		return
	}

	productOlsera := map[string]map[string]float64{
		"24K": {
			"qty":         0,
			"total_value": 0,
		},
		"12K": {
			"qty":         0,
			"total_value": 0,
		},
		"lainnya": {
			"qty":         0,
			"total_value": 0,
		},
	}

	type olseraResult struct {
		K24Qty   float64
		K24Value float64

		K12Qty   float64
		K12Value float64

		LainQty   float64
		LainValue float64

		GrandQty   int64
		GrandValue float64
	}

	sem := make(chan struct{}, 1)
	var wg sync.WaitGroup
	ctx := c.Request.Context()
	resultChan := make(chan olseraResult, len(destinations))

	for _, dest := range destinations {

		destination := dest // COPY (anti bug range)

		sem <- struct{}{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			olseraService := services.NewOlseraService(&destination, log)

			resultAPI, err := olseraService.GetProductList(ctx, nil)
			if err != nil {
				log.WithError(err).
					Error(fmt.Sprintf("Gagal mengambil product list dari %s", destination.ShopName))
				return
			}

			data, _ := resultAPI.Data.(map[string]interface{})
			itemList, _ := data["data"].([]interface{})

			local := olseraResult{}

			for _, rawItem := range itemList {
				item, ok := rawItem.(map[string]interface{})
				if !ok {
					continue
				}

				name, _ := item["name"].(string)
				qtyStr, _ := item["stock_qty"].(string)
				priceStr, _ := item["sell_price"].(string)

				qtyFloat, _ := strconv.ParseFloat(qtyStr, 64)
				price, _ := strconv.ParseFloat(priceStr, 64)

				totalValue := qtyFloat * price

				switch {
				case strings.Contains(name, "dummy_product_big"):
					local.K24Qty += qtyFloat
					local.K24Value += totalValue

				case strings.Contains(name, "dummy_product_small"):
					local.K12Qty += qtyFloat
					local.K12Value += totalValue

				default:
					local.LainQty += qtyFloat
					local.LainValue += totalValue
				}

				local.GrandQty += int64(qtyFloat)
				local.GrandValue += totalValue
			}

			resultChan <- local
		}()
	}

	// Tunggu semua selesai
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Agregasi final (single thread → NO RACE)
	final := olseraResult{}
	for r := range resultChan {
		final.K24Qty += r.K24Qty
		final.K24Value += r.K24Value

		final.K12Qty += r.K12Qty
		final.K12Value += r.K12Value

		final.LainQty += r.LainQty
		final.LainValue += r.LainValue

		final.GrandQty += r.GrandQty
		final.GrandValue += r.GrandValue
	}

	productOlsera["24K"]["qty"] = final.K24Qty
	productOlsera["24K"]["total_value"] = final.K24Value
	productOlsera["12K"]["qty"] = final.K12Qty
	productOlsera["12K"]["total_value"] = final.K12Value
	productOlsera["lainnya"]["qty"] = final.LainQty
	productOlsera["lainnya"]["total_value"] = final.LainValue

	c.JSON(200, gin.H{
		"success": false,
		"message": "Data statistik Stok dan Valuasi Product Color",
		"resource": gin.H{
			"product_sticker": gin.H{
				"grand_total_qty": grandTotalStickerQty,
				"grand_total_value": grandTotalStickerValue,
				"detail_per_colors": productSticker,
			},
			"olsera_stock": gin.H{
				"grand_total_qty": grandTotalOlseraQty,
				"grand_total_value": grandTotalOlseraValue,
				"detail_per_colors": productOlsera,
			},
		},
	})
}

func SyncOlseraToken(c *gin.Context) {
	var destinations []models.MigrateColorDestination
	if err := config.DB.Where("is_olsera_integreted", true).Find(&destinations).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mengambil data destinations", err)
		return
	}

	ctx := c.Request.Context()
	log := helpers.NewLogger("./logs/app.log")
	responseResult := []gin.H{}
	for _, destination := range destinations {
		dest := destination
		olseraService := services.NewOlseraService(&dest, log)
		err := olseraService.SyncOlseraToken(ctx)
		if err != nil {
			responseResult = append(responseResult, gin.H{
				"toko": destination.ShopName,
				"statug": "Gagal",
				"message": err.Error(),
			})
		}else {
			responseResult = append(responseResult, gin.H{
				"toko": destination.ShopName,
				"statug": "Sukses",
				"message": "Token berhasil diperbarui",
			})
		}
	}

	c.JSON(200, gin.H{
		"success": false,
		"message": "Proses singkronisasi selesai",
		"resource": responseResult,
	})
}