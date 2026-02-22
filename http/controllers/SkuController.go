package controllers

import (
	"errors"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"math"
	"os"
	"strconv"

	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//================= Handler Route =================
//inbound proses
func ImportExcelSkuHandler(c *gin.Context) {
    file, err := c.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
        return
    }

    ext := filepath.Ext(file.Filename)
    if ext != ".xlsx" && ext != ".xls" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type"})
        return
    }

    tempPath := fmt.Sprintf("uploads/expedisiData/SKU/%s", file.Filename)
    c.SaveUploadedFile(file, tempPath)

    code, headers, rows, err := processExcelSkuFile(tempPath, file.Filename)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Import berhasil",
        "data": gin.H{
            "code_document": code,
            "headers": headers,
            "file_name": file.Filename,
            "fileDetails": gin.H{
                "total_column_count": len(headers),
                "total_row_count": rows,
            },
        },
    })
}
func MapAndMergeHeadersSku(c *gin.Context) {
    type MapAndMergeRequest struct {
        HeaderMappings map[string][]string `json:"headerMappings" validate:"required,min=1"`
        CodeDocument   string              `json:"code_document" validate:"required"`
    }

    // validation
	validate := validator.New()

    var req MapAndMergeRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "detail": err.Error()})
        return
    }
    if err := validate.Struct(&req); err != nil {
        c.JSON(http.StatusUnprocessableEntity, gin.H{"errors": err.Error()})
        return
    }

    // Begin transaction
    tx := config.DB.Begin()
    if tx.Error != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot begin transaction"})
        return
    }

    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"error": "panic occurred"})
        }
    }()

    // Fetch generates by code_document only
    var generates []models.Generate
    if err := tx.Where("code_document = ?", req.CodeDocument).Find(&generates).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed get generates", "detail": err.Error()})
        return
    }

    // If none found, maybe return empty success or error
    if len(generates) == 0 {
        tx.Rollback()
        c.JSON(http.StatusNotFound, gin.H{"error": "no generate data found for code_document"})
        return
    }

	// ambil sample data pertama
	var sample map[string]interface{}
	if err := jsonUnmarshalToMap(generates[0].Data, &sample); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid generate data"})
		return
	}
	// validasi header yang dipilih user sebelum diproses
	for _, selectedHeaders := range req.HeaderMappings {
		for _, userSel := range selectedHeaders {
			key := strings.TrimSpace(userSel)
			if _, ok := sample[key]; !ok {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": fmt.Sprintf("header '%s' tidak ditemukan di file upload", key),
				})
				return
			}
		}
	}


    // Worker pool: parse each generate.Data concurrently and extract fields
    store := &mergedStore{}
    type job struct {
        rawJSON string
    }
    jobs := make(chan job, 4000)
    numWorkers := 8
    var wg sync.WaitGroup

    headerMap := req.HeaderMappings

    // Start workers
    for w := 0; w < numWorkers; w++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := range jobs {
                // parse json into map[string]interface{}
                var m map[string]interface{}
                decErr := jsonUnmarshalToMap(j.rawJSON, &m)
                if decErr != nil {
                    // ignore this row but log
                    log.Printf("failed unmarshal generate data: %v", decErr)
                    continue
                }
                // For each template header, and its userSelectedHeaders, pick values
                for templateHeader, selectedHeaders := range headerMap {
                    for _, userSel := range selectedHeaders {
                        // normalize keys just in case whitespace
                        key := strings.TrimSpace(userSel)
                        if v, ok := m[key]; ok && v != nil {
                            // convert v to string
                            val := interfaceToString(v)
                            // push into store under templateHeader
                            store.push(templateHeader, val)
                        }
                    }
                }
            }
        }()
    }

    // feed jobs
    for _, g := range generates {
        jobs <- job{rawJSON: g.Data}
    }
    close(jobs)
    wg.Wait()

    // After all workers done, insert into product_old
    records := make([]models.SkuProductOld, 0)
    maxIdx := len(store.OldBarcodeProduct)
    for i := 0; i < maxIdx; i++ {
        noResi := safeSliceGet(store.OldBarcodeProduct, i)
        nama := safeSliceGet(store.OldNameProduct, i)
        qtyStr := safeSliceGet(store.OldQuantityProduct, i)
        priceStr := safeSliceGet(store.OldPriceProduct, i)

        // Trim name if too long (PHP checked >2000 then cut to 250)
        if len(nama) > 2000 {
            log.Printf("Nama produk terlalu panjang (>2000): %s...", safeTruncate(nama, 50))
            nama = safeTruncate(nama, 250)
        }
        // parse qty
        qty := helpers.ParseIntOrDefault(qtyStr, 0)
        price := helpers.ParseFloatOrDefault(priceStr, 0.0)

        records = append(records, models.SkuProductOld{
            CodeDocument:       req.CodeDocument,
            OldBarcodeProduct:  noResi,
            OldNameProduct:     nama,
            OldQuantityProduct: int64(qty),
            OldPriceProduct:    price,
            CreatedAt:          time.Now(),
            UpdatedAt:          time.Now(),
        })
    }

    // Chunk insert using GORM CreateInBatches inside transaction
    chunkSize := 500
    if len(records) > 0 {
        if err := tx.CreateInBatches(records, chunkSize).Error; err != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed insert products", "detail": err.Error()})
            return
        }
    }

    // delete generate rows for this code_document
    if err := tx.Where("code_document = ?", req.CodeDocument).Delete(&models.Generate{}).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed delete generates", "detail": err.Error()})
        return
    }

    // compute totalPrice by summing old_price_product
    // var totalPriceRow struct {
    //     Sum float64
    // }
    // if err := tx.Model(&models.SkuProductOld{}).
    //     Select("COALESCE(SUM(old_price_product),0) as sum").
    //     Where("code_document = ?", req.CodeDocument).
    //     Scan(&totalPriceRow).Error; err != nil {
    //     tx.Rollback()
    //     c.JSON(http.StatusInternalServerError, gin.H{"error": "failed sum prices", "detail": err.Error()})
    //     return
    // }
    // totalPrice := totalPriceRow.Sum

    // fetch document
    var doc models.Document
    if err := tx.Where("code = ?", req.CodeDocument).First(&doc).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "document not found", "detail": err.Error()})
        return
    }

    // create riwayat_check (RiwayatCheck)
    user := c.MustGet("auth_user").(models.User)

    // log user action (implement function sesuai kebutuhan)
    metadata := map[string]interface{}{}
    if err := helpers.LogUserAction(user.ID, user.Name, "Import inbound sku batch " + req.CodeDocument, "inbound-sku/import", metadata); err != nil {
        // non-fatal — hanya log
        log.Printf("warn: logUserAction failed: %v", err)
        return
    }

    // commit transaction
    if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "message":       "Berhasil migrasi data, siap untuk proses scanning.",
        "total_imported": len(records),
    })
}

//manifest inbound
func SkuDocuments(c *gin.Context) {
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

	db := config.DB.Model(&models.Document{}).
        Where("document_product_type = ?", "sku")

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
			"message": "List Sku Documents",
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
func DetailSkuDocument(c *gin.Context) {
	query := c.Query("q")
	code_document := c.Param("code")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	limit := 50
	offset := (page - 1) * limit

	var productOlds []models.SkuProductOld
	var total int64

	db := config.DB.Model(&models.SkuProductOld{}).
		Where("code_document = ?", code_document)
		
	if query != "" {
		db = db.Where(
			"(old_barcode_product LIKE ? OR old_name_product LIKE ?)", "%"+query+"%", "%"+query+"%",
		)
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
			"message": "code document tidak ditemukan",
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
func UpdateSkuProductOld(c *gin.Context)  {
	product_id := c.Param("product_id")

	type payloadRequest struct {
        ActualQuantity         *int64  `json:"actual_quantity" binding:"required"`
        DamagedQuantity         *int64  `json:"damaged_quantity" binding:"required"`
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
                case "actualquantity":
                    errors["actual_quantity"] = "Actual quantity wajib diisi"
                case "damagedquantity":
                    errors["damaged_quantity"] = "Damaged quantity baru wajib diisi"
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

    if *payload.ActualQuantity < 0 {
        c.JSON(http.StatusBadRequest, gin.H{
            "status": false,
            "message": "Actual Quantity minimal 0",
        })
        return
    }

    if *payload.DamagedQuantity < 0 {
        c.JSON(http.StatusBadRequest, gin.H{
            "status": false,
            "message": "Damaged Quantity minimal 0",
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
            c.JSON(http.StatusInternalServerError, gin.H{
                "success": false, 
                "message": "Internal server error",
                "error": fmt.Sprintf("%v", r),
            })
        }
    }()
	// Query ProductOld
	var product models.SkuProductOld
	// Ambil product + document dalam 1 relasi (lebih efisien)
	if err := tx.
		Preload("Document").
		First(&product, product_id).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{
				"success": false,
				"message": "Product old not found",
			})
			return
		}else {
			c.JSON(500, gin.H{
				"success": false,
				"message": "server error: " + err.Error(),
			})
		}

        tx.Rollback()
		return
	}

	// Cek status document
	if product.Document.StatusDocument == "done" {
		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Dokumen sudah berstatus Done. Tidak bisa diubah.",
		})
		return
	}

	initialStock := product.OldQuantityProduct
	totalFound := *payload.ActualQuantity + *payload.DamagedQuantity

	if totalFound > initialStock {
		excess := totalFound - initialStock

		tx.Rollback()
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": fmt.Sprintf(
				"Total barang (%d) melebihi stok awal (%d). Kelebihan %d item.",
				totalFound, initialStock, excess,
			),
			"data": gin.H{
				"initial_stock": initialStock,
				"total_found":   totalFound,
				"excess":        excess,
			},
		})
		return
	}

	lostQuantity := initialStock - totalFound

	if err := tx.Model(&product).Updates(map[string]interface{}{
		"actual_quantity_product":  payload.ActualQuantity,
		"damaged_quantity_product": payload.DamagedQuantity,
		"lost_quantity_product":    lostQuantity,
	}).Error; err != nil {

		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal memperbarui data",
            "error": err.Error(),
		})
		return
	}

	//commit
    if err := tx.Commit().Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to commit transaction"})
        return
    }

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Data berhasil diperbarui. Lost Qty: %d", lostQuantity),
		"data":    product,
	})
}
func SkuCustomBarcode(c *gin.Context) {

	var request struct {
		CodeDocument string `json:"code_document" binding:"required"`
		CustomBarcode string `json:"custom_barcode" binding:"required"`
	}

	// ✅ Validasi body
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{
			"success": false,
			"message": "code_document atau custom_barcode wajib diisi",
		})
		return
	}

	// ✅ TRANSACTION
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

	// ✅ Update langsung (tanpa SELECT dulu → cepat)
	result := tx.
		Model(&models.Document{}).
		Where("code = ?", request.CodeDocument).
		Where("document_product_type = ?", "sku").
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
func DestroySkuDocument(c *gin.Context) {
	code_document := c.Param("code")

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

    // ===== Ambil document =====
	var document models.Document
	if err := tx.Where("code = ? AND document_product_type = ?", code_document, "sku").
        First(&document).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Dokumen tidak ditemukan",
            "error": err.Error(),
		})
		return
	}

    // HAPUS sku_product_olds YANG TIDAK DIPAKAI
    deleteUnusedProductOld := `
        DELETE FROM sku_product_olds
        WHERE code_document = ?
    `

	if err := tx.Exec(deleteUnusedProductOld, document.Code).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal hapus product_old", "error": err})
        return
    }

    //hapus sku_product
	if err := tx.Where("code_document", document.Code).Delete(&models.SkuProduct{}).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal hapus sku product", "error": err})
        return
    }

    //hapus data generate jika ada
	if err := tx.Where("code_document", document.Code).Delete(&models.Generate{}).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal hapus data generate", "error": err})
        return
    }

    //hapus riwayat check
	if err := tx.Where("code_document", document.Code).Delete(&models.RiwayatCheck{}).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal hapus riwayat check", "error": err})
        return
    }

    // ===== Hapus file jika ada =====
	if document.NameDocument != "" {
		filePath := filepath.Join("uploads/expedisiData/SKU", document.NameDocument)

		if _, err := os.Stat(filePath); err == nil {
			_ = os.Remove(filePath) // tidak fatal kalau gagal
		}
	}

    // Hapus document
    if err := tx.Delete(&document).Error; err != nil {
        tx.Rollback()
        c.JSON(500, gin.H{"status": false, "message": "Gagal menghapus document", "error": err.Error()})
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
func ExportSkuDocument(c *gin.Context) {
	code := c.Param("code")

	var doc models.Document
	if err := config.DB.Where("code = ?", code).First(&doc).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Dokumen tidak ditemukan",
		})
		return
	}

	// cek apakah ada data
	var exists bool
	if err := config.DB.
		Model(&models.SkuProductOld{}).
		Select("count(1) > 0").
		Where("code_document = ?", doc.Code).
		Find(&exists).Error; err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal cek data",
            "error": err.Error(),
		})
		return
	}

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Data produk tidak ditemukan untuk dokumen ini",
		})
		return
	}

	// ====== Prepare file ======

	fileName := fmt.Sprintf("%s.xlsx",doc.Code)

	folder := "./public/exports/sku"
	os.MkdirAll(folder, os.ModePerm)

	filePath := filepath.Join(folder, fileName)

	// ====== Create Excel ======
	f := excelize.NewFile()
	sheet := "Sheet1"
	streamWriter, err := f.NewStreamWriter(sheet)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal membuat file excel",
            "error": err.Error(),
		})
		return
	}

	// Header
	headers := []interface{}{
		"No",
		"Code Document",
		"Barcode",
		"Product Name",
		"Price",
		"QTY Awal",
		"QTY Aktual",
		"QTY Damaged",
		"QTY Lost",
	}

    //set columnt width
    // startCol,_ := excelize.ColumnNumberToName(2)
    endCol, _ := excelize.ColumnNumberToName(len(headers))

    if err := f.SetColWidth(sheet, "B", endCol, 20.0); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal mengatur lebar kolom header",
            "error": err.Error(),
		})
		return
    }

    if err := f.SetColWidth(sheet, "A", "A", 5.0); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal mengatur lebar kolom header",
            "error": err.Error(),
		})
		return
    }

    //style header
    styleID, _ := f.NewStyle(&excelize.Style{
        Font: &excelize.Font{
            Bold:  true,
            Size:  11,
        },
        Alignment: &excelize.Alignment{
            Vertical:   "center",
        },
        Fill: excelize.Fill{
            Type:    "pattern",
            Pattern: 1,
            Color:   []string{"e5e7eb"},
        },
        Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
    })

    // Bold style untuk summary
    bodyStyle, _ := f.NewStyle(&excelize.Style{
        NumFmt: 3,
        Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
    })

	var headerRow []interface{}
    for _, h := range headers {
        headerRow = append(headerRow, excelize.Cell{
            StyleID: styleID,
            Value:   h,
        })
    }

    cell, _ := excelize.CoordinatesToCellName(1, 1)
    if err := streamWriter.SetRow(cell, headerRow); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal mengatur kolom header",
            "error": err.Error(),
		})
		return
    }


	// ====== Streaming Data (Chunked) ======
	const chunkSize = 2
	rowNumber := 2
    lastId := 0

	for {
		var products []models.SkuProductOld

		result := config.DB.
			Where("code_document = ?", doc.Code).
            Where("id > ?", lastId).
            Order("id ASC").
			Limit(chunkSize).
			Find(&products)

		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Gagal mengambil data",
                "error": result.Error.Error(),
			})
			return
		}

		if len(products) == 0 {
			break
		}

		for i, p := range products {
            lastId = int(p.ID)
			row := []interface{}{
                excelize.Cell{StyleID: bodyStyle, Value: i + 1},
                excelize.Cell{StyleID: bodyStyle, Value: p.CodeDocument},
                excelize.Cell{StyleID: bodyStyle, Value: p.OldBarcodeProduct},
                excelize.Cell{StyleID: bodyStyle, Value: p.OldNameProduct},
                excelize.Cell{StyleID: bodyStyle, Value: p.OldPriceProduct},
                excelize.Cell{StyleID: bodyStyle, Value: p.OldQuantityProduct},
                excelize.Cell{StyleID: bodyStyle, Value: p.ActualQuantityProduct},
                excelize.Cell{StyleID: bodyStyle, Value: p.DamagedQuantityProduct},
                excelize.Cell{StyleID: bodyStyle, Value: p.LostQuantityProduct},
			}

			cell, _ := excelize.CoordinatesToCellName(1, rowNumber)
			if err := streamWriter.SetRow(cell, row); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
                    "success": false,
                    "error": err.Error(),
                })
                return
			}
			rowNumber++
		}
	}

	if err := streamWriter.Flush(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := f.SaveAs(filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal menyimpan file",
		})
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/sku/%s",
		os.Getenv("APP_URL"),
		fileName,
	)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File berhasil diexport",
		"data": gin.H{
			"download_url": downloadURL,
			"file_name":    fileName,
		},
	})
}
func SubmitSku(c *gin.Context) {
	code_document := c.Param("code")
	db := config.DB

	// ===== Pastikan dokumen ada =====
	var document models.Document
	if err := db.
		Where("code = ?", code_document).
		Where("document_product_type = ?", "sku").
		First(&document).Error; err != nil {

		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Dokumen tidak ditemukan",
		})
		return
	}

	if document.StatusDocument == "done" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Dokumen sudah disubmit sebelumnya",
		})
		return
	}

	// ===== Ambil semua product lama =====
	var oldProducts []models.SkuProductOld
	if err := db.
		Where("code_document = ?", document.Code).
		Find(&oldProducts).Error; err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal mengambil data produk",
            "error": err.Error(),
		})
		return
	}

	if len(oldProducts) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Tidak ada produk untuk disubmit",
		})
		return
	}

	// ===== Validasi Konsistensi =====
	var invalidProducts []gin.H

	for _, p := range oldProducts {
		initial := p.OldQuantityProduct
		total := p.ActualQuantityProduct +
			p.DamagedQuantityProduct +
			p.LostQuantityProduct

		if total != initial {
            if initial > 0 && total == 0 {
                invalidProducts = append(invalidProducts, gin.H{
                    "barcode": p.OldBarcodeProduct,
                    "name":    p.OldNameProduct,
                    "issue":   "Belum divalidasi (Semua nilai masih 0)",
                })
            }else {
                diff := total - initial
                status := "Missing"
                if diff > 0 {
                    status = "Excess"
                }
    
                invalidProducts = append(invalidProducts, gin.H{
                    "barcode":       p.OldBarcodeProduct,
                    "name":          p.OldNameProduct,
                    "initial_stock": initial,
                    "total_input":   total,
                    "issue":         fmt.Sprintf("Perhitungan tidak sesuai (%s %d item)", status, abs(diff)),
                })
            }
		}
	}

	if len(invalidProducts) > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": fmt.Sprintf("Terdapat %d produk tidak valid", len(invalidProducts)),
			"data": gin.H{
				"invalid_count": len(invalidProducts),
				"list":          invalidProducts,
			},
		})
		return
	}

	// ===== Transaction =====
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

	now := time.Now()
	user := c.MustGet("auth_user").(models.User) // asumsi sudah diset middleware

	var skuBulk []models.SkuProduct
	var stagingBulk []models.Product
    const chunkSize = 1000
    var totalsku, totalDataDamaged int64
    customeBarcode := ""
    if document.CustomBarcode != nil {
        customeBarcode = *document.CustomBarcode
    }

	for _, old := range oldProducts {
        totalsku += old.ActualQuantityProduct
		// Move ke SKU
		if old.ActualQuantityProduct > 0 {
			skuBulk = append(skuBulk, models.SkuProduct{
				CodeDocument:   old.CodeDocument,
				BarcodeProduct: old.OldBarcodeProduct,
				NameProduct:    old.OldNameProduct,
				PriceProduct:   old.OldPriceProduct,
				QuantityProduct: old.ActualQuantityProduct,
				CreatedAt:      now,
				UpdatedAt:      now,
			})

            if len(skuBulk) == chunkSize {
                if err := tx.Create(&skuBulk).Error; err != nil {
                    tx.Rollback()        
                    c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal create sku product", "error": err.Error()})
                    return  
                }
                skuBulk = skuBulk[:0] // reset tanpa alokasi ulang
            }
		}

		// Move ke staging damaged
		if old.DamagedQuantityProduct > 0 {
            quality := "damaged"
            location := "staging"
            totalDataDamaged += old.DamagedQuantityProduct
			// ⚡ Lebih efisien → generate bulk tanpa loop insert
			for i := 0; i < int(old.DamagedQuantityProduct); i++ {
                barcode, err := helpers.GenerateUniqueBarcode(tx, user.ID, customeBarcode)
                if err != nil {
                    tx.Rollback()        
                    c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal generate barcode product", "error": err.Error()})
                    return            
                }
				stagingBulk = append(stagingBulk, models.Product{
					CodeDocument:     &document.Code,
                    InboundType: "sku",
					OldBarcodeProduct: &old.OldBarcodeProduct,
                    OldNameProduct: old.OldNameProduct,
					OldPriceProduct:  old.OldPriceProduct,
                    OldQuantityProduct: 1,
					Barcode: barcode,
					Name:   old.OldNameProduct,
					Quantity: 1,
					Price:  old.OldPriceProduct,
                    DisplayPrice: old.OldPriceProduct,
                    ActualQuality: quality,
                    ActualOldPrice: old.OldPriceProduct,
					Status: "display",
                    Discount: helpers.Float64Ptr(0),
                    Quality: quality,
                    QualityText: &quality,
                    LocationType: &location,
					WarehouseType:    "type1",
					CreatedAt:        now,
					UpdatedAt:        now,
				})

                if len(stagingBulk) == chunkSize {
                    if err := tx.Create(&stagingBulk).Error; err != nil {
                        tx.Rollback()        
                        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal create product damaged", "error": err.Error()})
                        return  
                    }
                    stagingBulk = stagingBulk[:0] // reset tanpa alokasi ulang
                }
			}
		}
	}

	// ===== inser sisa =====
	if len(skuBulk) > 0 {
		if err := tx.Create(&skuBulk).Error; err != nil {
            tx.Rollback()        
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal create sisa sku product", "error": err.Error()})
            return  
        }
	}
    if len(stagingBulk) > 0 {
        if err := tx.Create(&stagingBulk).Error; err != nil {
            tx.Rollback()        
            c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal create product damaged", "error": err.Error()})
            return  
        }
    }

	// Update status document
	if err := tx.Model(&document).
		Update("status_document", "done").Error; err != nil {

		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal update status document",
            "error": err.Error(),
		})
		return
	}

    //create riwayat check
    // coming soon

	if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed commit", "detail": err.Error()})
        return
    }

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Validasi sukses. Produk berhasil disubmit",
		"data": gin.H{
			"total_moved_to_sku":              totalsku,
			"total_moved_to_staging_damaged":  totalDataDamaged,
			"code_document":                   document.Code,
		},
	})
}

//inventory - by sku
func SkuProducts(c *gin.Context) {
	query := c.Query("q")
	code_document := c.Param("code")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	limit := 50
	offset := (page - 1) * limit

    // Ambil document berdasarkan code_document
	var document models.Document
	if err := config.DB.Where("code = ?", code_document).First(&document).Error; err != nil {
		c.JSON(404, gin.H{
			"success": false,
			"message": "document tidak ditemukan",
            "error": err.Error(),
		})
		return
	}

	var products []models.SkuProduct
	var total int64

	db := config.DB.Model(&models.SkuProduct{}).
		Where("code_document = ?", code_document)
		
	if query != "" {
		db = db.Where(
			"(barcode_product LIKE ? OR name_product LIKE ?)", "%"+query+"%", "%"+query+"%",
		)
	}

	// TOTAL COUNT (for pagination info)
	if err := db.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": err})
		return
	}

	// Query Sku Product
	err := db.
		Limit(limit).
		Offset(offset).
		Find(&products).Error
	if err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// pagination links
	lastPage := int(math.Ceil(float64(total) / float64(limit)))
	links := helpers.BuildPaginationLinks(c, page, lastPage)

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
				"data":  products,
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
func DetailSkuProduct(c *gin.Context) {
    sku_product_id := c.Param("sku_product_id")
    var sku_product models.SkuProduct
    if err := config.DB.First(&sku_product, sku_product_id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(404, gin.H{"success": false, "message": "Sku Product tidak ditemukan"})
        }else {
            c.JSON(404, gin.H{"success": false, "message": "Gagal mengambil data sku product", "error": err.Error()})
        }

        return
    }

    c.JSON(200, gin.H{
        "success": true,
        "message": "Detail Sku Product",
        "resource": sku_product,
    })
}
func SkuStoreDamaged(c *gin.Context) {
	db := config.DB
	productID := c.Param("sku_product_id")

    type payloadRequest struct {
        DamagedQuantity int64  `json:"damaged_quantity" binding:"required,min=1"`
        Description     string `json:"description"`
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
                case "damagedquantity":
                    errors["damaged_quantity"] = "Damaged quantity minimal 1"
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

	user := c.MustGet("auth_user").(models.User)
    
	err := db.Transaction(func(tx *gorm.DB) error {
		var product models.SkuProduct

		// lock update
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
            Preload("Document").
			First(&product, productID).Error; err != nil {

			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("not_found")
			}
			return err
		}

		if product.QuantityProduct < payload.DamagedQuantity {
			return fmt.Errorf("stock_failed")
		}

		qtyBefore := product.QuantityProduct
		unitPrice := product.PriceProduct
		totalBefore := float64(qtyBefore) * unitPrice

		// Decrement stock (lebih efisien daripada Save)
		if err := tx.Model(&product).
			Update("quantity_product",
				gorm.Expr("quantity_product - ?", payload.DamagedQuantity)).
			Error; err != nil {
			return err
		}

		qtyAfter := qtyBefore - payload.DamagedQuantity
		totalAfter := float64(qtyAfter) * unitPrice

		// Insert History
		history := models.SkuBundleHistory{
			UserID:         uint64(user.ID),
			CodeDocument:   product.CodeDocument,
			BarcodeProduct: product.BarcodeProduct,
			NameProduct:    product.NameProduct,
			PriceBefore:    totalBefore,
			PriceAfter:     totalAfter,
			QtyBefore:      int(qtyBefore),
			QtyAfter:       int(qtyAfter),
			Type:           "damaged",
		}

		if err := tx.Create(&history).Error; err != nil {
			return err
		}

		// Prepare staging bulk insert
		now := time.Now()
		qualityText := payload.Description
        location := "staging"
        customeBarcode := ""
        if product.Document.CustomBarcode != nil {
            customeBarcode = *product.Document.CustomBarcode
        }

		stagingData := make([]models.Product, 0, payload.DamagedQuantity)

		for i := int64(0); i < payload.DamagedQuantity; i++ {
            barcode, err := helpers.GenerateUniqueBarcode(tx, user.ID, customeBarcode)
            if err != nil {
                return err
            }

			stagingData = append(stagingData, models.Product{
				CodeDocument:     &product.CodeDocument,
                InboundType: "sku",
                OldBarcodeProduct: &product.BarcodeProduct,
                OldNameProduct: product.NameProduct,
                OldPriceProduct:  product.PriceProduct,
                OldQuantityProduct: 1,
                Barcode: barcode,
                Name:   product.NameProduct,
                Quantity: 1,
                Price:  product.PriceProduct,
                DisplayPrice: product.PriceProduct,
                ActualQuality: "damaged",
                ActualOldPrice: product.PriceProduct,
                Status: "display",
                Discount: helpers.Float64Ptr(0),
                Quality: "damaged",
                QualityText: &qualityText,
                LocationType: &location,
                WarehouseType:    "type1",
                CreatedAt:        now,
                UpdatedAt:        now,
			})

            if len(stagingData) == 500 {
                if err := tx.Create(&stagingData).Error; err != nil {
                    return err
                }
                stagingData = stagingData[:0] // reset tanpa alokasi ulang
            }
		}

		// Bulk insert (500 per batch)
        if len(stagingData) > 0 {            
            if err := tx.Create(&stagingData).Error; err != nil {
                return err
            }
        }

		return nil
	})

	if err != nil {
		if err.Error() == "not_found" {
			c.JSON(http.StatusNotFound, gin.H{"success": false,"message": "sku product tidak ditemukan"})
			return
		}
		if err.Error() == "stock_failed" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false,"message": "stock actual tidak mencukupi"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"success": false,"message": "Terjadi kesalahan", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Berhasil memproses barang rusak (%d item)", payload.DamagedQuantity),
	})
}
func GetHistoryBundling(c *gin.Context) {
	db := config.DB

	search := c.Query("q")
	codeDocument := c.Param("code")

	perPageStr := c.DefaultQuery("per_page", "50")
	pageStr := c.DefaultQuery("page", "1")

	perPage, _ := strconv.Atoi(perPageStr)
	page, _ := strconv.Atoi(pageStr)

	if perPage <= 0 {
		perPage = 50
	}
	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * perPage

	var histories []models.SkuBundleHistory
	var total int64

	query := db.Model(&models.SkuBundleHistory{}).
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "name")
		}).Where("code_document = ?", codeDocument)

	if search != "" {
		query = query.Where(
			"(name_product LIKE ? OR barcode_product LIKE ?)",
			"%"+search+"%",
			"%"+search+"%",
		)
	}

	// Count total first
    countSession := query.Session(&gorm.Session{})
	if err := countSession.Count(&total).Error; err != nil {
        c.JSON(500, gin.H{
            "success": false,
            "message": "Gagal menghitung total history",
            "error": err.Error(),
        })

        return
    }

	// Get paginated result
	err := query.
		Order("created_at DESC").
		Limit(perPage).
		Offset(offset).
		Find(&histories).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch data",
            "error": err.Error(),
		})
		return
	}

	// Mapping to response
    type historyBundlingResponse struct {
        ID             uint64   `json:"id"`
        Tanggal        string `json:"tanggal"`
        User           string `json:"user"`
        Produk         string `json:"produk"`
        CodeDocument   string `json:"code_document"`
        PriceBefore    float64 `json:"price_before"`
        PriceAfter     float64 `json:"price_after"`
        QtyBefore      int    `json:"qty_before"`
        QtyAfter       int    `json:"qty_after"`
        Bundling       any    `json:"bundling"`
        ItemsPerBundle any    `json:"items_per_bundle"`
        TypeBadge      string `json:"type_badge"`
    }
	var response []historyBundlingResponse

	for _, item := range histories {

		userName := "Unknown"
		if item.User.ID != 0 {
			userName = item.User.Name
		}

		var bundling any = "-"
		var itemsPerBundle any = "-"

		if item.Type != "damaged" {
			bundling = item.TotalQtyBundle
			itemsPerBundle = item.ItemsPerBundle
		}

		response = append(response, historyBundlingResponse{
			ID:             item.ID,
			Tanggal:        item.CreatedAt.Format("2006-01-02 15:04:05"),
			User:           userName,
			Produk:         item.NameProduct,
			CodeDocument:   item.CodeDocument,
			PriceBefore:    item.PriceBefore,
			PriceAfter:     item.PriceAfter,
			QtyBefore:      item.QtyBefore,
			QtyAfter:       item.QtyAfter,
			Bundling:       bundling,
			ItemsPerBundle: itemsPerBundle,
			TypeBadge:      item.Type,
		})
	}

	// pagination links
    lastPage := int(math.Ceil(float64(total) / float64(perPage)))
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "List History Bundling",
		"resource": gin.H{
            "current_page": page,
            "data":     response,
			"total":     total,
			"per_page":  perPage,
            "links":    links,
            "from":   offset + 1,
            "to": offset + len(response),
            "last_page": lastPage,
		},
	})
}
func CheckTypeBundleSku(c *gin.Context) {
	var req struct {
		ProductID      uint   `json:"product_id" binding:"required"`
		ItemsPerBundle int64  `json:"items_per_bundle" binding:"required,min=1"`
		SelectedType   string `json:"selected_type" binding:"required,oneof=regular big small"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
        ve, ok := err.(validator.ValidationErrors)
        if !ok {
            c.JSON(400, gin.H{"status": false, "message": "Format JSON tidak valid"})
            return
        }

        errors := make(map[string]string)
        for _, e := range ve {
            fieldName := e.Field()

            switch fieldName {

            case "ProductID":
                errors["product_id"] = "Product wajib diisi"

            case "ItemsPerBundle":
                if e.Tag() == "required" {
                    errors["items_per_bundle"] = "Items per bundle wajib diisi"
                } else if e.Tag() == "min" {
                    errors["items_per_bundle"] = "Items per bundle minimal 1"
                }

            case "SelectedType":
                if e.Tag() == "required" {
                    errors["selected_type"] = "Tipe bundle wajib dipilih"
                } else if e.Tag() == "oneof" {
                    errors["selected_type"] = "Tipe harus regular, big, atau small"
                }

            default:
                errors[strings.ToLower(fieldName)] = "Field tidak valid"
            }
        }

        c.JSON(http.StatusBadRequest, gin.H{
            "status": false,
            "message": "Validasi gagal",
            "errors": errors,
        })
        return
    }

	var product models.SkuProduct
	if err := config.DB.First(&product, req.ProductID).Error; err != nil {
		c.JSON(404, gin.H{"error": "produk tidak ditemukan"})
		return
	}

	bundlePrice := product.PriceProduct * float64(req.ItemsPerBundle)
	selected := strings.ToLower(req.SelectedType)

	var naturalType, naturalName string

	if bundlePrice >= 100000 {
		naturalType = "regular"
		naturalName = "Regular (Kategori)"
	} else {
		var tag models.ColorTag
		err := config.DB.
			Where("min_price_color <= ? AND max_price_color >= ?", bundlePrice, bundlePrice).
			Where("name_color LIKE ? OR name_color LIKE ?", "%Big%", "%Small%").
			First(&tag).Error

		if err != nil {
			c.JSON(422, gin.H{"error": "range harga tidak terdaftar di tag big/small manapun."})
			return
		}

		name := strings.ToLower(tag.NameColor)

		if strings.Contains(name, "big") {
			naturalType = "big"
			naturalName = "Big"
		} else {
			naturalType = "small"
			naturalName = "Small"
		}
	}

	isMismatch := naturalType != selected

	var alert string
	if isMismatch {
		alert = fmt.Sprintf(
			`Barang "%s" seharusnya masuk kategori %s. Apakah yakin ingin membuatnya menjadi %s?`,
			product.NameProduct,
			naturalName,
			selected,
		)
	}

	c.JSON(200, gin.H{
		"status":        "success",
		"is_mismatch":   isMismatch,
		"natural_type":  naturalType,
		"selected_type": selected,
		"bundle_price":  bundlePrice,
		"message":       alert,
	})
}
func SkuStoreBundle(c *gin.Context) {
	id := c.Param("sku_product_id")

	var req struct {
		ItemsPerBundle     int64  `json:"items_per_bundle" binding:"required,min=1"`
		BundleQuantity     int64  `json:"bundle_quantity" binding:"required,min=1"`
		BundleType         string `json:"bundle_type" binding:"required,oneof=regular big small"`
		CategoryID         *uint64 `json:"category_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
        ve, ok := err.(validator.ValidationErrors)
        if !ok {
            c.JSON(400, gin.H{"status": false, "message": "Format JSON tidak valid"})
            return
        }

        errors := make(map[string]string)
        for _, e := range ve {
            fieldName := e.Field()

            switch fieldName {

            case "ItemsPerBundle":
                if e.Tag() == "required" {
                    errors["items_per_bundle"] = "Items per bundle wajib diisi"
                } else if e.Tag() == "min" {
                    errors["items_per_bundle"] = "Items per bundle minimal 1"
                }
            case "BundleQuantity":
                if e.Tag() == "required" {
                    errors["bundle_quantity"] = "Bundle quantity wajib diisi"
                } else if e.Tag() == "min" {
                    errors["bundle_quantity"] = "Bundle quantity minimal 1"
                }

            case "BundleType":
                if e.Tag() == "required" {
                    errors["bundle_type"] = "Tipe bundle wajib dipilih"
                } else if e.Tag() == "oneof" {
                    errors["bundle_type"] = "Tipe harus regular, big, atau small"
                }

            default:
                errors[strings.ToLower(fieldName)] = "Field tidak valid"
            }
        }

        c.JSON(http.StatusBadRequest, gin.H{
            "status": false,
            "message": "Validasi gagal",
            "errors": errors,
        })
        return
    }

	// Begin transaction
    tx := config.DB.Begin()
    if tx.Error != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot begin transaction"})
        return
    }

    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"error": "panic occurred"})
        }
    }()

    // ambil data sku product
	var product models.SkuProduct
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
        Preload("Document").
		First(&product, id).Error; err != nil {
		tx.Rollback()
		helpers.ErrorResponse(c, 404, "product sku tidak ditemukan", nil)
		return
	}
    // kalkulasi item per bundle * jumlah bundle
	totalNeeded := req.ItemsPerBundle * req.BundleQuantity
	if product.QuantityProduct < totalNeeded {
		tx.Rollback()
		helpers.ErrorResponse(c, 400, "stock sku tidak mencukupi", nil)
		return
	}

    qtyBefore := product.QuantityProduct
    qtyAfter := product.QuantityProduct - totalNeeded
    totalPriceBefore := float64(product.QuantityProduct) * product.PriceProduct
	bundlePrice := product.PriceProduct * float64(req.ItemsPerBundle)
	bundleType := strings.ToLower(req.BundleType)

	now := time.Now()
	user := c.MustGet("auth_user").(models.User)

	var insertData []models.Product
	var generatedProducts []map[string]interface{}
	var warning string

	// =========================
	// REGULAR
	// =========================
	if bundleType == "regular" {

		if bundlePrice < 100000 {
			tx.Rollback()
            helpers.ErrorResponse(c, 422, "regular hanya untuk >= 100k", nil)
			return
		}

		if req.CategoryID == nil {
			tx.Rollback()
            helpers.ErrorResponse(c, 422, "kategori wajib dipilih untuk regular", nil)
			return
		}

		var category models.Category
		if err := tx.First(&category, *req.CategoryID).
			First(&category).Error; err != nil {
			tx.Rollback()
			helpers.ErrorResponse(c, 404, "kategori tidak ditemukan", nil)
			return
		}

        customeBarcode := ""
        if product.Document.CustomBarcode != nil {
            customeBarcode = *product.Document.CustomBarcode
        }
        location:="staging"
		finalPrice := bundlePrice - (bundlePrice * (float64(category.DiscountCategory) / 100.0))
        if finalPrice > category.MaxPriceCategory {
            finalPrice = category.MaxPriceCategory
        }

		for i := int64(0); i < req.BundleQuantity; i++ {
			barcode, err := helpers.GenerateUniqueBarcode(tx, user.ID, customeBarcode)
            if err != nil {
                tx.Rollback()
                helpers.ErrorResponse(c, 400, "Gagal generate barcode", err)
                return
            }

            generatedProducts = append(generatedProducts, map[string]interface{}{
                "new_barode_product": barcode,
                "new_price_product": finalPrice,
                "category": category.NameCategory,
                "tag": nil,
            })  

			insertData = append(insertData, models.Product{
				CodeDocument:     &product.CodeDocument,
                InboundType: "sku",
                OldBarcodeProduct: &product.BarcodeProduct,
                OldNameProduct: "Bundling " + product.NameProduct,
                OldPriceProduct:  bundlePrice,
                OldQuantityProduct: 1,
                Barcode: barcode,
                Name:   "Bundling " + product.NameProduct,
                Quantity: 1,
                Price:  finalPrice,
                DisplayPrice: finalPrice,
                ActualQuality: "lolos",
                ActualOldPrice: bundlePrice,
                Status: "display",
                CategoryID: &category.ID,
                Discount: helpers.Float64Ptr(0),
                Quality: "lolos",
                LocationType: &location,
                WarehouseType:    "type1",
                CreatedAt:        now,
                UpdatedAt:        now,
			})

            if len(insertData) == 500 {
                if err := tx.Create(&insertData).Error; err != nil {
                    helpers.ErrorResponse(c, 400, "Gagal insert data sku product", err)
                    return
                }
                insertData = insertData[:0] // reset tanpa alokasi ulang
            }
		}

        if len(insertData) > 0 {
            if err := tx.Create(&insertData).Error; err != nil {
                helpers.ErrorResponse(c, 400, "Gagal insert data sku product", err)
                return
            }
        }

	} else {
		// BIG / SMALL
		var tag models.ColorTag

		if err := tx.
			Where("(name_color LIKE ?)", "%"+req.BundleType+"%").
			First(&tag).Error; err != nil {
			tx.Rollback()
			helpers.ErrorResponse(c, 404, fmt.Sprintf("Color tag %s tidak ditemukan", bundleType), err)
			return
		}

		if bundlePrice >= 100000 {
			warning = "regular price (>= 100k) converted to " + tag.NameColor
		}else {
            var naturalTag models.ColorTag
            err := tx.
                Where("min_price_color <= ?", bundlePrice).
                Where("max_price_color >= ?", bundlePrice).
                Where("LOWER(name_color) IN ?", []string{"big", "small"}).
                First(&naturalTag).Error; 
            if err == nil {
                if !strings.EqualFold(tag.NameColor, naturalTag.NameColor) {
                    warning = fmt.Sprintf("%s price converted to %s", naturalTag.NameColor, tag.NameColor)
                }
            }else {
                tx.Rollback()
                helpers.ErrorResponse(c, 404, fmt.Sprintf("Tidak ditemukan Color tag (big/small) yang sesuai dengan range harga %0.2f", bundlePrice), err)
                return
            }

        }

        location := "main"
        customeBarcode := ""
        if product.Document.CustomBarcode != nil {
            customeBarcode = *product.Document.CustomBarcode
        }

		for i := int64(0); i < req.BundleQuantity; i++ {
			barcode, err := helpers.GenerateUniqueBarcode(tx, user.ID, customeBarcode)
            if err != nil {
                tx.Rollback()
                helpers.ErrorResponse(c, 400, "Gagal generate barcode", err)
                return
            }

            generatedProducts = append(generatedProducts, map[string]interface{}{
                "new_barode_product": barcode,
                "new_price_product": tag.FixedPriceColor,
                "category": nil,
                "tag": tag.NameColor,
            })  

			insertData = append(insertData, models.Product{
				CodeDocument:     &product.CodeDocument,
                InboundType: "sku",
                OldBarcodeProduct: &product.BarcodeProduct,
                OldNameProduct: "Bundling " + product.NameProduct,
                OldPriceProduct:  bundlePrice,
                OldQuantityProduct: 1,
                Barcode: barcode,
                Name:   "Bundling " + product.NameProduct,
                Quantity: 1,
                Price:  tag.FixedPriceColor,
                DisplayPrice: tag.FixedPriceColor,
                ActualQuality: "lolos",
                ActualOldPrice: bundlePrice,
                Status: "display",
                TagColorID: &tag.ID,
                Discount: helpers.Float64Ptr(0),
                Quality: "lolos",
                LocationType: &location,
                WarehouseType:    "type1",
                CreatedAt:        now,
                UpdatedAt:        now,
			})

            if len(insertData) == 500 {
                if err := tx.Create(&insertData).Error; err != nil {
                    helpers.ErrorResponse(c, 400, "Gagal insert data sku product", err)
                    return
                }
                insertData = insertData[:0] // reset tanpa alokasi ulang
            }
		}

        if len(insertData) > 0 {
            if err := tx.Create(&insertData).Error; err != nil {
                helpers.ErrorResponse(c, 400, "Gagal insert data sku product", err)
                return
            }
        }
	}

	if err := tx.Model(&product).
		Update("quantity_product", gorm.Expr("quantity_product - ?", totalNeeded)).Error; err != nil {
		tx.Rollback()
        helpers.ErrorResponse(c, 500, "Gagal mmengurangi quantity sku product", err)
		return
	}

    totalPriceAfter := product.PriceProduct * float64(qtyAfter)
    // Insert History
    history := models.SkuBundleHistory{
        UserID:         uint64(user.ID),
        CodeDocument:   product.CodeDocument,
        BarcodeProduct: product.BarcodeProduct,
        NameProduct:    product.NameProduct,
        PriceBefore:    totalPriceBefore,
        PriceAfter:     totalPriceAfter,
        QtyBefore:      int(qtyBefore),
        QtyAfter:       int(qtyAfter),
        Type:           "bundling",
    }

    if err := tx.Create(&history).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Gagal membuat history bundling", err)
        return
    }

	tx.Commit()

	c.JSON(200, gin.H{
        "success": true,
		"message":          fmt.Sprintf("Berhasil membuat %d bundle", req.BundleQuantity),
        "resource": gin.H{
            "total_item_user": totalNeeded,
            "generated_products":  generatedProducts,
            "warning_message":  warning,
        },
	})
}
//================== Helper =======================
func processExcelSkuFile(filePath, fileName string) (string, []string, int, error) {

    f, err := excelize.OpenFile(filePath)
    if err != nil {
        return "", nil, 0, err
    }
	
    defer f.Close()

    sheetName := f.GetSheetName(0)
    if sheetName == "" {
        return "", nil, 0, errors.New("sheet excel tidak ditemukan")
    }

    rows, err := f.Rows(sheetName)
	if err != nil {
		return "", nil, 0, err
	}

    // Get headers from the first row
    headers := []string{}
    if rows.Next() {
		headerRow, _ := rows.Columns()
		headers = append(headers, headerRow...)
	}

	if len(headers) == 0 {
		return "", nil, 0, fmt.Errorf("header tidak ditemukan")
	}

	// Create document entry
    code_document, err := createDocumentSku(fileName, len(headers), 0)
    if err != nil {
        return "", nil, 0, fmt.Errorf("create document error: %w", err)
    }

    rowChan := make(chan []string, 4000)
    var wg sync.WaitGroup

    // Start worker goroutines
    for i := 0; i < WorkerCount; i++ {
        wg.Add(1)
        go worker(code_document, headers, rowChan, &wg)
    }

    // STREAM DATA ROW PER ROW
	// ======================
	totalRows := 0

	for rows.Next() {
		cols, _ := rows.Columns()
		totalRows++
		rowChan <- cols
	}

    close(rowChan)
    wg.Wait()

	// UPDATE DOCUMENT TOTAL ROWS
    if err := config.DB.Model(&models.Document{}).
		Where("code = ?", code_document).
		Update("total_row_data", totalRows).Error; err != nil {
		return "", nil, 0, err
	}

    return code_document, headers, totalRows, nil
}

func createDocumentSku(fileName string, colCount, rowCount int) (string, error) {
    now := time.Now()

    code, err := helpers.GenerateCodeDocumentSKU(config.DB)

    if err != nil {
        return "", err
    }

    doc := models.Document{
        Code:                code,
		DocumentProductType: "sku",
        NameDocument:        fileName,
        TotalColumnDocument: int64(colCount),
        TotalRowData: 		int64(rowCount),
        StatusDocument:      "pending",
        CreatedAt:           now,
        UpdatedAt:           now,
    }

    if err := config.DB.Create(&doc).Error; err != nil {
        return "", err
    }

    return code, nil
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
