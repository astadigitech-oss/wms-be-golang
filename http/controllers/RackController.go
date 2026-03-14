package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"math"
	"regexp"
	"strings"

	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

func GetRacks(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	source := strings.TrimSpace(c.Query("source"))

	if source != "staging" && source != "display" {
		c.JSON(400, gin.H{"success": false, "message": "source tidak valid"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	limit := 10
	offset := (page - 1) * limit

	//inisialisasi query
	baseQuery := config.DB.Model(&models.Rack{}).
		Where("source = ?", source)

	// Searching
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(name LIKE ? OR "+
            "barcode LIKE ?)", searchPattern, searchPattern)
	}

    // Hitung grand total price dari summary
    var total_rack int64
	var total_product_in_rack int64

    baseQuery.Session(&gorm.Session{}).Count(&total_rack)
	baseQuery.Session(&gorm.Session{}).
        Select("COALESCE(SUM(total_data), 0)").
        Scan(&total_product_in_rack)

	var racks []models.Rack
    // Ambil data detail
    err := baseQuery.Session(&gorm.Session{}).
        Order("created_at DESC").
        Limit(limit).Offset(offset).
        Find(&racks).Error

    if err != nil {
        c.JSON(500, gin.H{"success": false, "message": "error", "error": err.Error()})
        return
    }

	lastPage := int(math.Ceil(float64(total_rack) / float64(limit)))
	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List racks " + source,
			"resource": gin.H{
				"current_page": page,
                "data":                 racks,
                "total_rack":           total_rack,
                "total_product_in_rack":           total_product_in_rack,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + int(total_rack),
			},
		},
	})
}
func RackDetail(c *gin.Context) {
	rack_id := c.Param("rack_id")

	type Item struct {
		ID            uint64    `json:"id"`
		Name          string    `json:"name"`
		Barcode       string    `json:"barcode"`
		NewPrice         float64   `json:"new_price"`
		OldPrice         float64   `json:"old_price"`
		DisplayPrice  float64   `json:"display_price"`
		Status        string    `json:"status"`
		Type          string    `json:"type"` // product / bundle
		CreatedAt     time.Time `json:"created_at"`
	}

	// ====== Query Params ======
	search := strings.TrimSpace(c.Query("q"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	if page < 1 {
		page = 1
	}

	offset := (page - 1) * limit

	var rack models.Rack
	if err := config.DB.First(&rack, "id = ?", rack_id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"success": false, "message": "Rack not found"})
		}else {
			c.JSON(500, gin.H{"success": false, "message": "Server Error", "error": err.Error()})
		}

		return
	}

	var items []Item
	var total int64

	// ====== Base Query ======
	baseQuery := `
		SELECT id,
		       name,
		       barcode,
		       new_price,
		       old_price,
		       display_price,
		       status,
		       type,
		       created_at
		FROM (
			SELECT 
				id,
				name,
				barcode,
				price as new_price,
				old_price_product as old_price,
				display_price,
				status,
				'product' as type,
				created_at
			FROM products
			WHERE rack_id = ?

			UNION ALL

			SELECT
				id,
				CONCAT('[BUNDLE] ', name_bundle) as name,
				barcode,
				total_price_custom as new_price,
				total_price as old_price,
				total_price_custom as display_price,
				status,
				'bundle' as type,
				created_at
			FROM bundles
			WHERE rack_id = ?
		) as combined
		WHERE 1=1
	`

	args := []interface{}{rack.ID, rack.ID}

	// ====== Filtering ======
	if search != "" {
		baseQuery += " AND (name LIKE ? OR barcode LIKE ?)"
		searchLike := "%" + search + "%"
		args = append(args, searchLike, searchLike)
	}

	// ====== Count Total ======
	countQuery := "SELECT COUNT(*) FROM (" + baseQuery + ") as count_table"
	if err := config.DB.Raw(countQuery, args...).Scan(&total).Error; err != nil {
		helpers.ErrorResponse(c, 500, "internal server error", err)
		return
	}

	// ====== Pagination + Order ======
	baseQuery += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	if err := config.DB.Raw(baseQuery, args...).Scan(&items).Error; err != nil {
		helpers.ErrorResponse(c, 500, "internal server error", err)
		return
	}

	//pagination
	lastPage := int(math.Ceil(float64(total) / float64(limit)))
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"success":  true,
		"message": "Rack Detail",
		"resource": gin.H{
			"rack_info": rack,
			"products": gin.H{
				"data": items,
				"pagination": gin.H{
					"current_page": page,
					"total": total,
					"links": links,
					"per_page": limit,
					"from": offset + 1,
					"to": offset + limit,
				},
			},
		},
	})
}
func ProductBySourceRack(c *gin.Context) {
	rackID := strings.TrimSpace(c.Query("rack_id"))
	source := strings.TrimSpace(c.Query("source"))
	q := strings.TrimSpace(c.Query("q"))

	if source != "staging" && source != "display" {
		c.JSON(400, gin.H{"success": false, "message": "source tidak valid"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	limit := 10
	offset := (page - 1) * limit

	locationType := "main"
	if source == "staging" {
		locationType = "staging"
	}

	//inisialisasi query
	productQuery := config.DB.Model(&models.Product{}).
		Select(`
			products.id AS id,
			products.name AS product_name,
			products.barcode AS product_barcode,
			products.old_barcode_product AS product_old_barcode,
			COALESCE(categories.name_category, 'Unknown') AS category_name,
			'product' AS source_type,
			products.created_at
		`).
		Joins("LEFT JOIN categories ON categories.id = products.category_id").
		Where("products.tag_color_id IS NULL").
		Where("products.category_id IS NOT NULL").
		Where("products.rack_id IS NULL").
		Where("products.status IN ?", []string{"display", "expired", "slow_moving"}).
		Where("products.quality = ?", "lolos").
		Where("products.location_type = ?", locationType)
	
	bundleQuery := config.DB.Model(&models.Bundle{}).
		Select(`
			bundles.id AS id,
			CONCAT('[BUNDLE] ', bundles.name_bundle) AS product_name,
			bundles.barcode AS product_barcode,
			NULL AS product_old_barcode,
			COALESCE(categories.name_category, 'Unknown') AS category_name,
			'bundle' AS source_type,
			bundles.created_at
		`).
		Joins("LEFT JOIN categories ON categories.id = bundles.category_id").
		Where("bundles.tag_color_id IS NULL").
		Where("bundles.category_id IS NOT NULL").
		Where("bundles.rack_id IS NULL").
		Where("bundles.status != ?", "sale")

	// Filter by rack name
	if rackID != "" {
		var rack models.Rack
		if err := config.DB.First(&rack, "id = ?", rackID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(404, gin.H{"success": false, "message": "Rack not found"})
			} else {
				c.JSON(500, gin.H{"success": false, "message": "Server Error", "error": err.Error()})
			}
			return
		}
		rackName := strings.ToUpper(strings.TrimSpace(rack.Name))
		// ambil string setelah tanda "-"
		if idx := strings.Index(rackName, "-"); idx != -1 {
			rackName = rackName[idx+1:]
		}
		// ganti sisa "-" jadi spasi
		rackName = strings.ReplaceAll(rackName, "-", " ")
		// hapus spasi + angka di akhir (contoh: "ABC 12" → "ABC")
		re := regexp.MustCompile(`\s+\d+$`)
		rackName = re.ReplaceAllString(rackName, "")
		// pisah berdasarkan koma
		keywords := strings.Split(rackName, ",")
		var conditions []string
		var values []interface{}
		for _, keyword := range keywords {
			keyword = strings.TrimSpace(keyword)
			if keyword != "" {
				conditions = append(conditions, "categories.name_category LIKE ?")
				values = append(values, "%" + keyword + "%")
			}
		}

		if len(conditions) > 0 {
			cond := strings.Join(conditions, " OR ")
			productQuery = productQuery.Where(config.DB.Where(cond, values...))
			bundleQuery = bundleQuery.Where(config.DB.Where(cond, values...))
		}
	}

	// Searching
	if q != "" {
		searchPattern := "%" + q + "%"
		productQuery = productQuery.Where(
			"(products.name LIKE ? OR products.barcode LIKE ? OR products.old_barcode_product LIKE ?)",
			searchPattern, searchPattern, searchPattern,
		)

		bundleQuery = bundleQuery.Where(
			"(bundles.name_bundle LIKE ? OR bundles.barcode LIKE ?)",
			searchPattern, searchPattern,
		)
	}

	var finalQuery *gorm.DB
	if source == "display" {
		unionQuery := config.DB.Raw("? UNION ALL ?",productQuery,bundleQuery)
		finalQuery = config.DB.Table("(?) as combined", unionQuery)
	} else {
		finalQuery = productQuery
	}

    // Hitung total data
    var totalData int64
    finalQuery.Session(&gorm.Session{}).Count(&totalData)

	type productData struct {
		ID                uint   `json:"id"`
		ProductName       string `json:"product_name"`
		ProductBarcode    string `json:"product_barcode"`
		ProductOldBarcode string `json:"product_old_barcode"`
		CategoryName      string `json:"category_name"`
		SourceType		  string `json:"source_type"`
	}

	var products []productData
    // Ambil data detail
    err := finalQuery.Session(&gorm.Session{}).
        Order("created_at DESC").
        Limit(limit).Offset(offset).
        Scan(&products).Error

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
			"message": "List product " + source,
			"resource": gin.H{
                "data":                 products,
                "current_page":           page,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + len(products),
			},
		},
	})
}
func AddRack(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	type payloadRequest struct {
		DisplayRackID  uint   `json:"display_rack_id" binding:"required"`
		Source         string `json:"source" binding:"required,oneof=staging"`
	}

	//validasi
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
            case "displayrackid":
                if e.Tag() == "required" {
                    errors["display_rack_id"] = "Display Rack ID wajib diisi"
                }
            case "source":
                if e.Tag() == "required" {
                    errors["source"] = "Source wajib diisi"
                }else {
					errors["source"] = "Source harus staging"
				}
            default:
                // fallback: use the json tag name if possible
                errors[strings.ToLower(e.Field())] = e.Error()
            }
        }

        c.JSON(http.StatusBadRequest, gin.H{
            "status": false,
            "message": "Validasi gagal",
            "errors": errors,
        })
        return
    }

	//search parent rack
	var parentRack models.Rack
	if err := config.DB.First(&parentRack, payload.DisplayRackID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"status": false, "message": "Rak induk tidak ditemukan"})
		}else {
			c.JSON(500, gin.H{"success": false, "message": "Server Error", "error": err.Error()})
		}

		return
	}

	if parentRack.Source != "display" {
		c.JSON(400, gin.H{"success": false, "message": "Rak induk harus display"})
		return
	}

	prefix := fmt.Sprintf(
		"%s%d-%s",
		strings.ToUpper(payload.Source[:1]),
		user.ID,
		parentRack.Name,
	)

	var latestRack models.Rack
	config.DB.Where("source = ? AND name LIKE ?", "staging", prefix+"%").
		Order("LENGTH(name) DESC").
		Order("name DESC").
		First(&latestRack)

	nextNumber := 1
	if latestRack.ID != 0 {
		parts := strings.Split(latestRack.Name, " ")
		if n, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			nextNumber = n + 1
		}
	}

	finalName := fmt.Sprintf("%s %d", prefix, nextNumber)
	displayRackID := parentRack.ID

	barcode := helpers.RandomString(5)

	rack := models.Rack{
		DisplayRackID: &displayRackID,
		Name:          finalName,
		Source:       "staging",
		Barcode:      fmt.Sprintf("S%d-%s", user.ID, barcode),
	}

	if err := config.DB.Create(&rack).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			c.JSON(422, gin.H{
				"success": false,
				"message": "Duplicate Rack",
			})
		} else {
			c.JSON(500, gin.H{"success": false, "message": "Gagal membuat rak", "error": err.Error()})
		}

		return
	}

	c.JSON(200, gin.H{
		"status":  true,
		"message": "Berhasil membuat Rak " + rack.Name,
	})
}
func UpdateRack(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
	rackID, err := strconv.ParseUint(c.Param("rack_id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"message": "rack_id tidak valid"})
		return
	}

	type payloadRequest struct {
		DisplayRackID  *uint   `json:"display_rack_id"`
		Name		   string `json:"name" binding:"required"`
	}

	//validasi
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
            case "name":
                if e.Tag() == "required" {
                    errors["name"] = "Name wajib diisi"
                }
            default:
                // fallback: use the json tag name if possible
                errors[strings.ToLower(e.Field())] = e.Error()
            }
        }

        c.JSON(http.StatusBadRequest, gin.H{
            "status": false,
            "message": "Validasi gagal",
            "errors": errors,
        })
        return
    }

	var rack models.Rack
	if err := config.DB.First(&rack, rackID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"status": false, "message": "Rak tidak ditemukan"})
		}else {
			c.JSON(500, gin.H{"status": false, "message": "Server Error", "error": err.Error()})
		}
		return
	}

	updateData := map[string]interface{}{}
	if rack.Source == "staging" {
		if payload.DisplayRackID == nil {
			c.JSON(400, gin.H{"status": false, "message": "Display Rack ID wajib diisi"})
			return
		}

		//search parent rack
		var parentRack models.Rack
		if err := config.DB.First(&parentRack, payload.DisplayRackID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(404, gin.H{"status": false, "message": "Rak induk tidak ditemukan"})
			}else {
				c.JSON(500, gin.H{"success": false, "message": "Server Error", "error": err.Error()})
			}
	
			return
		}

		prefix := fmt.Sprintf("S%d-%s", user.ID, parentRack.Name)
		var latestRack models.Rack
		config.DB.Where("source = ? AND name LIKE ?", "staging", prefix+"%").
			Order("LENGTH(name) DESC").
			Order("name DESC").
			First(&latestRack)

		nextNumber := 1
		if latestRack.ID != 0 {
			parts := strings.Split(latestRack.Name, " ")
			if n, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
				nextNumber = n + 1
			}
		}

		finalName := fmt.Sprintf("%s %d", prefix, nextNumber)
		displayRackID := parentRack.ID

		updateData["name"] = finalName
		updateData["display_rack_id"] = &displayRackID
	}else {
		updateData["name"] = payload.Name
	}

	if err := config.DB.Model(&rack).Updates(updateData).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Gagal update data rack", "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status":  true,
		"message": "Berhasil mengubah data Rak",
	})
}
func AddProductToRack(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
	rackID, err := strconv.ParseUint(c.Param("rack_id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"message": "rack_id tidak valid"})
		return
	}
	barcode := strings.TrimSpace(c.Param("barcode"))

	var rack models.Rack
	if err := config.DB.First(&rack, rackID).Error; err != nil {
		c.JSON(404, gin.H{"status": false, "message": "Rack tidak ditemukan"})
		return
	}

	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var (
		product models.Product
		bundle  models.Bundle
		isBundle bool
	)

	// Filter berdasarkan source (staging vs display)
	locationType := "main"
	if rack.Source == "staging" {
		locationType = "staging"
	}

	err = tx.Preload("Category").
		Where("location_type = ?", locationType).
		Where("(barcode = ? OR old_barcode_product = ?)", barcode, barcode).
		First(&product).Error

	if err != nil {
		// Kalau product tidak ada → coba bundle
		errBundle := tx.Preload("Category").
			Where("barcode = ?", barcode).
			First(&bundle).Error

		if errBundle != nil {
			tx.Rollback()
			c.JSON(404, gin.H{
				"status": false,
				"message": "Produk / Bundle tidak ditemukan di source " + rack.Source,
			})
			return
		}

		isBundle = true
	}

	prodCatName := ""
	if isBundle {
		prodCatName = strings.ToUpper(strings.TrimSpace(bundle.Category.NameCategory))
	}else {
		prodCatName = strings.ToUpper(strings.TrimSpace(product.Category.NameCategory))
	}

	// Validasi Kesesuaian Kategori (Logic Parsing Nama Rak)
	if rack.Name != "" && prodCatName != "" {
		rackName := strings.ToUpper(strings.TrimSpace(rack.Name))

		// Parsing Core Name Rak
		rackCategoryCore := rackName
		if idx := strings.Index(rackName, "-"); idx != -1 {
			rackCategoryCore = rackName[idx+1:]
		}
		// ganti sisa "-" jadi spasi
		rackCategoryCore = strings.ReplaceAll(rackCategoryCore, "-", " ")
		// hapus spasi + angka di akhir (contoh: "ABC 12" → "ABC")
		re := regexp.MustCompile(`\s+\d+$`)
		rackCategoryCore = re.ReplaceAllString(rackCategoryCore, "")

		keywords := strings.Split(rackCategoryCore, ",")
		isMatch := false
		for _, k := range keywords {
			cleanK := strings.TrimSpace(k)
			if cleanK != "" && strings.Contains(prodCatName, cleanK) {
				isMatch = true
				break
			}
		}

		if !isMatch {
			tx.Rollback()
			c.JSON(422, gin.H{
				"status":  false,
				"message": fmt.Sprintf("Gagal: Kategori produk '%s' tidak sesuai dengan Rak '%s'.", prodCatName, rackName),
			})
			return
		}
	}

	// Cek apakah sudah di rak lain
	if !isBundle && product.RackID != nil {
		var otherRack models.Rack
		config.DB.First(&otherRack, *product.RackID)
		tx.Rollback()
		c.JSON(422, gin.H{"status": false, "message": "Produk sudah berada di rak lain: " + otherRack.Name})
		return
	}

	if isBundle && bundle.RackID != nil {
		var otherRack models.Rack
		config.DB.First(&otherRack, *bundle.RackID)
		tx.Rollback()
		c.JSON(422, gin.H{"status": false, "message": "Bundle sudah berada di rak lain: " + otherRack.Name})
		return
	}

	// Update Rak ID Produk
	rackID64 := uint64(rack.ID)
	if isBundle {
		if err := tx.Model(&bundle).Update("rack_id", rackID64).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"status": false, "message": "Gagal update produk", "error": err.Error()})
			return
		}
	}else {
		if err := tx.Model(&product).Update("rack_id", rackID64).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"status": false, "message": "Gagal update produk", "error": err.Error()})
			return
		}
	}

	//update rack
	if err := helpers.RecalculateRack(tx, rackID64); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal update statistik rak", "error": err.Error()})
		return
	}

	source := "display"
	product_name := ""
	if !isBundle {
		product_name = product.Name

		if *product.LocationType == "staging" {
			source = "staging"
		}
	}else {
		product_name = bundle.NameBundle
		source = "bundle"
	}

	rack_history := models.RackHistory{
		UserID: uint64(user.ID),
		RackID: uint64(rack.ID),
		Barcode: barcode,
		ProductName: &product_name,
		Action: "IN",
		Source: &source,
	}
	if err := tx.Create(&rack_history).Error; err != nil {
		tx.Rollback()
		helpers.ErrorResponse(c, 500, "Gagal membuat rack history", err)
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed Commit", err)
        return
    }

	c.JSON(200, gin.H{
		"status":  true,
		"message": "Berhasil menambahkan produk ke Rak " + rack.Name,
	})
}
func RemoveProductFromRack(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
	rackID, err := strconv.ParseUint(c.Param("rack_id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"message": "rack_id tidak valid"})
		return
	}

	barcode := c.Param("barcode")

	var rack models.Rack
	if err := config.DB.First(&rack, rackID).Error; err != nil {
		c.JSON(404, gin.H{"status": false, "message": "Rack tidak ditemukan"})
		return
	}

	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Cari Produk
	var (
		product models.Product
		bundle  models.Bundle
		isBundle bool
	)
	// Filter berdasarkan source (staging vs display)
	locationType := "main"
	if rack.Source == "staging" {
		locationType = "staging"
	}

	err = tx.
		Where("location_type = ?", locationType).
		Where("(barcode = ? OR old_barcode_product = ?)", barcode, barcode).
		First(&product).Error

	if err != nil {
		// Kalau product tidak ada → coba bundle
		errBundle := tx.
			Where("barcode = ?", barcode).
			First(&bundle).Error

		if errBundle != nil {
			tx.Rollback()
			c.JSON(404, gin.H{
				"status": false,
				"message": "Produk / Bundle tidak ditemukan di rack " + rack.Name,
			})
			return
		}

		isBundle = true
	}

	// Update Rak ID Produk
	if isBundle {
		if err := tx.Model(&bundle).Updates(map[string]interface{}{
			"rack_id": nil,
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"status": false, "message": "Gagal update bundle", "error": err.Error()})
			return
		}
	}else {
		if err := tx.Model(&product).Updates(map[string]interface{}{
			"rack_id": nil,
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"status": false, "message": "Gagal update produk", "error": err.Error()})
			return
		}
	}

	//update rack
	if err := helpers.RecalculateRack(tx, uint64(rack.ID)); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal update statistik rak", "error": err.Error()})
		return
	}

	source := "display"
	if !isBundle && *product.LocationType == "staging" {
		source = "staging"
	}
	if isBundle {
		source = "bundle"
	}

	rack_history := models.RackHistory{
		UserID: uint64(user.ID),
		RackID: uint64(rack.ID),
		Barcode: product.Barcode,
		ProductName: &product.Name,
		Action: "OUT",
		Source: &source,
	}
	if err := tx.Create(&rack_history).Error; err != nil {
		tx.Rollback()
		helpers.ErrorResponse(c, 500, "Gagal membuat rack history", err)
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed commit", err)
        return
    }

	c.JSON(200, gin.H{
		"status":  true,
		"message": "Berhasil menghapus produk dari Rak " + rack.Name,
	})
}
func DeleteRack(c *gin.Context) {
	rackID, err := strconv.ParseUint(c.Param("rack_id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"message": "rack_id tidak valid"})
		return
	}

	var rack models.Rack
	if err := config.DB.First(&rack, rackID).Error; err != nil {
		c.JSON(404, gin.H{"status": false, "message": "Rack tidak ditemukan"})
		return
	}

	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Update Rak ID Produk/Bundle
	if err := tx.Model(&models.Product{}).Where("rack_id = ?", rack.ID).Updates(map[string]interface{}{
        "rack_id": nil,
    }).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal update produk", "error": err.Error()})
        return
    }
	if err := tx.Model(&models.Bundle{}).Where("rack_id = ?", rack.ID).Updates(map[string]interface{}{
        "rack_id": nil,
    }).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal update Bundle", "error": err.Error()})
        return
    }

	//update rack
	if err := tx.Delete(&rack).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal menghapus rack", "error": err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(200, gin.H{
		"status":  true,
		"message": "Berhasil menghapus Rak " + rack.Name,
	})
}
func MoveRackToDisplay(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
	rackID, err := strconv.ParseUint(c.Param("rack_id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"message": "rack_id tidak valid"})
		return
	}

	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var rack models.Rack
	if err := tx.First(&rack, rackID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"status": false, "message": "Rak tidak ditemukan"})
		}else {
			c.JSON(500, gin.H{"status": false, "message": "Server Error", "error": err.Error()})
		}

		tx.Rollback()
		return
	}

	//SO Process
	if !rack.IsSo {
		tx.Rollback()
		helpers.ErrorResponse(c, 422, fmt.Sprintf("Rack %s belum di so, tidak bisa pindah ke display", rack.Name), nil)
		return
	}

	if rack.DisplayRackID == nil {
		tx.Rollback()
		c.JSON(422, gin.H{"status": false, "message": "Rak tidak memiliki tujuan display"})
		return
	}

	//search parent rack
	var parentRack models.Rack
	if err := tx.First(&parentRack, rack.DisplayRackID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"status": false, "message": "Rak tujuan tidak ditemukan atau dihapus"})
		}else {
			c.JSON(500, gin.H{"success": false, "message": "Server Error", "error": err.Error()})
		}

		tx.Rollback()
		return
	}

	var count int64
	tx.Model(&models.Product{}).
		Where("rack_id = ?", rack.ID).
		Count(&count)
	if count == 0 {
		var countBundle int64
		tx.Model(&models.Bundle{}).Where("rack_id = ?", rack.ID).Count(&countBundle)
		if countBundle == 0 {
			tx.Rollback()
			helpers.ErrorResponse(c, 422, "Rak kosong, tidak perlu dipindahkan ke display", nil)
			return
		}
	}

	location := "main"
	if err := tx.Model(&models.Product{}).Where("rack_id = ?", rack.ID).Updates(map[string]interface{}{
		"rack_id": rack.DisplayRackID,
		"location_type": location,
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal memindahkan produk", "error": err.Error()})
		return
	}
	if err := tx.Model(&models.Bundle{}).Where("rack_id = ?", rack.ID).Update("rack_id = ?", rack.DisplayRackID).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal memindahkan bundle", "error": err.Error()})
		return
	}

	//recalculate rack display
	if err := helpers.RecalculateRack(tx, uint64(parentRack.ID)); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal update statistik rak display", "error": err.Error()})
		return
	}

	//recalculate rack staging
	if err := helpers.RecalculateRack(tx, uint64(rack.ID)); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal memindahkan rak", "error": err.Error()})
		return
	}

	// update rack
	if err := tx.Model(&rack).Updates(map[string]interface{}{
		"user_display_id": user.ID,
		"move_to_display_at": helpers.GetCurentTime(),
	}).Error; err != nil {
		tx.Rollback()
		helpers.ErrorResponse(c, 500, "Gagal update rack", err)
		return
	}

	// rack_history := models.RackHistory{
	// 	UserID: uint64(user.ID),
	// 	RackID: uint64(rack.ID),
	// 	Action: "MOVE",
	// 	Source: &rack.Source,
	// }
	// if err := tx.Create(&rack_history).Error; err != nil {
	// 	tx.Rollback()
	// 	helpers.ErrorResponse(c, 500, "internal server error", err)
	// 	return
	// }

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"success": true, 
		"message": "Berhasil memindahkan rak",
	})
}
func GetRackInsertionStats(c *gin.Context) {
	source := c.Query("source")
	search := c.Query("q")

	if source != "staging" && source != "display" {
		helpers.ErrorResponse(c, 400, "Source harus staging atau display", nil)
		return
	}

	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	offset := (page - 1) * perPage

	db := config.DB

	// 🔹 Subquery ambil latest id per barcode
	latestSubQuery := db.
		Table("rack_histories").
		Select("MAX(id) as id").
		Group("barcode")

	type Result struct {
		RackID        uint
		RackName      string
		UserID        uint
		UserName      string
		TotalInserted int
	}

	query := db.
		Table("rack_histories rh").
		Select(`
			rh.rack_id,
			COALESCE(r.name, 'Rak Deleted') as rack_name,
			rh.user_id,
			COALESCE(u.name, 'User Deleted') as user_name,
			COUNT(*) as total_inserted
		`).
		Joins("JOIN (?) latest ON latest.id = rh.id", latestSubQuery).
		Joins("LEFT JOIN racks r ON r.id = rh.rack_id").
		Joins("LEFT JOIN users u ON u.id = rh.user_id").
		Where("rh.action = ?", "IN").
		Where("r.source = ?", source).
		Group("rh.rack_id, r.name, rh.user_id, u.name")

	if search != "" {
		query = query.Where(`
			r.name LIKE ? OR u.name LIKE ?
		`, "%"+search+"%", "%"+search+"%")
	}

	var total int64
	query.Count(&total) // total group rows

	var results []Result
	err := query.
		Limit(perPage).
		Offset(offset).
		Scan(&results).Error

	if err != nil {
		helpers.ErrorResponse(c, 500, "Internal server error", err)
		return
	}

	// 🔹 Format grouping by rack
	rackMap := make(map[uint]gin.H)
	totalAllUsers := 0

	for _, row := range results {

		totalAllUsers += row.TotalInserted

		if _, exists := rackMap[row.RackID]; !exists {
			rackMap[row.RackID] = gin.H{
				"rack_id":       row.RackID,
				"rack_name":     row.RackName,
				"total_in_rack": 0,
				"users":         []gin.H{},
			}
		}

		rackData := rackMap[row.RackID]
		rackData["total_in_rack"] = rackData["total_in_rack"].(int) + row.TotalInserted

		rackData["users"] = append(
			rackData["users"].([]gin.H),
			gin.H{
				"user_id":        row.UserID,
				"user_name":      row.UserName,
				"total_inserted": row.TotalInserted,
			},
		)

		rackMap[row.RackID] = rackData
	}

	finalData := []gin.H{}
	for _, v := range rackMap {
		finalData = append(finalData, v)
	}

	// pagination links
	lastPage := int(math.Ceil(float64(total) / float64(perPage)))
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "Statistik keseluruhan produk masuk di Rak " + source,
		"data": gin.H{
			"source":          source,
			"total_all_users": totalAllUsers,
			"data":         finalData,
			"pagination": gin.H{
				"current_page": page,
				"per_page": perPage,
				"total":    total,
				"links": links,
				"from": offset + 1,
				"to": offset + len(finalData),
			},
		},
	})
}
func ExportRackHistoryInsertation(c *gin.Context) {
	source := c.Query("source")
	date := c.DefaultQuery("date", time.Now().Format("2006-01-02"))

	if source != "staging" && source != "display" {
		helpers.ErrorResponse(c, 422, "Source harus staging atau display", nil)
		return
	}

	db := config.DB

	parsedDate, errParse := helpers.ParseFlexibleDate(date) 
	if errParse != nil {
		helpers.ErrorResponse(c, 400, "Gagal Parsed Date", errParse)
		return
	}
	start := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, time.Local)
	end := start.Add(24*time.Hour).Add(-time.Nanosecond)

	// Subquery untuk ambil ID terakhir per barcode di tanggal tersebut
	latestSubQuery := db.
		Table("rack_histories").
		Select("MAX(id) as id").
		Group("barcode")

	type Result struct {
		RackID        uint
		RackName      string
		UserID        uint
		UserName      string
		ProductName 	string
		Barcode		string
		CreatedAt		time.Time
	}

	var histories []Result
	err := db.
		Table("rack_histories rh").
		Select(`
			rh.rack_id,
			COALESCE(r.name, 'Rak Deleted') as rack_name,
			rh.user_id,
			COALESCE(u.name, 'User Deleted') as user_name,
			rh.product_name,
			rh.barcode,
			rh.created_at
		`).
		Joins("JOIN (?) latest ON latest.id = rh.id", latestSubQuery).
		Joins("LEFT JOIN racks r ON r.id = rh.rack_id").
		Joins("LEFT JOIN users u ON u.id = rh.user_id").
		Where("rh.action = ?", "IN").
		Where("r.source = ?", source).
		Where("rh.created_at BETWEEN ? AND ?", start, end).
		Order("rh.rack_id ASC").
		Order("rh.created_at DESC").
		Scan(&histories).Error

	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mengambil data histories", err)
		return
	}

	// Generate Excel
	file := excelize.NewFile()
	sheet := "Sheet1"
	file.SetSheetName("Sheet1", sheet)

	// Style Config 
	var BorderStyle = excelize.Style{
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
	}
	var FillGrayStyle = excelize.Style{
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"e5e7eb"},
			Pattern: 1,
		},
	}
	var BoldStyle = excelize.Style{Font: &excelize.Font{Bold: true}}

	border, _ := helpers.BuildStyle(file, BorderStyle)
	headerStyle, _ := helpers.BuildStyle(file, BorderStyle, BoldStyle, FillGrayStyle)
	file.SetCellStyle(sheet, "A1", "F1", headerStyle)

	// Header
	file.SetColWidth(sheet, "A", "A", 3)
	file.SetColWidth(sheet, "B", "B", 25)
	file.SetColWidth(sheet, "C", "E", 15)
	file.SetColWidth(sheet, "F", "F", 65)
	headers := []string{
		"No",
		"Tanggal & Waktu Masuk",
		"Nama Rak",
		"Operator (User)",
		"Barcode",
		"Nama Produk",
	}

	for i, h := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		file.SetCellValue(sheet, cell, h)
	}

	// Data
	baris := 2
	for i, row := range histories {
		file.SetCellValue(sheet, fmt.Sprintf("A%d", baris), i+1)
		file.SetCellValue(sheet, fmt.Sprintf("B%d", baris), row.CreatedAt.Format("2006-01-02 15:04:05"))
		file.SetCellValue(sheet, fmt.Sprintf("C%d", baris), row.RackName)
		file.SetCellValue(sheet, fmt.Sprintf("D%d", baris), row.UserName)
		file.SetCellValue(sheet, fmt.Sprintf("E%d", baris), row.Barcode)
		file.SetCellValue(sheet, fmt.Sprintf("F%d", baris), row.ProductName)

		baris++
	}
	file.SetCellStyle(sheet, fmt.Sprintf("A%d", 2), fmt.Sprintf("F%d", baris-1), border)

	fileName := fmt.Sprintf("DETAIL_RAK_%s_%s.xlsx", strings.ToUpper(source), date)

	// Save file
	dir := "./public/exports"
	os.MkdirAll(dir, 0755)

	fullPath := filepath.Join(dir, fileName)
	if err := file.SaveAs(fullPath); err != nil {
		helpers.ErrorResponse(c, 500, "Internal Server Erorr", err)
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/%s", os.Getenv("APP_URL"), fileName)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File berhasil diunduh",
		"url":     downloadURL,
	})
}
func ExportDataRack(c *gin.Context) {
	source := c.Query("source")
	// date := c.DefaultQuery("date", time.Now().Format("2006-01-02"))

	if source != "staging" && source != "display" {
		helpers.ErrorResponse(c, 422, "Source harus staging atau display", nil)
		return
	}

	db := config.DB

	parsedDate := helpers.GetCurentTime()
	start := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, time.Local)
	end := start.Add(24*time.Hour).Add(-time.Nanosecond)

	// type rackData struct {
	// 	NameRack	string
	// 	Barcode		string
	// 	Category		string
	// 	Status		string
	// 	Source		string
	// 	TotalData		int64
	// 	TotalOldPrice		float64
	// 	TotalNewPrice		float64
	// 	CreatedAt			time.Time
	// 	SoAt			time.Time
	// 	ToDisplayAt			time.Time
	// 	UserSo			string
	// 	UserDisplay			string
	// }
	var histories []models.Rack
	err := db.Model(&models.Rack{}).Preload("UserSO").Preload("UserDisplay").
		Where("source = ?", source).
		Where("created_at BETWEEN ? AND ?", start, end).
		Order("created_at DESC").
		Find(&histories).Error

	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mengambil data rack", err)
		return
	}

	// Generate Excel
	file := excelize.NewFile()
	sheet := "Sheet1"
	file.SetSheetName("Sheet1", sheet)

	// Style Config 
	headerStyle, _ := helpers.BuildStyle(
		file, 
		config.ExcelStyles["font_bold"],
		config.ExcelStyles["fill_gray"],
	)
	file.SetCellStyle(sheet, "A1", "M1", headerStyle)

	// Header
	file.SetColWidth(sheet, "A", "A", 40)
	file.SetColWidth(sheet, "B", "B", 16)
	file.SetColWidth(sheet, "C", "C", 35)
	file.SetColWidth(sheet, "D", "D", 18)
	file.SetColWidth(sheet, "E", "E", 10)
	file.SetColWidth(sheet, "F", "M", 15)
	headers := []string{
		"Nama Rak",
		"Barcode",
		"Kategori",
		"Status",
		"Source",
		"Total Data",
		"Total Old Price",
		"Total New Price",
		"Waktu Buat Rak",
		"Waktu SO",
		"Waktu To Display",
		"User SO",
		"User Display",
	}

	for i, h := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		file.SetCellValue(sheet, cell, h)
	}

	// Data
	baris := 2
	for _, row := range histories {
		status := row.Source
		if row.Source == "staging" && row.MoveToDisplayAt != nil {
			status = "To Display"
		}
		so_at := "-"
		if row.SoAt != nil {
			so_at = row.SoAt.Format("2006-01-02 15:04")
		}
		move_at := "-"
		if row.MoveToDisplayAt != nil {
			move_at = row.MoveToDisplayAt.Format("2006-01-02 15:04")
		}
		user_so := "-"
		if row.UserSO != nil {
			user_so = row.UserSO.Name
		}
		user_display := "-"
		if row.UserDisplay != nil {
			user_display = row.UserDisplay.Name
		}

		file.SetCellValue(sheet, fmt.Sprintf("A%d", baris), row.Name)
		file.SetCellValue(sheet, fmt.Sprintf("B%d", baris), row.Barcode)
		file.SetCellValue(sheet, fmt.Sprintf("C%d", baris), helpers.NormalizeRackName(row.Name))
		file.SetCellValue(sheet, fmt.Sprintf("D%d", baris), status)
		file.SetCellValue(sheet, fmt.Sprintf("E%d", baris), row.Source)
		file.SetCellValue(sheet, fmt.Sprintf("F%d", baris), row.TotalData)
		file.SetCellValue(sheet, fmt.Sprintf("G%d", baris), row.TotalOldPriceProduct)
		file.SetCellValue(sheet, fmt.Sprintf("H%d", baris), row.TotalNewPriceProduct)
		file.SetCellValue(sheet, fmt.Sprintf("I%d", baris), row.CreatedAt.Format("2006-01-02 15:04"))
		file.SetCellValue(sheet, fmt.Sprintf("J%d", baris), so_at)
		file.SetCellValue(sheet, fmt.Sprintf("K%d", baris), move_at)
		file.SetCellValue(sheet, fmt.Sprintf("L%d", baris), user_so)
		file.SetCellValue(sheet, fmt.Sprintf("M%d", baris), user_display)

		baris++
	}
	// file.SetCellStyle(sheet, fmt.Sprintf("A%d", 2), fmt.Sprintf("F%d", baris-1), border)

	fileName := fmt.Sprintf("DATA_RAK_%s_%s.xlsx", strings.ToUpper(source), parsedDate.Format("2006-01-02"))

	// Save file
	dir := "./public/exports"
	os.MkdirAll(dir, 0755)

	fullPath := filepath.Join(dir, fileName)
	if err := file.SaveAs(fullPath); err != nil {
		helpers.ErrorResponse(c, 500, "Internal Server Erorr", err)
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/%s", os.Getenv("APP_URL"), fileName)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File berhasil diunduh",
		"url":     downloadURL,
	})
}