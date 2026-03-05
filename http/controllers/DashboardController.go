package controllers

import (
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	// "fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type categoryAggregate struct {
	CategoryName        string  `json:"category_name"`
	TotalProduct        int64   `json:"total_product"`
	TotalPrice          float64 `json:"total_price"`
	TotalOldPrice          float64 `json:"total_old_price"`
}

type colorTagAggregate struct {
	TagColorID    uint64  `json:"tag_color_id"`
	NameColor     string  `json:"name_color"`
	TotalProduct  int64   `json:"total_product"`
	TotalPrice    float64 `json:"total_price"`
	TotalOldPrice    float64 `json:"total_old_price"`
	PercentageTagProduct        float64 `json:"percentage_tag_product"`
	PercentagePriceTagProduct   float64 `json:"percentage_price_tag_product"`
}

type listAnalyticSale struct {
	ProductCategory string  `json:"product_category_sale"`
	TotalCategory   int64   `json:"total_category"`
	DisplayPrice    float64 `json:"display_price_sale"`
	Purchase        float64 `json:"purchase"`
}

type monthlySummary struct {
	TotalCategory int64   `json:"total_category"`
	DisplayPrice  float64 `json:"display_price_sale"`
	Purchase      float64 `json:"purchase"`
}

type annualSummary struct {
	TotalAllCategory        int64   `json:"total_all_category"`
	TotalDisplayPriceSale   float64 `json:"total_display_price_sale"`
	TotalProductPriceSale   float64 `json:"total_product_price_sale"`
}

type analyticSaleMonthly struct {
	Date            time.Time `json:"date"`
	ProductCategory string    `json:"product_category_sale"`
	TotalCategory   int64     `json:"total_category"`
	DisplayPrice    float64   `json:"display_price_sale"`
	Purchase        float64   `json:"purchase"`
}

type monthlyAnalyticSaleResponse struct {
	Month struct {
		CurrentMonth struct {
			Month string `json:"month"`
			Year  string `json:"year"`
		} `json:"current_month"`

		DateFrom *struct {
			Date  string `json:"date"`
			Month string `json:"month"`
			Year  string `json:"year"`
		} `json:"date_from"`

		DateTo *struct {
			Date  string `json:"date"`
			Month string `json:"month"`
			Year  string `json:"year"`
		} `json:"date_to"`
	} `json:"month"`

	Chart       []map[string]interface{} `json:"chart"`
	ListAnalyticSale []listAnalyticSale `json:"list_analytic_sale"`
	MonthlySummary     monthlySummary   `json:"monthly_summary"`
}

type yearlyAnalyticSaleResponse struct {
	Year struct {
		CurrentYear  string `json:"current_year"`
		PrevYear     string `json:"prev_year"`
		SelectedYear string `json:"selected_year"`
		NextYear     string `json:"next_year"`
	} `json:"year"`

	Chart       []map[string]interface{} `json:"chart"`
	ListAnalyticSale []listAnalyticSale `json:"list_analytic_sale"`
	AnnualSummary     annualSummary   `json:"annual_summary"`
}



// ===================== route handler ============================
//storage report
func GetStorageReport(c *gin.Context) {
	defer func() {
        if r := recover(); r != nil {
            c.JSON(http.StatusInternalServerError, gin.H{
				"success": false, 
				"message": "Internal server error",
				"error": fmt.Sprintf("%v", r),
			})
        }
    }()

	resource, err := storageReport()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": "Failed to retrieve storage report", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Laporan Data Perkategori",
		"resource": resource,
	})
}

type summary struct {
	TotalAllProduct        int64
	TotalAllPrice          float64
	TotalProductInventory  int64
	PriceInventory         float64
	TotalProductStaging    int64
	PriceStaging           float64
	TotalProductColor      int64
	PriceColor             float64
}

func ExportStorageReport(c *gin.Context) {
	defer func() {
        if r := recover(); r != nil {
            c.JSON(http.StatusInternalServerError, gin.H{
				"success": false, 
				"message": "Internal server error",
				"error": fmt.Sprintf("%v", r),
			})
        }
    }()

	resource, err := storageReport()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to retrieve storage report", "error": err.Error()})
		return
	}

	inventories := resource["chart"].(map[string]interface{})["inventories"].([]categoryAggregate)
	stagings := resource["chart"].(map[string]interface{})["stagings"].([]categoryAggregate)
	colors := resource["color_tags"].([]colorTagAggregate)
	
	var totalProductColor int64
	var totalPriceColor float64
	for _, item := range colors {
		totalProductColor += item.TotalProduct
		totalPriceColor += item.TotalPrice
	}
	summary := summary{
		TotalAllProduct:       resource["total_all_product"].(int64),
		TotalAllPrice:         resource["total_all_price"].(float64),
		TotalProductInventory: resource["total_display"].(int64),
		PriceInventory:       resource["total_display_price"].(float64),
		TotalProductStaging:  resource["total_staging"].(int64),
		PriceStaging:        resource["total_staging_price"].(float64),
		TotalProductColor:   totalProductColor,
		PriceColor:         totalPriceColor,
	}
	

	f := excelize.NewFile()

	// INVENTORIES SHEET
	createCategorySheet(f, "Inventories", inventories)

	// STAGINGS SHEET
	createCategorySheet(f, "Stagings", stagings)

	// COLORS SHEET
	createColorSheet(f, "Colors", colors)

	// SUMMARY SHEET
	createSummarySheet(f, "Summary", summary)

	// Hapus default sheet
	f.DeleteSheet("Sheet1")

	fileName := "storage-report.xlsx"
	dir := "./public/exports/storage-report"
	os.MkdirAll(dir, 0755)
	fullPath := filepath.Join(dir, fileName)

	if err := f.SaveAs(fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to save file",
			"error": err.Error(),
		})
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/storage-report/%s", os.Getenv("APP_URL"), fileName)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"url":     downloadURL,
	})
}
func ExportArchiveStorageReport(c *gin.Context) {
	var indonesianMonths = []string{
		"Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember",
	}

	var types = []string{"type1", "type2", "color"}

	var displayNames = map[string]string{
		"type1": "Inventories",
		"type2": "Stagings",
		"color": "Colors",
	}

	db := config.DB

	year := c.DefaultQuery("year", fmt.Sprintf("%d", time.Now().Year()))
	month := c.Query("month")

	var months []string
	if month != "" {
		mInt := parseMonth(month)
		months = []string{indonesianMonths[mInt-1]}
	} else {
		months = indonesianMonths
	}

	fileName := fmt.Sprintf("storage-report-%s.xlsx", year)
	publicPath := "./public/exports"
	os.MkdirAll(publicPath, os.ModePerm)
	fullPath := filepath.Join(publicPath, fileName)

	f := excelize.NewFile()
	sheet := "Storage Report"
	f.SetSheetName("Sheet1", sheet)

	// ======================
	// Style Config 
	// ======================
	var BorderStyle = excelize.Style{
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
	}
	var FillYellowStyle = excelize.Style{
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"FFFF00"},
			Pattern: 1,
		},
	}
	var BoldStyle = excelize.Style{Font: &excelize.Font{Bold: true}}
	var FontSize14 = excelize.Style{Font: &excelize.Font{Size: 14, Bold: true}}
	var AlignCenter = excelize.Style{Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "center"}}

	allBorder, _ := helpers.BuildStyle(f, BorderStyle)
	styleTitle,_ := helpers.BuildStyle(f, BorderStyle,BoldStyle, FontSize14, AlignCenter)
	styleSub, _ := helpers.BuildStyle(f, BorderStyle,BoldStyle, AlignCenter)
	styleSub1, _ := helpers.BuildStyle(f, BorderStyle,BoldStyle, FillYellowStyle, AlignCenter)
	styleLeft, _ := helpers.BuildStyle(f, BorderStyle,FillYellowStyle)
	f.SetCellStyle(sheet, "A1", "M13", allBorder)
	// ======================
	// TITLE
	// ======================
	f.MergeCell(sheet, "A1", "M1")
	f.SetCellValue(sheet, "A1", "STORAGE REPORT - "+year)
	f.SetCellStyle(sheet, "A1", "A1", styleTitle)

	// ======================
	// TOTAL ITEM
	// ======================
	f.MergeCell(sheet, "A4", "C4")
	f.SetCellValue(sheet, "A4", "TOTAL ITEM")
	f.SetCellValue(sheet, "A5", "Storage Type")
	f.SetCellStyle(sheet, "A4", "A4", styleSub)
	f.SetCellStyle(sheet, "A5", "A5", styleSub1)

	col := 2
	for _, m := range months {
		cell, _ := excelize.CoordinatesToCellName(col, 5)
		f.SetCellValue(sheet, cell, m)
		f.SetCellStyle(sheet, cell, cell, styleSub1)
		col++
	}

	row := 6
	for _, t := range types {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), displayNames[t])
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), styleLeft)

		col = 2
		for _, m := range months {
			englishMonth := convertToEnglishMonth(m)

			var total int64
			query := db.Table("archive_storages").
				Where("year LIKE ? AND month LIKE ?", "%"+year+"%", "%"+englishMonth+"%")

			switch t {
			case "type1":
				query = query.Where("(type = ? OR type IS NULL)", "type1")
				query.Select("COALESCE(SUM(total_category),0)").Scan(&total)
			case "color":
				query = query.Where("type = ?", "color")
				query.Select("COALESCE(SUM(total_color),0)").Scan(&total)
			default:
				query = query.Where("type = ?", t)
				query.Select("COALESCE(SUM(total_category),0)").Scan(&total)
			}

			cell, _ := excelize.CoordinatesToCellName(col, row)
			if total > 0 {
				f.SetCellValue(sheet, cell, total)
			}
			col++
		}
		row++
	}

	// ======================
	// TOTAL VALUE
	// ======================
	f.MergeCell(sheet, "A9", "C9")
	f.SetCellValue(sheet, "A9", "TOTAL VALUE")
	f.SetCellValue(sheet, "A10", "Storage Type")
	f.SetCellStyle(sheet, "A9", "A9", styleSub)
	f.SetCellStyle(sheet, "A10", "A10", styleSub1)

	col = 2
	for _, m := range months {
		cell, _ := excelize.CoordinatesToCellName(col, 10)
		f.SetCellValue(sheet, cell, m)
		f.SetCellStyle(sheet, cell, cell, styleSub1)
		col++
	}

	row = 11
	for _, t := range types {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), displayNames[t])
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), styleLeft)

		col = 2
		for _, m := range months {
			englishMonth := convertToEnglishMonth(m)

			var totalValue float64
			query := db.Table("archive_storages").
				Where("year LIKE ? AND month LIKE ?", "%"+year+"%", "%"+englishMonth+"%")

			if t == "type1" {
				query = query.Where("(type = ? OR type IS NULL)", "type1")
			} else {
				query = query.Where("type = ?", t)
			}

			query.Select("COALESCE(SUM(value_product),0)").Scan(&totalValue)

			cell, _ := excelize.CoordinatesToCellName(col, row)
			if totalValue > 0 {
				f.SetCellValue(sheet, cell, totalValue)
			}
			col++
		}
		row++
	}

	if err := f.SaveAs(fullPath); err != nil {
		helpers.ErrorResponse(c, 500, "Gagal menyimpan data", err)
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/%s", os.Getenv("APP_URL"), fileName)
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"filename": fileName,
		"download_url": downloadURL,
	})
}

//General Sale
func GetGeneralSales(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(500, gin.H{
				"success": false,
				"message": "Terjadi kesalahan internal",
				"error":   fmt.Sprintf("%v", r),
			})
		}
	}()

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		c.JSON(500, gin.H{"message": "Gagal memuat lokasi waktu", "error": fmt.Sprintf("%v", err)})
		return
	}

	//ambil waktu sekarang
	now := time.Now().In(loc)

    fromInput := c.Query("from")
    toInput := c.Query("to")

    // ===== parsing tanggal =====
    var fromDate, toDate time.Time
    // var err error

    if fromInput != "" {
        fromDate, err = helpers.ParseFlexibleDate(fromInput)
    	if err != nil { 
			c.JSON(500, gin.H{"message": "Gagal memparsing tanggal", "error": fmt.Sprintf("%v", err)})
			return
		}
    } else {
        fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
    }

    if toInput != "" {
		toDate, err = helpers.ParseFlexibleDate(toInput)
		if err != nil {
			c.JSON(500, gin.H{"message": "Gagal memparsing tanggal", "error": fmt.Sprintf("%v", err)})
			return
		}
		toDate = toDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	} else {
		// Mencari hari terakhir bulan ini: tgl 1 bulan berikutnya minus 1 hari
		toDate = time.Date(now.Year(), now.Month()+1, 0, 23, 59, 59, 0, loc)
	}

	type chartItem struct {
		Date              string  `json:"date"`
		TotalPriceSale    float64 `json:"total_price_sale"`
		TotalDisplayPrice float64 `json:"total_display_price"`
	}

	type documentItem struct {
		ID                uint64  `json:"id"`
		TotalPurchase     float64 `json:"total_purchase"`
		TotalDisplayPrice float64 `json:"total_display_price"`
		CodeDocument      string  `json:"code_document_sale"`
		BuyerName         string  `json:"buyer_name_document_sale"`
	}

	type topBuyerItem struct {
		BuyerID    uint64  `json:"buyer_id"`
		TotalPoint float64 `json:"total_point"`
		NameBuyer  string  `json:"name_buyer"`
	}

	type generalSaleResponse struct {
		Month struct {
			CurrentMonth struct {
				Month string `json:"month"`
				Year  string `json:"year"`
			} `json:"current_month"`

			DateFrom *struct {
				Date  string `json:"date"`
				Month string `json:"month"`
				Year  string `json:"year"`
			} `json:"date_from"`

			DateTo *struct {
				Date  string `json:"date"`
				Month string `json:"month"`
				Year  string `json:"year"`
			} `json:"date_to"`
		} `json:"month"`

		Chart       []chartItem      `json:"chart"`
		ListDocumentSale []documentItem   `json:"list_document_sale"`
		ListTopBuyer     []topBuyerItem   `json:"list_top_buyer"`
	}

    // ======================= CHART PER HARI =========================
    type chartRaw struct {
        Date               time.Time
        TotalPriceSale    float64
        TotalDisplayPrice float64
    }

    var rawChart []chartRaw

    config.DB.Raw(`
        SELECT 
            DATE(created_at) as date,
            SUM(total_price) as total_price_sale,
            SUM(total_old_price) as total_display_price
        FROM sale_documents
        WHERE status = 'selesai'
        AND created_at BETWEEN ? AND ?
        GROUP BY DATE(created_at)
        ORDER BY DATE(created_at)
    `, fromDate, toDate).Scan(&rawChart)

    chart := make([]chartItem, 0)

    for _, r := range rawChart {
        chart = append(chart, chartItem{
            Date:              r.Date.Format("02-01-2006"),
            TotalPriceSale:    r.TotalPriceSale,
            TotalDisplayPrice: r.TotalDisplayPrice,
        })
    }

    // =========================================
    // LIST DOCUMENT SALE
    // =========================================

    var listDocument []documentItem

    config.DB.Raw(`
        SELECT 
            id,
            total_price as total_purchase,
            total_old_price as total_display_price,
            code_document_sale,
            buyer_name
        FROM sale_documents
        WHERE status = 'selesai'
        AND created_at BETWEEN ? AND ?
    `, fromDate, toDate).Scan(&listDocument)

    // =========================================
    // 3. TOP BUYER
    // =========================================

    var topBuyer []topBuyerItem

    config.DB.Raw(`
        SELECT 
            b.buyer_id,
            SUM(b.point_buyer) as total_point,
            b.name_buyer
        FROM buyers b
        WHERE b.created_at BETWEEN ? AND ?
        GROUP BY b.id, b.name_buyer
        ORDER BY total_point DESC
        LIMIT 10
    `, fromDate, toDate).Scan(&topBuyer)

    // =========================================
    // RESPONSE
    // =========================================

    response := generalSaleResponse{}
    response.Month.CurrentMonth.Month = now.Format("January")
    response.Month.CurrentMonth.Year = now.Format("2006")

    if fromInput != "" {
        response.Month.DateFrom = &struct {
            Date  string `json:"date"`
            Month string `json:"month"`
            Year  string `json:"year"`
        }{
            Date:  fromDate.Format("02"),
            Month: fromDate.Format("Jan"),
            Year:  fromDate.Format("2006"),
        }
    }

    if toInput != "" {
        response.Month.DateTo = &struct {
            Date  string `json:"date"`
            Month string `json:"month"`
            Year  string `json:"year"`
        }{
            Date:  toDate.Format("02"),
            Month: toDate.Format("Jan"),
            Year:  toDate.Format("2006"),
        }
    }

    response.Chart = chart
    response.ListDocumentSale = listDocument
    response.ListTopBuyer = topBuyer

    c.JSON(200, gin.H{
        "success": true,
        "message": "Laporan Data General",
        "data":    response,
    })
}
func ExportMonthlyAnalyticSales(c *gin.Context) {

	fromInput := c.Query("from")
	toInput := c.Query("to")

	downloadURL, err := prepareExportMonthlyAnalyticSales(config.DB, fromInput, toInput)

	if err != nil {
		c.JSON(500, gin.H{
			"status":  false,
			"message": "Gagal membuat file export",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  true,
		"message": "File export berhasil dibuat",
		"resource":    downloadURL,
	})
}
func ExportYearlyAnalyticSales(c *gin.Context) {

	year := c.Query("y")

	downloadURL, err := prepareExportYearlyAnalyticSales(config.DB, year)

	if err != nil {
		c.JSON(500, gin.H{
			"status":  false,
			"message": "Gagal membuat file export",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  true,
		"message": "File export berhasil dibuat",
		"resource":    downloadURL,
	})
}

//Analytic Sale
func GetMonthlyAnalyticSale(c *gin.Context) {
	from := c.Query("from")
	to := c.Query("to")

	response, err := monthlyAnalyticSales(config.DB, from, to)
	if err != nil {
		c.JSON(500, gin.H{
			"status":  false,
			"message": "Gagal mengambil data",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  true,
		"message": "Laporan Data Sale",
		"resource":    response,
	})
}
func GetYearlyAnalyticSale(c *gin.Context) {
	year := c.Query("y")

	response, err := yearlyAnalyticSales(config.DB, year)
	if err != nil {
		c.JSON(500, gin.H{
			"status":  false,
			"message": "Gagal mengambil data",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  true,
		"message": "Laporan Data Sale",
		"resource":    response,
	})
}

//======================== helper =================================
//storage-report
func storageReport() (map[string]interface{}, error) {
	db := config.DB
	now := time.Now()
	staging := "staging"

	var (
		displayMain        []categoryAggregate
		displayStaging     []categoryAggregate
		slowMoving         []categoryAggregate
		colorTags          []colorTagAggregate
		dumpProducts       []categoryAggregate
		scrapProducts      []categoryAggregate
		b2bProducts        []categoryAggregate

		totalInventory		int64
		totalStaging		int64
		totalSlowMoving		int64
		totalDump			int64
		totalColor			int64
		totalScrap			int64
		totalB2B			int64
		totalSKU			int64

		priceInventory		float64
		priceOldInventory		float64
		priceStaging		float64
		priceOldStaging		float64
		priceSlowMoving		float64
		priceOldSlowMoving		float64
		priceDump			float64
		priceColor			float64
		priceOldColor			float64
		priceScrap			float64
		priceB2B			float64
		priceOldB2B		float64
		priceSkuValuation	float64

	)

	wg := sync.WaitGroup{}
	errCh := make(chan error, 6)

	wg.Add(8)

	//1. get total product per category di inventory
	go func() {
		defer wg.Done()
		res, err := queryInventoryAggregate(db)
		displayMain = res
		if err != nil { errCh <- err }

		totalInventory, priceInventory, priceOldInventory = sumAggCategory(res)
	}()

	//2. get total product per category di staging
	go func() {
		defer wg.Done()
		is_so_done := "done"
		res, err := queryCategoryAggregate(db, []string{"display", "expired"}, "lolos", &staging, &is_so_done)
		displayStaging = res
		if err != nil { errCh <- err }

		totalStaging, priceStaging, priceOldStaging = sumAggCategory(res)
	}()

	//3. get total product per category di b2b
	go func() {
		defer wg.Done()
		res, err := queryB2BAggregate(db, "selesai")
		res2, err := queryB2BAggregate(db, "proses")
		b2bProducts = res
		if err != nil { errCh <- err }

		totalB2B, priceB2B, priceOldB2B = sumAggCategory(res2)
	}()

	//4. get total product per category by status dump
	go func() {
		defer wg.Done()
		res, err := queryCategoryAggregate(db, []string{"dump"}, "lolos", nil, nil)
		dumpProducts = res
		if err != nil { errCh <- err }

		totalDump, priceDump, _ = sumAggCategory(res)
	}()

	//5. get total product per category by status scrap
	go func() {
		defer wg.Done()
		res, err := queryCategoryAggregate(db, []string{"scrap_qcd"}, "lolos", nil, nil)
		scrapProducts = res
		if err != nil { errCh <- err }

		totalScrap, priceScrap, _ = sumAggCategory(res)
	}()

	//6. get total product per category by status slow_moving
	go func() {
		defer wg.Done()
		res, err := queryCategoryAggregate(db, []string{"slow_moving"}, "lolos", nil, nil)
		slowMoving = res
		if err != nil { errCh <- err }

		totalSlowMoving, priceSlowMoving, priceOldSlowMoving = sumAggCategory(res)
	}()

	//7. get total product per color
	go func() {
		defer wg.Done()
		res, err := queryColorTagAggregate(db, "lolos")
		colorTags = res
		if err != nil { errCh <- err }

		totalColor, priceColor, priceOldColor = sumAggColor(res)
	}()

	//8. get summary sku
	go func() {
		defer wg.Done()

		var data struct {
			TotalQty int64
			TotalValuation float64
		}

		if err := db.Model(&models.SkuProduct{}).
			Select(`
				COALESCE(SUM(quantity_product),0) AS total_qty,
				COALESCE(SUM(price_product * quantity_product),0) AS total_valuation
			`).Scan(&data).Error; err != nil { 
			errCh <- err
			return 
		}

		totalSKU = data.TotalQty
		priceSkuValuation = data.TotalValuation
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		return nil, err
	}

	totalAll := totalInventory + totalStaging + totalSlowMoving + totalDump + totalScrap + totalColor
	totalPriceAll := priceInventory + priceStaging + priceSlowMoving + priceDump + priceScrap + priceColor
	totalOldPriceAll := priceOldInventory + priceOldSlowMoving + priceOldStaging + priceOldColor 

	percentageProductDisplay := percent(float64(totalInventory), float64(totalAll))
	percentageProductDisplayPrice := percent(float64(priceInventory), float64(totalPriceAll))
	percentageProductStaging := percent(float64(totalStaging), float64(totalAll))
	percentageProductStagingPrice := percent(float64(priceStaging), float64(totalPriceAll))
	percentageProductB2B := percent(float64(totalB2B), float64(totalAll + totalB2B))
	percentageProductB2BPrice := percent(float64(priceB2B), float64(totalPriceAll + priceB2B))
	percentageProductSlowMoving := percent(float64(totalSlowMoving), float64(totalAll))
	percentageProductSlowMovingPrice := percent(float64(priceSlowMoving), float64(totalPriceAll))
	percentageProductDump := percent(float64(totalDump), float64(totalAll))
	percentageProductDumpPrice := percent(float64(priceDump), float64(totalPriceAll))
	percentageProductScrap := percent(float64(totalScrap), float64(totalAll))
	percentageProductScrapPrice := percent(float64(priceScrap), float64(totalPriceAll))
	percentageProductSKU := percent(float64(totalSKU), float64(totalAll))
	percentageProductSKUPrice := percent(float64(priceSkuValuation), float64(totalPriceAll))
	colorTag, skuTag := mapColorTagReport(colorTags, totalAll, totalPriceAll, false)

	data := map[string]interface{}{
		"month": map[string]interface{}{
			"month": now.Format("January"),
			"year":  now.Year(),
		},
		"chart": map[string]interface{}{
			"inventories":   displayMain,
			"stagings":     displayStaging,
			"b2bs":         b2bProducts,
			"slow_movings": slowMoving,
			"dumps":       dumpProducts,
			"scraps":      scrapProducts,
		},
		"color_tags": map[string]interface{}{
			"color": colorTag,
			"sku": skuTag,
		},
		"total_all_product":        totalAll,
		"total_all_price":          totalPriceAll,
		"total_all_old_price":          totalOldPriceAll,
		"total_percentage_product": percent(float64(totalAll), float64(totalAll)),
		"total_percentage_price":   percent(float64(totalPriceAll), float64(totalPriceAll)),
		"total_percentage_old_price":   percent(float64(totalOldPriceAll), float64(totalOldPriceAll)),

		"total_display":               totalInventory,
		"total_display_price":         priceInventory,
		"percentage_display":          percentageProductDisplay,
		"percentage_display_price":    percentageProductDisplayPrice,

		"total_staging":               totalStaging,
		"total_staging_price":         priceStaging,
		"percentage_staging":          percentageProductStaging,
		"percentage_staging_price":    percentageProductStagingPrice,

		"total_product_b2b":           totalB2B,
		"total_product_b2b_price":     priceOldB2B,
		"percentage_product_b2b":      percentageProductB2B,
		"percentage_product_b2b_price": percentageProductB2BPrice,

		"total_slow_moving":           totalSlowMoving,
		"total_slow_moving_price":     priceSlowMoving,
		"percentage_slow_moving":      percentageProductSlowMoving,
		"percentage_slow_moving_price": percentageProductSlowMovingPrice,

		"total_dump":                  totalDump,
		"total_dump_price":            priceDump,
		"percentage_dump":             percentageProductDump,
		"percentage_dump_price":       percentageProductDumpPrice,

		"total_scrap":                 totalScrap,
		"total_scrap_price":           priceScrap,
		"percentage_scrap":            percentageProductScrap,
		"percentage_scrap_price":      percentageProductScrapPrice,
		
		"total_sku":                 totalSKU,
		"total_sku_price":           priceSkuValuation,
		"percentage_sku":            percentageProductSKU,
		"percentage_sku_price":      percentageProductSKUPrice,
	}

	return data, nil

}

type StorageReportArchive struct {
	Inventories      []categoryAggregate
	Staging          []categoryAggregate
	SlowMoving       []categoryAggregate
	TagProducts      []colorTagAggregate
	Month            string
	Year             int
}

func StorageReportForArchive() (*StorageReportArchive, error) {
	db := config.DB
	now := time.Now()
	staging := "staging"

	var (
		displayMain        []categoryAggregate
		displayStaging     []categoryAggregate
		slowMoving         []categoryAggregate
		colorTags          []colorTagAggregate

		totalInventory		int64
		totalStaging		int64
		totalSlowMoving		int64
		totalColor			int64
		totalSKU			int64

		priceInventory		float64
		priceStaging		float64
		priceSlowMoving		float64
		priceColor			float64
		priceSkuValuation	float64

	)

	wg := sync.WaitGroup{}
	errCh := make(chan error, 4)

	wg.Add(4)

	//1. get total product per category di inventory
	go func() {
		defer wg.Done()
		res, err := queryInventoryAggregate(db)
		displayMain = res
		if err != nil { errCh <- err }

		totalInventory, priceInventory,_ = sumAggCategory(res)
	}()

	//2. get total product per category di staging
	go func() {
		defer wg.Done()
		res, err := queryCategoryAggregate(db, []string{"display", "expired"}, "lolos", &staging, nil)
		displayStaging = res
		if err != nil { errCh <- err }

		totalStaging, priceStaging, _ = sumAggCategory(res)
	}()

	//3. get total product per color
	go func() {
		defer wg.Done()
		res, err := queryColorTagAggregate(db, "lolos")
		colorTags = res
		if err != nil { errCh <- err }

		totalColor, priceColor, _ = sumAggColor(res)
	}()

	//4. get summary sku
	go func() {
		defer wg.Done()

		var data struct {
			TotalQty int64
			TotalValuation float64
		}

		if err := db.Model(&models.SkuProduct{}).
			Select(`
				COALESCE(SUM(quantity_product),0) AS total_qty,
				COALESCE(SUM(price_product * quantity_product),0) AS total_valuation
			`).Scan(&data).Error; err != nil { 
			errCh <- err
			return 
		}

		totalSKU = data.TotalQty
		priceSkuValuation = data.TotalValuation
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		return nil, err
	}

	totalAll := totalInventory + totalStaging + totalSlowMoving + totalColor + totalSKU
	totalPriceAll := priceInventory + priceStaging + priceSlowMoving + priceColor + priceSkuValuation

	// percentageProductDisplay := percent(float64(totalInventory), float64(totalAll))
	// percentageProductDisplayPrice := percent(float64(priceInventory), float64(totalPriceAll))
	// percentageProductStaging := percent(float64(totalStaging), float64(totalAll))
	// percentageProductStagingPrice := percent(float64(priceStaging), float64(totalPriceAll))
	// percentageProductSlowMoving := percent(float64(totalSlowMoving), float64(totalAll))
	// percentageProductSlowMovingPrice := percent(float64(priceSlowMoving), float64(totalPriceAll))
	tagProducts, _ := mapColorTagReport(colorTags, totalAll, totalPriceAll, true)

	return &StorageReportArchive{
		Inventories: displayMain,
		Staging:     displayStaging,
		SlowMoving: slowMoving,
		TagProducts: tagProducts,
		Month:       now.Format("January"),
		Year:        now.Year(),
	}, nil
}

func createCategorySheet(f *excelize.File, sheetName string, data []categoryAggregate) {
	f.NewSheet(sheetName)

	headers := []string{"Category Name", "Total Product", "Value Product"}

	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(sheetName, cell, h)
	}

	for row, d := range data {
		r := row + 2
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", r), d.CategoryName)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", r), d.TotalProduct)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", r), d.TotalPrice)
	}
}

func createColorSheet(f *excelize.File, sheetName string, data []colorTagAggregate) {
	f.NewSheet(sheetName)

	headers := []string{"Category Name", "Total Product", "Value Product"}

	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(sheetName, cell, h)
	}

	for row, d := range data {
		r := row + 2
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", r), d.NameColor)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", r), d.TotalProduct)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", r), d.TotalPrice)
	}
}

func createSummarySheet(f *excelize.File, sheetName string, s summary) {
	f.NewSheet(sheetName)

	headers := []string{
		"total_all_product",
		"total_all_price",
		"total_product_inventory",
		"price_inventory",
		"total_product_staging",
		"price_staging",
		"total_product_color",
		"price_color",
	}

	values := []interface{}{
		s.TotalAllProduct,
		s.TotalAllPrice,
		s.TotalProductInventory,
		s.PriceInventory,
		s.TotalProductStaging,
		s.PriceStaging,
		s.TotalProductColor,
		s.PriceColor,
	}

	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(sheetName, cell, h)
	}

	for col, v := range values {
		cell, _ := excelize.CoordinatesToCellName(col+1, 2)
		f.SetCellValue(sheetName, cell, v)
	}
}

//general sale
func prepareExportMonthlyAnalyticSales(db *gorm.DB, from, to string) (string, error) {

	response, err := monthlyAnalyticSales(db, from, to)
	if err != nil {
		return "", err
	}

	f := excelize.NewFile()
	sheet := "Sheet1"

	// Header
	headers := []string{
		"Category Name",
		"Qty",
		"Display Price",
		"Sale Price",
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Isi data
	for i, v := range response.ListAnalyticSale {
		row := i + 2

		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), v.ProductCategory)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), v.TotalCategory)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), v.DisplayPrice)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), v.Purchase)
	}

	// Auto size
	f.SetColWidth(sheet, "A", "A", 25)
	f.SetColWidth(sheet, "B", "D", 18)

	fileName := "list-monthly-analytic-sales.xlsx"
	path := "./public/exports/general-sale/" + fileName

	os.MkdirAll("./public/exports/general-sale/", 0755)
	if err := f.SaveAs(path); err != nil {
		return "", err
	}

	downloadURL := fmt.Sprintf("%s/public/exports/general-sale/%s", os.Getenv("APP_URL"), fileName)
	return downloadURL, nil
}

func prepareExportYearlyAnalyticSales(db *gorm.DB, year string) (string, error) {

	response, err := yearlyAnalyticSales(db, year)
	if err != nil {
		return "", err
	}

	f := excelize.NewFile()
	sheet := "Sheet1"

	// Header
	headers := []string{
		"Category Name",
		"Qty",
		"Display Price",
		"Sale Price",
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Isi data
	for i, v := range response.ListAnalyticSale {
		row := i + 2

		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), v.ProductCategory)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), v.TotalCategory)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), v.DisplayPrice)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), v.Purchase)
	}

	// Auto size
	f.SetColWidth(sheet, "A", "A", 25)
	f.SetColWidth(sheet, "B", "D", 18)

	fileName := "list-yearly-analytic-sales.xlsx"
	path := "./public/exports/general-sale/" + fileName

	os.MkdirAll("./public/exports/general-sale/", 0755)
	if err := f.SaveAs(path); err != nil {
		return "", err
	}

	downloadURL := fmt.Sprintf("%s/public/exports/general-sale/%s", os.Getenv("APP_URL"), fileName)
	return downloadURL, nil
}

func monthlyAnalyticSales(db *gorm.DB, from, to string) (monthlyAnalyticSaleResponse, error) {

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return monthlyAnalyticSaleResponse{}, err
	}

	now := time.Now().In(loc)

    // ===== parsing tanggal =====
    var fromDate, toDate time.Time
    // var err error

    if from != "" {
        fromDate, err = helpers.ParseFlexibleDate(from)
    	if err != nil { 
			return monthlyAnalyticSaleResponse{}, err
		}
    } else {
		//mencari tanggal 1 di bulan ini
        fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
    }

    if to != "" {
		toDate, err = helpers.ParseFlexibleDate(to)
		if err != nil {
			return monthlyAnalyticSaleResponse{}, err
		}
		toDate = toDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	} else {
		// Mencari hari terakhir bulan ini: tgl 1 bulan berikutnya minus 1 hari
		toDate = time.Date(now.Year(), now.Month()+1, 0, 23, 59, 59, 0, loc)
	}

	response := monthlyAnalyticSaleResponse{}
    response.Month.CurrentMonth.Month = now.Format("January")
    response.Month.CurrentMonth.Year = now.Format("2006")

    if from != "" {
        response.Month.DateFrom = &struct {
            Date  string `json:"date"`
            Month string `json:"month"`
            Year  string `json:"year"`
        }{
            Date:  fromDate.Format("02"),
            Month: fromDate.Format("Jan"),
            Year:  fromDate.Format("2006"),
        }
    }

    if to != "" {
        response.Month.DateTo = &struct {
            Date  string `json:"date"`
            Month string `json:"month"`
            Year  string `json:"year"`
        }{
            Date:  toDate.Format("02"),
            Month: toDate.Format("Jan"),
            Year:  toDate.Format("2006"),
        }
    }

	// data per hari
	var analyticSaleMonthly []analyticSaleMonthly
	if err := db.Table("sales").
		Select(`
			DATE(created_at) as date,
			product_category,
			COUNT(product_category) as total_category,
			SUM(product_old_price) as display_price_sale,
			SUM(product_price_sale) as purchase
		`).
		Where("status_sale = ?", "selesai").
		Where("created_at BETWEEN ? AND ?", fromDate, toDate).
		Group("DATE(created_at), product_category").
		Order("date").
		Scan(&analyticSaleMonthly).Error; err != nil {
		return monthlyAnalyticSaleResponse{}, err
	}

	// PROSES GROUPING DATA
	// ============================
	grouped := map[string]map[string]interface{}{}

	for _, r := range analyticSaleMonthly {

		dateKey := r.Date.Format("02-01-2006")
		// kalau belum ada map untuk tanggal itu
		if _, ok := grouped[dateKey]; !ok {
			grouped[dateKey] = map[string]interface{}{
				"date": dateKey,
			}
		}
		// isi dinamis: nama kategori => total
		grouped[dateKey][r.ProductCategory] = r.TotalCategory
	}

	// ubah ke array (values seperti Laravel ->values())
	result := []map[string]interface{}{}

	for _, v := range grouped {
		result = append(result, v)
	}

	response.Chart = result

	// data per kategori
	var listAnalyticSale []listAnalyticSale

	if err := db.Table("sales").
		Select(`
			product_category,
			COUNT(product_category) as total_category,
			SUM(product_old_price) as display_price,
			SUM(product_price_sale) as purchase
		`).
		Where("status_sale = ?", "selesai").
		Where("created_at BETWEEN ? AND ?", fromDate, toDate).
		Group("product_category").
		Scan(&listAnalyticSale).Error;  err != nil {
			return monthlyAnalyticSaleResponse{}, err
	}
	response.ListAnalyticSale = listAnalyticSale

	// summary
	var monthlySummary monthlySummary
	err = db.Table("sales").
		Select(`
			COUNT(product_category) as total_category,
			SUM(product_old_price) as display_price,
			SUM(product_price_sale) as purchase
		`).Where("status_sale = ?", "selesai").
		Where("created_at BETWEEN ? AND ?", fromDate, toDate).
		Scan(&monthlySummary).Error

	response.MonthlySummary = monthlySummary

	return response, err
}

func yearlyAnalyticSales(db *gorm.DB, yearInput string) (yearlyAnalyticSaleResponse, error) {
    
	// ===== Load timezone =====
    loc, err := time.LoadLocation("Asia/Jakarta")
    if err != nil {
        return yearlyAnalyticSaleResponse{}, err
    }
	
    // ===== Ambil parameter tahun =====
    now := time.Now().In(loc)
    currentYear := now.Format("2006")
	
    year := yearInput
    if year == "" {
        year = currentYear
    }

    // ===== Validasi format tahun (YYYY) =====
    if matched := regexp.MustCompile(`^\d{4}$`).MatchString(year); !matched {
        return yearlyAnalyticSaleResponse{}, fmt.Errorf("Invalid input format. Year should be in format YYYY")
    }

	var response yearlyAnalyticSaleResponse

    selectedYear, _ := time.ParseInLocation("2006", year, loc)
    prevYear := selectedYear.AddDate(-1, 0, 0).Format("2006")
    nextYear := selectedYear.AddDate(1, 0, 0).Format("2006")

	response.Year.CurrentYear = currentYear
	response.Year.PrevYear = prevYear
	response.Year.SelectedYear = selectedYear.Format("2006")
	response.Year.NextYear = nextYear

    analyticSalesYearly := []map[string]interface{}{}

    // ===== 5. Loop 12 bulan =====
    for month := 1; month <= 12; month++ {

        // --- Summary bulan ---
        var sale struct {
            TotalAllCategory int64
            DisplayPriceSale float64
            Purchase         float64
        }

        if err := db.Model(&models.Sale{}).
            Select(`
                COUNT(product_category) as total_all_category,
                SUM(product_old_price) as display_price_sale,
                SUM(product_price_sale) as purchase
            `).
            Where("status_sale = ?", "selesai").
            Where("YEAR(created_at) = ?", year).
            Where("MONTH(created_at) = ?", month).
            Scan(&sale).Error; err != nil {
			return yearlyAnalyticSaleResponse{}, err
		}

		monthName := time.Date(selectedYear.Year(), time.Month(month), 1, 0, 0, 0, 0, loc).
            Format("January")

        analyticSalesPerMonth := map[string]interface{}{
            "month":             monthName,
            "total_all_category": sale.TotalAllCategory,
            "display_price_sale": sale.DisplayPriceSale,
            "purchase":          sale.Purchase,
        }
		
        // --- Per category ---
        rows, _ := db.Model(&models.Sale{}).
            Select(`
                product_category,
                COUNT(product_category) as total_category
            `).
            Where("status_sale = ?", "selesai").
            Where("YEAR(created_at) = ?", year).
            Where("MONTH(created_at) = ?", month).
            Group("product_category").
            Rows()

        for rows.Next() {
            var cat string
            var total int64
            rows.Scan(&cat, &total)

            analyticSalesPerMonth[cat] = total
        }

		analyticSalesYearly = append(analyticSalesYearly, analyticSalesPerMonth)
    }

	response.Chart = analyticSalesYearly

    // =====List analytic per category (tahunan) =====
    var listAnalyticSales []listAnalyticSale

    if err := db.Model(&models.Sale{}).
        Select(`
            product_category,
            COUNT(product_category) as total_category,
            SUM(product_old_price) as display_price,
            SUM(product_price_sale) as purchase
        `).
        Where("status_sale = ?", "selesai").
        Where("YEAR(created_at) = ?", year).
        Group("product_category").
        Scan(&listAnalyticSales).Error; err != nil {
			return yearlyAnalyticSaleResponse{}, err
	}

	response.ListAnalyticSale = listAnalyticSales

    // ===== Summary tahunan =====
    var annSummary annualSummary

    if err := db.Model(&models.Sale{}).
        Select(`
            COUNT(product_category) as total_all_category,
            SUM(product_old_price) as total_display_price_sale,
            SUM(product_price_sale) as total_product_price_sale
        `).
        Where("status_sale = ?", "selesai").
        Where("YEAR(created_at) = ?", year).
        Scan(&annSummary).Error; err != nil {
		return yearlyAnalyticSaleResponse{}, err
	}

    // ===== Response =====
	response.AnnualSummary = annSummary

	return response, nil
}

func percent(v, t float64) float64 {
	if t == 0 {
		return 0
	}
	return math.Round((v/t)*100*100) / 100
}

func sumAggCategory(data []categoryAggregate) (int64, float64, float64) {
	var total int64
	var price float64
	var old_price float64
	for _, v := range data {
		total += v.TotalProduct
		old_price += v.TotalOldPrice
	}
	return total, price, old_price
}

func sumAggColor(data []colorTagAggregate) (int64, float64, float64) {
	var total int64
	var price float64
	var old_price float64
	for _, v := range data {
		total += v.TotalProduct
		price += v.TotalPrice
		old_price += v.TotalOldPrice
	}
	return total, price, old_price
}

func mapColorTagReport(
	data []colorTagAggregate,
	totalAllProduct int64,
	totalAllPrice float64,
	merge bool,
) ([]colorTagAggregate, []colorTagAggregate) {

	colorTags := make([]colorTagAggregate, 0, len(data))
	skuTags := make([]colorTagAggregate, 0, len(data))

	for _, tag := range data {
		item := colorTagAggregate{
			TagColorID:            tag.TagColorID,
			NameColor:             tag.NameColor,
			TotalProduct:       tag.TotalProduct,
			TotalPrice:  tag.TotalPrice,
			TotalOldPrice:  tag.TotalOldPrice,
			PercentageTagProduct: percent(
				float64(tag.TotalProduct),
				float64(totalAllProduct),
			),
			PercentagePriceTagProduct: percent(
				tag.TotalPrice,
				totalAllPrice,
			),
		}

		if merge {
			colorTags = append(colorTags, item)
		}else {
			nameLower := strings.ToLower(item.NameColor)
			if strings.Contains(nameLower, "big") || strings.Contains(nameLower, "small") {
				skuTags = append(skuTags, item)
			} else {
				colorTags = append(colorTags, item)
			}
		}
	}

	return colorTags, skuTags
}

func queryColorTagAggregate(db *gorm.DB, quality string) ([]colorTagAggregate, error) {
	var result []colorTagAggregate

	err := db.
		Table("products p").
		Select(`
			ct.id AS tag_color_id,
			ct.name_color,
			COUNT(p.id) AS total_product,
			COALESCE(SUM(p.price),0) AS total_price,
			COALESCE(SUM(p.old_price_product),0) AS total_old_price
		`).
		Joins("JOIN color_tags ct ON ct.id = p.tag_color_id").
		Where("p.tag_color_id IS NOT NULL").
		Where("p.category_id IS NULL").
		Where("p.status = 'display'").
		Where("p.quality = ?", quality).
		Where("p.location_type = 'main'").
		Group("ct.id, ct.name_color").
		Order("ct.name_color ASC").
		Scan(&result).Error

	return result, err
}

func queryB2BAggregate(db *gorm.DB, status string) ([]categoryAggregate, error) {
	var results []categoryAggregate

	err := db.
		Table("bulky_sales bs").
		Select(`
			bs.product_category AS category_name,
			COUNT(bs.id) AS total_product,
			SUM(bs.product_old_price) AS total_old_price,
			SUM(bs.after_price_bulky_sale) AS total_price
		`).
		Joins(`JOIN bulky_documents ON bulky_documents.id = bs.bulky_document_id`).
		Where("bulky_documents.status_bulky = ?", status).
		Where("bs.product_category IS NOT NULL").
		Group("bs.product_category").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	return results, nil
}

func queryCategoryAggregate(
	db *gorm.DB,
	status []string,
	quality string,
	locationType *string, // main / staging / nil
	is_so *string,
) ([]categoryAggregate, error) {

	var result []categoryAggregate

	q := db.
		Table("products p").
		Select(`
			c.name_category AS category_name,
			COUNT(p.id) AS total_product,
			COALESCE(SUM(p.price),0) AS total_price,
			COALESCE(SUM(p.old_price_product),0) AS total_old_price
		`).
		Joins("JOIN categories c ON c.id = p.category_id").
		Where("p.category_id IS NOT NULL").
		Where("p.tag_color_id IS NULL").
		Where("p.quality = ?", quality).
		Where("p.status IN ?", status)

	if is_so != nil {
		q = q.Where("p.is_so = ?", *is_so)
	}

	if locationType != nil {
		q = q.Where("p.location_type = ?", *locationType)
	}

	err := q.
		Group("c.id, c.name_category").
		Order("c.name_category ASC").
		Scan(&result).Error

	return result, err
}

func queryInventoryAggregate(db *gorm.DB) ([]categoryAggregate, error) {
	var result []categoryAggregate

	query := `
		SELECT
			category_name,
			SUM(total_product) AS total_product,
			SUM(total_price) AS total_price,
			SUM(total_old_price) AS total_old_price
		FROM (
			SELECT
				c.id AS category_id,
				c.name_category AS category_name,
				COUNT(p.id) AS total_product,
				COALESCE(SUM(p.price),0) AS total_price,
				COALESCE(SUM(p.old_price_product),0) AS total_old_price
			FROM products p
			JOIN categories c ON c.id = p.category_id
			WHERE p.tag_color_id IS NULL
				AND p.category_id IS NOT NULL
				AND p.status IN ('display','expired')
				AND p.location_type = 'main'
				AND p.quality = 'lolos'
				AND p.is_so = 'done'
			GROUP BY c.id, c.name_category

			UNION ALL

			SELECT
				c.id AS category_id,
				c.name_category AS category_name,
				COUNT(b.id) AS total_product,
				COALESCE(SUM(b.total_price_custom),0) AS total_price,
				COALESCE(SUM(b.total_price),0) AS total_old_price
			FROM bundles b
			JOIN categories c ON c.id = b.category_id
			WHERE b.category_id IS NOT NULL
				AND b.status != 'bundle'
			GROUP BY c.id, c.name_category
		) AS aggregated
		GROUP BY category_id, category_name
		ORDER BY category_name ASC
	`

	err := db.Raw(query).Scan(&result).Error
	return result, err
}

func convertToEnglishMonth(indonesian string) string {
	months := map[string]string{
		"Januari":   "January",
		"Februari":  "February",
		"Maret":     "March",
		"April":     "April",
		"Mei":       "May",
		"Juni":      "June",
		"Juli":      "July",
		"Agustus":   "August",
		"September": "September",
		"Oktober":   "October",
		"November":  "November",
		"Desember":  "December",
	}
	return months[indonesian]
}

func parseMonth(month string) int {
	m, err := strconv.Atoi(month)
	if err != nil || m < 1 || m > 12 {
		return int(time.Now().Month())
	}
	return m
}


