package controllers

import (
	"errors"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"time"

	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

func ImportBulkingCategory(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
    file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"message": "File harus diunggah.",
			"error":   err.Error(),
		})
		return
	}

    ext := filepath.Ext(file.Filename)
    if ext != ".xlsx" && ext != ".xls" {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "message": "File harus berupa file Excel dengan ekstensi .xlsx atau .xls."})
        return
    }

    tempPath := fmt.Sprintf("uploads/expedisiData/%s", file.Filename)
    c.SaveUploadedFile(file, tempPath)

    code_document, colCount, rowCount, err := processBulkingExcel(tempPath, user.ID, file.Filename)
    if err != nil {
        c.JSON(http.StatusUnprocessableEntity, gin.H{
            "success": false,
            "message": "Gagal memproses file excel",
            "error":   err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Import berhasil",
        "data": gin.H{
            "code_document": code_document,
            "file_name": file.Filename,
            "fileDetails": gin.H{
                "total_column_count": colCount,
                "total_row_count": rowCount,
            },
        },
    })
}

func processBulkingExcel(filePath string, userId uint, fileName string) (string, int, int, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return "", 0, 0, err
	}
	defer f.Close()

    sheetName := f.GetSheetName(0)
    if sheetName == "" {
        return "", 0, 0, errors.New("sheet excel tidak ditemukan")
    }

	rows, err := f.GetRows(sheetName)
	if err != nil || len(rows) < 2 {
		return "", 0, 0, errors.New("file excel kosong atau tidak valid")
	}

    headers := rows[0]
	dataRows := rows[1:]

    normalize := func(s string) string {
        s = strings.ReplaceAll(s, "\u00A0", " ") // hapus NBSP
        return strings.TrimSpace(s)
    }

    // Validasi Header (Fast Fail)
	expectedHeaders := []string{"Barcode", "Description", "Category", "Qty", "Unit Price", "Bast", "Discount", "Price After Discount"}
	for i, h := range expectedHeaders {
		if i >= len(headers) || normalize(headers[i]) != h {
			return "", 0, 0, fmt.Errorf("header kolom ke-%d tidak sesuai %s, diharapkan: %s", i+1, headers[i], h)
		}
	}

    // Ambil semua barcode dari file untuk cek duplikat internal
	excelBarcodes := make(map[string]bool)
	var duplicateInExcel []string
	for i := 1; i < len(rows); i++ {
		barcode := strings.TrimSpace(rows[i][0])
		if barcode == "" {
			continue
		}
		if excelBarcodes[barcode] {
			duplicateInExcel = append(duplicateInExcel, barcode)
		}
		excelBarcodes[barcode] = true
	}

    if len(duplicateInExcel) > 0 {
		return "", 0, 0, fmt.Errorf("barcode duplikat di dalam excel: %s", strings.Join(duplicateInExcel, ", "))
	}
	
	// check barcode di database
	var keys []string
	for k := range excelBarcodes {
		keys = append(keys, k)
	}
    var dbBarcodes []string
	config.DB.Table("products").Where("barcode IN ?", keys).Pluck("barcode", &dbBarcodes)
	if len(dbBarcodes) > 0 {
        return "", 0, 0, fmt.Errorf("barcode sudah ada dalam database: %s", strings.Join(dbBarcodes, ", "))
    }

    // Cache Category (Menghindari N+1 query)
	categoryMap := make(map[string]uint64)
	var categories []models.Category
	config.DB.Table("categories").Find(&categories)
	for _, c := range categories {
		categoryMap[strings.ToLower(c.NameCategory)] = c.ID
	}

    //generate code document
    codeDocument, err := helpers.GenerateCodeDocument(config.DB)
    if err != nil {
        return "", 0, 0, err
    }

    currentDocCode := codeDocument
    // Worker Pool Setup
    type Job struct {
        Row    []string
        RowNum int
    }

    // type ParsedRow struct {
    //     Old models.ProductOld
    //     New models.Product
    // }

	numWorkers := runtime.NumCPU()
    errCh := make(chan error, 1)
	jobs := make(chan Job, 4000)
	results := make(chan models.Product, 4000)
	
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
                row := job.Row
                rowNum := job.RowNum
				// Pastikan row memiliki kolom yang cukup
				if len(row) < 8 { continue }

				barcode := strings.TrimSpace(row[0])
                if barcode == "" {
                    select {
                    case errCh <- fmt.Errorf("baris %d kolom Barcode kosong. [product: %s]", rowNum, row[1]):
                    default:
                    }
                    return
                }

                qty, err := helpers.StringToInt(normalize(row[3]))
                if err != nil {
                    select {
                    case errCh <- fmt.Errorf("baris %d kolom Qty tidak valid. [product: %s, value: %s -> %d]", rowNum, row[1], row[3], qty):
                    default:
                    }
                    return
                }

                oldPrice, err := strconv.ParseFloat(strings.ReplaceAll(normalize(row[4]), ",", ""), 64)
                if err != nil {
                    select {
                    case errCh <- fmt.Errorf("baris %d kolom Unit Price tidak valid. [product: %s, value: %s -> %f]", rowNum, row[1], row[4], oldPrice):
                    default:
                    }
                    return
                }

                displayPrice, err := strconv.ParseFloat(strings.ReplaceAll(normalize(row[7]), ",", ""), 64)
                if err != nil {
                    select {
                    case errCh <- fmt.Errorf("baris %d kolom Price After Discount tidak valid. [product: %s, value: %s -> %f]", rowNum, row[1], row[7], displayPrice):
                    default:
                    }
                    return
                }
				// discount, _ := strconv.ParseFloat(row[6], 64)

				catID, exists := categoryMap[strings.ToLower(row[2])]
				var finalCatID *uint64
				if exists { finalCatID = &catID }

				// Mapping ke Product
				location := "staging"
				pNew := models.Product{
					CodeDocument:  &currentDocCode,
					InboundType:        "bulking-category",
					OldBarcodeProduct:  &row[0],
					OldNameProduct:     normalize(row[1]),
					OldQuantityProduct: qty,
					OldPriceProduct:    oldPrice,
					ActualOldPrice:   oldPrice,
					ActualQuality:    "lolos",
					Barcode:       row[0],
					Name:          normalize(row[1]),
					Quantity:      int64(qty),
					Price:         displayPrice,
					Status:        "display",
					Quality:       "lolos",
					CategoryID:    finalCatID,
					DisplayPrice:  displayPrice,
					LocationType:  &location,
					WarehouseType: "type1",
				}

				results <- pNew
			}
		}()
	}

    go func() {
		for i, row := range dataRows {
			jobs <- Job{
                Row: row,
                RowNum: i + 1,
            }
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
        close(errCh)
	}()

    var parsed []models.Product
	for r := range results {
		parsed = append(parsed, r)
	}

    select {
    case err := <-errCh:
        if err != nil {
            return "", 0, 0, err
        }
    default:
    }

    //begin transaction
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		var totalOldPrice float64
		const chunkSize = 500

		// Simpan Document
        totalColumn := len(headers)
		doc := models.Document{
			Code: codeDocument,
			NameDocument: fileName,
			StatusDocument: "done",
            TotalColumnDocument: int64(totalColumn),
            TotalRowData: int64(len(dataRows)),
		}

		if err := tx.Create(&doc).Error; err != nil {
			return err
		}
		
		// push data Product
		for i := 0; i < len(parsed); i += chunkSize {
			end := i + chunkSize
			if end > len(parsed) {
				end = len(parsed)
			}

			var batchNew []models.Product
			for _, p := range parsed[i:end] {
				totalOldPrice += p.OldPriceProduct
				batchNew = append(batchNew, p)
			}

			if err := tx.Create(&batchNew).Error; err != nil {
				return err
			}
		}

		// Simpan History (RiwayatCheck)
        totalRowData := len(dataRows)
        percentageTotlData := helpers.Float64Ptr(100)
		history := models.RiwayatCheck{
			UserID: userId,
			CodeDocument: codeDocument,
            NameDocument: fileName,
            TotalData: totalRowData,
            TotalDataIn: totalRowData,
            TotalDataLolos: totalRowData,
            TotalDataDamaged: 0,
            TotalDataAbnormal: 0,
            TotalDiscrepancy: 0,
            StatusApprove: "display",
            PrecentageTotalData: percentageTotlData,
            PercentageIn: percentageTotlData,
            PercentageLolos: helpers.Float64Ptr(0),
            PercentageDamaged: helpers.Float64Ptr(0),
            PercentageAbnormal: helpers.Float64Ptr(0),
            PercentageDiscrepancy: helpers.Float64Ptr(0),
            TotalPrice: &totalOldPrice,
            ValueDataLolos: helpers.Float64Ptr(0),
            ValueDataDamaged: helpers.Float64Ptr(0),
            ValueDataAbnormal: helpers.Float64Ptr(0),
            ValueDataDiscrepancy: helpers.Float64Ptr(0),
            StatusFile: true,
		}
		if err := tx.Create(&history).Error; err != nil {
			return err
		}

		now := time.Now().In(time.FixedZone("Asia/Jakarta", 7*3600))
		var user models.User
		if err:= tx.Preload("Role").First(&user, userId).Error; err != nil {
			return err	
		}

		metadata := map[string]interface{}{}
		if err := helpers.LogUserAction(userId, user.Name, "import bulking product category", "inbound/bulking-product", metadata); err != nil {
			return err
		}

		notification := models.Notification{
			UserID: userId,
			NotificationName: "bulking category staging",
			Role: user.Role.RoleName,
			ReadAt: &now,
			RiwayatCheckID: &history.ID,
			Status: "display",
		}

		return tx.Create(&notification).Error
	})

    return codeDocument, len(headers), len(dataRows), err
}


