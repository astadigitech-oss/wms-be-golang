package controllers

import (
	"errors"
	"fmt"
	"net/http"

	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"math"
	"regexp"
	"strings"

	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
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

	var rack models.Rack
	if err := config.DB.Preload("Products").First(&rack, "id = ?", rack_id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"success": false, "message": "Rack not found"})
		}else {
			c.JSON(500, gin.H{"success": false, "message": "Server Error", "error": err.Error()})
		}

		return
	}

	c.JSON(200, gin.H{
		"success":  true,
		"message": "Rack Detail",
		"resource": rack,
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
	baseQuery := config.DB.Model(&models.Product{}).
		Select(`
			products.id AS id,
			products.name AS product_name,
			products.barcode AS product_barcode,
			products.old_barcode_product AS product_old_barcode,
			categories.name_category AS category_name
		`).
		Joins("LEFT JOIN categories ON categories.id = products.category_id").
		Where("products.tag_color_id IS NULL").
		Where("products.category_id IS NOT NULL").
		Where("products.rack_id IS NULL").
		Where("products.status = ?", "display").
		Where("products.quality = ?", "lolos").
		Where("products.location_type = ?", &locationType)

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

		baseQuery = baseQuery.Where(config.DB.Where(
            strings.Join(conditions, " OR "),
            values...
		))
	}

	// Searching
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(products.name LIKE ? OR "+
            "products.barcode LIKE ? OR " + 
            "products.old_barcode_product LIKE ? OR " + 
			"categories.name_category LIKE ?)", searchPattern, searchPattern, searchPattern, searchPattern)
	}

    // Hitung total data
    var totalData int64
    baseQuery.Session(&gorm.Session{}).Count(&totalData)

	type productData struct {
		ID                uint   `json:"id"`
		ProductName       string `json:"product_name"`
		ProductBarcode    string `json:"product_barcode"`
		ProductOldBarcode string `json:"product_old_barcode"`
		CategoryName      string `json:"category_name"`
	}

	var products []productData
    // Ambil data detail
    err := baseQuery.Session(&gorm.Session{}).
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
			"message": "List product " + source,
			"resource": gin.H{
                "data":                 products,
                "current_page":           page,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + int(totalData),
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

	// Cari Produk & Join ke Category
	var product models.Product
	query := tx.Preload("Category")

	// Filter berdasarkan source (staging vs display)
	locationType := "main"
	if rack.Source == "staging" {
		locationType = "staging"
	}

	if err := query.Where("location_type = ?", locationType).
		Where("(barcode = ? OR old_barcode_product = ?)", barcode, barcode).
		First(&product).Error; err != nil {

		tx.Rollback()
		c.JSON(404, gin.H{"status": false, "message": "Produk tidak ditemukan di source " + rack.Source})
		return
	}

	// Validasi Kesesuaian Kategori (Logic Parsing Nama Rak)
	if rack.Name != "" && product.Category != nil {
		rackName := strings.ToUpper(strings.TrimSpace(rack.Name))
		prodCatName := strings.ToUpper(strings.TrimSpace(product.Category.NameCategory))

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
	if product.RackID != nil {
		var otherRack models.Rack
		config.DB.First(&otherRack, *product.RackID)
		tx.Rollback()
		c.JSON(422, gin.H{"status": false, "message": "Produk sudah berada di rak lain: " + otherRack.Name})
		return
	}

	// Update Rak ID Produk
	rackID64 := uint64(rack.ID)
	if err := tx.Model(&product).Update("rack_id", rackID64).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal update produk", "error": err.Error()})
		return
	}

	//update rack
	if err := tx.Model(&rack).Updates(map[string]interface{}{
		"total_data":                    gorm.Expr("total_data + ?", 1),
		"total_new_price_product":      gorm.Expr("total_new_price_product + ?", product.Price),
		"total_old_price_product":      gorm.Expr("total_old_price_product + ?", product.OldPriceProduct),
		"total_display_price_product":  gorm.Expr("total_display_price_product + ?", product.DisplayPrice),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal update statistik rak", "error": err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(200, gin.H{
		"status":  true,
		"message": "Berhasil menambahkan produk ke Rak " + rack.Name,
		"data":    product,
	})
}

func RemoveProductFromRack(c *gin.Context) {
	rackID, err := strconv.ParseUint(c.Param("rack_id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"message": "rack_id tidak valid"})
		return
	}

	productID, err := strconv.ParseUint(c.Param("product_id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"message": "product_id tidak valid"})
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

	// Cari Produk
	var product models.Product
	query := tx.Where("id = ?", productID).Where("rack_id = ?", rack.ID)

	// Filter berdasarkan source (staging vs display)
	locationType := "main"
	if rack.Source == "staging" {
		locationType = "staging"
	}

	if err := query.Where("location_type = ?", locationType).First(&product).Error; err != nil {
		tx.Rollback()
		c.JSON(404, gin.H{"status": false, "message": "Produk tidak ditemukan di rack ini: " + rack.Name})
		return
	}

	// Update Rak ID Produk
	if err := tx.Model(&product).Updates(map[string]interface{}{
        "rack_id": nil,
    }).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal update produk", "error": err.Error()})
        return
    }

	oldPrice := product.OldPriceProduct

	//update rack
	if err := tx.Model(&rack).Updates(map[string]interface{}{
		"total_data":                    gorm.Expr("total_data - ?", 1),
		"total_new_price_product":      gorm.Expr("total_new_price_product - ?", product.Price),
		"total_old_price_product":      gorm.Expr("total_old_price_product - ?", oldPrice),
		"total_display_price_product":  gorm.Expr("total_display_price_product - ?", product.DisplayPrice),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal update statistik rak", "error": err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
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

	// Update Rak ID Produk
	if err := tx.Model(&models.Product{}).Where("rack_id = ?", rack.ID).Updates(map[string]interface{}{
        "rack_id": nil,
    }).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal update produk", "error": err.Error()})
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
		tx.Rollback()
		c.JSON(422, gin.H{"status": false, "message": "Rak kosong"})
		return
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

	//update rack display
	if err := tx.Model(&parentRack).Updates(map[string]interface{}{
		"total_data": gorm.Expr("total_data + ?", rack.TotalData),
		"total_new_price_product": gorm.Expr(
			"total_new_price_product + ?", rack.TotalNewPriceProduct,
		),
		"total_old_price_product": gorm.Expr(
			"total_old_price_product + ?", rack.TotalOldPriceProduct,
		),
		"total_display_price_product": gorm.Expr(
			"total_display_price_product + ?", rack.TotalDisplayPriceProduct,
		),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal update statistik rak display", "error": err.Error()})
		return
	}

	//update rack staging
	if err := tx.Model(&rack).Updates(map[string]interface{}{
		"total_data": 0,
		"total_new_price_product": 0,
		"total_old_price_product": 0,
		"total_display_price_product": 0,
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "Gagal memindahkan rak", "error": err.Error()})
		return
	}

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