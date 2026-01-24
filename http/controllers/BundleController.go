package controllers

import (
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"runtime/debug"

	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

// ============================= INVENTORY PRODUCT =============================
//Moving Product -> bundle
func GetBundles(c *gin.Context) {
    q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	//inisialisasi query
	baseQuery := config.DB.Model(&models.Bundle{}).
        Where("bundle_type = ?", "bundle").
        Where("(warehouse_type IS NULL OR warehouse_type = 'type1')")

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(name_bundle LIKE ?)", searchPattern)
	}

    var bundles []models.Bundle
	var totalData int64

    baseQuery.Session(&gorm.Session{}).Count(&totalData)

    // Ambil data detail
    err := baseQuery.Session(&gorm.Session{}).
        Order("created_at DESC").
        Limit(limit).Offset(offset).
        Find(&bundles).Error

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
			"message": "List bundle",
			"resource": gin.H{
				"current_page": 	page,
                "total_data":           totalData,
                "data":                 bundles,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + len(bundles),
			},
		},
	})
}

func GetProductTypeColor(c *gin.Context) {
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
        Joins("LEFT JOIN product_olds ON product_olds.id = products.product_old_id").
        Where("products.status IN ?", []string{"display", "expired"}).
        Where("products.location_type = ?", "main").
        Where("products.category_id IS NULL").
        Where("products.tag_color_id IS NOT NULL").
        Where("products.quality = ?", "lolos").
        Where("(products.warehouse_type IS NULL OR products.warehouse_type = 'type1')")

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(products.barcode LIKE ? OR "+
            "product_olds.old_barcode_product LIKE ? OR " + 
            "products.name LIKE ? OR " + 
            "products.code_document LIKE ?)", searchPattern, searchPattern, searchPattern, searchPattern)
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
            product_olds.old_barcode_product AS old_barcode, 
            products.barcode AS new_barcode, 
            products.name AS name, 
            products.price AS price, 
            product_olds.old_price_product AS old_price, 
            products.status AS status, 
            color_tags.name_color AS category
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
			"message": "List Product Display & Expired",
			"resource": gin.H{
                "total_data":           totalData,
                "data":                 products,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + len(products),
				"total":          totalData,
			},
		},
	})
}

func GetBundleDetail(c *gin.Context) {
    bundle_id := c.Param("bundle_id")

	var bundle models.Bundle
	if err := config.DB.First(&bundle, bundle_id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "Data bundle tidak ditemukan",
		})
		return
	}

	type productBundle struct {
		ID string `json:"id"`
		CodeDocument string `json:"code_document"`
		NewBarcode string `json:"new_barcode"`
		NameProduct string `json:"name_product"`
		Price string `json:"price"`
		Status string `json:"status"`
		Category string `json:"category"`
	}

	var bundle_item []productBundle
	if err := config.DB.Table("bundle_items").
		Select(`
			bundle_items.id AS id,
			products.code_document AS code_document,
			products.barcode AS new_barcode,
			products.name AS name_product,
			products.price AS price,
			products.status AS status,
			COALESCE(color_tags.name_color, categories.name_category) AS category
		`).
		Joins("LEFT JOIN products ON products.id = bundle_items.product_id").
		Joins("LEFT JOIN categories ON categories.id = products.category_id").
		Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
		Where("bundle_items.bundle_id = ?", bundle.ID).
		Scan(&bundle_item).Error; err != nil {
		c.JSON(500, gin.H{
			"status":  false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Data Bundle",
			"resource": gin.H{
                "data_bundle": bundle,
				"product_bundles": bundle_item,
			},
		},
	})
}

func GetBundleFilterProduct(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	// =========================
	// Hitung total harga
	// =========================
	var totalPrice float64
	if err := config.DB.
		Table("bundle_items").
		Joins("JOIN products ON products.id = bundle_items.product_id").
		Joins("JOIN product_olds po ON po.id = products.product_old_id").
		Where("bundle_items.user_id = ?", user.ID).
		Where("bundle_items.bundle_id IS NULL").
		Where("bundle_items.bundle_stage = ?", "bundle_filter").
		Select("COALESCE(SUM(po.old_price_product), 0)").
		Scan(&totalPrice).Error; err != nil {

		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	// =========================
	// Variabel response
	// =========================
	var (
		colorTag      []models.ColorTag
		categories []models.Category
	)

	// =========================
	// Logic harga
	// =========================
	if totalPrice > 99999 {
		// Ambil semua category
		if err := config.DB.Find(&categories).Error; err != nil {
			c.JSON(500, gin.H{"success": false, "error": err.Error()})
			return
		}
	} else {
		// Cari color tag berdasarkan range harga
		// var colorTag []models.ColorTag
		if err := config.DB.
			Where("min_price_color <= ?", totalPrice).
			Where("max_price_color >= ?", totalPrice).
			Find(&colorTag).Error; err != nil {

			c.JSON(500, gin.H{"success": false, "error": err.Error()})
			return
		}
	}

	// =========================
	// Data product (join product)
	// =========================
	type productData struct {
		ID          string  `json:"id"`
		NewBarcode  string  `json:"new_barcode"`
		ProductName string  `json:"product_name"`
		OldPrice    float64 `json:"old_price"`
	}

	var result []productData
	if err := config.DB.
		Table("bundle_items").
		Joins("LEFT JOIN products ON products.id = bundle_items.product_id").
		Joins("LEFT JOIN product_olds po ON po.id = products.product_old_id").
		Select(`
			bundle_items.id,
			products.barcode AS new_barcode,
			products.name AS product_name,
			po.old_price_product AS old_price
		`).
		Where("bundle_items.user_id = ?", user.ID).
		Where("bundle_items.bundle_id IS NULL").
		Where("bundle_items.bundle_stage = ?", "bundle_filter").
		Order("bundle_items.created_at DESC").
		Scan(&result).Error; err != nil {
		c.JSON(500, gin.H{"status": false, "error": err.Error()})
		return
	}

	// =========================
	// Response
	// =========================
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "list product filter bundle",
		"resource": gin.H{
			"total_new_price": totalPrice,
			"color_tag":           colorTag,
			"category":        categories,
			"data":            result,
		},
	})
}

func AddProductBundle(c *gin.Context) {
	bundle_id := c.Param("bundle_id")
	product_id := c.Param("product_id")

	bundleID, _ := strconv.ParseUint(bundle_id, 10, 64)
	productID, _ := strconv.ParseUint(product_id, 10, 64)
	user := c.MustGet("auth_user").(models.User)

	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}

	defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error"})
        }
    }()

	// get data bundle
    var bundle models.Bundle
    if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&bundle, bundleID).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Bundle not found"})
        return
    }

	//cek apakah produk sudah ada
	var exist models.BundleItem
    if err := tx.Where("bundle_id = ? AND product_id = ?", bundleID, productID).First(&exist).Error; err == nil {
        tx.Rollback()
        c.JSON(400, gin.H{"status": false, "message": "Product already in this bundle"})
        return
    }

	//get data product
	var product models.Product
    if err := tx.Preload("ProductOld").
		Where("status IN ?", []string{"display", "expired"}).
		Where("category_id IS NULL").
		Where("tag_color_id IS NOT NULL").
		Where("location_type = ?", "main").
		First(&product, productID).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Product not found"})
        return
    }

	//create bundle item
	bundle_item := models.BundleItem{
		BundleID:    &bundleID,
		ProductID:    productID,
		Status: product.Status,
	}

	if err := tx.Create(&bundle_item).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to create bundle item",
			"error":   err.Error(),
		})
		return
	}

	//update bundle
	var newTotalPrice float64
	switch bundle.BundleType {
	case "bundle":
		newTotalPrice = bundle.TotalPrice + product.ProductOld.OldPriceProduct
	case "repair":
		newTotalPrice = bundle.TotalPrice + product.Price
	} 

	totalProduct := bundle.TotalProduct + 1
	if err := tx.Model(&bundle).Updates(map[string]interface{}{
		"total_price": newTotalPrice,
		"total_price_custom": newTotalPrice,
		"total_product": totalProduct,
		"category_id": nil,
		"tag_color_id": nil,
		"status": "draft",
	}).Error; err != nil {

		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to update bundle",
			"error":   err.Error(),
		})
		return
	}

	//update product status
	if err := tx.Model(&product).Update("status", bundle.BundleType).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to update status product",
			"error":   err.Error(),
		})
		return
	}

	//buat user log action
	action := fmt.Sprintf("Menambah product (%s) pada bundle %s (%s)", product.Barcode, bundle.NameBundle, bundle.Barcode)
	metadata := map[string]interface{}{}
	if err := helpers.LogUserAction(user.ID, user.Name, action, "/moving-product/bundle/detail", metadata); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status":false,"message":"gagal membuat log user action","error":err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "berhasil menambah list product bundle",
	})
}

func DeleteProductBundle(c *gin.Context) {
	itemId := c.Param("item_id")

	itemID, _ := strconv.ParseUint(itemId, 10, 64)
	user := c.MustGet("auth_user").(models.User)

	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}

	defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error"})
        }
    }()

	//cari bundle item terkait
	var item models.BundleItem
    if err := tx.First(&item, itemID).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Item tidak ditemukan dalam bundle ini"})
        return
    }

	// get data bundle
    var bundle models.Bundle
    if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&bundle, item.BundleID).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Bundle not found"})
        return
    }

	//get data product
	var product models.Product
    if err := tx.Preload("ProductOld").First(&product, item.ProductID).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Product not found"})
        return
    }

	//hapus bundle item
	if err := tx.Delete(&item).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal menghapus item dari bundle"})
        return
    }

	//update bundle
	var newTotalPrice float64
	oldPrice := float64(0)
	if product.ProductOld != nil {
		oldPrice = product.ProductOld.OldPriceProduct
	}
	
	newTotalPrice = bundle.TotalPrice - oldPrice	
	totalProduct := bundle.TotalProduct - 1

	if newTotalPrice < 0 { newTotalPrice = 0 }
	if totalProduct < 0 { totalProduct = 0 }

	if totalProduct <= 0 {
		// HAPUS BUNDLE JIKA SUDAH KOSONG
		if err := tx.Delete(&bundle).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"status": false,
				"message": "Gagal menghapus bundle kosong",
				"error": err.Error(),
			})
			return
		}
	} else {
		if err := tx.Model(&bundle).Updates(map[string]interface{}{
			"total_price": newTotalPrice,
			"total_price_custom": newTotalPrice,
			"total_product": totalProduct,
			"category_id": nil,
			"tag_color_id": nil,
			"status": "draft",
		}).Error; err != nil {

			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "failed to update bundle",
				"error":   err.Error(),
			})
			return
		}
	}

	//update product status jdi bundle
	if err := tx.Model(&product).Update("status", item.Status).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to update status product",
			"error":   err.Error(),
		})
		return
	}

	//buat user log action
	action := fmt.Sprintf("Menghapus product (%s) dari bundle %s (%s)", product.Barcode, bundle.NameBundle, bundle.Barcode)
	metadata := map[string]interface{}{}
	if err := helpers.LogUserAction(user.ID, user.Name, action, "/moving-product/bundle/detail", metadata); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status":false,"message":"gagal membuat log user action","error":err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "berhasil menghapus product dari bundle",
	})
}

func CreateBundleProduct(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

    type payloadRequest struct {
        NameBundle string  `json:"name_bundle" binding:"required"`
        BundleType string  `json:"bundle_type" binding:"required,oneof=bundle"`
        CategoryID *uint64 `json:"category_id"`
        TagColorID *uint64 `json:"tag_color_id"`
        CustomPrice *float64 `json:"custom_price"`
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
				case "namebundle":
					if e.Tag() == "required" {
						errors["name_bundle"] = "Nama bundle wajib diisi"
					}
				case "bundletype":
					if e.Tag() == "required" {
						errors["bundle_type"] = "Bundle type wajib diisi"
					}else if e.Tag() == "oneof" {
						errors["bundle_type"] = "Bundle type harus bundle"
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

	var item_filter, action, info_page string
	switch payload.BundleType {
	case "bundle":
		item_filter = "bundle_filter"
		action = fmt.Sprintf("Create bundle %s", payload.NameBundle)
		info_page = "/moving-product/bundle"
	case "repair":
		item_filter = "repair_filter"
		action = fmt.Sprintf("Create repair bundle %s", payload.NameBundle)
		info_page = "/moving-product/repair"
	case "qcd":
		item_filter = "qcd_filter"
		action = fmt.Sprintf("Create qcd bundle %s", payload.NameBundle)
		info_page = "/moving-product/qcd"
	}

    var bundleItems []models.BundleItem
    err := config.DB.Preload("Product.ProductOld").
        Where("user_id = ? AND bundle_stage = ?", user.ID, item_filter).
        Find(&bundleItems).Error

    if err != nil {
        c.JSON(500, gin.H{"status": false, "message": "Gagal mengambil data item", "error": err.Error()})
        return      
    }

    if len(bundleItems) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"status": false, "message": "Tidak ada produk dalam antrian filter"})
        return
    }

	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error"})
        }
    }()

	// Hitung Total Price
    var totalPrice float64
    var totalPriceCustom float64
	var itemIDs []uint64
    for _, item := range bundleItems {
        // Pastikan relasi tidak nil untuk menghindari panic
		switch payload.BundleType {
		case "bundle":
			if item.Product != nil && item.Product.ProductOld != nil {
				totalPrice += item.Product.ProductOld.OldPriceProduct
			}
		case "repair":
			totalPrice += item.Product.Price
			totalPriceCustom += item.Product.ProductOld.OldPriceProduct
		}

		itemIDs = append(itemIDs, item.ID)
		// productIDs = append(productIDs, item.ProductID)
    }

    // proses pembuatan Bundle (Header)
	userIDValue := uint64(user.ID)
	bundle := models.Bundle{
		UserID: &userIDValue,
		NameBundle: payload.NameBundle,
		TotalPrice: totalPrice,
		TotalPriceCustom: totalPriceCustom,
		TotalProduct: int64(len(bundleItems)),
		BundleType: payload.BundleType,
		Status: "not_sale",
	}

	if payload.BundleType == "bundle" {		
		if totalPrice >= 100000 {
			bundle.ColorTag = nil
			if payload.CategoryID == nil {
				tx.Rollback()
				c.JSON(400, gin.H{"status": false, "message":"Total price > 100k, wajib mengisi category id"})
				return
			}

			var category models.Category
			if err := tx.Where("id = ?", payload.CategoryID).First(&category).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					c.JSON(404, gin.H{"status": false, "message":"category not found", "error": err.Error()})
				} else {
					c.JSON(500, gin.H{"status": false, "message":"failed to query Category", "error": err.Error()})
				}
	
				tx.Rollback()
				return
			}
	
			discount := totalPrice * (float64(category.DiscountCategory)/100.0)
			discount = math.Round(discount)
			if discount > category.MaxPriceCategory {
				discount = category.MaxPriceCategory
			} 
			bundle.TotalPriceCustom = totalPrice - discount
			bundle.CategoryID = payload.CategoryID
		} else {
			bundle.CategoryID = nil
			if payload.TagColorID == nil {
				tx.Rollback()
				c.JSON(400, gin.H{"status": false, "message":"Total price kurang dari 100k, wajib mengisi tag color id"})
				return
			}

			var color_tag models.ColorTag
			if err := tx.Where("id = ?", payload.TagColorID).First(&color_tag).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					c.JSON(404, gin.H{"status": false, "message":"color tag not found", "error": err.Error()})
				} else {
					c.JSON(500, gin.H{"status": false, "message":"failed to query Color Tag", "error": err.Error()})
				}
	
				tx.Rollback()
				return
			}
	
			if totalPrice < color_tag.MinPriceColor || totalPrice > color_tag.MaxPriceColor {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{"status": false, "message": "total price bundle tidak masuk rentang color tag"})
				return
			}
	
			bundle.TotalPriceCustom = color_tag.FixedPriceColor
			bundle.TagColorID = payload.TagColorID
		}
	}

	barcodeBundle := ""
	switch payload.BundleType {
	case "bundle":
		barcodeBundle, err = helpers.GenerateBarcodeBundle(config.DB)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"status" : false, "message": "failed to generate barcode", "error": err.Error()})
			return
		}
	case "repair":
		barcodeBundle, err = helpers.GenerateBarcodeBundleRepair(config.DB)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"status" : false, "message": "failed to generate barcode", "error": err.Error()})
			return
		}
	}

	bundle.Barcode = barcodeBundle

	if err := tx.Create(&bundle).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "error": err.Error()})
		return
	}

	if err := tx.Model(&models.BundleItem{}).
		Where("id IN ?", itemIDs). 
		Updates(map[string]interface{}{
			"bundle_id":    bundle.ID,
			"bundle_stage": nil,
			"user_id": nil,
		}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "message": "failed to update bundle items", "error": err.Error()})
		return
	}
	
	metadata := map[string]interface{}{}
	if err:=helpers.LogUserAction(user.ID, user.Name, action, info_page, metadata); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status":false,"message":"gagal membuat log user action","error":err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "status":      true,
        "message":     "Bundle Berhasil dibuat",
		"resource" : gin.H{
			"total_price": totalPrice,
			"item_count":  len(bundleItems),
			"data": bundle,
		},
    })
}

func UpdateBundle(c *gin.Context) {
	bundle_id := c.Param("bundle_id")
	user := c.MustGet("auth_user").(models.User)

    type payloadRequest struct {
        NameBundle string  `json:"name_bundle" binding:"required"`
        BundleType string  `json:"bundle_type" binding:"required,oneof=bundle repair qcd"`
        CategoryID *uint64 `json:"category_id"`
        TagColorID *uint64 `json:"tag_color_id"`
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
				case "namebundle":
					if e.Tag() == "required" {
						errors["name_bundle"] = "Nama bundle wajib diisi"
					}
				case "bundletype":
					if e.Tag() == "required" {
						errors["bundle_type"] = "Bundle type wajib diisi"
					}else if e.Tag() == "oneof" {
						errors["bundle_type"] = "Bundle type harus salah satu dari: bundle, repair, atau qcd"
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

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error"})
        }
    }()

	//get data bundle
	var bundle models.Bundle
    if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&bundle, bundle_id).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Data bundle tidak ditemukan"})
        return
    }

	var action, info_page string
	switch payload.BundleType {
	case "bundle":
		action = fmt.Sprintf("Update data bundle -> barcode: %s", bundle.Barcode)
		info_page = "/moving-product/bundle"
	case "repair":
		action = fmt.Sprintf("Update data repair bundle -> barcode: %s", bundle.Barcode)
		info_page = "/moving-product/repair"
	case "qcd":
		action = fmt.Sprintf("Update data qcd bundle -> barcode: %s", bundle.Barcode)
		info_page = "/moving-product/qcd"
	}

	logDetails := map[string]interface{}{
        "changes": map[string]interface{}{
            "bundle_name":        payload.NameBundle,
            "total_price":     bundle.TotalPrice,
            "total_price_custom":     bundle.TotalPriceCustom,
			"category_id":	bundle.CategoryID,
			"tag_color_id":	bundle.TagColorID,
        },
        "Before Edit : ": map[string]interface{}{
            "bundle_name":        bundle.NameBundle,
            "total_price":     bundle.TotalPrice,
            "total_price_custom":     bundle.TotalPriceCustom,
			"category_id":	bundle.CategoryID,
			"tag_color_id":	bundle.TagColorID,
        },
    }

	updateData := map[string]interface{}{
		"name_bundle":   payload.NameBundle,
		"status":   "not_sale",
	}

	if payload.BundleType == "bundle" {		
		totalPrice := bundle.TotalPrice
		if totalPrice >= 100000 {
			if payload.CategoryID == nil {
				tx.Rollback()
				c.JSON(400, gin.H{"status": false, "message": "Total price bundle >= 100rb, wajib pilih kategori"})
				return
			}
	
			var category models.Category
			if err := tx.First(&category, payload.CategoryID).Error; err != nil {
				tx.Rollback()
				c.JSON(404, gin.H{"status": false, "message": "Category tidak ditemukan"})
				return
			}
	
			discount := math.Round(totalPrice * (float64(category.DiscountCategory) / 100.0))
			if discount > category.MaxPriceCategory {
				discount = category.MaxPriceCategory
			}
	
			updateData["total_price_custom"] = totalPrice - discount
			updateData["category_id"] = payload.CategoryID
			updateData["tag_color_id"] = nil // Force null
			logDetails["changes"].(map[string]interface{})["total_price_custom"] = totalPrice - discount
			logDetails["changes"].(map[string]interface{})["category_id"] = payload.CategoryID
			logDetails["changes"].(map[string]interface{})["tag_color_id"] = nil
		} else {
			if payload.TagColorID == nil {
				tx.Rollback()
				c.JSON(400, gin.H{"status": false, "message": "Total harga < 100rb, wajib pilih color tag"})
				return
			}
	
			var color_tag models.ColorTag
			if err := tx.First(&color_tag, payload.TagColorID).Error; err != nil {
				tx.Rollback()
				c.JSON(404, gin.H{"status": false, "message": "Color tag tidak ditemukan"})
				return
			}
	
			if totalPrice < color_tag.MinPriceColor || totalPrice > color_tag.MaxPriceColor {
				tx.Rollback()
				c.JSON(400, gin.H{"status": false, "message": "Harga tidak masuk rentang color tag"})
				return
			}
	
			updateData["total_price_custom"] = color_tag.FixedPriceColor
			updateData["tag_color_id"] = payload.TagColorID
			updateData["category_id"] = nil // Force null
			logDetails["changes"].(map[string]interface{})["total_price_custom"] = color_tag.FixedPriceColor
			logDetails["changes"].(map[string]interface{})["category_id"] = nil
			logDetails["changes"].(map[string]interface{})["tag_color_id"] = color_tag.ID
		}
	}

	//UPDATE BUNDLE
	if err := tx.Model(&bundle).Updates(updateData).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "error": err.Error()})
        return
    }
	//create log action
	if err:=helpers.LogUserAction(user.ID, user.Name, action, info_page, logDetails); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status":false,"message":"gagal membuat log user action","error":err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "status":      true,
        "message":     "Bundle Berhasil diupdate",
    })
}

func BundleAddFilterProduct(c *gin.Context) {
	id := c.Param("id")
	user := c.MustGet("auth_user").(models.User)

	type payloadRequest struct {
        BundleType string  `json:"bundle_type" binding:"required,oneof=bundle repair qcd"`
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
				case "bundletype":
					if e.Tag() == "required" {
						errors["bundle_type"] = "Bundle type wajib diisi"
					}else if e.Tag() == "oneof" {
						errors["bundle_type"] = "Bundle type harus salah satu dari: bundle, repair, atau qcd"
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

	productID, _ := strconv.ParseUint(id, 10, 64)
    var product models.Product
    if err := config.DB.Where("status IN ?", []string{"display", "expired"}).
		Where("location_type = ?", "main").
		Where("category_id IS NULL").
		Where("tag_color_id IS NOT NULL").
		First(&product, productID).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusForbidden, gin.H{"status": false, "message": "product tidak ditemukan"})
            return
        }else {
            c.JSON(500, gin.H{"status": false, "error": err.Error()})
            return
        }
    }

	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}

	defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
        }
    }()

	var existingItem models.BundleItem
    if err := tx.Where("product_id = ?", productID).First(&existingItem).Error; err == nil {
        tx.Rollback()
        c.JSON(http.StatusBadRequest, gin.H{"status": false, "message": "produk sudah ada dalam bundle / filter"})
        return
    }

	item_filter := ""
	switch payload.BundleType {
	case "bundle":
		item_filter = "bundle_filter"
	case "repair":
		item_filter = "repair_filter"
	case "qcd":
		item_filter = "qcd_filter"
	}


	bundle_item := models.BundleItem{
		UserID: 	&user.ID,
		ProductID:    productID,
		BundleStage: &item_filter,
		Status: product.Status,
	}

	if err := tx.Create(&bundle_item).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to create bundle item",
			"error":   err.Error(),
		})
		return
	}

	if err := tx.Model(&models.Product{}).Where("id = ?", productID).Update("status", payload.BundleType).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to update status product",
			"error":   err.Error(),
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "berhasil menambah list product bundle",
	})
}

func BundleDeleteFilterProduct(c *gin.Context) {
	id := c.Param("id")

	var bundle_item models.BundleItem
	if err := config.DB.First(&bundle_item, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "Data tidak ditemukan",
		})
		return
	}

	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}

	if err := tx.Model(&models.Product{}).Where("id = ?", bundle_item.ProductID).Update("status", bundle_item.Status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to update status product",
			"error":   err.Error(),
		})
		return
	}

	if err := tx.Delete(&bundle_item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "Gagal menghapus data dari filter",
		})
		return
	}


	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "berhasil hapus item dari filter",
	})
}

func Unbundle(c *gin.Context) {
	bundle_id := c.Param("bundle_id")
	user := c.MustGet("auth_user").(models.User)

	type payloadRequest struct {
        BundleType string  `json:"bundle_type" binding:"required,oneof=bundle repair qcd"`
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
				case "bundletype":
					if e.Tag() == "required" {
						errors["bundle_type"] = "Bundle type wajib diisi"
					}else if e.Tag() == "oneof" {
						errors["bundle_type"] = "Bundle type harus salah satu dari: bundle, repair, atau qcd"
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

	tx := config.DB.Begin()
    if tx.Error != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"message": "db transaction start error", "error":tx.Error.Error()})
        return
    }

    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
        }
    }()

	var bundle models.Bundle
	if err := tx.First(&bundle, bundle_id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "Data bundle tidak ditemukan",
		})
		return
	}

	var bundle_item []models.BundleItem
	if err := tx.Where("bundle_id = ?", bundle.ID).Find(&bundle_item).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "error":err.Error()})
		return
	}

	for _, item := range bundle_item {
		if err := tx.Model(&models.Product{}).
			Where("id = ?", item.ProductID).
			Update("status", item.Status).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"status": false, "error": "Gagal mengembalikan status produk"})
			return
		}
	}

	if err := tx.Where("bundle_id = ?", bundle_id).Delete(&models.BundleItem{}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "error": "Gagal menghapus item bundle"})
		return
	}

	if err := tx.Delete(&bundle).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "Gagal melakukan unbundle",
			"error":err.Error(),
		})
		return
	}

	action := fmt.Sprintf("%s melakukan unbundle -> %s", user.Username, bundle.NameBundle)
	var info_page string
	switch payload.BundleType {
	case "bundle":
		info_page = "/moving-product/bundle"
	case "repair":
		info_page = "/moving-product/repair"
	case "qcd":
		info_page = "/moving-product/qcd"
	}
	metadata := map[string]interface{}{}
	if err := helpers.LogUserAction(user.ID, user.Name, action, info_page, metadata); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status":false,"message":"gagal membuat log user action","error":err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit transaction"})
        return
    }

	c.JSON(http.StatusOK, gin.H{
        "status":      true,
        "message":     "Unbundle berhasil",
		"resource" : gin.H{
			"data": bundle,
		},
    })
}

//Moving Product -> repair
func GetRepairBundles(c *gin.Context) {
    q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	//inisialisasi query
	baseQuery := config.DB.Model(&models.Bundle{}).
        Where("bundle_type = ?", "repair").
        Where("(warehouse_type IS NULL OR warehouse_type = 'type1')")

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(name_bundle LIKE ?)", searchPattern)
	}

    var bundles []models.Bundle
	var totalData int64

    baseQuery.Session(&gorm.Session{}).Count(&totalData)

    // Ambil data detail
    err := baseQuery.Session(&gorm.Session{}).
        Order("created_at DESC").
        Limit(limit).Offset(offset).
        Find(&bundles).Error

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
			"message": "List repair bundle",
			"resource": gin.H{
                "total_data":           totalData,
                "data":                 bundles,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + len(bundles),
			},
		},
	})
}

func GetRepairFilterProduct(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
	db := config.DB

	// =====================
	// QUERY PARAM
	// =====================
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit := 100

	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	// =====================
	// BASE QUERY
	// =====================
	baseQuery := db.Table("bundle_items").
		Joins("JOIN products ON products.id = bundle_items.product_id").
		Joins("JOIN product_olds po ON po.id = products.product_old_id").
		Where("bundle_items.user_id = ?", user.ID).
		Where("bundle_items.bundle_id IS NULL").
		Where("bundle_items.bundle_stage = ?", "repair_filter")

	// =====================
	// TOTAL PRICE
	// =====================
	var totalEstimatedPrice float64
	if err := baseQuery.Session(&gorm.Session{}).
		Select("COALESCE(SUM(products.price), 0)").
		Scan(&totalEstimatedPrice).Error; err != nil {

		c.JSON(500, gin.H{
			"success": false,
			"message": "gagal menghitung total estimate price",
			"error":   err.Error(),
		})
		return
	}

	// =====================
	// TOTAL ITEMS (UNTUK PAGINATION)
	// =====================
	var totalItems int64
	if err := baseQuery.Session(&gorm.Session{}).
		Count(&totalItems).Error; err != nil {

		c.JSON(500, gin.H{
			"success": false,
			"message": "gagal menghitung total items",
			"error":   err.Error(),
		})
		return
	}

	// =====================
	// PAGINATED DATA
	// =====================
	type productData struct {
		ID          uint64  `json:"id"`
		NewBarcode  string  `json:"new_barcode"`
		ProductName string  `json:"product_name"`
		NewPrice    float64 `json:"new_price"`
		OldPrice    float64 `json:"old_price"`
	}

	var items []productData
	if err := baseQuery.Session(&gorm.Session{}).
		Select(`
			bundle_items.id AS id,
			products.barcode AS new_barcode,
			products.name AS product_name,
			products.price AS new_price,
			po.old_price_product AS old_price
		`).
		Order("bundle_items.created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&items).Error; err != nil {

		c.JSON(500, gin.H{
			"success": false,
			"message": "gagal mengambil data items",
			"error":   err.Error(),
		})
		return
	}

	lastPage := int(math.Ceil(float64(totalItems) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// =====================
	// RESPONSE
	// =====================
	c.JSON(200, gin.H{
		"success": true,
		"message": "List product di keranjang repair filter",
		"resource": gin.H{
			"total_estimated_price": totalEstimatedPrice,
			"total_items":           totalItems,
			"current_page":          page,
			"per_page":              limit,
			"links":				 links,	
			"data":                  items,
		},
	})
}

func UpdateRepairProduct(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
	id := c.Param("item_id")

	// =====================
	// REQUEST PAYLOAD
	// =====================
	type request struct {
		NewNameProduct     string   `json:"new_name_product" binding:"required"`
		NewQuantityProduct float64  `json:"new_quantity_product" binding:"required,numeric"`
		OldPriceProduct    float64  `json:"old_price_product" binding:"required,numeric"`
		CategoryID 		  *uint64  `json:"category_id"`
		TagColorID 		  *uint64  `json:"tag_color_id"`
	}

	var req request
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

			case "newnameproduct":
				if e.Tag() == "required" {
					errors["new_name_product"] = "Nama produk wajib diisi"
				}

			case "newquantityproduct":
				if e.Tag() == "required" {
					errors["new_quantity_product"] = "Jumlah produk wajib diisi"
				} else if e.Tag() == "numeric" || e.Tag() == "gt" {
					errors["new_quantity_product"] = "Jumlah produk harus berupa angka dan lebih dari 0"
				}

			case "oldpriceproduct":
				if e.Tag() == "required" {
					errors["old_price_product"] = "Harga lama wajib diisi"
				} else if e.Tag() == "numeric" || e.Tag() == "gt" {
					errors["old_price_product"] = "Harga lama harus berupa angka dan lebih dari 0"
				}

			case "newpriceproduct":
				if e.Tag() == "numeric" || e.Tag() == "gt" {
					errors["new_price_product"] = "Harga baru harus berupa angka dan lebih dari 0"
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

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
		// log ke file / stdout
        if r := recover(); r != nil {
			stack := debug.Stack() // ← full stack trace
			fmt.Printf("PANIC: %v\n%s\n", r, stack)
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error", "error": fmt.Sprintf("%v", r)})
        }
    }()

	var item models.BundleItem
	if err := tx.First(&item, id).Error; err != nil {
		c.JSON(404, gin.H{
			"success": false,
			"message": "Data item tidak ditemukan",
		})
		return
	}

	var product models.Product
	if err := tx.Preload("ProductOld").First(&product, item.ProductID).Error; err != nil {
		c.JSON(404, gin.H{
			"success": false,
			"message": "Data product tidak ditemukan",
		})
		return
	}
	// =====================
	// UPDATE DATA
	// =====================
	updateData := map[string]interface{}{
		"name":     req.NewNameProduct,
		"quantity": req.NewQuantityProduct,
	}

	if req.OldPriceProduct >= 100000 {
		if req.CategoryID == nil {
			c.JSON(400, gin.H{
				"success": false,
				"message": "category id wajib diisi untuk harga >= 100000",
			})
			tx.Rollback()
			return
		}

		var category models.Category
		if err := tx.Where("id = ?", req.CategoryID).First(&category).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(404, gin.H{"status": false, "message":"category tidak ditemukan", "error": err.Error()})
			} else {
				c.JSON(500, gin.H{"status": false, "message":"failed to query Category", "error": err.Error()})
			}

			tx.Rollback()
			return
		}

		discount := req.OldPriceProduct * (float64(category.DiscountCategory)/100.0)
		discount = math.Round(discount)
		if discount > category.MaxPriceCategory {
			discount = category.MaxPriceCategory
		} 
		updateData["price"] = req.OldPriceProduct - discount
		updateData["category_id"] = req.CategoryID
		updateData["tag_color_id"] = nil
	} else {
		if req.TagColorID == nil {
			c.JSON(400, gin.H{
				"success": false,
				"message": "tag_color_id wajib diisi untuk harga < 100000",
			})
			tx.Rollback()
			return
		}

		var color_tag models.ColorTag
		if err := tx.Where("id = ?", req.TagColorID).First(&color_tag).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(404, gin.H{"status": false, "message":"color tag not found", "error": err.Error()})
			} else {
				c.JSON(500, gin.H{"status": false, "message":"failed to query Color Tag", "error": err.Error()})
			}

			tx.Rollback()
			return
		}

		if req.OldPriceProduct < color_tag.MinPriceColor || req.OldPriceProduct > color_tag.MaxPriceColor {
			c.JSON(http.StatusBadRequest, gin.H{"status": false, "message": "harga produk tidak masuk rentang color tag ini"})
			return
		}

		updateData["price"] = color_tag.FixedPriceColor
		updateData["category_id"] = nil
		updateData["tag_color_id"] = color_tag.ID
	}

	logDetails := map[string]interface{}{
        "changes": map[string]interface{}{
            "new_name":         req.NewNameProduct,
            "new_quantity":     req.NewQuantityProduct,
            "new_price":        updateData["price"],
            "old_price":        req.OldPriceProduct,
        },
        "Before Edit : ": map[string]interface{}{
            "new_name":       product.Name,
            "new_quantity":   product.Quantity,
            "new_price":   product.Price,
            "old_price":      product.ProductOld.OldPriceProduct, // Harga lama di Staging
        },
    }
	action := fmt.Sprintf("Melakukan update data repair product %s (%s)", product.Name, product.Barcode)
	if err := helpers.LogUserAction(user.ID, user.Name, action, "moving-product/repair/detail/edit", logDetails); err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal membuat user log action!",
			"error":   err.Error(),
		})

		tx.Rollback()
		return
	}

	//update product
	if err := tx.Model(&product).Updates(updateData).Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal update data product!",
			"error":   err.Error(),
		})

		tx.Rollback()
		return
	}

	if err := tx.Model(&models.ProductOld{}).Where("id = ?", product.ProductOldID).
		Update("old_price_product", req.OldPriceProduct).Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal update data product old!",
			"error":   err.Error(),
		})

		tx.Rollback()
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal commit transaksi",
			"error": err.Error(),
		})
		return
	}
	// =====================
	// RESPONSE
	// =====================
	c.JSON(200, gin.H{
		"success": true,
		"message": "Data berhasil di ganti!",
		"data":    product,
	})
}

func ProductRepairToDisplay(c *gin.Context) {
	item_id := c.Param("item_id")
	user := c.MustGet("auth_user").(models.User)

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error"})
        }
    }()

	//cari bundle item terkait
	var item models.BundleItem
    if err := tx.Where("id = ?", item_id).First(&item).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Bundle item tidak ditemukan"})
        return
    }

	//cari bundle item terkait
	var product models.Product
    if err := tx.Preload("ProductOld").
		Where("id = ? AND status = ?", item.ProductID, "repair").First(&product).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Produk item tidak ditemukan"})
        return
    }

	//hapus bundle item
	if err := tx.Delete(&item).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal menghapus bundle item"})
        return
    }

	//get data bundle
	var bundle models.Bundle
    if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&bundle, item.BundleID).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Data bundle tidak ditemukan"})
        return
    }

	oldPrice := float64(0)
	if product.ProductOld != nil {
		oldPrice = product.ProductOld.OldPriceProduct
	}

	newTotalPrice := bundle.TotalPrice - product.Price
	newTotalPriceCustom := bundle.TotalPriceCustom - oldPrice
	totalProduct := bundle.TotalProduct - 1

	if newTotalPrice < 0 { newTotalPrice = 0 }
	if newTotalPriceCustom < 0 { newTotalPriceCustom = 0 }
	if totalProduct < 0 { totalProduct = 0 }

	if totalProduct <= 0 {
		// HAPUS BUNDLE JIKA SUDAH KOSONG
		if err := tx.Delete(&bundle).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"status": false,
				"message": "Gagal menghapus bundle kosong",
				"error": err.Error(),
			})
			return
		}
	} else {
		updateData := map[string]interface{}{
			"total_price":        newTotalPrice,
			"total_price_custom": newTotalPriceCustom,
			"total_product":      totalProduct,
		}

		if err := tx.Model(&bundle).Updates(updateData).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"status": false, "error": err.Error()})
			return
		}
	}

	//update status product
	if err := tx.Model(&product).Update("status", "display").Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal mengubah status product menjadi display"})
        return
    }
	
	//create log action
	action := fmt.Sprintf("Memindahkan repair product[%s] ke display | repar name: %s (%s)", product.Barcode, bundle.NameBundle, bundle.Barcode)
	metadata := map[string]interface{}{}
	if err:=helpers.LogUserAction(user.ID, user.Name, action, "/moving-product/repair/detail/to-display", metadata); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status":false,"message":"gagal membuat log user action","error":err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "status":      true,
        "message":     "Product Berhasil dipindah ke display",
    })
}

func DumpProductRepair(c *gin.Context) {
	item_id := c.Param("item_id")
	user := c.MustGet("auth_user").(models.User)

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error"})
        }
    }()

	//cari bundle item terkait
	var item models.BundleItem
    if err := tx.Where("id = ?", item_id).First(&item).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Bundle item tidak ditemukan"})
        return
    }

	//cari bundle item terkait
	var product models.Product
    if err := tx.Preload("ProductOld").
		Where("id = ? AND status != ?", item.ProductID, "dump").First(&product).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Produk item tidak ditemukan"})
        return
    }

	//get data bundle
	var bundle models.Bundle
    if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&bundle, item.BundleID).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Data bundle tidak ditemukan"})
        return
    }

	oldPrice := float64(0)
	if product.ProductOld != nil {
		oldPrice = product.ProductOld.OldPriceProduct
	}

	newTotalPrice := bundle.TotalPrice - product.Price
	newTotalPriceCustom := bundle.TotalPriceCustom - oldPrice
	totalProduct := bundle.TotalProduct - 1

	if newTotalPrice < 0 { newTotalPrice = 0 }
	if newTotalPriceCustom < 0 { newTotalPriceCustom = 0 }
	if totalProduct < 0 { totalProduct = 0 }

	if totalProduct <= 0 {
		// HAPUS BUNDLE JIKA SUDAH KOSONG
		if err := tx.Delete(&bundle).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{
				"status": false,
				"message": "Gagal menghapus bundle kosong",
				"error": err.Error(),
			})
			return
		}
	} else {
		updateData := map[string]interface{}{
			"total_price":        newTotalPrice,
			"total_price_custom": newTotalPriceCustom,
			"total_product":      totalProduct,
		}

		if err := tx.Model(&bundle).Updates(updateData).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"status": false, "error": err.Error()})
			return
		}
	}

	//hapus bundle item
	if err := tx.Delete(&item).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal menghapus bundle item"})
        return
    }

	//update status product
	if err := tx.Model(&product).Update("status", "dump").Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal mengubah status product menjadi dump"})
        return
    }
	
	//create log action
	action := fmt.Sprintf("Dump product[%s] repair | repar name: %s (%s)", product.Barcode, bundle.NameBundle, bundle.Barcode)
	metadata := map[string]interface{}{}
	if err:=helpers.LogUserAction(user.ID, user.Name, action, "/moving-product/repair/detail/qcd", metadata); err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status":false,"message":"gagal membuat log user action","error":err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "status":      true,
        "message":     "Product Berhasil didump",
    })
}
