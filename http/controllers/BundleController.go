package controllers

import (
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"

	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"github.com/go-playground/validator/v10"
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

	baseURL := c.Request.Host + c.Request.URL.Path
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	fullURL := scheme + "://" + baseURL

	// pagination links
	links := []gin.H{
		{
			"url":    nil,
			"label":  "&laquo; Previous",
			"active": false,
		},
	}

	if lastPage <= 8 {
        // Jika total halaman 10 atau kurang, tampilkan semua
        for i := 1; i <= lastPage; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d&q=%s", fullURL, i, q),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }
    } else {
        for i := 1; i <= 8; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d&q=%s", fullURL, i, q),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }

        // Tambahkan separator "..."
        links = append(links, gin.H{
            "url":    nil,
            "label":  "...",
            "active": false,
        })

        for i := lastPage - 1; i <= lastPage; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d&q=%s", fullURL, i, q),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }
    }

	links = append(links, gin.H{
		"url":    nil,
		"label":  "Next &raquo;",
		"active": false,
	})

	var nextPageURL interface{} = nil
	var prevPageURL interface{} = nil

	if page < lastPage {
		nextPageURL = fmt.Sprintf("%s?page=%d&q=%s", fullURL, page+1, q)
	}
	if page > 1 {
		prevPageURL = fmt.Sprintf("%s?page=%d&q=%s", fullURL, page-1, q)
	}

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List bundle",
			"resource": gin.H{
                "total_data":           totalData,
                "data":                 bundles,
				"first_page_url": fmt.Sprintf("%s?page=1&q=%s", fullURL, q),
				"from":           offset + 1,
				"last_page":      lastPage,
				"last_page_url":  fmt.Sprintf("%s?page=%d&q=%s", fullURL, lastPage, q),
				"links":          links,
				"next_page_url":  nextPageURL,
				"path":           fullURL,
				"per_page":       limit,
				"prev_page_url":  prevPageURL,
				"to":             offset + len(bundles),
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
	type productData struct {
		ID string `json:"id"`
		NewBarcode string `json:"new_barcode"`
		ProductName string `json:"product_name"`
	}

	var result []productData
	if err := config.DB.Model(&models.BundleItem{}).
		Joins("LEFT JOIN products ON products.id = bundle_items.product_id").
		Select(`
			bundle_items.id AS id,
			products.name AS product_name,
			products.barcode AS new_barcode
		`).
		Where("(bundle_id IS NULL)").
		Where("bundle_stage = ?", "bundle_filter").
		Scan(&result).Error; err != nil {
		c.JSON(500, gin.H{"status": false, "error":err.Error()})
		return
	} 

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "list product filter bundle",
		"resource": gin.H{
			"data": result,
		},
	})

}

func AddProductBundle(c *gin.Context) {
	bundle_id := c.Param("bundle_id")
	product_id := c.Param("product_id")

	bundleID, _ := strconv.ParseUint(bundle_id, 10, 64)
	productID, _ := strconv.ParseUint(product_id, 10, 64)
	userID, _ := c.Get("user_id")

	//get data user
    var user models.User
    if err := config.DB.Where("id = ?", userID).First(&user).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusForbidden, gin.H{"status": false, "message": "user not found"})
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
	bundle_id := c.Param("bundle_id")
	product_id := c.Param("product_id")

	bundleID, _ := strconv.ParseUint(bundle_id, 10, 64)
	productID, _ := strconv.ParseUint(product_id, 10, 64)
	userID, _ := c.Get("user_id")

	//get data user
    var user models.User
    if err := config.DB.Where("id = ?", userID).First(&user).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusForbidden, gin.H{"status": false, "message": "user not found"})
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
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error"})
        }
    }()

	//cari bundle item terkait
	var item models.BundleItem
    if err := tx.Where("bundle_id = ? AND product_id = ?", bundleID, productID).First(&item).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Produk tidak ditemukan dalam bundle ini"})
        return
    }

	// get data bundle
    var bundle models.Bundle
    if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&bundle, bundleID).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Bundle not found"})
        return
    }

	//get data product
	var product models.Product
    if err := tx.Preload("ProductOld").First(&product, productID).Error; err != nil {
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
	switch bundle.BundleType {
	case "bundle":
		newTotalPrice = bundle.TotalPrice - product.ProductOld.OldPriceProduct
	case "repair":
		newTotalPrice = bundle.TotalPrice - product.Price
	} 

	totalProduct := bundle.TotalProduct - 1
	if newTotalPrice < 0 { newTotalPrice = 0 }
	if totalProduct < 0 { totalProduct = 0 }

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
	userID, _ := c.Get("user_id")
    var user models.User
    if err := config.DB.Preload("Role").Where("id = ?", userID).First(&user).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusForbidden, gin.H{"status": false, "message": "user not found"})
            return
        }else {
            c.JSON(500, gin.H{"status": false, "error": err.Error()})
            return
        }
    }

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
        Where("bundle_stage = ?", item_filter).
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
		TotalPriceCustom: totalPrice,
		TotalProduct: int64(len(bundleItems)),
		BundleType: payload.BundleType,
	}

	if payload.BundleType == "bundle" {		
		if totalPrice >= 100000 {
			bundle.ColorTag = nil
			var category models.Category
			if err := config.DB.Where("id = ?", payload.CategoryID).First(&category).Error; err != nil {
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
			var color_tag models.ColorTag
			if err := config.DB.Where("id = ?", payload.TagColorID).First(&color_tag).Error; err != nil {
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
				c.JSON(http.StatusBadRequest, gin.H{"status": false, "message": "harga produk tidak masuk rentang color tag"})
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
	userID, _ := c.Get("user_id")
    var user models.User
    if err := config.DB.Preload("Role").First(&user, userID).Error; err != nil {
        c.JSON(http.StatusForbidden, gin.H{"status": false, "message": "user not found"})
        return
    }

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
		"status":   "not sale",
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
    if err := config.DB.Where("status IN ?", []string{"display", "expired"}).First(&product, productID).Error; err != nil {
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
    if err := tx.Where("product_id = ? AND bundle_id IS NULL", productID).First(&existingItem).Error; err == nil {
        tx.Rollback()
        c.JSON(http.StatusBadRequest, gin.H{"status": false, "message": "produk sudah ada dalam antrian filter"})
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
	userID, _ := c.Get("user_id")
    var user models.User
    if err := config.DB.Preload("Role").Where("id = ?", userID).First(&user).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusForbidden, gin.H{"status": false, "message": "user not found"})
            return
        }else {
            c.JSON(500, gin.H{"status": false, "error": err.Error()})
            return
        }
    }

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

	var bundle_item []models.BundleItem
	if err := tx.Where("bundle_id = ?", bundle_id).Find(&bundle_item).Error; err != nil {
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

	var bundle models.Bundle
	if err := tx.First(&bundle, bundle_id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "Data bundle tidak ditemukan",
		})
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

	baseURL := c.Request.Host + c.Request.URL.Path
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	fullURL := scheme + "://" + baseURL

	// pagination links
	links := []gin.H{
		{
			"url":    nil,
			"label":  "&laquo; Previous",
			"active": false,
		},
	}

	if lastPage <= 8 {
        // Jika total halaman 10 atau kurang, tampilkan semua
        for i := 1; i <= lastPage; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d&q=%s", fullURL, i, q),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }
    } else {
        for i := 1; i <= 8; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d&q=%s", fullURL, i, q),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }

        // Tambahkan separator "..."
        links = append(links, gin.H{
            "url":    nil,
            "label":  "...",
            "active": false,
        })

        for i := lastPage - 1; i <= lastPage; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d&q=%s", fullURL, i, q),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }
    }

	links = append(links, gin.H{
		"url":    nil,
		"label":  "Next &raquo;",
		"active": false,
	})

	var nextPageURL interface{} = nil
	var prevPageURL interface{} = nil

	if page < lastPage {
		nextPageURL = fmt.Sprintf("%s?page=%d&q=%s", fullURL, page+1, q)
	}
	if page > 1 {
		prevPageURL = fmt.Sprintf("%s?page=%d&q=%s", fullURL, page-1, q)
	}

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List repair bundle",
			"resource": gin.H{
                "total_data":           totalData,
                "data":                 bundles,
				"first_page_url": fmt.Sprintf("%s?page=1&q=%s", fullURL, q),
				"from":           offset + 1,
				"last_page":      lastPage,
				"last_page_url":  fmt.Sprintf("%s?page=%d&q=%s", fullURL, lastPage, q),
				"links":          links,
				"next_page_url":  nextPageURL,
				"path":           fullURL,
				"per_page":       limit,
				"prev_page_url":  prevPageURL,
				"to":             offset + len(bundles),
			},
		},
	})
}

func GetRepairFilterProduct(c *gin.Context) {
	type productData struct {
		ID string `json:"id"`
		NewBarcode string `json:"new_barcode"`
		ProductName string `json:"product_name"`
	}

	var result []productData
	if err := config.DB.Model(&models.BundleItem{}).
		Joins("LEFT JOIN products ON products.id = bundle_items.product_id").
		Select(`
			bundle_items.id AS id,
			products.name AS product_name,
			products.barcode AS new_barcode
		`).
		Where("(bundle_id IS NULL)").
		Where("bundle_stage = ?", "repair_filter").
		Scan(&result).Error; err != nil {
		c.JSON(500, gin.H{"status": false, "error":err.Error()})
		return
	} 

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "list product filter repair",
		"resource": gin.H{
			"data": result,
		},
	})

}

func DumpProductRepair(c *gin.Context) {
	item_id := c.Param("item_id")
	userID, _ := c.Get("user_id")

    var user models.User
    if err := config.DB.Preload("Role").First(&user, userID).Error; err != nil {
        c.JSON(http.StatusForbidden, gin.H{"status": false, "message": "user not found"})
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

	//cari bundle item terkait
	var item models.BundleItem
    if err := tx.Where("id = ?", item_id).First(&item).Error; err != nil {
        tx.Rollback()
        c.JSON(404, gin.H{"status": false, "message": "Bundle item tidak ditemukan"})
        return
    }

	//cari bundle item terkait
	var product models.Product
    if err := tx.Where("id = ? AND status != ?", item.ProductID, "dump").First(&product).Error; err != nil {
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

	newTotalPrice := bundle.TotalPrice - product.Price
	totalProduct := bundle.TotalProduct - 1
	if newTotalPrice < 0 { newTotalPrice = 0 }
	if totalProduct < 0 { totalProduct = 0 }

	updateData := map[string]interface{}{
		"total_price": newTotalPrice,
		"total_price_custom": newTotalPrice,
		"total_product": totalProduct,
	}

	//UPDATE BUNDLE
	if err := tx.Model(&bundle).Updates(updateData).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "error": err.Error()})
        return
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
	if err:=helpers.LogUserAction(user.ID, user.Name, action, "/moving-product/repair", metadata); err != nil {
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
