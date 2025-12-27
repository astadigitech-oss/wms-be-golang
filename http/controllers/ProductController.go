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
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type RiwayatCheckUpdateData struct {
    RiwayatCheck models.RiwayatCheck 
    TotalDiscrepancy int64         
}

// ProductApprove menangani alur persetujuan produk baru.
func ProductApprove(c *gin.Context) {
    // Tangkap user id
    userID, _ := c.Get("user_id")
    userIDUint := userID.(uint) 
    product_old_id := c.Param("product_old_id")

    // Validator payload (di sini juga terjadi paralelisme non-DB)
    type ProductApprovePayload struct {
        CodeDocument string `json:"code_document" binding:"required"`
        NewNameProduct string `json:"new_name_product" binding:"required"`
        NewQuantityProduct int `json:"new_quantity_product" binding:"required,gt=0"`
        Quality string `json:"quality" binding:"required,oneof=lolos damaged abnormal"`
        QualityText string `json:"quality_text"`
        CategoryID *uint64 `json:"category_id"`
        TagColorID *uint64 `json:"tag_color_id"`
    }

    var payload ProductApprovePayload
    if err := c.ShouldBindJSON(&payload); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload", "detail": err.Error()})
        return
    }

    errChan := make(chan error, 2) 
    var wg sync.WaitGroup

    // Variabel hasil dari Go routine
    var old models.ProductOld
    var document models.Document
    var newProduct models.Product
    var riwayatCheckData RiwayatCheckUpdateData
    var isLolos, isDamaged, isAbnormal bool

    err_doc := config.DB.Where("code = ?", payload.CodeDocument).First(&document).Error
    if err_doc != nil {
        if errors.Is(err_doc, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "document not found"})
            return
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query document"})
            return
        }
    }

    // PARALLEL READ (RiwayatCheck & Discrepancy Count)
    wg.Add(1)
    go func() {
        defer wg.Done()

        var riwayatCheck models.RiwayatCheck
        if err := config.DB.Where("code_document = ?", payload.CodeDocument).First(&riwayatCheck).Error; err != nil {
            if errors.Is(err, gorm.ErrRecordNotFound) {
                errChan <- helpers.NewCustomError(http.StatusNotFound, "riwayat check not found", err)
            } else {
                errChan <- helpers.NewCustomError(http.StatusInternalServerError, "failed to query riwayat check", err)
            }
            return
        }

        riwayatCheckData.RiwayatCheck = riwayatCheck

        var totalDiscrepancy int64
        
        // Hitung total discrepancy (ProductOld yang belum ada di tabel products)
        err := config.DB.Model(&models.ProductOld{}).
            Where("code_document = ?", payload.CodeDocument).
            Where("NOT EXISTS (?)", 
                config.DB.Select("1").
                    Table("products").
                    Where("products.product_old_id = product_olds.id"),
            ).
            Count(&totalDiscrepancy).Error 
        
        if err != nil {
            errChan <- helpers.NewCustomError(http.StatusInternalServerError, "failed to count discrepancy", err)
            return
        }

        riwayatCheckData.TotalDiscrepancy = totalDiscrepancy
    }()

    // GO ROUTINE 2: PARALLEL SETUP (Load Old Product, Generate Barcode, Build New Product)
    wg.Add(1)
    go func() {
        defer wg.Done()
        
        // Load product_old (menggunakan config.DB, karena ini adalah data statis awal) ---
        if err := config.DB.Where("id = ?", product_old_id).First(&old).Error; err != nil {
            if errors.Is(err, gorm.ErrRecordNotFound) {
                errChan <- helpers.NewCustomError(http.StatusNotFound, "product_old not found", err)
            } else {
                errChan <- helpers.NewCustomError(http.StatusInternalServerError, "failed to query product_old", err)
            }
            return
        }
    
        // generate barcode
        customeBarcode := ""
        if document.CustomBarcode != nil {
            customeBarcode = *document.CustomBarcode
        }
        barcode, err := helpers.GenerateUniqueBarcode(config.DB, userIDUint, customeBarcode)
        if err != nil {
            errChan <- helpers.NewCustomError(http.StatusInternalServerError, "failed to generate barcode", err)
            return
        }

        // Build product to insert
        newProduct = models.Product{
            CodeDocument: 	 *old.CodeDocument,
            ProductOldID: old.ID,
            Barcode: 	 barcode,
            Name: 	 payload.NewNameProduct,
            Quantity: int64(payload.NewQuantityProduct),
            Status: 	 "display",
            Quality: payload.Quality,
            QualityText: &payload.QualityText,
        }
        
        // determine LocationType rules:
        if old.OldPriceProduct >= 100000 {
            status := "staging"
            newProduct.LocationType = &status
            var category models.Category
            if err := config.DB.Where("id = ?", payload.CategoryID).First(&category).Error; err != nil {
                if errors.Is(err, gorm.ErrRecordNotFound) {
                    errChan <- helpers.NewCustomError(http.StatusNotFound, "category not found", err)
                } else {
                    errChan <- helpers.NewCustomError(http.StatusInternalServerError, "failed to query Category", err)
                }
                return
            }
            discount := old.OldPriceProduct * (float64(category.DiscountCategory)/100.0)
            discount = math.Round(discount)
            if discount > category.MaxPriceCategory {
                discount = category.MaxPriceCategory
            } 
            newProduct.Discount = &discount
            newProduct.Price = old.OldPriceProduct - discount
            newProduct.CategoryID = payload.CategoryID
            newProduct.TagColorID = nil
        } else {
            var color_tag models.ColorTag
            if err := config.DB.Where("id = ?", payload.TagColorID).First(&color_tag).Error; err != nil {
                if errors.Is(err, gorm.ErrRecordNotFound) {
                    errChan <- helpers.NewCustomError(http.StatusNotFound, "color tag not found", err)
                } else {
                    errChan <- helpers.NewCustomError(http.StatusInternalServerError, "failed to query Color Tag", err)
                }
                return
            }
            if (old.OldPriceProduct < color_tag.MinPriceColor || old.OldPriceProduct > color_tag.MaxPriceColor) {
                errChan <- helpers.NewCustomError(http.StatusBadRequest, "data color tag tidak sesuai dengan harga produk", errors.New("price not in range of color tag"))
                return
            }
            newProduct.Price = color_tag.FixedPriceColor
            newProduct.TagColorID = payload.TagColorID
            newProduct.CategoryID = nil
        }
        
        newProduct.DisplayPrice = newProduct.Price

        // Simpan status kualitas untuk perhitungan
        isLolos = (payload.Quality == "lolos")
        isDamaged = (payload.Quality == "damaged")
        isAbnormal = (payload.Quality == "abnormal")
    }()
    
    wg.Wait()
    close(errChan)

    // Cek Error dari Go routine (jika ada error di sini, tidak perlu mulai tx)
    for err := range errChan {
        if err != nil {
            if customErr, ok := err.(*helpers.CustomError); ok {
                c.JSON(customErr.StatusCode, gin.H{"success": false, "message": customErr.Error()})
            } else {
                c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Parallel setup failed."})
            }
            return
        }
    }

    // --- Mulai Transaksi (Setelah semua data input siap) ---
    tx := config.DB.Begin()
    if tx.Error != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "db transaction start error"})
        return
    }
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
        }
    }()


    // 1. SEQUENTIAL WRITE: Insert Product
    if err := tx.Create(&newProduct).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to insert product", "error": err.Error()})
        return
    }
    
    // 2. SEQUENTIAL WRITE: Update User Scan Web
    if err := updateOrCreateDailyScan(tx, userIDUint, payload.CodeDocument); err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create or update user scan", "error": err.Error()})
        return
    }

    // 3. SEQUENTIAL WRITE: Update Document status -> in_progress
    if err := tx.Model(&models.Document{}).Where("code = ?", payload.CodeDocument).
        Update("status_document", "inprogress").Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to update document status"})
        return
    }
    
    // 4. Perhitungan dan Update Riwayat
    riwayatCheck := riwayatCheckData.RiwayatCheck
    totalDataIn := riwayatCheck.TotalDataIn + 1
    totalDiscrepancy := riwayatCheckData.TotalDiscrepancy
    if totalDiscrepancy > 0 {
        totalDiscrepancy -= 1 
    }

    totalDataLolos := riwayatCheck.TotalDataLolos
    totalDataDamaged := riwayatCheck.TotalDataDamaged
    totalDataAbnormal := riwayatCheck.TotalDataAbnormal

    // Penambahan TotalPriceIn
    currentTotalPrice := helpers.GetFloat64Value(riwayatCheck.TotalPriceIn)
    totalPriceIn := currentTotalPrice + old.OldPriceProduct 
    
    // Update count berdasarkan kualitas produk yang baru dibuat
    if isLolos {
        totalDataLolos += 1
    } else if isDamaged {
        totalDataDamaged += 1
    } else if isAbnormal {
        totalDataAbnormal += 1
    }

    // Perhitungan Persentase
    totalData := float64(riwayatCheck.TotalData) 
    if totalData == 0 { totalData = 1 } 
    
    percentageIn := (float64(totalDataIn) / totalData) * 100.0
    percentageDiscrepancy := (float64(totalDiscrepancy) / totalData) * 100.0
    percentageLolos := (float64(totalDataLolos) / totalData) * 100.0
    percentageDamaged := (float64(totalDataDamaged) / totalData) * 100.0
    percentageAbnormal := (float64(totalDataAbnormal) / totalData) * 100.0
    percentageTotalData := 100.0 

    // 5. SEQUENTIAL WRITE: Update RiwayatCheck menggunakan Map
    updateData := map[string]interface{}{
        "total_data_in": totalDataIn,
        "total_discrepancy": totalDiscrepancy,
        "total_data_lolos": totalDataLolos,
        "total_data_damaged": totalDataDamaged,
        "total_data_abnormal": totalDataAbnormal,
        "total_price_in": &totalPriceIn, 

        "precentage_total_data": &percentageTotalData,
        "percentage_in": &percentageIn,
        "percentage_discrepancy": &percentageDiscrepancy,
        "percentage_lolos": &percentageLolos,
        "percentage_damaged": &percentageDamaged,
        "percentage_abnormal": &percentageAbnormal,
    }

    if err := tx.Model(&models.RiwayatCheck{}).
        Where("id = ?", riwayatCheck.ID).
        Updates(updateData).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update RiwayatCheck: " + err.Error()})
        return
    }

    // --- COMMIT ---
    if err := tx.Commit().Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit transaction"})
        return
    }

    // sukses
    c.JSON(200, gin.H{
        "status": true,
        "message": "Product berhasil di approve",
        "product": gin.H{
            "old_data": old,
            "new_data": newProduct,
        },
    })
}

// updateOrCreateDailyScan tetap menggunakan tx yang sama
func updateOrCreateDailyScan(tx *gorm.DB, userID uint, codeDocument string) error {
    today := models.Date(time.Now())

    var usw models.UserScanWeb

    err := tx.
        Where("code_document = ?", codeDocument).
        Where("scan_date = ?", today).
        Where("user_id = ?", userID).
        First(&usw).
        Error

    if err != nil {
        // === JIKA BELUM ADA -> CREATE ===
        if errors.Is(err, gorm.ErrRecordNotFound) {

            totalScan := 1
            newScan := models.UserScanWeb{
                CodeDocument: &codeDocument,
                UserID:       &userID,
                ScanDate:     &today,
                TotalScans:   &totalScan,
            }

            if err := tx.Create(&newScan).Error; err != nil {
                return err
            }

            return nil
        }

        return err
    }

    // === JIKA ADA -> UPDATE total_scans + 1 ===
    if err := tx.Model(&usw).
        UpdateColumn("total_scans", gorm.Expr("total_scans + ?", 1)).
        Error; err != nil {
        return err
    }

    return nil
}

func AddProductManual(c *gin.Context) {
    
}

// ============================= STAGGING =============================
type productWithCategoryName struct {
    ID          uint      `json:"id"`
    Barcode     string    `json:"barcode"`
    Name        string    `json:"name"`
    Price       float64   `json:"price"`
    Status       string   `json:"status"`
    DisplayPrice       float64   `json:"display_price"`
    Quantity    int       `json:"quantity"`
    CategoryID  uint      `json:"category_id"`
    CategoryName string   `json:"category_name"` // Harus sesuai dengan alias SELECT
    CreatedAt string   `json:"created_at"` // Harus sesuai dengan alias SELECT
}

type stagProductUpdatePayload struct {
    CodeDocument string `json:"code_document" binding:"required"`
    NewNameProduct string `json:"new_name_product" binding:"required"`
    NewQuantityProduct int `json:"new_quantity_product" binding:"required,gt=0"`
    NewPriceProduct float64 `json:"new_price_product" binding:"required,gt=0"`
    CategoryID uint64 `json:"category_id" binding:"required"`
}


// stagging
func StaggingProduct(c *gin.Context) {
    q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	var products []productWithCategoryName
	var total int64

	db := config.DB.Model(&models.Product{}).
        Select(`
            products.id, 
            products.barcode,
            products.name,
            products.price,
            products.status,
            products.display_price,
            products.quantity,
            products.category_id, 
            products.created_at, 
            categories.name_category AS category_name
        `).
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
        Where("products.location_type = ?", "staging").
        Where("products.staging_stage IS NULL")

    // FILTERING
	if q != "" {
        like := "%" + q + "%"

        db = db.Where(`
            products.barcode LIKE ? OR
            products.name LIKE ? OR
            categories.name_category LIKE ?
        `, like, like, like)
	}

	// TOTAL COUNT (for pagination info)
    countDB := db.Session(&gorm.Session{})
	if err := countDB.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": err})
		return
	}

	// GET DATA
	if err := db.
		Limit(limit).
		Offset(offset).
		Find(&products).Error; err != nil {

		c.JSON(500, gin.H{"success": false, "message": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))

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

	for i := 1; i <= lastPage; i++ {
		links = append(links, gin.H{
			"url":    fmt.Sprintf("%s?page=%d", fullURL, i),
			"label":  strconv.Itoa(i),
			"active": i == page,
		})
	}

	links = append(links, gin.H{
		"url":    nil,
		"label":  "Next &raquo;",
		"active": false,
	})

	var nextPageURL interface{} = nil
	var prevPageURL interface{} = nil

	if page < lastPage {
		nextPageURL = fmt.Sprintf("%s?page=%d", fullURL, page+1)
	}
	if page > 1 {
		prevPageURL = fmt.Sprintf("%s?page=%d", fullURL, page-1)
	}

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Documents",
			"resource": gin.H{
				"current_page":   page,
				"data":           products,
				"first_page_url": fmt.Sprintf("%s?page=1", fullURL),
				"from":           offset + 1,
				"last_page":      lastPage,
				"last_page_url":  fmt.Sprintf("%s?page=%d", fullURL, lastPage),
				"links":          links,
				"next_page_url":  nextPageURL,
				"path":           fullURL,
				"per_page":       limit,
				"prev_page_url":  prevPageURL,
				"to":             offset + len(products),
				"total":          total,
			},
		},
	})
}

func StaggingProductDetail(c *gin.Context) {
    product_id := c.Param("product_id")

    var product models.Product

	err := config.DB.WithContext(c.Request.Context()).
                Preload("ProductOld").
                Preload("Category").
                First(&product, "id = ?", product_id).Error

    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{
                "success": false,
                "message": "Product not found",
            })
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": "Internal server error: " + err.Error(),
        })
        return
    }

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "Detail Stagging Product",
			"resource": product,
		},
	})
}

func StaggingProductUpdate(c *gin.Context) {
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

    productID, err := strconv.ParseUint(c.Param("product_id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID format"})
        return
    }
    productIDUint := uint(productID)

    var payload stagProductUpdatePayload
    if err := c.ShouldBindJSON(&payload); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload", "detail": err.Error()})
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
		}
	}()

    //load data product
    var product models.Product

    if err := config.DB.Preload("ProductOld").
            Where("id = ?", productIDUint).
            Where("location_type = ?", "staging").
            Where("staging_stage IS NULL").
            First(&product).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "product not found"})
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query product_old"})
        }
        return
    }

    updateData := map[string]interface{}{
        "name": payload.NewNameProduct,
        "quantity": payload.NewQuantityProduct,
    }
    var discount float64
    if product.ProductOld.OldPriceProduct >= 100000 {
        var category models.Category
        if err := config.DB.Where("id = ?", payload.CategoryID).First(&category).Error; err != nil {
            if errors.Is(err, gorm.ErrRecordNotFound) {
                c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "category not found"})
            } else {
                c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query Category"})
            }
            return
        }

        discount = product.ProductOld.OldPriceProduct * (float64(category.DiscountCategory)/100.0)
        discount = math.Round(discount)
        if discount > category.MaxPriceCategory {
            discount = category.MaxPriceCategory
        } 

        calculatedPrice := product.ProductOld.OldPriceProduct - discount
        if math.Round(calculatedPrice) != math.Round(payload.NewPriceProduct) {
            c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Harga setelah diskon kategori tidak sesuai. Harap periksa kembali."})
            return
        }

        updateData["price"] = calculatedPrice
        updateData["discount"] = discount
        updateData["category_id"] = category.ID
        updateData["display_price"] = calculatedPrice
    }

    logDetails := map[string]interface{}{
        "status": "Awaiting ADMIN/SPV Approval",
        "changes": map[string]interface{}{
            "new_name":         payload.NewNameProduct,
            "new_quantity":     payload.NewQuantityProduct,
            "new_price":        payload.NewPriceProduct,
        },
        "Before Edit : ": map[string]interface{}{
            "old_name":       product.Name,
            "old_quantity":   product.Quantity,
            "old_price":      product.Price, // Harga lama di Staging
            "old_display_price": product.DisplayPrice,
        },
    }

    if user.Role.RoleName != "Admin" && user.Role.RoleName != "Spv" {
        tipe := "staging"

        approveQueue := models.ApproveQueue{
            UserID: &user.ID,
            ProductID: &productIDUint,
            Type: &tipe,
            CodeDocument: &payload.CodeDocument,
            OldPriceProduct: &product.ProductOld.OldPriceProduct,
            NewNameProduct: &payload.NewNameProduct,
            NewQuantityProduct: &payload.NewQuantityProduct,
            NewPriceProduct: &payload.NewPriceProduct,
            NewDiscount: &discount,
            CategoryID: &payload.CategoryID,
            Status: "1",
        }

        if err := tx.Create(&approveQueue).Error; err != nil {
            tx.Rollback()
            c.JSON(500, gin.H{"status": false, "error": err.Error()})
            return
        }

        approved := "0"
        notification := models.Notification{
            UserID: user.ID,
            NotificationName: "Edit product staging" + " " +product.Barcode,
            Role: user.Role.RoleName,
            Status: "staging",
            ExternalID: &productIDUint,
            Approved: &approved,
        }

        if err := tx.Create(&notification).Error; err != nil {
            tx.Rollback()
            c.JSON(500, gin.H{"status": false, "error": err.Error()})
            return
        }
    }else {
        if err := tx.Model(&models.Product{}).Where("id = ?", productIDUint).Updates(updateData).Error; err != nil {
            tx.Rollback()
            c.JSON(500, gin.H{"status": false, "message": "Gagal update data product", "error": err.Error()})
            return
        }
    }

    if err := helpers.LogUserAction(user.ID, user.Name, "Edit Product Stagging "+product.Barcode, "staging/product/detail", logDetails); err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal membuat log user action", "error": err.Error()})
        return
    }

    if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

    c.JSON(201, gin.H{
        "status": true,
        "message": "Product staging berhasil di update",
    })
    
}

func StaggingFilterProduct(c *gin.Context) {
    q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	var products []productWithCategoryName
	var total int64

	db := config.DB.Model(&models.Product{}).
        Select(`
            products.id, 
            products.barcode,
            products.name,
            products.price,
            products.status,
            products.display_price,
            products.quantity,
            products.category_id, 
            products.created_at, 
            categories.name_category AS category_name
        `).
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
        Where("products.location_type = ?", "staging").
        Where("products.staging_stage = ?", "process")

    // FILTERING
	if q != "" {
        like := "%" + q + "%"

        db = db.Where(`
            products.barcode LIKE ? OR
            products.name LIKE ? OR
            categories.name_category LIKE ?
        `, like, like, like)
	}

	// TOTAL COUNT (for pagination info)
    countDB := db.Session(&gorm.Session{})
	if err := countDB.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": err})
		return
	}

	// GET DATA
	if err := db.
		Limit(limit).
		Offset(offset).
		Find(&products).Error; err != nil {

		c.JSON(500, gin.H{"success": false, "message": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))

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

	for i := 1; i <= lastPage; i++ {
		links = append(links, gin.H{
			"url":    fmt.Sprintf("%s?page=%d", fullURL, i),
			"label":  strconv.Itoa(i),
			"active": i == page,
		})
	}

	links = append(links, gin.H{
		"url":    nil,
		"label":  "Next &raquo;",
		"active": false,
	})

	var nextPageURL interface{} = nil
	var prevPageURL interface{} = nil

	if page < lastPage {
		nextPageURL = fmt.Sprintf("%s?page=%d", fullURL, page+1)
	}
	if page > 1 {
		prevPageURL = fmt.Sprintf("%s?page=%d", fullURL, page-1)
	}

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Documents",
			"resource": gin.H{
				"current_page":   page,
				"data":           products,
				"first_page_url": fmt.Sprintf("%s?page=1", fullURL),
				"from":           offset + 1,
				"last_page":      lastPage,
				"last_page_url":  fmt.Sprintf("%s?page=%d", fullURL, lastPage),
				"links":          links,
				"next_page_url":  nextPageURL,
				"path":           fullURL,
				"per_page":       limit,
				"prev_page_url":  prevPageURL,
				"to":             offset + len(products),
				"total":          total,
			},
		},
	})
}

func AddToFilterStaging(c *gin.Context) {
    product_id := c.Param("product_id")

	result := config.DB.Model(&models.Product{}).
        Where("id = ?", product_id).Update("staging_stage", "process")

    err := result.Error
    if err != nil {
        c.JSON(500, gin.H{"success": false, "message": "failed to update product staging stage", "error": err.Error()})
        return
    }

    if result.RowsAffected == 0 {
        c.JSON(404, gin.H{"success": false, "message": "product not found"})
        return
    }

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "Product berhasil dipindah ke filter staging",
		},
	})
}

func StaggingFilterApprove(c *gin.Context) {
	result := config.DB.Model(&models.Product{}).
        Where("staging_stage = ?", "process").Update("staging_stage", "approve")

    err := result.Error
    if err != nil {
        c.JSON(500, gin.H{"success": false, "message": "failed to update product staging stage", "error": err.Error()})
        return
    }

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "Product berhasil diapprove semua dari filter staging",
		},
	})
}

func DestroyFilterProduct(c *gin.Context) {
    product_id := c.Param("product_id")

	result := config.DB.Model(&models.Product{}).
        Where("id = ?", product_id).Update("staging_stage", nil)

    err := result.Error
    if err != nil {
        c.JSON(500, gin.H{"success": false, "message": "failed to update product staging stage", "error": err.Error()})
        return
    }

    if result.RowsAffected == 0 {
        c.JSON(404, gin.H{"success": false, "message": "product not found"})
        return
    }

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "Product berhasil dihapus dari filter staging",
		},
	})
}

func StaggingMoveToLPR(c *gin.Context) {
    //payload
    type StagMoveToLPRPayload struct {
        Quality string `json:"quality" binding:"required"`
        QualityText string `json:"quality_text" binding:"required"`
    }

    var payload StagMoveToLPRPayload
    if err := c.ShouldBindJSON(&payload); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload", "detail": err.Error()})
        return
    }

    productID, err := strconv.ParseUint(c.Param("product_id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID format"})
        return
    }
    productIDUint := uint(productID)

    result := config.DB.Model(&models.Product{}).
        Where("id = ?", productIDUint).
        Where("location_type = ?", "staging").
        Where("staging_stage IS NULL").
        Updates(payload)

    if err := result.Error; err != nil {
        c.JSON(500, gin.H{"success": false, "message": "failed to update product staging stage", "error": err.Error()})
        return
    }

    if result.RowsAffected == 0 {
        c.JSON(404, gin.H{"success": false, "message": "product not found or not eligible to move to LPR"})
        return
    }

    c.JSON(200, gin.H{
        "status": true,
        "message": "Product berhasil dipindah ke LPR",
    })
}

// approvement
func StaggingApprovement(c *gin.Context) {
    q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	var products []productWithCategoryName
	var total int64

	db := config.DB.Model(&models.Product{}).
        Select(`
            products.id, 
            products.barcode,
            products.name,
            products.price,
            products.status,
            products.display_price,
            products.quantity,
            products.category_id, 
            products.created_at, 
            categories.name_category AS category_name
        `).
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
        Where("products.location_type = ?", "staging").
        Where("products.staging_stage = ?", "approve")
    // FILTERING
	if q != "" {
        like := "%" + q + "%"

        db = db.Where(`
            products.barcode LIKE ? OR
            products.name LIKE ? OR
            categories.name_category LIKE ?
        `, like, like, like)
	}

	// TOTAL COUNT (for pagination info)
    countDB := db.Session(&gorm.Session{}) // Salin sesi DB untuk menghitung ulang
	if err := countDB.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": err})
		return
	}

	// GET DATA
	if err := db.
		Limit(limit).
		Offset(offset).
		Find(&products).Error; err != nil {

		c.JSON(500, gin.H{"success": false, "message": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))

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

	for i := 1; i <= lastPage; i++ {
		links = append(links, gin.H{
			"url":    fmt.Sprintf("%s?page=%d", fullURL, i),
			"label":  strconv.Itoa(i),
			"active": i == page,
		})
	}

	links = append(links, gin.H{
		"url":    nil,
		"label":  "Next &raquo;",
		"active": false,
	})

	var nextPageURL interface{} = nil
	var prevPageURL interface{} = nil

	if page < lastPage {
		nextPageURL = fmt.Sprintf("%s?page=%d", fullURL, page+1)
	}
	if page > 1 {
		prevPageURL = fmt.Sprintf("%s?page=%d", fullURL, page-1)
	}

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Documents",
			"resource": gin.H{
				"current_page":   page,
				"data":           products,
				"first_page_url": fmt.Sprintf("%s?page=1", fullURL),
				"from":           offset + 1,
				"last_page":      lastPage,
				"last_page_url":  fmt.Sprintf("%s?page=%d", fullURL, lastPage),
				"links":          links,
				"next_page_url":  nextPageURL,
				"path":           fullURL,
				"per_page":       limit,
				"prev_page_url":  prevPageURL,
				"to":             offset + len(products),
				"total":          total,
			},
		},
	})
}

func DestroyStaggingApprove(c *gin.Context) {
    product_id := c.Param("product_id")

	result := config.DB.Model(&models.Product{}).
        Where("id = ?", product_id).Update("staging_stage", "process")

    err := result.Error
    if err != nil {
        c.JSON(500, gin.H{"success": false, "message": "failed to update product staging stage", "error": err.Error()})
        return
    }

    if result.RowsAffected == 0 {
        c.JSON(404, gin.H{"success": false, "message": "product not found"})
        return
    }

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "Product staging approve berhasil di batalkan",
		},
	})
}

func StaggingApprovesStore(c *gin.Context) {
    updateData := map[string]interface{}{
        "staging_stage": nil,
        "location_type": "main",
    }

	result := config.DB.Model(&models.Product{}).
        Where("staging_stage = ?", "approve").Updates(updateData)

    err := result.Error
    if err != nil {
        c.JSON(500, gin.H{"success": false, "message": "failed to update product staging stage", "error": err.Error()})
        return
    }

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "Product berhasil diapprove",
		},
	})
}

// ============================= INVENTORY PRODUCT =============================
//Product
func GetProductsByColor(c *gin.Context) {
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
        Where("products.tag_color_id IS NOT NULL").
        Where("products.category_id IS NULL").
        Where("products.is_so IS NULL").
        Where("products.status = ?", "display").
        Where("products.location_type = ?", "main").
        Where("products.quality = ?", "lolos").
        Where("(products.warehouse_type IS NULL OR products.warehouse_type = 'type1')")

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(products.barcode LIKE ? OR "+
            "product_olds.old_barcode_product LIKE ? OR " + 
            "products.name LIKE ? OR " + 
            "color_tags.name_color LIKE ?)", searchPattern, searchPattern, searchPattern, searchPattern)
	}

    // Summary (Gunakan GroupBy Nama Tag)
    type TagSummary struct {
        TagName    string  `json:"tag_name"`
        TotalData  int64   `json:"total_data"`
        TotalPrice float64 `json:"total_price"`
    }

    var summaries []TagSummary

    // jalankan query summary terlebih dahulu
    baseQuery.Session(&gorm.Session{}).
		Select("color_tags.name_color as tag_name, COUNT(products.id) as total_data, SUM(products.price) as total_price").
		Group("color_tags.name_color").
		Scan(&summaries)

    // Hitung grand total price dari summary
    var totalPriceAll float64
    for _, s := range summaries {
        totalPriceAll += s.TotalPrice
    }

    // Paginate Data
    type productsData struct {
        ID          uint64  `json:"id"`
        OldBarcodeProduct  string  `json:"old_barcode"`
        Barcode     string  `json:"new_barcode"`
        Name        string  `json:"name"`
        Price       float64 `json:"price"`
        Status      string  `json:"status"`
        NameColor   string  `json:"name_color"`
    }

    var products []productsData
	var totalData int64

    baseQuery.Session(&gorm.Session{}).Count(&totalData)

    // Ambil data detail
    err := baseQuery.Session(&gorm.Session{}).
        Select(`
            products.id, 
            product_olds.old_barcode_product, 
            products.barcode, 
            products.name, 
            products.price, 
            products.status, 
            color_tags.name_color
        `).
        Order("products.created_at DESC").
        Limit(limit).Offset(offset).
        Find(&products).Error

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
                "url":    fmt.Sprintf("%s?page=%d", fullURL, i),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }
    } else {
        for i := 1; i <= 8; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d", fullURL, i),
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
                "url":    fmt.Sprintf("%s?page=%d", fullURL, i),
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
		nextPageURL = fmt.Sprintf("%s?page=%d", fullURL, page+1)
	}
	if page > 1 {
		prevPageURL = fmt.Sprintf("%s?page=%d", fullURL, page-1)
	}

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Product by color",
			"resource": gin.H{
                "total_data":           totalData,
                "total_price_all":      totalPriceAll,
                "tags_summary":         summaries,
                "data":                 products,
				"first_page_url": fmt.Sprintf("%s?page=1", fullURL),
				"from":           offset + 1,
				"last_page":      lastPage,
				"last_page_url":  fmt.Sprintf("%s?page=%d", fullURL, lastPage),
				"links":          links,
				"next_page_url":  nextPageURL,
				"path":           fullURL,
				"per_page":       limit,
				"prev_page_url":  prevPageURL,
				"to":             offset + len(products),
				"total":          totalData,
			},
		},
	})
}

func GetProductsByCategory(c *gin.Context) {
    type ProductResult struct {
        ID                 uint      `json:"id"`
        SourceType         string    `json:"source_type"` // product | bundle
        Barcode            string    `json:"barcode"`
        Name               string    `json:"name"`
        NameCategory       string    `json:"name_category"`
        Price              float64   `json:"price"`
        CreatedAt          time.Time `json:"created_at"`
        Status             string    `json:"new_status_product"`
        DisplayPrice       float64   `json:"display_price"`
        OldBarcodeProduct  string    `json:"old_barcode_product"`
        OldNameProduct     string    `json:"old_name_product"`
        OldQuantityProduct int       `json:"old_quantity_product"`
        OldPriceProduct    float64   `json:"old_price_product"`
    }

	q := strings.TrimSpace(c.Query("q"))

	// PAGINATION
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 33
	offset := (page - 1) * limit

	// SEARCH CONDITION
	searchCondition := ""
	args := []interface{}{}

	if q != "" {
		searchCondition = `
			AND (
				name_category LIKE ?
				OR barcode LIKE ?
				OR name LIKE ?
				OR status LIKE ?
			)
		`
		search := "%" + q + "%"
		args = append(args, search, search, search, search)
	}

	// UNION QUERY (DATA)
	dataQuery := fmt.Sprintf(`
		SELECT * FROM (
			SELECT
				p.id,
                'product' AS source_type,
				p.barcode AS barcode,
				p.name AS name,
				c.name_category AS name_category,
				p.price,
				p.created_at,
				p.status AS status,
				p.display_price,
				po.old_barcode_product,
				po.old_name_product,
				po.old_quantity_product,
				po.old_price_product
			FROM products p
			LEFT JOIN categories c ON c.id = p.category_id
			LEFT JOIN product_olds po ON po.id = p.product_old_id
			WHERE p.tag_color_id IS NULL
				AND p.category_id IS NOT NULL
				AND p.status IN ('display','expired')
				AND p.location_type = 'main'
				AND p.quality = 'lolos'
				AND (p.warehouse_type IS NULL OR p.warehouse_type = 'type1')

			UNION ALL

			SELECT
				b.id,
                'bundle' AS source_type,
				b.barcode AS barcode,
				b.name_bundle AS name,
				c.name_category AS name_category,
				b.total_price_custom AS price,
				b.created_at,
				CASE 
					WHEN b.status = 'not sale' THEN 'display'
					ELSE b.status
				END AS status,
				b.total_price_custom AS display_price,
				NULL,
				NULL,
				NULL,
				NULL
			FROM bundles b
			LEFT JOIN categories c ON c.id = b.category_id
			WHERE b.total_price_custom >= 100000
				AND b.tag_color_id IS NULL
				AND b.category_id IS NOT NULL
				AND b.status != 'bundle'
				AND (b.warehouse_type IS NULL OR b.warehouse_type = 'type1')
		) x
		WHERE 1=1
		%s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, searchCondition)

	argsData := append(args, limit, offset)

	var results []ProductResult
	if err := config.DB.Raw(dataQuery, argsData...).Scan(&results).Error; err != nil {
		c.JSON(500, gin.H{"status": false, "error": err.Error()})
		return
	}

	// COUNT QUERY
	countQuery := fmt.Sprintf(`
        SELECT COUNT(*) FROM (
            SELECT 
                p.id AS id,
                p.barcode AS barcode,
                p.name AS name,
                c.name_category AS name_category,
                p.status AS status
            FROM products p
            LEFT JOIN categories c ON c.id = p.category_id
            WHERE p.tag_color_id IS NULL
                AND p.category_id IS NOT NULL
                AND p.status IN ('display','expired')
                AND p.location_type = 'main'
                AND p.quality = 'lolos'
                AND (p.warehouse_type IS NULL OR p.warehouse_type = 'type1')

            UNION ALL

            SELECT 
                b.id AS id,
                b.barcode AS barcode,
                b.name_bundle AS name,
                c.name_category AS name_category,
                CASE 
                    WHEN b.status = 'not sale' THEN 'display'
                    ELSE b.status
                END AS status
            FROM bundles b
            LEFT JOIN categories c ON c.id = b.category_id
            WHERE b.total_price_custom >= 100000
                AND b.tag_color_id IS NULL
                AND b.category_id IS NOT NULL
                AND b.status != 'bundle'
                AND (b.warehouse_type IS NULL OR b.warehouse_type = 'type1')
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
	links := []gin.H{
		{
			"url":    nil,
			"label":  "&laquo; Previous",
			"active": false,
		},
	}

    scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	fullURL := fmt.Sprintf("%s://%s%s", scheme, c.Request.Host, c.Request.URL.Path)


	if lastPage <= 8 {
        // Jika total halaman 10 atau kurang, tampilkan semua
        for i := 1; i <= lastPage; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d", fullURL, i),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }
    } else {
        for i := 1; i <= 8; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d", fullURL, i),
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
                "url":    fmt.Sprintf("%s?page=%d", fullURL, i),
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

	var nextPageURL, prevPageURL interface{}

	if page < lastPage {
		nextPageURL = fmt.Sprintf("%s?page=%d", fullURL, page+1)
	}

	if page > 1 {
		prevPageURL = fmt.Sprintf("%s?page=%d", fullURL, page-1)
	}

	c.JSON(200, gin.H{
		"status":  true,
		"message": "List Product by category",
		"resource": gin.H{
			"total":          totalData,
			"data":           results,
			"current_page":   page,
			"last_page":      lastPage,
			"per_page":       limit,
			"next_page_url":  nextPageURL,
            "links":           links,
			"prev_page_url":  prevPageURL,
		},
	})
}

func GetProductsStatusDisplayExpired(c *gin.Context) {
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
        Joins("LEFT JOIN product_olds ON product_olds.id = products.product_old_id").
        Where("products.status IN ?", []string{"display", "expired"}).
        Where("products.location_type = ?", "main").
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
                "url":    fmt.Sprintf("%s?page=%d", fullURL, i),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }
    } else {
        for i := 1; i <= 8; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d", fullURL, i),
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
                "url":    fmt.Sprintf("%s?page=%d", fullURL, i),
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
		nextPageURL = fmt.Sprintf("%s?page=%d", fullURL, page+1)
	}
	if page > 1 {
		prevPageURL = fmt.Sprintf("%s?page=%d", fullURL, page-1)
	}

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Product Display & Expired",
			"resource": gin.H{
                "total_data":           totalData,
                "data":                 products,
				"first_page_url": fmt.Sprintf("%s?page=1", fullURL),
				"from":           offset + 1,
				"last_page":      lastPage,
				"last_page_url":  fmt.Sprintf("%s?page=%d", fullURL, lastPage),
				"links":          links,
				"next_page_url":  nextPageURL,
				"per_page":       limit,
				"prev_page_url":  prevPageURL,
				"to":             offset + len(products),
				"total":          totalData,
			},
		},
	})
}

func UpdateProductStatus(c *gin.Context) {
    // Definisikan struct untuk request
    var input struct {
        SourceType string `json:"source_type" binding:"required"`
    }

    if err := c.ShouldBindJSON(&input); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "source_type wajib diisi", "error":err.Error()})
        return
    }

    //Cek apakah source_type adalah 'product'
    if input.SourceType != "product" {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Hanya tipe 'product' yang diizinkan"})
        return
    }

    product_id := c.Param("id")
    result := config.DB.Model(&models.Product{}).
        Where("id = ?", product_id).Update("status", "dump")

    err := result.Error
    if err != nil {
        c.JSON(500, gin.H{"success": false, "message": "failed to update product", "error": err.Error()})
        return
    }

    if result.RowsAffected == 0 {
        c.JSON(404, gin.H{"success": false, "message": "product not found"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Berhasil mengubah status produk menjadi dump",
    })
}

func DeleteProductInventory(c *gin.Context) {
    // Definisikan struct untuk request
    var input struct {
        SourceType string `json:"source_type" binding:"required"`
    }

    if err := c.ShouldBindJSON(&input); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "source_type wajib diisi"})
        return
    }

    //Cek apakah source_type adalah 'product'
    if input.SourceType != "product" {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Hanya tipe 'product' yang diizinkan"})
        return
    }

    id := c.Param("id")
    userID, _ := c.Get("user_id")

    // Ambil data user
    var user models.User
    if err := config.DB.Select("id", "name").First(&user, userID).Error; err != nil {
        status := http.StatusInternalServerError
        if errors.Is(err, gorm.ErrRecordNotFound) {
            status = http.StatusForbidden
        }
        c.JSON(status, gin.H{"success": false, "message": "user not found"})
        return
    }

    // Ambil data produk (Di luar transaksi untuk efisiensi)
    var product models.Product
    if err := config.DB.First(&product, id).Error; err != nil {
        c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Data tidak ditemukan"})
        return
    }

    // 3. Mulai Transaksi
    tx := config.DB.Begin()
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error"})
        }
    }()

    // Proses Hapus
    if err := tx.Delete(&product).Error; err != nil {
        tx.Rollback() 
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal menghapus data"})
        return
    }

    // Logging
    metadata := map[string]interface{}{}
    logMsg := fmt.Sprintf("%s menghapus product dengan barcode %s", user.Name, product.Barcode)
    if err := helpers.LogUserAction(user.ID, user.Name, logMsg, "inventory/product", metadata); err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal membuat log"})
        return
    }

    // 6. Commit
    if err := tx.Commit().Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to commit transaction"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "data berhasil di hapus",
        "data":    product,
    })
}