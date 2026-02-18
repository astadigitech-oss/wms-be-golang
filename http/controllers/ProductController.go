package controllers

import (
	"database/sql"
	"fmt"
	services "liquid8/wms/Services"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"os"
	"path/filepath"

	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// ============================= INBOUND =============================
type RiwayatCheckUpdateData struct {
    RiwayatCheck models.RiwayatCheck 
    TotalDiscrepancy int64         
}

// ProductApprove menangani alur persetujuan produk baru.
func ProductApprove(c *gin.Context) {
    // Tangkap user id
    user := c.MustGet("auth_user").(models.User)
    product_old_id := c.Param("product_old_id")

    // Validator payload (di sini juga terjadi paralelisme non-DB)
    type ProductApprovePayload struct {
        CodeDocument string `json:"code_document" binding:"required"`
        NewNameProduct string `json:"new_name_product" binding:"required"`
        NewQuantityProduct int `json:"new_quantity_product" binding:"required,gt=0"`
        Quality string `json:"quality" binding:"required,oneof=lolos damaged abnormal"`
        QualityText *string `json:"quality_text"`
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
        barcode, err := helpers.GenerateUniqueBarcode(config.DB, user.ID, customeBarcode)
        if err != nil {
            errChan <- helpers.NewCustomError(http.StatusInternalServerError, "failed to generate barcode", err)
            return
        }

        // Build product to insert
        newProduct = models.Product{
            CodeDocument: 	 old.CodeDocument,
            InboundType:   old.InboundType,
            OldBarcodeProduct: old.OldBarcodeProduct,
            OldNameProduct: old.OldNameProduct,
            OldQuantityProduct: old.OldQuantityProduct,
            OldPriceProduct: old.OldPriceProduct,
            ActualOldPrice: old.OldPriceProduct,
            Barcode: 	 barcode,
            Name: 	 payload.NewNameProduct,
            Price: 0,
            Quantity: int64(payload.NewQuantityProduct),
            Status: 	 "display",
            Quality: payload.Quality,
            ActualQuality: payload.Quality,
        }
        
        location_type := "main"
        if payload.Quality != "lolos" {
            newProduct.QualityText = payload.QualityText
            newProduct.LocationType = &location_type
        }else {
            if old.OldPriceProduct >= 100000 {
                location_type = "staging"    
                newProduct.LocationType = &location_type
                if payload.CategoryID == nil {
                    errChan <- helpers.NewCustomError(400, "Total price >= 100rb, wajib pilih kategori", nil)
                    return
                }
        
                var category models.Category
                if err := config.DB.First(&category, payload.CategoryID).Error; err != nil {
                    errChan <- helpers.NewCustomError(400, "Category tidak ditemukan", err)
                    return
                }
    
                discount := old.OldPriceProduct * (float64(category.DiscountCategory)/100.0)
                discount = math.Round(discount)
                if discount > category.MaxPriceCategory {
                    discount = category.MaxPriceCategory
                } 
                // newProduct.Discount = &discount
                newProduct.Price = old.OldPriceProduct - discount
                newProduct.CategoryID = payload.CategoryID
                newProduct.TagColorID = nil
            } else {
                newProduct.LocationType = &location_type
                if payload.TagColorID == nil {
                    errChan <- helpers.NewCustomError(400, "Total price < 100rb, wajib pilih tag color", nil)
                    return
                }
    
                var color_tag models.ColorTag
                if err := config.DB.First(&color_tag, payload.TagColorID).Error; err != nil {
                    errChan <- helpers.NewCustomError(400, "Color tag tidak ditemukan", err)
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


    // SEQUENTIAL WRITE: Insert Product
    if err := tx.Create(&newProduct).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to insert product", "error": err.Error()})
        return
    }

    //Delete product_old item
    if err := tx.Delete(&old).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to delete old product", "error": err.Error()})
        return
    }
    
    // SEQUENTIAL WRITE: Update User Scan Web
    if err := updateOrCreateDailyScan(tx, user.ID, payload.CodeDocument); err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create or update user scan", "error": err.Error()})
        return
    }

    // SEQUENTIAL WRITE: Update Document status -> in_progress
    if err := tx.Model(&models.Document{}).Where("code = ?", payload.CodeDocument).
        Update("status_document", "inprogress").Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to update document status"})
        return
    }
    
    // Perhitungan dan Update Riwayat
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
    type AddProductPayload struct {
        NameProduct     string  `json:"name_product" binding:"required"`
        QuantityProduct int64     `json:"quantity_product" binding:"required,gt=0"`
        PriceProduct    float64 `json:"price_product" binding:"required,gt=0"`
        Quality         string  `json:"quality" binding:"required,oneof=lolos damaged abnormal"`
        CategoryID      *uint64    `json:"category_id" binding:"omitempty,gt=0"`
        TagColorID      *uint64    `json:"tag_color_id" binding:"omitempty,gt=0"`
        Description     *string  `json:"description" binding:"omitempty"`
    }

    var payload AddProductPayload
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
            case "nameproduct":
                errors["name_product"] = "Nama produk wajib diisi"
            case "quantityproduct":
                if e.Tag() == "required" {
                    errors["quantity_product"] = "Jumlah produk wajib diisi"
                } else {
                    errors["quantity_product"] = "Jumlah harus lebih besar dari 0"
                }
            case "priceproduct":
                if e.Tag() == "required" {
                    errors["price_product"] = "Harga wajib diisi"
                } else {
                    errors["price_product"] = "Harga harus lebih besar dari 0"
                }
           case "quality":
                errors["quality"] = "Kualitas harus salah satu dari: lolos, damaged, abnormal"
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

    user := c.MustGet("auth_user").(models.User)

    tx := config.DB.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

    status := "main"
    if payload.PriceProduct >= 100000 {
        status = "staging"
    }

    // Persiapkan Data Produk Baru
	newProduct := models.Product{
        InboundType:        "manual-inbound",
        OldNameProduct:     payload.NameProduct,
        OldQuantityProduct: int(payload.QuantityProduct),
        OldPriceProduct:    payload.PriceProduct,
        ActualOldPrice:     payload.PriceProduct,
        ActualQuality:      payload.Quality,
		Name:               payload.NameProduct,
        Price:              payload.PriceProduct,
        DisplayPrice:       payload.PriceProduct,
		Quantity:           payload.QuantityProduct,
		Status:             "display",
		Quality:            payload.Quality,
        LocationType:       &status,
	}


    if payload.Quality != "lolos" {
        newProduct.QualityText = payload.Description    
    }else {
        newProduct.QualityText = nil
        if payload.PriceProduct >= 100000 {
            if payload.CategoryID == nil {
                c.JSON(400, gin.H{"success": false, "message": "Total price >= 100rb, wajib pilih kategori"})
                tx.Rollback()
                return
            }
    
            var category models.Category
            if err := tx.First(&category, payload.CategoryID).Error; err != nil {
                tx.Rollback()
                c.JSON(404, gin.H{"success": false, "message": "Category tidak ditemukan", "error": err.Error()})
                return
            }
    
            discount := payload.PriceProduct * (float64(category.DiscountCategory)/100.0)
            discount = math.Round(discount)
            if discount > category.MaxPriceCategory {
                discount = category.MaxPriceCategory
            } 
            // newProduct.Discount = &discount
            newProduct.Price = payload.PriceProduct - discount
            newProduct.DisplayPrice = payload.PriceProduct - discount
            newProduct.CategoryID = payload.CategoryID
            newProduct.TagColorID = nil
    
            // Cek Summary SO Category
            var checkSoCategory models.SummarySoCategory
            if err := tx.Where("type = ?", "process").First(&checkSoCategory).Error; err != nil {
                if err != gorm.ErrRecordNotFound {
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal mengambil summary SO category",
                        "error": err.Error(),
                    })
                    return
                }
            }
    
            if checkSoCategory.ID != 0 {
                columnName := "product_staging"
                switch payload.Quality {
                case "damaged":
                    columnName = "product_damaged"
                case "abnormal":
                    columnName = "product_abnormal"
                }
    
                if err := tx.Model(&checkSoCategory).
                    UpdateColumn(columnName, gorm.Expr(columnName+" + ?", 1)).
                    Error; err != nil {
    
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal memperbarui summary category",
                        "column": columnName,
                        "error": err.Error(),
                    })
                    return
                }
    
                *newProduct.IsSo = "check"
            } 
        } else {
            if payload.TagColorID == nil {
                tx.Rollback()
                c.JSON(400, gin.H{"success": false, "message": "Total price < 100rb, wajib pilih tag color"})
                return
            }
    
            var color_tag models.ColorTag
            if err := tx.First(&color_tag, payload.TagColorID).Error; err != nil {
                tx.Rollback()
                c.JSON(404, gin.H{"success": false, "message": "Color tag tidak ditemukan", "error": err.Error()})
                return
            }
    
            if (payload.PriceProduct < color_tag.MinPriceColor || payload.PriceProduct > color_tag.MaxPriceColor) {
                tx.Rollback()
                c.JSON(400, gin.H{"success": false, "message": "data color tag tidak sesuai dengan harga produk"})
                return
            }
    
            newProduct.Price = color_tag.FixedPriceColor
            newProduct.DisplayPrice = color_tag.FixedPriceColor
            newProduct.TagColorID = payload.TagColorID
            newProduct.CategoryID = nil
    
            // Cek Summary SO Color
            var checkSoColor models.SummarySoColor
            if err := tx.Where("type = ?", "process").First(&checkSoColor).Error; err != nil {
                if err != gorm.ErrRecordNotFound {
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal mengambil summary SO color",
                        "error": err.Error(),
                    })
                    return
                }
            }
    
            // SO COLOR (UPSERT)
            if checkSoColor.ID != 0 {
                if err := incrementOrCreateSoColor(
                    tx,
                    checkSoColor.ID,
                    color_tag.NameColor,
                    payload.Quality,
                ); err != nil {
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal memperbarui summary SO color",
                        "error": err.Error(),
                    })
                    return
                }
    
                *newProduct.IsSo = "check"
            }
        }
    }

	// Generate Barcode
	barcode, err := helpers.GenerateUniqueBarcode(tx, user.ID, "")
    if err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"success": false, "message": "Gagal generate barcode product", "error": err.Error()})
        return
    }

    newProduct.Barcode = barcode
	// Simpan Produk
	if err := tx.Create(&newProduct).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
    
    metadata := map[string]interface{}{
        "product_id": newProduct.ID,
        "barcode": newProduct.Barcode,
        "product_name": newProduct.Name,
        "quantity": newProduct.Quantity,
        "price": newProduct.Price,
        "quality": newProduct.Quality,
    }

    if err := helpers.LogUserAction(user.ID, user.Name, "Menambahkan product di manual inbound", "inbound/manual-inbound", metadata); err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{
            "success": false,
            "message": "Gagal membuat user log",
            "error": err.Error(),
        })
    }

	// Commit Transaksi
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "berhasil menambah data",
		"data":    newProduct,
	})
}

func incrementOrCreateSoColor(tx *gorm.DB,summaryColorID uint,color string,quality string) error {

	var soColor models.SoColor
	err := tx.
		Where("summary_so_color_id = ? AND color = ?", summaryColorID, color).
		First(&soColor).Error

	if err == nil {
        // set kolom sesuai kondisi
        updateData := map[string]interface{}{
			"total_color": gorm.Expr("total_color + ?", 1),
		}
		switch quality {
		case "lolos":
			updateData["product_addition"] = gorm.Expr("product_addition + ?", 1)
		case "abnormal":
			updateData["product_abnormal"] = gorm.Expr("product_abnormal + ?", 1)
		case "damaged":
			updateData["product_damaged"] = gorm.Expr("product_damaged + ?", 1)
		}
		return tx.Model(&soColor).Updates(updateData).Error
	}

	if err == gorm.ErrRecordNotFound {
		// record tidak ada → create baru
		data := models.SoColor{
			SummarySoColorID: uint64(summaryColorID),
            TotalColor: 1,
			Color: color,
		}

		// set kolom sesuai kondisi
		switch quality {
		case "lolos":
			data.ProductAddition = 1
		case "abnormal":
			data.ProductAbnormal = 1
		case "damaged":
			data.ProductDamaged = 1
		}

		return tx.Create(&data).Error
	}

	return err
}


// ============================= STAGGING =============================
type productWithCategoryName struct {
    ID          uint      `json:"id"`
    CodeDocument          string      `json:"code_document"`
    Barcode     string    `json:"barcode"`
    Name        string    `json:"name"`
    Price       float64   `json:"price"`
    OldPrice    float64   `json:"old_price"`
    Status      string    `json:"status"`
    DisplayPrice float64   `json:"display_price"`
    Quantity    int       `json:"quantity"`
    CategoryID  uint      `json:"category_id"`
    CategoryName string   `json:"category_name"` // Harus sesuai dengan alias SELECT
    CreatedAt string   `json:"created_at"` // Harus sesuai dengan alias SELECT
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
            products.code_document,
            products.barcode,
            products.name,
            products.price,
            products.old_price_product AS old_price,
            products.status,
            products.display_price,
            products.quantity,
            products.category_id, 
            products.created_at, 
            categories.name_category AS category_name
        `).
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
        Where("products.status IN ?", []string{"display", "expired", "slow_moving"}).
        Where("products.quality = ?", "lolos").
        Where("products.location_type = ?", "staging").
        Where("products.staging_stage IS NULL")

    // FILTERING
	if q != "" {
        like := "%" + q + "%"

        db = db.Where(`(
            products.barcode LIKE ? OR
            products.name LIKE ? OR
            categories.name_category LIKE ?)`, like, like, like)
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
	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Product Stagging",
			"resource": gin.H{
				"current_page":   page,
				"data":           products,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
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
                Preload("Category").
                Where("location_type = ?", "staging").
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

func UpdateDataProduct(c *gin.Context) {
    type payloadUpdateProduct struct {
        CodeDocument       string  `json:"code_document" binding:"required"`
        NewNameProduct     string  `json:"new_name_product" binding:"required"`
        NewQuantityProduct int     `json:"new_quantity_product" binding:"required,gt=0"`
        NewPriceProduct    float64 `json:"new_price_product" binding:"required,gt=0"`
        CategoryID         uint64  `json:"category_id" binding:"required"`
        // Diskon hanya bisa angka bulat (int) dan rentang 0-100
        Discount           int     `json:"discount" binding:"required,min=0,max=100"`
        OldPriceProduct    float64 `json:"old_price_product" binding:"required,gt=0"`
    }

    user := c.MustGet("auth_user").(models.User)

    barcode := c.Param("barcode")

    var payload payloadUpdateProduct
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
                case "codedocument":
                    errors["code_document"] = "Kode dokumen wajib diisi"
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
                case "categoryid":
                    errors["category_id"] = "Kategori wajib dipilih"
                case "discount":
                    if e.Tag() == "min" || e.Tag() == "max" {
                        errors["discount"] = "Diskon harus berupa angka bulat antara 0 sampai 100"
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
        c.JSON(400, gin.H{"status": false, "message":"Old price tidak boleh kurang dari 100.000"})
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
			c.JSON(http.StatusInternalServerError, gin.H{
                "success": false, 
                "message": "Internal server error occurred and transaction rolled back",
                "error": fmt.Sprintf("%v", r),
            })
            return
		}
	}()

    //cek product
    var product models.Product
    if err := tx.Where("barcode = ?", barcode).
            Where("status IN ?", []string{"display", "expired", "slow_moving"}).
            Where("quality = ?", "lolos").
            Where("staging_stage IS NULL").
            First(&product).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "product not found"})
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query product"})
        }

        tx.Rollback()
        return
    }

    //cek product di apprve queue
    var count int64
    tx.Model(&models.ApproveQueue{}).
       Where("product_id = ? AND type = ? AND status = ?", product.ID, "staging", "1").
       Count(&count)
    if count > 0 {
        tx.Rollback()
        c.JSON(400, gin.H{"success": false, "message": "Product sedang dalam antrian approval"})
        return
    }

    updateData := map[string]interface{}{
        "name": payload.NewNameProduct,
        "old_price_product": payload.OldPriceProduct,
        "quantity": payload.NewQuantityProduct,
        "discount": payload.Discount,
    }

    var discount float64
    var category models.Category
    if err := tx.Where("id = ?", payload.CategoryID).First(&category).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "category not found"})
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query Category"})
        }

        tx.Rollback()
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
    updateData["display_price"] = calculatedPrice - (calculatedPrice * float64(payload.Discount) / 100)
    

    logDetails := map[string]interface{}{
        "status": "Awaiting ADMIN/SPV Approval",
        "changes": map[string]interface{}{
            "new_name":         payload.NewNameProduct,
            "new_quantity":     payload.NewQuantityProduct,
            "new_price":        payload.NewPriceProduct,
            "discount":        payload.Discount,
            "display_price":    updateData["display_price"],
            "old_price":        payload.OldPriceProduct,
        },
        "Before Edit : ": map[string]interface{}{
            "new_name":       product.Name,
            "new_quantity":   product.Quantity,
            "new_price":   product.Price,
            "discount":   product.Discount,
            "display_price": product.DisplayPrice,
            "old_price":      product.OldPriceProduct,
        },
    }

    var actionName, page, tipe string

    if product.LocationType != nil && *product.LocationType == "main" {
        tipe = "inventory"
        page = "inventory/product/category/update"
        actionName = fmt.Sprintf("Edit Product inventory -> barcode: %s", product.Barcode)
    }else {
        tipe = "staging"
        page = "staging/product/update"
        actionName = fmt.Sprintf("Edit Product staging -> barcode: %s", product.Barcode)
    }

    if user.Role.RoleName != "Admin" && user.Role.RoleName != "Spv" {

        new_discount := float64(payload.Discount)
        pID := uint(product.ID)
        approveQueue := models.ApproveQueue{
            UserID: &user.ID,
            ProductID: &pID,
            Type: &tipe,
            CodeDocument: &payload.CodeDocument,
            OldPriceProduct: &payload.OldPriceProduct,
            NewNameProduct: &payload.NewNameProduct,
            NewQuantityProduct: &payload.NewQuantityProduct,
            NewPriceProduct: &payload.NewPriceProduct,
            NewDiscount: &new_discount,
            CategoryID: &category.ID,
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
            NotificationName: actionName,
            Role: "Spv",
            Status: tipe,
            ExternalID: &pID,
            Approved: &approved,
        }

        if err := tx.Create(&notification).Error; err != nil {
            tx.Rollback()
            c.JSON(500, gin.H{"status": false, "error": err.Error()})
            return
        }
    }else {
        if err := tx.Model(&product).Updates(updateData).Error; err != nil {
            tx.Rollback()
            c.JSON(500, gin.H{"status": false, "message": "Gagal update data product", "error": err.Error()})
            return
        }
    }

    if err := helpers.LogUserAction(user.ID, user.Name, actionName, page, logDetails); err != nil {
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
        "message": "Product berhasil di update",
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
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
        Where("products.location_type = ?", "staging").
        Where("products.staging_stage = ?", "process")

    // FILTERING
	if q != "" {
        like := "%" + q + "%"

        db = db.Where(`(
            products.barcode LIKE ? OR
            products.name LIKE ? OR
            categories.name_category LIKE ?)
        `, like, like, like)
	}

    sessionDB := db.Session(&gorm.Session{})

    var total_price float64
    if err := sessionDB.Select("COALESCE(SUM(products.price), 0)").Scan(&total_price).Error; err != nil {
        c.JSON(500, gin.H{"success": false, "message": "gagal menghitung total price product", "error":err.Error()})
		return
    }

	// TOTAL COUNT (for pagination info)
	if err := sessionDB.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "gagal menghitung total product", "error":err.Error()})
		return
	}

	// GET DATA
	if err := db.
        Select(`
            products.id, 
            products.barcode,
            products.name,
            products.price,
            products.old_price_product AS old_price,
            products.status,
            products.display_price,
            products.quantity,
            products.category_id, 
            products.created_at, 
            categories.name_category AS category_name
        `).
		Limit(limit).
		Offset(offset).
		Find(&products).Error; err != nil {

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
			"message": "List Product Filter",
			"resource": gin.H{
                "total_new_price": total_price,
                "data" : gin.H{
                    "current_page":   page,
                    "data":           products,
                    "from":           offset + 1,
                    "last_page":      lastPage,
                    "links":          links,
                    "per_page":       limit,
                    "to":             offset + len(products),
                    "total":          total,
                },
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

func ProductToDamaged(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
    //payload
    type payloadRequest struct {
        Quality string `json:"quality" binding:"required"`
        Description string `json:"description" binding:"required"`
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
				case "quality":
                    errors["quality"] = "Quality wajib diisi"
                case "description":
                    errors["description"] = "Description wajib diisi"
                default:
                    errors[field] = "terdapat error pada field ini"
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"message": "Validasi gagal",
			"errors": errors,
		})
		
		return
	}

    if payload.Quality != "damaged" {
        c.JSON(400, gin.H{"success": false, "message": "quality harus damaged"})
        return
    }

    barcode := c.Param("barcode")

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
            c.JSON(http.StatusInternalServerError, gin.H{
                "success": false, 
                "message": "Internal server error",
                "error": fmt.Sprintf("%v", r),
            })
        }
    }()

    var product models.Product
    if err := tx.Where("barcode = ?", barcode).
        Where("status IN ?", []string{"display", "expired", "slow_moving"}).
        Where("quality = ?", "lolos").
        Where("staging_stage IS NULL").First(&product).Error; err != nil {
        
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(404, gin.H{"success": false, "message": "product tidak ditemukan"})
        }else {
            c.JSON(500, gin.H{"success": false, "message": "failed to update product staging stage", "error": err.Error()})
        }

        return
    }

    rackID := product.RackID
    updateProduct := map[string]interface{}{
        "rack_id": nil,
        "quality": payload.Quality,
        "quality_text": payload.Description,
    }

    if product.CategoryID != nil {
        // Cek Summary SO Category
	    var checkSoCategory models.SummarySoCategory
        if err := tx.Where("type = ?", "process").First(&checkSoCategory).Error; err != nil {
            if err != gorm.ErrRecordNotFound {
                tx.Rollback()
                c.JSON(http.StatusInternalServerError, gin.H{
                    "status": false,
                    "message": "Gagal mengambil summary SO category",
                    "error": err.Error(),
                })
                return
            }
        }

        if checkSoCategory.ID != 0 {
            updateSo := map[string]interface{}{
                "product_damaged":      gorm.Expr("product_damaged + 1"),
            }

            if product.IsSo != nil && *product.IsSo == "check" {
                if product.LocationType != nil && *product.LocationType == "main" {
                    updateSo["product_inventory"] = gorm.Expr("product_inventory - 1")
                }else {
                    updateSo["product_staging"] = gorm.Expr("product_staging - 1")
                }
            }

            if product.IsSo == nil {
                updateProduct["is_so"] = "check"
            }

            if err := tx.Model(&checkSoCategory).Updates(updateSo).Error; err != nil {
                tx.Rollback()
                c.JSON(http.StatusInternalServerError, gin.H{
                    "status": false,
                    "message": "Gagal memperbarui summary category",
                    "error": err.Error(),
                })
                return
            }
        } 
    }

    // update product to damaged
    if err := tx.Model(&product).Updates(updateProduct).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{
            "success": false,
            "message": "Product gagal diupdate",
            "error": err.Error(),
        })
        return
    }

    //update rack
    if rackID != nil {
		result := tx.Model(&models.Rack{}).Where("id = ?", rackID).Updates(map[string]interface{}{
			"total_data":                    gorm.Expr("total_data - ?", 1),
			"total_new_price_product":      gorm.Expr("total_new_price_product - ?", product.Price),
			"total_old_price_product":      gorm.Expr("total_old_price_product - ?", product.OldPriceProduct),
			"total_display_price_product":  gorm.Expr("total_display_price_product - ?", product.DisplayPrice),
		})

		if result.RowsAffected == 0 {
			tx.Rollback()
			c.JSON(404, gin.H{
				"success": false,
				"message": "Rak tidak ditemukan atau tidak berubah",
			})
			return
		}
	}

    //user log
    var page string 
    if *product.LocationType == "main" {
        page = "Invenvtory/product"
    }else {
        page = "Staging/product"
    }

    actionName := fmt.Sprintf("Mengubah status product menjadi Damaged. Barcode: %s",  product.Barcode)
    if err := helpers.LogUserAction(user.ID, user.Name, actionName, page, map[string]interface{}{}); err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"success": false, "message": "Gagal membuat user log", "error": err.Error()})
        return
    }

    //commit
    if err := tx.Commit().Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to commit transaction"})
        return
    }

    c.JSON(200, gin.H{
        "status": true,
        "message": "Product berhasil diubah ke damaged",
    })
}

func ExportStagingProduct(c *gin.Context) {
    loc, err := time.LoadLocation("Asia/Jakarta")
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

	const fileName = "product-staging.xlsx"
	const chunkSize = 500

	f := excelize.NewFile()
	sheetName := "Sheet1"
	streamWriter, err := f.NewStreamWriter(sheetName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Write Header
	headers := []interface{}{
		"Code Document",
		"Old Barcode Product",
		"New Barcode Product",
		"New Name Product",
		"New Quantity Product",
		"New Price Product",
		"Old Price Product",
		"New Date In Product",
		"New Status Product",
		"New Quality",
		"New Category Product",
	}

	cell, _ := excelize.CoordinatesToCellName(1, 1)
	if err := streamWriter.SetRow(cell, headers); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowID := 2
    lastID := uint64(0)

	for {
		var products []models.Product

		err := config.DB.
            Preload("Category").
			Where("id > ?", lastID).
			Where("tag_color_id IS NULL").
			Where("staging_stage IS NULL").
            Where("location_type = ?", "staging").
			Where("status NOT IN ?", []string{"dump", "expired", "sale", "migrate", "repair"}).
			Order("id ASC").
			Limit(chunkSize).
			Find(&products).Error

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if len(products) == 0 {
			break
		}

		for _, p := range products {
            lastID = p.ID
            date_in := p.CreatedAt.In(loc).Format("2006-01-02")
			row := []interface{}{
				*p.CodeDocument,
				*p.OldBarcodeProduct,
				p.Barcode,
				p.Name,
				p.Quantity,
				p.Price,
				p.OldPriceProduct,
				date_in,
				p.Status,
				p.Quality,
				p.Category.NameCategory,
			}

			cell, _ := excelize.CoordinatesToCellName(1, rowID)
			if err := streamWriter.SetRow(cell, row); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			rowID++
		}
	}

	if err := streamWriter.Flush(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Save file
	dir := "./public/exports"
	os.MkdirAll(dir, 0755)
	fullPath := filepath.Join(dir, fileName)
	if err := f.SaveAs(fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/%s", os.Getenv("APP_URL"), fileName)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File berhasil diunduh",
		"url":     downloadURL,
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

        db = db.Where(`(
            products.barcode LIKE ? OR
            products.name LIKE ? OR
            categories.name_category LIKE ?)
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
	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// FINAL RESPONSE
	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Documents",
			"resource": gin.H{
				"current_page":   page,
				"data":           products,
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

// ============================= INVENTORY =============================
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
            "products.old_barcode_product LIKE ? OR " + 
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
            products.old_barcode_product, 
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
	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Product by color",
			"resource": gin.H{
                "total_data":           totalData,
                "total_price_all":      totalPriceAll,
                "tags_summary":         summaries,
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

func GetDetailProduct(c *gin.Context) {
    product_id := c.Param("product_id")

	//inisialisasi query
	baseQuery := config.DB.Model(&models.Product{}).
        Select(`
            products.id AS id,
            products.barcode AS new_barcode,
            products.old_barcode_product AS old_barcode,
            products.name AS new_name,
            products.old_name_product AS old_name,
            products.quantity AS new_quantity,
            products.old_quantity_product AS old_quantity,
            products.price AS new_price,
            products.old_price_product AS old_price,
            products.status AS status,
            COALESCE(color_tags.name_color, categories.name_category) AS category
        `).
        Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
        Where("products.id = ?", product_id).
        Where("products.location_type = ?", "main")

    // Paginate Data
    type productsData struct {
        ID          uint64  `json:"id"`
        NewBarcode  string  `json:"new_barcode"`
        OldBarcode  string  `json:"old_barcode"`
        OldName        string  `json:"old_name"`
        NewName        string  `json:"new_name"`
        OldPrice       float64 `json:"old_price"`
        NewPrice       float64 `json:"new_price"`
        OldQuntity       float64 `json:"old_quantity"`
        NewQuantity       float64 `json:"new_quantity"`
        Status      string  `json:"status"`
        Category   string  `json:"category"`
    }

    var product productsData

    // Ambil data detail
    if err := baseQuery.First(&product).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(404, gin.H{"success": false, "message": "Product tidak ditemukan"})
        }else {
            c.JSON(500, gin.H{"success": false, "message": "Gagal mengambil product", "error": err.Error()})
        }

        return
    }


	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "Detail data product",
			"resource": product,
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
				p.old_barcode_product,
				p.old_name_product,
				p.old_quantity_product,
				p.old_price_product
			FROM products p
			LEFT JOIN categories c ON c.id = p.category_id
			WHERE p.tag_color_id IS NULL
				AND p.category_id IS NOT NULL
				AND p.status IN ('display','expired','slow_moving')
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
					WHEN b.status = 'not_sale' THEN 'display'
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

	argsData := append([]interface{}{}, args...)
	argsData = append(argsData, limit, offset)

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
                    WHEN b.status = 'not_sale' THEN 'display'
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
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"status":  true,
		"message": "List Product by category",
		"resource": gin.H{
			"total":          totalData,
			"data":           results,
			"current_page":   page,
			"last_page":      lastPage,
			"per_page":       limit,
            "links":           links,
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
        Where("products.status IN ?", []string{"display", "expired"}).
        Where("products.location_type = ?", "main").
        Where("products.quality = ?", "lolos").
        Where("(products.warehouse_type IS NULL OR products.warehouse_type = 'type1')")

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(products.barcode LIKE ? OR "+
            "products.old_barcode_product LIKE ? OR " + 
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

func ProductToDump(c *gin.Context) {
    // Definisikan struct untuk request
    // type input struct {
    //     Source string `json:"source" binding:"required,oneof=staging display migrate"`
    // }

    // var payload input
	// if err := c.ShouldBindJSON(&payload); err != nil {
	// 	ve, ok := err.(validator.ValidationErrors)
	// 	if !ok {
	// 		c.JSON(400, gin.H{"status": false, "message": "Format JSON tidak valid"})
	// 		return
	// 	}
	// 	errors := make(map[string]string)
	// 	for _, e := range ve {
	// 		field := strings.ToLower(e.Field())

	// 		switch field {
	// 			case "source":
	// 				if e.Tag() == "required" {
	// 					errors["source"] = "Source wajib diisi"
	// 				}else {
    //                     errors["source"] = "Source tidak valid, hanya diperbolehkan staging, display, migrate"
    //                 }
	// 		}
	// 	}

	// 	c.JSON(http.StatusBadRequest, gin.H{
	// 		"status": false,
	// 		"message": "Validasi gagal",
	// 		"errors": errors,
	// 	})
		
	// 	return
	// }

    //Cek apakah source_type adalah 'product'
    // if input.SourceType != "product" {
    //     c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Hanya tipe 'product' yang diizinkan"})
    //     return
    // }
        
    barcode := c.Param("barcode")
    result := config.DB.Model(&models.Product{}).
        Where("barcode = ?", barcode).Update("status", "dump")

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
    user := c.MustGet("auth_user").(models.User)

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

//Slow Moving Product -> Promo
func GetPromos(c *gin.Context) {
    q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	//inisialisasi query
	baseQuery := config.DB.Model(&models.Promo{}).
        Joins("LEFT JOIN products ON products.id = promos.product_id").
        Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
        Joins("LEFT JOIN categories ON categories.id = products.category_id")

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(promos.name_promo LIKE ? OR "+
            "products.barcode LIKE ? OR " + 
            "products.old_barcode_product LIKE ? OR " + 
            "categories.name_category LIKE ? OR " + 
            "color_tags.name_color LIKE ?)", searchPattern, searchPattern, searchPattern, searchPattern, searchPattern)
	}

    type dataPromo struct {
        ID           uint64     `json:"id"`
        ProductID    uint64     `json:"product_id"`
        NamePromo    string    `json:"name_promo"`
        DiscountPromo float64  `json:"discount_promo"`
        PricePromo   float64   `json:"price_promo"`
        ProductName string `json:"product_name"`
        ProductNewBarcode string `json:"product_new_barcode"`
        ProductOldBarcode string `json:"product_old_barcode"`
        ProductCategory string `json:"product_category"`
        ProductQuantity int64 `json:"product_quantity"`
        ProductOldPrice float64 `json:"product_old_price"`
        ProductNewPrice float64 `json:"product_new_price"`
        ProductStatus string `json:"product_status"`
    }

    var promos []dataPromo
	var totalData int64

    baseQuery.Session(&gorm.Session{}).Count(&totalData)

    // Ambil data detail
    err := baseQuery.Session(&gorm.Session{}).
        Select(`
            promos.id AS id, 
            promos.product_id AS product_id, 
            promos.name_promo AS name_promo, 
            promos.discount_promo AS discount_promo, 
            promos.price_promo AS price_promo,
            products.name AS product_name, 
            products.barcode AS product_new_barcode,
            products.old_barcode_product AS product_old_barcode, 
            COALESCE(color_tags.name_color, categories.name_category) AS product_category,
            products.quantity AS product_quantity, 
            products.old_price_product AS product_old_price, 
            products.price AS product_new_price, 
            products.status AS product_status
        `).
        Order("promos.created_at DESC").
        Limit(limit).Offset(offset).
        Find(&promos).Error

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
			"message": "List Promo",
			"resource": gin.H{
                "total_data":           totalData,
                "data":                 promos,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + int(totalData),
			},
		},
	})
}

func AddPromoProduct(c *gin.Context) {
    type payloadRequest struct {
        NamePromo     string  `json:"name_promo" binding:"required"`
        DiscountPromo float64 `json:"discount_promo" binding:"required,min=0,max=100"`
        ProductID   int64 `json:"product_id" binding:"required"`
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
				case "namepromo":
					if e.Tag() == "required" {
						errors["name_promo"] = "Nama promo wajib diisi"
					}
				case "discountpromo":
					if e.Tag() == "required" {
						errors["discount_promo"] = "Discount promo wajib diisi"
					}else if e.Tag() == "min" {
                        errors["discount_promo"] = "Diskon tidak boleh negatif"
                    } else if e.Tag() == "max" {
                        errors["discount_promo"] = "Diskon tidak boleh lebih dari 100%"
                    }
				case "productid":
					if e.Tag() == "required" {
						errors["product_id"] = "Product id wajib diisi"
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
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal server error", "error": fmt.Sprintf("%v", r)})
        }
    }()

    // Cari produk untuk mendapatkan harga asli
	var product models.Product
	if err := tx.Where("status IN ?", []string{"display", "expired"}).First(&product, payload.ProductID).Error; err != nil {
        tx.Rollback()
		c.JSON(404, gin.H{"status": false, "message": "Produk tidak ditemukan"})
		return
	}

    // Hitung harga setelah promo
	// Rumus: Harga - (Harga * (Diskon / 100))
	pricePromo := product.Price - (product.Price * (payload.DiscountPromo / 100))

    promo := models.Promo{
        NamePromo: payload.NamePromo,
        DiscountPromo: payload.DiscountPromo,
        ProductID: uint64(payload.ProductID),
        PricePromo: pricePromo,
    }

    //create promo
    if err := tx.Create(&promo).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": false, "error": err.Error()})
		return
	}

    //update product status
	if err := tx.Model(&product).Update("status", "promo").Error; err != nil {
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
		"message":  "berhasil membuat promo product",
	})
}

// ============================= REPAIR STATION =============================
//Abnormal
func GetProductAbnormal(c *gin.Context) {
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
        Where("products.is_so IS NULL").
        Where("products.quality = ?", "abnormal").
        Where("products.status NOT IN ?", []string{"migrate", "sale", "dump", "scrap_qcd"})

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(products.barcode LIKE ? OR "+
            "products.old_barcode_product LIKE ? OR " + 
            "products.name LIKE ?)", searchPattern, searchPattern, searchPattern)
	}

    // Paginate Data
    type productsData struct {
        ID                 uint      `json:"id"`
        Source          string    `json:"source"` // product | bundle
        Barcode            string    `json:"barcode"`
        Name               string    `json:"name"`
        Category       string    `json:"category"`
        Price              float64   `json:"price"`
        CreatedAt          time.Time `json:"created_at"`
        StatusProduct             string    `json:"status_product"`
        QualityProduct             string    `json:"quality_product"`
        QualityTextProduct             string    `json:"quality_text_product"`
        DisplayPrice       float64   `json:"display_price"`
        OldBarcodeProduct  string    `json:"old_barcode_product"`
        OldNameProduct     string    `json:"old_name_product"`
        OldQuantityProduct int       `json:"old_quantity_product"`
        OldPriceProduct    float64   `json:"old_price_product"`
        IsSo               string    `json:"is_so"`
    }

    var products []productsData
	var totalData int64

    baseQuery.Session(&gorm.Session{}).Count(&totalData)

    // Ambil data detail
    err := baseQuery.Session(&gorm.Session{}).
        Select(`
            products.id AS id,
            CASE 
                WHEN products.location_type = 'main' THEN 'display'
                ELSE 'staging'
            END AS source, 
            products.barcode AS barcode, 
            products.name AS name, 
            COALESCE(color_tags.name_color, categories.name_category) AS category,
            products.price AS price, 
            products.created_at AS created_at, 
            products.status AS status_product, 
            products.quality AS quality_product, 
            products.quality_text AS quality_text_product, 
            products.display_price AS display_price, 
            products.old_barcode_product AS old_barcode_product, 
            products.old_name_product AS old_name_product, 
            products.old_quantity_product AS old_quantity_product, 
            products.old_price_product AS old_price_product,
            products.is_so AS is_so
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
			"message": "List Product Abnormal",
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

func AbnormalToDisplay(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
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

    productID, err := strconv.ParseUint(c.Param("product_id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID format"})
        return
    }
    productIDUint := uint(productID)

    tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}
    
	// Pastikan Rollback dipanggil jika ada panic atau error di tengah proses
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
                "success": false, 
                "message": "Internal server error occurred and transaction rolled back",
                "error": fmt.Sprintf("%v", r),
            })
            return
		}
	}()

    //load data product
    var product models.Product

    //cek product
    if err := tx.Where("id = ?", productIDUint).
            Where("quality = ?", "abnormal").
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
        "name": payload.NewNameProduct,
        "old_price_product": payload.OldPriceProduct,
        "quality": "lolos",
        "quality_text": nil,
    }

    var discount float64
    if payload.OldPriceProduct >= 100000 {
        if payload.CategoryID == nil {
            tx.Rollback()
            c.JSON(400, gin.H{"status": false, "message": "Total price >= 100rb, wajib pilih kategori"})
            return
        }

        var category models.Category
        if err := tx.First(&category, payload.CategoryID).Error; err != nil {
            tx.Rollback()
            c.JSON(404, gin.H{"status": false, "message": "Category tidak ditemukan"})
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
        updateData["location_type"] = "staging"
        updateData["category_id"] = category.ID
        updateData["tag_color_id"] = nil
        updateData["display_price"] = calculatedPrice

        // Cek summary so category
        if *product.IsSo == "check" {
            var checkSoCategory models.SummarySoCategory
            if err := tx.Where("type = ?", "process").First(&checkSoCategory).Error; err != nil {
                if err != gorm.ErrRecordNotFound {
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal mengambil summary SO category",
                        "error": err.Error(),
                    })
                    return
                }
            }

            if checkSoCategory.ID != 0 {
                columnLocation := "product_staging"
                if product.LocationType != nil && *product.LocationType == "main" {
                    columnLocation = "product_inventory"
                }

                updateSo := map[string]interface{}{
                    "product_abnormal":      gorm.Expr("product_abnormal - 1"),
                    columnLocation :         gorm.Expr(columnLocation+" + 1"),
                }

                if err := tx.Model(&checkSoCategory).Updates(updateSo).Error; err != nil {
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal memperbarui summary category",
                        "error": err.Error(),
                    })
                    return
                }
            } 
        }
    }else {
        var color_tag models.ColorTag
        if payload.TagColorID == nil {
            tx.Rollback()
            c.JSON(400, gin.H{"status": false, "message": "Total price < 100rb, wajib pilih color"})
            return
        }

        if err := tx.First(&color_tag, payload.TagColorID).Error; err != nil {
            tx.Rollback()
            c.JSON(404, gin.H{"status": false, "message": "Color tag tidak ditemukan"})
            return
        }

        updateData["price"] = color_tag.FixedPriceColor
        updateData["location_type"] = "main"
        updateData["category_id"] = nil
        updateData["tag_color_id"] = color_tag.ID
        updateData["display_price"] = color_tag.FixedPriceColor

        // Cek Summary SO Color
        if *product.IsSo == "check" {
            var checkSoColor models.SummarySoColor
            if err := tx.Where("type = ?", "process").First(&checkSoColor).Error; err != nil {
                if err != gorm.ErrRecordNotFound {
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal mengambil summary SO color",
                        "error": err.Error(),
                    })
                    return
                }
            }
            // SO COLOR (UPSERT)
            if checkSoColor.ID != 0 {
                if err := incrementOrCreateSoColor(
                    tx,
                    checkSoColor.ID,
                    color_tag.NameColor,
                    "lolos",
                ); err != nil {
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal memperbarui summary SO color",
                        "error": err.Error(),
                    })
                    return
                }
            }
        }
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

    logDetails := map[string]interface{}{
        "changes": map[string]interface{}{
            "new_name":         payload.NewNameProduct,
            "new_quantity":     payload.NewQuantityProduct,
            "new_price":        payload.NewPriceProduct,
            "display_price":    updateData["display_price"],
            "old_price":        payload.OldPriceProduct,
        },
        "Before Edit : ": map[string]interface{}{
            "new_name":       product.Name,
            "new_quantity":   product.Quantity,
            "new_price":   product.Price,
            "display_price": product.DisplayPrice,
            "old_price":      product.OldPriceProduct, // Harga lama di Staging
        },
    }
    
    action := fmt.Sprintf("Memindahkan product abnormal: %s (%s) ke display", product.Name, product.Barcode)
    if err := helpers.LogUserAction(user.ID, user.Name, action, "repair-station/abnormal/to-display", logDetails); err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal membuat user log action", "error": err.Error()})
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

func ExportAbnormalProduct(c *gin.Context) {
	loc, _ := time.LoadLocation("Asia/Jakarta")

	const chunkSize = 500
	fileName := fmt.Sprintf("product-abnormal-%s.xlsx",
		time.Now().In(loc).Format("2006-01-02"),
	)

	f := excelize.NewFile()
	sheetName := "Sheet1"

	streamWriter, err := f.NewStreamWriter(sheetName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// ================= HEADER =================
	headers := []interface{}{
		"Code Document",
		"Old Barcode Product",
		"New Barcode Product",
		"Keterangan",
		"New Name Product",
		"New Quantity Product",
		"New Price Product",
		"Old Price Product",
		"New Status Product",
		"New Category Product",
		"New Tag Product",
	}

	cell, _ := excelize.CoordinatesToCellName(1, 1)
	if err := streamWriter.SetRow(cell, headers); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowID := 2
	var lastID uint64 = 0

	for {
		var products []models.Product

		err := config.DB.
            Preload("Category").
            Preload("ColorTag").
			Where("id > ?", lastID).
			Where("quality = ?", "abnormal").
			Where("is_so IS NULL").
			Where("status NOT IN ?", []string{"migrate", "sale"}).
			Order("id ASC").
			Limit(chunkSize).
			Find(&products).Error

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if len(products) == 0 {
			break
		}

		for _, p := range products {
			lastID = p.ID

			row := []interface{}{
				*p.CodeDocument,
				*p.OldBarcodeProduct,
				p.Barcode,
				p.QualityText,
				p.Name,
				p.Quantity,
				p.Price,
				p.OldPriceProduct,
				p.Status,
				p.Category.NameCategory,
				p.ColorTag.NameColor,
			}

			cell, _ := excelize.CoordinatesToCellName(1, rowID)
			streamWriter.SetRow(cell, row)
			rowID++
		}
	}

	if err := streamWriter.Flush(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Save file
	dir := "./public/exports"
	os.MkdirAll(dir, 0755)

	fullPath := filepath.Join(dir, fileName)
	if err := f.SaveAs(fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/%s", os.Getenv("APP_URL"), fileName)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File berhasil diunduh",
		"url":     downloadURL,
	})
}


//Damaged
func GetProductDamageds(c *gin.Context) {
    q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 30
	offset := (page - 1) * limit

	//inisialisasi query
	baseQuery := config.DB.Table("products").
        Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
        Where("NOT EXISTS (SELECT 1 FROM repair_document_items rdi WHERE rdi.product_id = products.id)").
        Where("products.quality = ?", "damaged")

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
        Source      string  `json:"source"`
        IsSo       string  `json:"is_so"`
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
                WHEN products.location_type = 'main' THEN 'display'
                ELSE 'staging'
            END AS source, 
            products.is_so AS is_so,
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
			"message": "List Product Damaged",
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

//Non
func GetProductNon(c *gin.Context) {
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
        Where("products.is_so IS NULL").
        Where("products.quality = ?", "non").
        Where("NOT EXISTS (SELECT 1 FROM repair_document_items rdi WHERE rdi.product_id = products.id)").
        Where("products.status NOT IN ?", []string{"migrate", "sale", "dump", "scrap_qcd"})

	// Searching
	if q != "" {
		searchPattern := "%" + q + "%"
		baseQuery = baseQuery.Where("(products.barcode LIKE ? OR "+
            "products.old_barcode_product LIKE ? OR " + 
            "products.name LIKE ?)", searchPattern, searchPattern, searchPattern)
	}

    // Paginate Data
    type productsData struct {
        ID                 uint      `json:"id"`
        Source          string    `json:"source"` // product | bundle
        Barcode            string    `json:"barcode"`
        Name               string    `json:"name"`
        Category       string    `json:"category"`
        Price              float64   `json:"price"`
        CreatedAt          time.Time `json:"created_at"`
        StatusProduct             string    `json:"status_product"`
        QualityProduct             string    `json:"quality_product"`
        QualityTextProduct             string    `json:"quality_text_product"`
        DisplayPrice       float64   `json:"display_price"`
        OldBarcodeProduct  string    `json:"old_barcode_product"`
        OldNameProduct     string    `json:"old_name_product"`
        OldQuantityProduct int       `json:"old_quantity_product"`
        OldPriceProduct    float64   `json:"old_price_product"`
    }

    var products []productsData
	var totalData int64

    baseQuery.Session(&gorm.Session{}).Count(&totalData)

    // Ambil data detail
    err := baseQuery.Session(&gorm.Session{}).
        Select(`
            products.id AS id,
            CASE 
                WHEN products.location_type = 'main' THEN 'display'
                ELSE 'staging'
            END AS source, 
            products.barcode AS barcode, 
            products.name AS name, 
            COALESCE(color_tags.name_color, categories.name_category) AS category,
            products.price AS price, 
            products.created_at AS created_at, 
            products.status AS status_product, 
            products.quality AS quality_product, 
            products.quality_text AS quality_text_product, 
            products.display_price AS display_price, 
            products.old_barcode_product AS old_barcode_product, 
            products.old_name_product AS old_name_product, 
            products.old_quantity_product AS old_quantity_product, 
            products.old_price_product AS old_price_product
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
			"message": "List Product Non",
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

func NonToDisplay(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
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

    productID, err := strconv.ParseUint(c.Param("product_id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID format"})
        return
    }
    productIDUint := uint(productID)

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

    //load data product
    var product models.Product

    //cek product
    if err := tx.Where("id = ?", productIDUint).
            Where("quality = ?", "non").
            First(&product).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "product not found"})
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query product_old"})
        }

        tx.Rollback()
        return
    }

    // create update data
    updateData := map[string]interface{}{
        "name": payload.NewNameProduct,
        "quantity": payload.NewQuantityProduct,
        "quality": "lolos",
        "quality_text": nil,
    }

    var discount float64
    if payload.OldPriceProduct >= 100000 {
        if payload.CategoryID == nil {
            tx.Rollback()
            c.JSON(400, gin.H{"status": false, "message": "Total price >= 100rb, wajib pilih kategori"})
            return
        }

        var category models.Category
        if err := tx.First(&category, payload.CategoryID).Error; err != nil {
            tx.Rollback()
            c.JSON(404, gin.H{"status": false, "message": "Category tidak ditemukan"})
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
        updateData["location_type"] = "staging"
        updateData["category_id"] = category.ID
        updateData["tag_color_id"] = nil
        updateData["display_price"] = calculatedPrice

        // Cek summary so category
        if *product.IsSo == "check" {
            var checkSoCategory models.SummarySoCategory
            if err := tx.Where("type = ?", "process").First(&checkSoCategory).Error; err != nil {
                if err != gorm.ErrRecordNotFound {
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal mengambil summary SO category",
                        "error": err.Error(),
                    })
                    return
                }
            }

            if checkSoCategory.ID != 0 {
                columnLocation := "product_staging"
                if product.LocationType != nil && *product.LocationType == "main" {
                    columnLocation = "product_inventory"
                }

                updateSo := map[string]interface{}{
                    "product_non":      gorm.Expr("product_non - 1"),
                    columnLocation :         gorm.Expr(columnLocation+" + 1"),
                }

                if err := tx.Model(&checkSoCategory).Updates(updateSo).Error; err != nil {
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal memperbarui summary category",
                        "error": err.Error(),
                    })
                    return
                }
            } 
        }
    }else {
        var color_tag models.ColorTag
        if payload.TagColorID == nil {
            tx.Rollback()
            c.JSON(400, gin.H{"status": false, "message": "Total price < 100rb, wajib pilih color"})
            return
        }

        if err := tx.First(&color_tag, payload.TagColorID).Error; err != nil {
            tx.Rollback()
            c.JSON(404, gin.H{"status": false, "message": "Color tag tidak ditemukan"})
            return
        }

        updateData["price"] = color_tag.FixedPriceColor
        updateData["location_type"] = "main"
        updateData["category_id"] = nil
        updateData["tag_color_id"] = color_tag.ID
        updateData["display_price"] = color_tag.FixedPriceColor

        // Cek Summary SO Color
        if *product.IsSo == "check" {
            var checkSoColor models.SummarySoColor
            if err := tx.Where("type = ?", "process").First(&checkSoColor).Error; err != nil {
                if err != gorm.ErrRecordNotFound {
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal mengambil summary SO color",
                        "error": err.Error(),
                    })
                    return
                }
            }
            // SO COLOR (UPSERT)
            if checkSoColor.ID != 0 {
                if err := incrementOrCreateSoColor(
                    tx,
                    checkSoColor.ID,
                    color_tag.NameColor,
                    "lolos",
                ); err != nil {
                    tx.Rollback()
                    c.JSON(http.StatusInternalServerError, gin.H{
                        "status": false,
                        "message": "Gagal memperbarui summary SO color",
                        "error": err.Error(),
                    })
                    return
                }
            }
        }
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

    logDetails := map[string]interface{}{
        "changes": map[string]interface{}{
            "new_name":         payload.NewNameProduct,
            "new_quantity":     payload.NewQuantityProduct,
            "new_price":        payload.NewPriceProduct,
            "display_price":    updateData["display_price"],
            "old_price":        payload.OldPriceProduct,
        },
        "Before Edit : ": map[string]interface{}{
            "new_name":       product.Name,
            "new_quantity":   product.Quantity,
            "new_price":   product.Price,
            "display_price": product.DisplayPrice,
            "old_price":      product.OldPriceProduct, // Harga lama di Staging
        },
    }
    
    action := fmt.Sprintf("Memindahkan product non: %s (%s) ke display", product.Name, product.Barcode)
    if err := helpers.LogUserAction(user.ID, user.Name, action, "repair-station/non/to-display", logDetails); err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal membuat user log action", "error": err.Error()})
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

//RepairDocument (document for damaged & non)
func GetRepairDocuments(c *gin.Context) {
	db := config.DB

	q := c.Query("q")
	type_document := c.Param("type")

    if type_document != "damaged" && type_document != "non" {
        c.String(404, "Page not found")
        return
    }

	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "15"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	offset := (page - 1) * limit

    type repairDocumentResponse struct {
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

	var results []repairDocumentResponse
	var total int64

	baseQuery := db.Table("repair_documents rd").
		Select(`
			rd.id AS id,
			rd.code_document AS code_document,
			rd.status AS status,
			rd.total_product AS total_product,
			rd.total_new_price AS total_new_price,
			rd.total_old_price AS total_old_price,
			rd.created_at AS created_at,
			u.id   AS user_id,
			u.name AS user_name
		`).
		Joins("LEFT JOIN users u ON u.id = rd.user_id").
        Where("type_document = ?", type_document)

	// ===== Filter q =====
	if q != "" {
		like := "%" + q + "%"

		baseQuery = baseQuery.Where(`
			rd.code_document LIKE ?
			OR u.name LIKE ?
			OR EXISTS (
				SELECT 1 
                FROM repair_document_items rdi
                JOIN products p ON p.id = rdi.product_id
                WHERE rdi.repair_document_id = rd.id
                AND p.barcode LIKE ?
			)
		`,
			like, like, like,
		)
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
		Order("rd.created_at DESC").
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
		"message": fmt.Sprintf("List Data %s Documents", type_document),
		"data": gin.H{
			"current_page": page,
			"per_page":     limit,
			"total":        total,
			"data":         results,
			"links" :		links,
		},
	})
}

func ExportRepairDocument(c *gin.Context) {
    doc_id := c.Param("doc_id")
    type_document := c.Param("type")

    if type_document != "damaged" && type_document != "non" {
        c.JSON(400, gin.H{
            "success": false,
            "message": "Document type is invalid",
        })
        return
    }

	loc, _ := time.LoadLocation("Asia/Jakarta")
	const chunkSize = 500

    var document models.RepairDocument
    if err := config.DB.Where("id = ? AND type_document = ?", doc_id, type_document).
        First(&document).Error; err != nil {
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

	fileName := fmt.Sprintf("%s.xlsx", document.CodeDocument)

	f := excelize.NewFile()
	sheetName := "Sheet1"

	streamWriter, err := f.NewStreamWriter(sheetName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// ================= HEADER =================
	headers := []interface{}{
		// "Source",
		"Code Document",
		"Old Barcode Product",
		"New Barcode Product",
		"Name Product",
		"Category Product",
		"Quantity Product",
		"Old Price Product",
		"New Price Product",
		"Date In",
		"Status",
		"Description",
		"Color Tag",
		"Discount",
        "CreatedAt",
	}

	//set width colom
    startCol,_ := excelize.ColumnNumberToName(1)
    endCol, _ := excelize.ColumnNumberToName(len(headers))

    if err := f.SetColWidth(sheetName, startCol, endCol, 20.0); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
        return
    }

    //style header
    styleID, _ := f.NewStyle(&excelize.Style{
        Font: &excelize.Font{
            Bold:  true,
            Size:  11,
            Color: "FFFFFF",
        },
        Alignment: &excelize.Alignment{
            Vertical:   "center",
        },
        Fill: excelize.Fill{
            Type:    "pattern",
            Pattern: 1,
            Color:   []string{"4472C4"},
        },
    })

    // Bold style untuk summary
    boldStyle, _ := f.NewStyle(&excelize.Style{
        Font: &excelize.Font{
            Bold: true,
        },
        NumFmt: 3,
    })

    // ROW 1 - 3 (SUMMARY)
    rows := [][]interface{}{
        {
            excelize.Cell{StyleID: boldStyle, Value: "Total Product"},
            fmt.Sprintf("%d pcs", document.TotalProduct),
        },
        {
            excelize.Cell{StyleID: boldStyle, Value: "Total New Price"},
            "Rp " + helpers.HumanizeNumber(document.TotalNewPrice),
        },
        {
            excelize.Cell{StyleID: boldStyle, Value: "Total Old Price"},
            "Rp " + helpers.HumanizeNumber(document.TotalOldPrice),
        },
        {""}, // baris kosong
    }

    for i, row := range rows {
        cell, _ := excelize.CoordinatesToCellName(1, i+1)
        if err := streamWriter.SetRow(cell, row); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
            return
        }
    }   

    var headerRow []interface{}
    for _, h := range headers {
        headerRow = append(headerRow, excelize.Cell{
            StyleID: styleID,
            Value:   h,
        })
    }

    //header row 5
    cell, _ := excelize.CoordinatesToCellName(1, 5)
    if err := streamWriter.SetRow(cell, headerRow); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
        return
    }

	lastID := 0
	rowIndex := 6

	for {
		query := `
			SELECT 
				p.id,
				CASE 
					WHEN p.quality = 'migrate' THEN 'migrate'
					WHEN p.location_type = 'main' THEN 'display'
					ELSE 'staging' 
				END AS source,
				COALESCE(rd.status, 'N/A') AS document_status,
				COALESCE(p.code_document, 'NULL') AS code_document,
				COALESCE(p.old_barcode_product, 'NULL') AS old_barcode_product,
				p.barcode,
				p.name,
				COALESCE(c.name_category, 'NULL') AS category,
				p.quantity,
				p.old_price_product,
				p.price,
				COALESCE(p.quality_text, 'NULL') AS quality_text,
				p.status,
				COALESCE(ct.name_color, 'NULL') AS color_tag,
				p.discount,
				p.created_at
			FROM products p
			LEFT JOIN categories c 
				ON c.id = p.category_id
			LEFT JOIN color_tags ct 
				ON ct.id = p.tag_color_id
			JOIN repair_document_items rdi 
				ON rdi.product_id = p.id
			JOIN repair_documents rd 
				ON rd.id = rdi.repair_document_id
			WHERE p.id > ?
			AND rd.id = ?
			ORDER BY p.id ASC
			LIMIT ?
		`
		rows, err := config.DB.Raw(query, lastID, document.ID, chunkSize).Rows()
		if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}

		count := 0

		for rows.Next() {
			var (
				idProd int
				source, status, docStatus, code string
				oldBarcode, newBarcode string
				name, category, qualityText, colorTag string
				qty int
				oldPrice, newPrice float64
                discount sql.NullFloat64
				createdAt time.Time
			)

			err := rows.Scan(
				&idProd,
				&source,
				&docStatus,
				&code,
				&oldBarcode,
				&newBarcode,
				&name,
				&category,
				&qty,
				&oldPrice,
				&newPrice,
				&qualityText,
				&status,
				&colorTag,
				&discount,
				&createdAt,
			)
			if err != nil {
				rows.Close()
                c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}

			dateIn := createdAt.In(loc).Format("2006-01-02")
            var discountValue float64
            if discount.Valid {
                discountValue = discount.Float64
            } else {
                discountValue = 0 // atau sesuai kebutuhan
            }

			row := []interface{}{
				// source,
				code,
				" " + oldBarcode,
				" " + newBarcode,
				name,
				category,
				qty,
				oldPrice,
				newPrice,
				dateIn,
                status,
				qualityText,
				colorTag,
				discountValue,
				createdAt.In(loc).Format("2006-01-02 15:04"),
			}

			cell, _ := excelize.CoordinatesToCellName(1, rowIndex)
			if err := streamWriter.SetRow(cell, row); err != nil {
				rows.Close()
                c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}

			lastID = idProd
			rowIndex++
			count++
		}

		rows.Close()

		// Kalau tidak ada data lagi, stop loop
		if count == 0 {
			break
		}
	}

	if err := streamWriter.Flush(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Save file
	dir := "./public/exports"
	os.MkdirAll(dir, 0755)

	fullPath := filepath.Join(dir, fileName)
	if err := f.SaveAs(fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/%s", os.Getenv("APP_URL"), fileName)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File berhasil diunduh",
		"url":     downloadURL,
	})
}

func ExportAllProductRepairByType(c *gin.Context) {
    type_document := c.Param("type")

    if type_document != "damaged" && type_document != "non" {
        c.JSON(400, gin.H{
            "success": false,
            "message": "Document type is invalid",
        })
        return
    }

    f := excelize.NewFile()
    //mengubah nama sheet
    sheet1 := "All Products"
    f.SetSheetName("Sheet1", sheet1)

    if err := services.WriteAllProductRepair(f, sheet1, type_document); err != nil {
        c.JSON(500, gin.H{
            "success": false,
            "message": "Gagal memproses " + sheet1,
            "error": err.Error(),
        })
        return
    }

    sheet2 := "Document Summary"
    f.NewSheet(sheet2)

    if err := services.WriteRepairSummaryStream(f, sheet2, type_document); err != nil {
        c.JSON(500, gin.H{
            "success": false,
            "message": "Gagal memproses " + sheet2,
            "error": err.Error(),
        })
        return
    }

    //save file
    fileName := fmt.Sprintf("All_%s_%s.xlsx",type_document,time.Now().Format("2006-01-02"))
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

func DetailRepairDocuments(c *gin.Context) {
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
	doc_id := c.Param("doc_id")	
    type_document := c.Param("type")

    if type_document != "damaged" && type_document != "non" {
        c.String(404, "Page not found")
        return
    }

	limit := 30
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	offset := (page - 1) * limit

	var doc models.RepairDocument
	db := config.DB

	// 1. Cari repair doc
	err := db.Where("id = ? AND type_document = ?", doc_id, type_document).First(&doc).Error

	//Jika TIDAK ADA repair document
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(404, gin.H{
			"success": false,
			"message": "document tidak ditemukan",
		})
		
		return
	}

	//Jika ADA sesi aktif
	// Ambil item lewat repair_items → products
	type ItemResponse struct {
		ID              uint64    `json:"id"`
		NameProduct  string    `json:"name_product"`
		Barcode      string    `json:"barcode"`
		NewPrice        float64   `json:"new_price"`
		OldPrice        float64   `json:"old_price"`
		Status          string    `json:"status"`
		Source          string    `json:"source"`
		IsSo           string    `json:"is_so"`
		Category        string    `json:"category"`
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
	}

	var items []ItemResponse
	var total int64

	baseQuery := db.Table("repair_document_items rdi").
		Select(`
			rdi.id, 
            products.name AS name_product, 
            products.barcode AS barcode, 
            products.price AS new_price, 
            products.old_price_product AS old_price, 
            products.status AS status, 
			CASE 
                WHEN products.location_type = 'main' THEN 'display'
                ELSE 'staging'
            END AS source, 
            products.is_so AS is_so,
            COALESCE(color_tags.name_color, categories.name_category) AS category,
			products.created_at,
			products.updated_at
		`).
		Joins("JOIN products ON products.id = rdi.product_id").
		Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
		Where("rdi.repair_document_id = ?", doc.ID)

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
		"message": "Detil Dokumen",
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

func GetActiveSessionRepairDoc(c *gin.Context) {
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
    type_document := c.Param("type")

    if type_document != "damaged" && type_document != "non" {
        c.String(404, "Page not found")
        return
    }

	limit := 15
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	offset := (page - 1) * limit

	var doc models.RepairDocument
	db := config.DB

	// 1. Cari sesi aktif
	err := db.
        Where("type_document = ?", type_document).
		Where("user_id = ? AND status = ?", user.ID, "proses").
		First(&doc).Error

	//Jika TIDAK ADA sesi aktif → buat baru
	if errors.Is(err, gorm.ErrRecordNotFound) {
		now := time.Now()
		prefix := fmt.Sprintf("%02d%d", now.Month(), now.Year())

		var lastDoc models.RepairDocument
		nextNumber := 1

        if type_document == "damaged" {
            prefix = fmt.Sprintf("DMG%s", prefix)
        }else {
            prefix = fmt.Sprintf("NON%s", prefix)
        }

		db.
			Where("code_document LIKE ?", prefix+"%").
			Order("id DESC").
			First(&lastDoc)

		if lastDoc.ID != 0 {
			lastCode := lastDoc.CodeDocument
            if len(lastCode) > len(prefix) {
                // Ambil setelah prefix
                runningStr := lastCode[len(prefix):]
                if n, err := strconv.Atoi(runningStr); err == nil {
                    nextNumber = n + 1
                }
            }

		}

		code := fmt.Sprintf("%s%04d", prefix, nextNumber)

		doc = models.RepairDocument{
			CodeDocument: code,
            TypeDocument: type_document,
			UserID:       user.ID,
			Status:       "proses",
		}

		if err := db.Create(&doc).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Gagal membuat sesi document",
			})
			return
		}

		links := helpers.BuildPaginationLinks(c, page, 1)

		c.JSON(http.StatusCreated, gin.H{
			"success": true,
			"message": "Sesi Document Baru Berhasil Dibuat",
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
	// Ambil item lewat damaged_document_item → products
	type ItemResponse struct {
		ID              uint64    `json:"id"`
		NameProduct  string    `json:"name_product"`
		Barcode      string    `json:"barcode"`
		NewPrice        float64   `json:"new_price"`
		OldPrice        float64   `json:"old_price"`
		Status          string    `json:"status"`
		Source          string    `json:"source"`
        IsSo           string    `json:"is_so"`
		Category        string    `json:"category"`
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
	}

	var items []ItemResponse
	var total int64

	baseQuery := db.Table("repair_document_items rdi").
		Select(`
			rdi.id, 
            products.name AS name_product, 
            products.barcode AS barcode, 
            products.price AS new_price, 
            products.old_price_product AS old_price, 
            products.status AS status, 
			CASE 
                WHEN products.location_type = 'main' THEN 'display'
                ELSE 'staging'
            END AS source, 
            products.is_so AS is_so,
            COALESCE(color_tags.name_color, categories.name_category) AS category,
			products.created_at,
			products.updated_at
		`).
		Joins("JOIN products ON products.id = rdi.product_id").
		Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
        Joins("LEFT JOIN categories ON categories.id = products.category_id").
		Where("rdi.repair_document_id = ?", doc.ID)

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
		"message": "Sesi Document Aktif Ditemukan",
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

func AddProductToRepairDocument(c *gin.Context) {
    type payloadRequest struct {
        RepairDocumentID uint   `json:"repair_document_id" binding:"required"`
        Barcode          string `json:"barcode" binding:"required"`
        TypeDocument     string `json:"type_document" binding:"required,oneof=damaged non"`
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

			switch field {
			case "RepairDocumentID":
				errorsMap["repair_document_id"] = "Repair Document ID wajib diisi"
			case "Barcode":
				errorsMap["barcode"] = "Barcode wajib diisi"
			case "TypeDocument":
				errorsMap["type_document"] = "Type Document harus bernilai damaged atau non"
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

	// Ambil Damaged Document
	var doc models.RepairDocument
	if err := tx.Where("id = ? AND type_document = ?", payload.RepairDocumentID, payload.TypeDocument).
        First(&doc).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Dokumen tidak ditemukan",
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
	if err := tx.Where("barcode = ?", payload.Barcode).
		First(&product).Error; err != nil {

		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Produk tidak ditemukan",
			"error": err.Error(),
		})

		return
	}

	if product.Quality != payload.TypeDocument {
		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Status produk harus " + payload.TypeDocument,
		})
		return
	}

	// Cek apakah produk sedang di damaged
	var count int64
	tx.Model(&models.RepairDocumentItem{}).Where("product_id = ?", product.ID).
		Count(&count)

	if count > 0 {
		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Produk sudah masuk dalam document / sedang di document lain",
		})
		return
	}

	// Insert ke repair_document_items
	item := models.RepairDocumentItem{
		RepairDocumentID: doc.ID,
		ProductID:       uint(product.ID),
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
			"message": "Document tidak ditemukan atau tidak berubah",
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
		"message": fmt.Sprintf("Produk masuk list %s document", payload.TypeDocument),
	})
}

func RemoveProductRepairDocument(c *gin.Context) {
    type payloadRequest struct {
        RepairItemID uint   `json:"repair_item_id" binding:"required"`
        TypeDocument     string `json:"type_document" binding:"required,oneof=damaged non"`
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

			switch field {
			case "RepairItemID":
				errorsMap["repair_item_id"] = "Repair Item ID wajib diisi"
			case "TypeDocument":
				errorsMap["type_document"] = "Type Document harus bernilai damaged atau non"
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

	// Ambil Item Document
	var repair_item models.RepairDocumentItem
	if err := tx.First(&repair_item, payload.RepairItemID).Error; err != nil {
		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": "Item tidak ditemukan",
		})
		return
	}

	// Ambil Document
	var doc models.RepairDocument
	if err := tx.Where("id = ? AND type_document = ?", repair_item.RepairDocumentID, payload.TypeDocument).
        First(&doc, repair_item.RepairDocumentID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Document tidak ditemukan",
		})
		return
	}

	if doc.Status != "proses" {
		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Product tidak bisa dihapus. Dokumen terkunci / sudah selesai",
		})
		return
	}

	// Ambil Produk
	var product models.Product
	if err := tx.First(&product, repair_item.ProductID).Error; err != nil {

		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Produk tidak ditemukan",
			"error": err.Error(),
		})

		return
	}

	// Recalculate Total
    total_product := doc.TotalProduct - 1
    if total_product <= 0 {
        total_product = 0
    }
    total_new_price := doc.TotalNewPrice - product.Price
    if total_new_price <= 0 {
        total_new_price = 0
    }
    total_old_price := doc.TotalOldPrice - product.OldPriceProduct
    if total_old_price <= 0 {
        total_old_price = 0
    }

	result := tx.Model(&doc).Updates(map[string]interface{}{
		"total_product":                    total_product,
		"total_new_price":      total_new_price,
		"total_old_price":      total_old_price,
	})

	if result.RowsAffected == 0 {
		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": "Document tidak ditemukan atau tidak berubah",
		})
		return
	}

    if err := tx.Delete(&repair_item).Error;  err != nil {
        tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal menghapus item",
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
		"message": fmt.Sprintf("Produk berhasil dihapus dari %s document", payload.TypeDocument),
	})
}

func AddAllProductToRepairDocument(c *gin.Context) {
	type payloadRequest struct {
        RepairDocumentID uint   `json:"repair_document_id" binding:"required"`
        TypeDocument     string `json:"type_document" binding:"required,oneof=damaged non"`
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

			switch field {
			case "RepairDocumentID":
				errorsMap["repair_document_id"] = "Repair Document ID wajib diisi"
			case "TypeDocument":
				errorsMap["type_document"] = "Type Document harus bernilai damaged atau non"
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

	// Ambil repair document aktif
	var doc models.RepairDocument
	if err := tx.Where("id = ?", payload.RepairDocumentID).
        Where("type_document = ?", payload.TypeDocument).
        First(&doc).Error; err != nil {
		tx.Rollback()
		c.JSON(404, gin.H{
			"success": false,
			"message": fmt.Sprintf("%s document tidak ditemukan", payload.TypeDocument),
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
		Where("products.quality = ?", payload.TypeDocument).
		Where("NOT EXISTS (SELECT 1 FROM repair_document_items rdi WHERE rdi.product_id = products.id)").
		Count(&count).Error
        
    if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": "Gagal menghitung total product", "error": err.Error()})
		return
	}

	if count <= 0 {
		tx.Rollback()
		c.JSON(200, gin.H{"success": true, "message": "Tidak ada product " + payload.TypeDocument})
		return
	}

	// insert items
	err = tx.Exec(`
		INSERT INTO repair_document_items (repair_document_id, product_id, created_at, updated_at)
		SELECT ?, p.id, NOW(), NOW()
		FROM products p
		WHERE p.quality = ?
		AND NOT EXISTS (
			SELECT 1 FROM repair_document_items rdi WHERE rdi.product_id = p.id
		)
	`, doc.ID, payload.TypeDocument).Error

	if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{
			"success": false,
			"message": "Product Gagal Ditambahkan",
			"error":   err.Error(),
		})
		return
	}

	// menghitung total repair document
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
		FROM repair_document_items rdi
		JOIN products p ON p.id = rdi.product_id
		WHERE rdi.repair_document_id = ?
	`, doc.ID).Scan(&totals).Error

	if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": fmt.Sprintf("Gagal menghitung total %s document", payload.TypeDocument), "error": err.Error()})
		return
	}

	err = tx.
		Model(&doc).
		Updates(map[string]interface{}{
			"total_product":   totals.TotalProduct,
			"total_new_price": totals.TotalNewPrice,
			"total_old_price": totals.TotalOldPrice,
		}).Error

	if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"success": false, "message": fmt.Sprintf("Gagal update %s document", payload.TypeDocument), "error": err.Error()})
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
		"message": fmt.Sprintf("Semua product %s berhasil ditambahkan ke %s document", payload.TypeDocument, payload.TypeDocument),
		"data": gin.H{
			"total_product":   totals.TotalProduct,
			"total_new_price": totals.TotalNewPrice,
			"total_old_price": totals.TotalOldPrice,
		},
	})
}

func LockRepairDocument(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	doc_id := c.Param("doc_id")
    type_document := c.Param("type")

    if type_document != "damaged" && type_document != "non" {
        c.String(404, "Page not found")
        return
    }

	var doc models.RepairDocument
	db := config.DB

	// 1. Cari doc
	err := db.Where("id = ? AND type_document = ?", doc_id, type_document).First(&doc).Error

	//Jika TIDAK ADA damaged document
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(404, gin.H{
			"success": false,
			"message": "document tidak ditemukan",
		})
		
		return
	}

	if doc.Status != "proses" {
		c.JSON(422, gin.H{
			"success": false,
			"message": "document sudah terkunci / selesai",
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
			"message": "document gagal diupdate",
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "document berhasil terkunci",
	})
}

func FinishRepairDocument(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	doc_id := c.Param("doc_id")
    type_document := c.Param("type")

    if type_document != "damaged" && type_document != "non" {
        c.String(404, "Page not found")
        return
    }

	var doc models.RepairDocument
	db := config.DB

	// 1. Cari Damaged doc
	err := db.Where("id = ? AND type_document = ?", doc_id, type_document).First(&doc).Error

	//Jika TIDAK ADA repair document
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(404, gin.H{
			"success": false,
			"message": "document tidak ditemukan",
		})
		
		return
	}

	if doc.Status == "selesai" {
		c.JSON(422, gin.H{
			"success": false,
			"message": "document sudah terkunci / selesai",
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
			"message": "document gagal diupdate",
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "document berhasil selesai",
	})
}

// ============================= OUTBOUND =============================
//sale
func GetProductsForSale(c *gin.Context) {
    type ProductResult struct {
        Barcode            string    `json:"barcode"`
        Name               string    `json:"name"`
        Category       string    `json:"category"`
        Price              float64   `json:"price"`
		CreatedDate       time.Time `json:"created_date"`
    }

	q := strings.TrimSpace(c.Query("q"))

	// PAGINATION
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 15
	offset := (page - 1) * limit

	// SEARCH CONDITION
	searchCondition := ""
	args := []interface{}{}

	if q != "" {
		searchCondition = `
			AND (
				barcode LIKE ?
				OR name LIKE ?
				OR category LIKE ?
			)
		`
		search := "%" + q + "%"
		args = append(args, search, search, search)
	}

	// UNION QUERY (DATA)
	dataQuery := fmt.Sprintf(`
		SELECT * FROM (
			SELECT
				p.barcode AS barcode,
				p.name AS name,
				c.name_category AS category,
				p.price AS price,
				p.created_at AS created_date
			FROM products p
			LEFT JOIN categories c ON c.id = p.category_id
			WHERE p.tag_color_id IS NULL
				AND p.category_id IS NOT NULL
				AND p.status != 'sale'
				AND p.quality = 'lolos'

			UNION ALL

			SELECT
				b.barcode AS barcode,
				b.name_bundle AS name,
				c.name_category AS category,
				b.total_price_custom AS price,
				b.created_at AS created_date
			FROM bundles b
			LEFT JOIN categories c ON c.id = b.category_id
			WHERE b.total_price_custom >= 100000
				AND b.tag_color_id IS NULL
				AND b.category_id IS NOT NULL
				AND b.status != 'sale'
				AND (b.warehouse_type IS NULL OR b.warehouse_type = 'type1')
		) x
		WHERE 1=1
		%s
		ORDER BY created_date DESC
		LIMIT ? OFFSET ?
	`, searchCondition)

	argsData := append([]interface{}{}, args...)
	argsData = append(argsData, limit, offset)

	var results []ProductResult
	if err := config.DB.Raw(dataQuery, argsData...).Scan(&results).Error; err != nil {
		c.JSON(500, gin.H{"status": false, "error": err.Error()})
		return
	}

	// COUNT QUERY
	countQuery := fmt.Sprintf(`
        SELECT COUNT(*) FROM (
            SELECT 
                p.barcode AS barcode,
                p.name AS name,
                c.name_category AS category
            FROM products p
            LEFT JOIN categories c ON c.id = p.category_id
            WHERE p.tag_color_id IS NULL
                AND p.category_id IS NOT NULL
				AND p.status != 'sale'
				AND p.quality = 'lolos'

            UNION ALL

            SELECT 
                b.barcode AS barcode,
                b.name_bundle AS name,
                c.name_category AS category
            FROM bundles b
            LEFT JOIN categories c ON c.id = b.category_id
            WHERE b.total_price_custom >= 100000
                AND b.tag_color_id IS NULL
                AND b.category_id IS NOT NULL
                AND b.status != 'sale'
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
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"status":  true,
		"message": "List Data Products",
		"resource": gin.H{
			"total":          totalData,
			"data":           results,
			"current_page":   page,
			"last_page":      lastPage,
			"per_page":       limit,
            "links":           links,
		},
	})
}
