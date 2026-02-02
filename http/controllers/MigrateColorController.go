package controllers

import (
	"errors"
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"math"
	"net/http"
	"strconv"
	"strings"

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
	}


	var colors []colorCount

	// HITUNG BERDASARKAN JOIN COLOR TAG
	err := config.DB.Table("products as p").
		Select("ct.name_color as color, COUNT(*) as total").
		Joins("JOIN color_tags ct ON ct.id = p.tag_color_id").
		Where("p.tag_color_id IS NOT NULL").
		Where("p.category_id IS NULL").
		Where("p.quality = ?", "lolos").
		Where("p.status = ?", "display").
		Group("ct.name_color").
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
			"color":        colors,
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
		Where("p.status = ?", "display").
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
		ProductColor:        fmt.Sprint(req.ProductColor), // sesuaikan tipe di struct
		ProductTotal:        req.ProductTotal,
		Status:       "proses",
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

	// Ambil Document
	var documents []models.MigrateColorDocument

	err := tx.Preload("Migrates").
		Where("user_id = ? AND status_document = ?", user.ID, "proses").
		Find(&documents).Error

	if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal mengambil migrate document",
			"error":   err.Error(),
		})
		return
	}

	if len(documents) == 0 {
		tx.Rollback()
		c.JSON(422, gin.H{
			"success": false,
			"message": "Tidak ada data migrate yang sedang proses",
		})
		return
	}

	// ================= LOOP DOCUMENT =================
	for _, doc := range documents {

		totalUpdated := 0

		for _, m := range doc.Migrates {

			// Ambil Product ID
			var productIDs []uint

			err := tx.Table("products p").
				Select("p.id").
				Joins("JOIN color_tags c ON c.id = p.tag_color_id").
				Where("c.name_color = ?", m.ProductColor).
				Where("p.status = ?", "display").
				Order("p.created_at ASC").
				Limit(m.ProductTotal).
				Pluck("p.id", &productIDs).Error

			if err != nil {
				tx.Rollback()
				c.JSON(500, gin.H{
					"success": false,
					"message": "Gagal mengambil data product",
					"color":   m.ProductColor,
					"error":   err.Error(),
				})
				return
			}

			if len(productIDs) == 0 {
				tx.Rollback()
				c.JSON(422, gin.H{
					"success": false,
					"message": "Data product color tidak ditemukan",
					"color":   m.ProductColor,
				})
				return
			}

			// Update Product
			res := tx.Table("products").
				Where("id IN ?", productIDs).
				Update("status", "migrate")

			if res.Error != nil {
				tx.Rollback()
				c.JSON(500, gin.H{
					"success": false,
					"message": "Gagal update status product",
					"error":   res.Error.Error(),
				})
				return
			}

			if res.RowsAffected == 0 {
				tx.Rollback()
				c.JSON(422, gin.H{
					"success": false,
					"message": "Tidak ada product yang terupdate",
					"color":   m.ProductColor,
				})
				return
			}

			totalUpdated += int(res.RowsAffected)
		}

		//  Update Table Migrates 
		err = tx.Model(&models.MigrateColorItem{}).
			Where("code_document_migrate = ?", doc.CodeDocument).
			Update("status", "selesai").Error

		if err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"success": false,
				"message": "Gagal update status migrate",
				"error":   err.Error(),
			})
			return
		}

		// Update Document
		err = tx.Model(&doc).Updates(map[string]interface{}{
				"total_product_document": totalUpdated,
				"status_document":        "selesai",
			}).Error

		if err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"success": false,
				"message": "Gagal update migrate document",
				"error":   err.Error(),
			})
			return
		}
	}

	// ================= COMMIT =================
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal commit transaksi",
			"error":   err.Error(),
		})
		return
	}

	// ================= SUCCESS =================
	c.JSON(200, gin.H{
		"success": true,
		"message": "Migrate document berhasil difinish",
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