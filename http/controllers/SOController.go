package controllers

import (
	"errors"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"strings"

	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

//Stock Opname -> color
func GetSummarySoColors(c *gin.Context) {
	q := c.DefaultQuery("q", "")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	var summary_so_colors []models.SummarySoColor
	var totalData int64

	//inisialisasi query
	query := config.DB.Model(&models.SummarySoColor{})

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		query = query.Where("(start_date LIKE ?)", searchPattern)
	}

	// Menghitung total data yang sesuai dengan filter/search sebelum diterapkan limit/offset
	if err := query.Session(&gorm.Session{}).Count(&totalData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal menghitung total data", "error": err.Error()})
		return
	}

	err := query.
		Limit(limit).
		Offset(offset).
		Order("created_at desc"). // Sorting data terbaru di atas
		Find(&summary_so_colors).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"data": gin.H{
			"success":  true,
			"message": "List Summary So Color",
			"resource": gin.H{
				"current_page":   page,
				"data":           summary_so_colors,
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

func DetailSummarySoColor(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Format ID Summary SO tidak valid"})
        return
    }
    summary_so_id := uint(id)

	//load data sumary so
	var summary_so_color models.SummarySoColor
	if err := config.DB.Preload("SoColors").First(&summary_so_color, summary_so_id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"success": false, "message": "Data so tidak ditemukan"})
		}else {
			c.JSON(500, gin.H{"success": false, "message": "Server error", "err": err.Error()})
		}

		return
	}

	c.JSON(http.StatusOK, gin.H{
        "success": true, // Gunakan success agar konsisten dengan error di atas
        "message": "Detail Summary SO Color berhasil diambil",
        "data":    summary_so_color,
    })
}

func StartSoColor(c *gin.Context) {
	tx := config.DB.Begin()
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(500, gin.H{"success": false, "message": "Internal Server Error"})
			return
        }
    }()

	var existing models.SummarySoColor
    err := tx.Where("type = ?", "process").First(&existing).Error
    
    if err == nil {
        // Jika ditemukan (err nil berarti record ada), batalkan transaksi
        tx.Rollback()
        c.JSON(http.StatusUnprocessableEntity, gin.H{
            "success": false,
            "message": "SO process already started",
            "data":    nil,
        })
        return
    }

	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)

	today := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		0, 0, 0, 0,
		loc,
	)
	startDate := models.Date(today)
	newSoColor := models.SummarySoColor{
		StartDate: startDate,
		Type: "process",
		EndDate: nil,
	}

	//create so
	if err := tx.Create(&newSoColor).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": "An error occurred: " + err.Error(),
            "data":    nil,
        })
        return
    }

    // Commit Transaksi
    if err := tx.Commit().Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to commit transaction"})
        return
    }

	c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "SO process started successfully",
        "data":    newSoColor,
    })
}

func SubmitSoColor(c *gin.Context) {
	type itemColor struct {
		NameColor       string `json:"name_color" binding:"required"`
		TotalAll        int    `json:"total_all" binding:"gte=0"`
		ProductDamaged  int    `json:"product_damaged" binding:"gte=0"`
		ProductAbnormal int    `json:"product_abnormal" binding:"gte=0"`
		Lost            int    `json:"lost" binding:"gte=0"`
		Addition        int    `json:"addition" binding:"gte=0"`
	}

	type payloadRequest struct {
		Colors []itemColor `json:"colors" binding:"required,min=1,dive"`
	}

	var payload payloadRequest
	// Validasi JSON
	if err := c.ShouldBindJSON(&payload); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(400, gin.H{"status": false, "message": "Format JSON tidak valid"})
			return
		}
		errorsMap := make(map[string]string)
		for _, e := range ve {
			// e.Field() untuk struct 'Colors' akan menghasilkan "Colors"
			// e.StructField() untuk item di dalamnya akan menghasilkan "NameColor", "TotalAll", dll.
			field := e.Field()
			structField := e.StructField()

			// Logika pesan error berdasarkan field atau tag
			switch field {
			case "Colors":
				if e.Tag() == "required" || e.Tag() == "min" {
					errorsMap["colors"] = "Daftar warna tidak boleh kosong"
				}
			default:
				// Menangani field di dalam array (NameColor, TotalAll, dll)
				// Namespace akan berisi "payloadRequest.Colors[0].NameColor"
	
				key := strings.ToLower(structField)
				
				switch structField {
				case "NameColor":
					errorsMap["name_color"] = "Nama warna wajib diisi"
				case "TotalAll":
					errorsMap["total_all"] = "Total harus berupa angka dan tidak boleh negatif"
				case "ProductDamaged":
					errorsMap["product_damaged"] = "Jumlah rusak tidak boleh negatif"
				case "ProductAbnormal":
					errorsMap["product_abnormal"] = "Jumlah abnormal tidak boleh negatif"
				case "Lost":
					errorsMap["lost"] = "Jumlah hilang tidak boleh negatif"
				case "Addition":
					errorsMap["addition"] = "Jumlah tambahan tidak boleh negatif"
				default:
					errorsMap[key] = "Validasi gagal pada field " + structField
				}
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"message": "Validasi gagal",
			"errors": errorsMap,
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"success": false, "message": "System Panic Occurred"})
		}
	}()

	// Cari Periode SO yang Aktif
	var activePeriod models.SummarySoColor
	if err := tx.Where("type = ? AND end_date IS NULL", "process").First(&activePeriod).Error; err != nil {
		tx.Rollback()
		c.JSON(422, gin.H{"success": false, "message": "No active SO period found"})
		return
	}

	// Ambil semua SoColor yang sudah ada untuk periode ini
	var existingSoColors []models.SoColor
	tx.Where("summary_so_color_id = ?", activePeriod.ID).Find(&existingSoColors)

	// Masukkan ke Map untuk lookup cepat
	existingMap := make(map[string]*models.SoColor)
	for i := range existingSoColors {
		colorName := existingSoColors[i].Color
		existingMap[colorName] = &existingSoColors[i]
	}

	// Iterasi Payload
	for _, col := range payload.Colors {
		if existing, found := existingMap[col.NameColor]; found {
			// UPDATE jika ditemukan
			updates := map[string]interface{}{
				"total_color":      col.TotalAll,
				"product_damaged":  col.ProductDamaged,
				"product_abnormal": col.ProductAbnormal,
				"product_lost":     col.Lost,
				"product_addition": col.Addition,
			}
			if err := tx.Model(existing).Updates(updates).Error; err != nil {
				tx.Rollback()
				c.JSON(500, gin.H{"success": false, "message": "Failed to update " + col.NameColor})
				return
			}
		} else {
			// CREATE jika tidak ada
			newColor := models.SoColor{
				SummarySoColorID: uint64(activePeriod.ID),
				Color:           col.NameColor,
				TotalColor:      col.TotalAll, 
				ProductDamaged:  col.ProductDamaged,
				ProductAbnormal: col.ProductAbnormal,
				ProductLost:     col.Lost,
				ProductAddition: col.Addition,
			}

			if err := tx.Create(&newColor).Error; err != nil {
				tx.Rollback()
				c.JSON(500, gin.H{"success": false, "message": "Failed to create " + col.NameColor})
				return
			}
		}
	}

	// Commit
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Commit failed"})
		return
	}

	c.JSON(200, gin.H{"success": true, "message": "SO colors processed successfully"})
}

func StopSoColor(c *gin.Context) {
	tx := config.DB.Begin()
    if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start transaction",
		})
		return
	}

	// Rollback jika panic
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var activePeriod models.SummarySoColor
	if err := tx.Where("type = ? AND end_date IS NULL", "process").First(&activePeriod).Error; err != nil {
		tx.Rollback()
		c.JSON(422, gin.H{"success": false, "message": "No active SO period found"})
		return
	}

	
	//update so
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)

	today := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		0, 0, 0, 0,
		loc,
	)
	endDate := models.Date(today)
	if err := tx.Model(&activePeriod).Updates(map[string]interface{}{
		"type": "done",
		"end_date": endDate,
	}).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": "An error occurred: " + err.Error(),
        })
        return
    }

    // Commit Transaksi
    if err := tx.Commit().Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to commit transaction"})
        return
    }

	c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "SO stop successfully",
        "data":    activePeriod,
    })
}

//Stock Opname -> category
func GetSummarySoCategories(c *gin.Context) {
	q := c.DefaultQuery("q", "")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	var summary_so_category []models.SummarySoCategory
	var totalData int64

	//inisialisasi query
	query := config.DB.Model(&models.SummarySoCategory{})

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		query = query.Where("(start_date LIKE ?)", searchPattern)
	}

	// Menghitung total data yang sesuai dengan filter/search sebelum diterapkan limit/offset
	if err := query.Session(&gorm.Session{}).Count(&totalData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal menghitung total data", "error": err.Error()})
		return
	}

	err := query.
		Limit(limit).
		Offset(offset).
		Order("created_at desc"). // Sorting data terbaru di atas
		Find(&summary_so_category).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))

	baseURL := c.Request.Host + c.Request.URL.Path
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	fullURL := scheme + "://" + baseURL

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"data": gin.H{
			"success":  true,
			"message": "List Summary So Category",
			"resource": gin.H{
				"current_page":   page,
				"data":           summary_so_category,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"path":           fullURL,
				"per_page":       limit,
				"to":             offset + int(totalData),
				"total":          totalData,
			},
		},
	})
}

func DetailSummarySoCategory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Format ID Summary SO tidak valid"})
        return
    }
    summary_so_id := uint(id)

	//load data sumary so
	var summary_so_category models.SummarySoCategory
	if err := config.DB.First(&summary_so_category, summary_so_id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"success": false, "message": "Data so tidak ditemukan"})
		}else {
			c.JSON(500, gin.H{"success": false, "message": "Server error", "err": err.Error()})
		}

		return
	}

	c.JSON(http.StatusOK, gin.H{
        "success": true, // Gunakan success agar konsisten dengan error di atas
        "message": "Detail Summary SO Category berhasil diambil",
        "data":    summary_so_category,
    })
}

func FilterSoCategory(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	q := c.DefaultQuery("q", "")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 10
	offset := (page - 1) * limit

	type paginateData struct {
		Barcode string `json:"barcode"`
		Name string `json:"name"`
		Type string `json:"type"`
	}

	searchCondition := ""
	args := []interface{}{}

	if q != "" {
		searchCondition = `
			AND (
				barcode LIKE ?
				OR name LIKE ?
			)
		`
		search := "%" + q + "%"
		args = append(args, search, search)
	}

	//UNION PRODUCT WITH BUNDLE
	dataQuery := fmt.Sprintf(`
		SELECT * FROM (
			SELECT
				barcode,
				name,
				CASE 
					WHEN quality = 'damage' THEN 'damaged'
					WHEN quality = 'abnormal' THEN 'abnormal'
					WHEN is_so = 'addition' THEN 'addition'
					WHEN location_type = 'main' THEN 'inventory'
					ELSE 'staging'
				END as type,
				created_at
			FROM products
			WHERE tag_color_id IS NULL 
				AND is_so IN ('check', 'addition')
				AND user_so = %d

			UNION ALL

			SELECT
				barcode,
				name_bundle AS name,
				'bundle' as type,
				created_at
			FROM bundles
			WHERE user_so = %d 
				AND is_so IN ('check', 'addition')	
				AND warehouse_type != 'type2'
		) x
		WHERE 1=1
		%s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, user.ID, user.ID, searchCondition)

	argsData := append(args, limit, offset)
	var filter_so []paginateData
	if err := config.DB.Raw(dataQuery, argsData...).Scan(&filter_so).Error; err != nil {
		c.JSON(500, gin.H{"status": false, "error": err.Error()})
		return
	}

	//count
	countQuery := fmt.Sprintf(`
		SELECT count(*) FROM (
			SELECT
				barcode,
				name
			FROM products
			WHERE tag_color_id IS NULL 
				AND is_so IN ('check', 'addition')
				AND user_so = %d

			UNION ALL

			SELECT
				barcode,
				name_bundle AS name
			FROM bundles
			WHERE user_so = %d
				AND is_so IN ('check', 'addition')	
		) x
		WHERE 1=1
		%s
	`, user.ID, user.ID, searchCondition)

	var totalData int64
	if err := config.DB.Raw(countQuery, args...).Scan(&totalData).Error; err != nil {
		c.JSON(500, gin.H{"status": false, "error": err.Error()})
		return
	}

    // pagination links
    lastPage := int(math.Ceil(float64(totalData) / float64(limit)))
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"status":  true,
		"message": "List Filter So Category",
		"resource": gin.H{
			"total":          totalData,
			"data":           filter_so,
			"current_page":   page,
			"last_page":      lastPage,
			"per_page":       limit,
            "links":           links,
		},
	})
}

func SearchSoCategory(c *gin.Context)  {
	q := c.DefaultQuery("q", "")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 10
	offset := (page - 1) * limit

	type paginateData struct {
		Barcode string `json:"barcode"`
		Name string `json:"name"`
		Category string `json:"category"`
		CreatedAt string `json:"created_at"`
		Type string `json:"type"`
	}

	searchCondition := ""
	args := []interface{}{}

	if q != "" {
		searchCondition = `
			AND (
				barcode LIKE ?
				OR name LIKE ?
			)
		`
		search := "%" + q + "%"
		args = append(args, search, search)
	}

	//UNION PRODUCT WITH BUNDLE
	dataQuery := fmt.Sprintf(`
		SELECT * FROM (
			SELECT
				p.barcode AS barcode,
				p.name AS name,
				CASE 
					WHEN p.quality = 'damage' THEN 'damaged'
					WHEN p.quality = 'abnormal' THEN 'abnormal'
					WHEN p.location_type = 'main' THEN 'inventory'
					ELSE 'staging'
				END as type,
				c.name_category AS category,
				p.created_at AS created_at
			FROM products p
			LEFT JOIN categories c ON c.id = p.category_id
			WHERE p.tag_color_id IS NULL 
				AND p.is_so IS NULL
				AND p.status != 'sale'

			UNION ALL

			SELECT
				b.barcode AS barcode,
				b.name_bundle AS name,
				'bundle' as type,
				c.name_category AS category,
				b.created_at AS created_at
			FROM bundles b
			LEFT JOIN categories c ON c.id = b.category_id
			WHERE b.warehouse_type != 'type2'	
		) x
		WHERE 1=1
		%s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, searchCondition)

	argsData := append(args, limit, offset)
	var search_so []paginateData
	if err := config.DB.Raw(dataQuery, argsData...).Scan(&search_so).Error; err != nil {
		c.JSON(500, gin.H{"status": false, "error": err.Error()})
		return
	}

	//count
	countQuery := fmt.Sprintf(`
		SELECT count(*) FROM (
			SELECT
				p.barcode,
				p.name
			FROM products p
			LEFT JOIN categories c ON c.id = p.category_id
			WHERE p.tag_color_id IS NULL 
				AND p.is_so IS NULL
				and p.status != 'sale'

			UNION ALL

			SELECT
				b.barcode,
				b.name_bundle AS name
			FROM bundles b
			LEFT JOIN categories c ON c.id = b.category_id
			WHERE b.warehouse_type != 'type2'
		) x
		WHERE 1=1
		%s
	`, searchCondition)

	var totalData int64
	if err := config.DB.Raw(countQuery, args...).Scan(&totalData).Error; err != nil {
		c.JSON(500, gin.H{"status": false, "error": err.Error()})
		return
	}

    // pagination links
    lastPage := int(math.Ceil(float64(totalData) / float64(limit)))
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"status":  true,
		"message": "List Data Product",
		"resource": gin.H{
			"total":          totalData,
			"data":           search_so,
			"current_page":   page,
			"last_page":      lastPage,
			"per_page":       limit,
            "links":           links,
		},
	})
}

func StartSoCategory(c *gin.Context) {
	tx := config.DB.Begin()
    if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start transaction",
		})
		return
	}

	// Rollback jika panic
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var existing models.SummarySoCategory
    err := tx.Where("type = ?", "process").First(&existing).Error
    
    if err == nil {
        // Jika ditemukan (err nil berarti record ada), batalkan transaksi
        tx.Rollback()
        c.JSON(http.StatusUnprocessableEntity, gin.H{
            "success": false,
            "message": "SO process already started",
            "data":    nil,
        })
        return
    }

	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)

	today := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		0, 0, 0, 0,
		loc,
	)
	startDate := models.Date(today)
	newPeriod := models.SummarySoCategory{
        Type:             "process",
        ProductInventory: 0,
        ProductStaging:   0,
        ProductBundle:    0,
        ProductDamaged:   0,
        ProductAbnormal:  0,
        ProductLost:      0,
        ProductAddition:  0,
        StartDate:        startDate,
        EndDate:          nil,
    }

	//create so
	if err := tx.Create(&newPeriod).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": "An error occurred: " + err.Error(),
            "data":    nil,
        })
        return
    }

    // Commit Transaksi
    if err := tx.Commit().Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to commit transaction"})
        return
    }

	c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "SO process started successfully",
        "data":    newPeriod,
    })
}

func UpdateCheck(c *gin.Context) {
	type payloadRequest struct {
		// binding oneof harus sesuai dengan case di logic bisnis Anda
		Type    string `json:"type" binding:"omitempty,oneof=inventory staging bundle damaged abnormal lost manual"`
		Barcode string `json:"barcode" binding:"required"`
	}

	var req payloadRequest

	// Validasi Input
	if err := c.ShouldBindJSON(&req); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"status": false, "message": "Format JSON tidak valid"})
			return
		}

		errors := make(map[string]string)
		for _, e := range ve {
			// e.Field() akan mengambil nama field Struct (misal: "Barcode")
			// e.Tag() akan mengambil nama tag validator (misal: "required")
			switch e.Field() {
			case "Barcode":
				if e.Tag() == "required" {
					errors["barcode"] = "Barcode wajib diisi"
				}
			case "Type":
				if e.Tag() == "oneof" {
					errors["type"] = "Tipe tidak valid. Pilih: inventory, staging, bundle, damaged, abnormal, lost, atau manual"
				}
			}
		}

		// Jika ada field lain yang tertinggal namun ada error (fallback)
		if len(errors) == 0 && len(ve) > 0 {
			for _, e := range ve {
				errors[strings.ToLower(e.Field())] = fmt.Sprintf("Error pada validasi %s", e.Tag())
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{ // Gunakan 422 agar sama dengan logic Laravel sebelumnya
			"status":  false,
			"message": "Validasi gagal",
			"errors":   errors,
		})
		return
	}

	// Mulai Transaksi Database
	err := config.DB.Transaction(func(tx *gorm.DB) error {
        var activePeriod models.SummarySoCategory
        // 1. Cek periode SO
        if err := tx.Where("type = ?", "process").First(&activePeriod).Error; err != nil {
            return fmt.Errorf("periode_not_found") 
        }

        var product models.Product
        var bundle models.Bundle

        // 2. Gunakan userID dari context (Pastikan middleware auth sudah benar)
        user := c.MustGet("auth_user").(models.User)

        // Cari data
        hasInventory := tx.Where("barcode = ?", req.Barcode).First(&product).Error == nil
        hasBundle := tx.Where("barcode = ?", req.Barcode).First(&bundle).Error == nil

        // 3. Logika Bisnis
        if req.Type == "lost" {
            return tx.Model(&activePeriod).Update("product_lost", gorm.Expr("product_lost + ?", 1)).Error
        }

        // Logic Manual atau default check
        if hasInventory {
            // Perbaikan Bug: Gunakan helper untuk cek string pointer agar tidak panic
            if getString(product.IsSo) == "check" {
                return fmt.Errorf("product_already_checked")
            }
            
            incrementActivePeriod(tx, &activePeriod, &product)
            return tx.Model(&product).Updates(map[string]interface{}{
                "is_so": "check", 
                "user_so": user.ID,
            }).Error

        } else if hasBundle {
            if getString(bundle.IsSo) == "check" {
                return fmt.Errorf("bundle_already_checked")
            }
            
            if err := tx.Model(&activePeriod).Update("product_bundle", gorm.Expr("product_bundle + ?", 1)).Error; err != nil {
                return err
            }
            return tx.Model(&bundle).Updates(map[string]interface{}{
                "is_so": "check", 
                "user_so": user.ID,
            }).Error

        } else {
            return fmt.Errorf("product_not_found")
        }
    })

	// Response Handling
	if err != nil {
        msg := err.Error()
        code := http.StatusInternalServerError
        
        switch msg {
        case "periode_not_found":
            msg, code = "Tidak ada periode SO aktif", http.StatusUnprocessableEntity
        case "product_already_checked", "bundle_already_checked":
            msg, code = "Produk/Bundle sudah pernah dicek", http.StatusUnprocessableEntity
        case "product_not_found":
            msg, code = "Product tidak ditemukan", http.StatusNotFound
        }
        
        c.JSON(code, gin.H{"success": false, "message": msg})
        return
    }

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message": "Product checked successfully",
		"data":    req.Barcode,
	})
}

// Helper
func getString(s *string) string {
    if s == nil {
        return ""
    }
    return *s
}

func incrementActivePeriod(tx *gorm.DB, activePeriod *models.SummarySoCategory, inventory *models.Product) {	
	location := getString(inventory.LocationType)
	if location == "main" {		
		column := "product_inventory"
        if inventory.Quality == "damage" {
            column = "product_damaged"
        } else if inventory.Quality == "abnormal" {
            column = "product_abnormal"
        }
        tx.Model(activePeriod).Update(column, gorm.Expr(column+" + ?", 1))
	}else {
		column := "product_staging"
        if inventory.Quality == "damage" {
            column = "product_damaged"
        } else if inventory.Quality == "abnormal" {
            column = "product_abnormal"
        }
		tx.Model(activePeriod).Update(column, gorm.Expr(column+" + ?", 1))
	}
}

func StopSoCategory(c *gin.Context) {
	tx := config.DB.Begin()
    if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start transaction",
		})
		return
	}

	// Rollback jika panic
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var activePeriod models.SummarySoCategory
	if err := tx.Where("type = ? AND end_date IS NULL", "process").First(&activePeriod).Error; err != nil {
		tx.Rollback()
		c.JSON(422, gin.H{"success": false, "message": "No active SO period found"})
		return
	}

	
	//update so
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)

	today := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		0, 0, 0, 0,
		loc,
	)
	endDate := models.Date(today)
	if err := tx.Model(&activePeriod).Updates(map[string]interface{}{
		"type": "done",
		"end_date": endDate,
	}).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": "An error occurred: " + err.Error(),
        })
        return
    }

	if err := tx.First(&activePeriod, activePeriod.ID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to load updated data",
		})
		return
	}

    // Commit Transaksi
    if err := tx.Commit().Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to commit transaction"})
        return
    }

	c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "SO stop successfully",
        "data":    activePeriod,
    })
}