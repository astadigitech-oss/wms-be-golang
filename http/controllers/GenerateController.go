package controllers

import (
	"errors"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"

	"encoding/json"
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
)


const (
    WorkerCount = 8
    BatchSize   = 1000
)

func ProcessExcelHandler(c *gin.Context) {
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

    tempPath := fmt.Sprintf("uploads/expedisiData/%s", file.Filename)
    c.SaveUploadedFile(file, tempPath)

    code, headers, rows, err := processExcelFile(tempPath, file.Filename)
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

func processExcelFile(filePath, fileName string) (string, []string, int, error) {

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
    code_document, err := createDocument(fileName, len(headers), 0)
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

func worker(code_document string, headers []string, ch <-chan []string, wg *sync.WaitGroup) {
    defer wg.Done()

    batch := make([]models.Generate, 0, BatchSize)

    for row := range ch {
        data := map[string]string{}
        for i := 0; i < len(headers); i++ {
            if i < len(row) {
                data[headers[i]] = helpers.FlexibleNormalize(row[i])
            } else {
                data[headers[i]] = ""
            }
        }

        b, _ := json.Marshal(data)
        batch = append(batch, models.Generate{
			CodeDocument: code_document,
			Data:         string(b),
		})

        if len(batch) >= BatchSize {
            config.DB.Create(batch)
            batch = batch[:0]
        }
    }

    if len(batch) > 0 {
        config.DB.Create(&batch)
    }
}

func createDocument(fileName string, colCount, rowCount int) (string, error) {
    now := time.Now()

    code, err := helpers.GenerateCodeDocument(config.DB)

    if err != nil {
        return "", err
    }

    doc := models.Document{
        Code:                code,
        DocumentProductType: "reguler",
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


//========================= Merge and Map Headers ========================
type mergedStore struct {
	mu                   sync.Mutex
	OldBarcodeProduct    []string
	OldNameProduct       []string
	OldQuantityProduct   []string
	OldPriceProduct      []string
}

func (m *mergedStore) push(header string, value string) {
    /* 
		Mekanisme untuk memastikan hanya 1 goroutine yang bisa 
		mengakses data tertentu pada satu waktu.
	*/
	m.mu.Lock()
	defer m.mu.Unlock()
	switch header {
	case "old_barcode_product":
		m.OldBarcodeProduct = append(m.OldBarcodeProduct, value)
	case "old_name_product":
		m.OldNameProduct = append(m.OldNameProduct, value)
	case "old_quantity_product":
		m.OldQuantityProduct = append(m.OldQuantityProduct, value)
	case "old_price_product":
		m.OldPriceProduct = append(m.OldPriceProduct, value)
	}
}

func MapAndMergeHeaders(c *gin.Context) {
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
    records := make([]models.ProductOld, 0)
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

        records = append(records, models.ProductOld{
            CodeDocument:       &req.CodeDocument,
            InboundType:        "inbound-proses",
            OldBarcodeProduct:  &noResi,
            OldNameProduct:     nama,
            OldQuantityProduct: qty,
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
    var totalPriceRow struct {
        Sum float64
    }
    if err := tx.Model(&models.ProductOld{}).
        Select("COALESCE(SUM(old_price_product),0) as sum").
        Where("code_document = ?", req.CodeDocument).
        Scan(&totalPriceRow).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed sum prices", "detail": err.Error()})
        return
    }
    totalPrice := totalPriceRow.Sum

    // fetch document
    var doc models.Document
    if err := tx.Where("code = ?", req.CodeDocument).First(&doc).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "document not found", "detail": err.Error()})
        return
    }

    // create riwayat_check (RiwayatCheck)
    user := c.MustGet("auth_user").(models.User)

    riwayat := models.RiwayatCheck{
        UserID:                user.ID,
        CodeDocument:          req.CodeDocument,
        NameDocument:          doc.NameDocument,
        TotalData:             int(doc.TotalRowData),
        TotalDataIn:           0,
        TotalDataLolos:        0,
        TotalDataDamaged:      0,
        TotalDataAbnormal:     0,
        TotalDiscrepancy:      0,
        StatusApprove:         "done",
        PrecentageTotalData:   helpers.Float64Ptr(0),
        PercentageIn:          helpers.Float64Ptr(0),
        PercentageLolos:       helpers.Float64Ptr(0),
        PercentageDamaged:     helpers.Float64Ptr(0),
        PercentageAbnormal:    helpers.Float64Ptr(0),
        PercentageDiscrepancy: helpers.Float64Ptr(0),
        TotalPrice:            &totalPrice,
        ValueDataLolos:        helpers.Float64Ptr(0),
        ValueDataDamaged:      helpers.Float64Ptr(0),
        ValueDataAbnormal:     helpers.Float64Ptr(0),
        ValueDataDiscrepancy:  helpers.Float64Ptr(0),
        StatusFile:            true,
        CreatedAt:             time.Now(),
        UpdatedAt:             time.Now(),
    }
    if err := tx.Create(&riwayat).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed create riwayat", "detail": err.Error()})
        return
    }

    // log user action (implement function sesuai kebutuhan)
    metadata := map[string]interface{}{}
    if err := helpers.LogUserAction(user.ID, user.Name, "Upload inbound batch " + req.CodeDocument, "inbound/data_process/data_input", metadata); err != nil {
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
        "message":       "Berhasil menggabungkan data",
        "inserted_rows": len(records),
        "riwayat_check": riwayat,
    })
}

func jsonUnmarshalToMap(raw string, out *map[string]interface{}) error {
	if strings.TrimSpace(raw) == "" {
		*out = map[string]interface{}{}
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func interfaceToString(v interface{}) string { 
    switch vv := v.(type) { 
        case string: 
            return vv 
        case json.Number: 
            return vv.String() 
        case float64: 
            return fmt.Sprintf("%v", vv) 
        case int: 
            return fmt.Sprintf("%d", vv) 
        case bool: 
            return fmt.Sprintf("%v", vv) 
        default: b, _ := json.Marshal(vv) 
            return string(b)
    } 
}

func safeSliceGet(arr []string, idx int) string {
	if idx < 0 || idx >= len(arr) {
		return ""
	}
	return arr[idx]
}

func safeTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

