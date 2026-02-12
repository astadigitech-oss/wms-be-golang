package controllers

import (
	"errors"
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"os"
	"regexp"

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
	CategoryID          uint64  `json:"category_id"`
	CategoryName        string  `json:"category_name"`
	TotalProduct        int64   `json:"total_product"`
	TotalPrice          float64 `json:"total_price"`
}

type colorTagAggregate struct {
	TagColorID    uint64  `json:"tag_color_id"`
	NameColor     string  `json:"name_color"`
	TotalProduct  int64   `json:"total_product"`
	TotalPrice    float64 `json:"total_price"`
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

		totalInventory		int64
		totalStaging		int64
		totalSlowMoving		int64
		totalDump			int64
		totalColor			int64
		totalScrap			int64
		
		priceInventory		float64
		priceStaging		float64
		priceSlowMoving		float64
		priceDump			float64
		priceColor			float64
		priceScrap			float64

	)

	wg := sync.WaitGroup{}
	errCh := make(chan error, 6)

	wg.Add(6)

	//get total product per category di inventory
	go func() {
		defer wg.Done()
		res, err := queryInventoryAggregate(db)
		displayMain = res
		if err != nil { errCh <- err }

		totalInventory, priceInventory = sumAggCategory(res)
	}()

	//get total product per category di staging
	go func() {
		defer wg.Done()
		res, err := queryCategoryAggregate(db, []string{"display", "expired"}, "lolos", &staging)
		displayStaging = res
		if err != nil { errCh <- err }

		totalStaging, priceStaging = sumAggCategory(res)
	}()

	//get total product per category by status dump
	go func() {
		defer wg.Done()
		res, err := queryCategoryAggregate(db, []string{"dump"}, "lolos", nil)
		dumpProducts = res
		if err != nil { errCh <- err }

		totalDump, priceDump = sumAggCategory(res)
	}()

	//get total product per category by status scrap
	go func() {
		defer wg.Done()
		res, err := queryCategoryAggregate(db, []string{"scrap_qcd"}, "lolos", nil)
		scrapProducts = res
		if err != nil { errCh <- err }

		totalScrap, priceScrap = sumAggCategory(res)
	}()

	//get total product per category by status slow_moving
	go func() {
		defer wg.Done()
		res, err := queryCategoryAggregate(db, []string{"slow_moving"}, "lolos", nil)
		slowMoving = res
		if err != nil { errCh <- err }

		totalSlowMoving, priceSlowMoving = sumAggCategory(res)
	}()

	//get total product per color
	go func() {
		defer wg.Done()
		res, err := queryColorTagAggregate(db, "lolos")
		colorTags = res
		if err != nil { errCh <- err }

		totalColor, priceColor = sumAggColor(res)
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}

	totalAll := totalInventory + totalStaging + totalSlowMoving + totalDump + totalScrap + totalColor
	totalPriceAll := priceInventory + priceStaging + priceSlowMoving + priceDump + priceScrap + priceColor
	
	percentageProductDisplay := percent(float64(totalInventory), float64(totalAll))
	percentageProductDisplayPrice := percent(float64(priceInventory), float64(totalPriceAll))
	percentageProductStaging := percent(float64(totalStaging), float64(totalAll))
	percentageProductStagingPrice := percent(float64(priceStaging), float64(totalPriceAll))
	percentageProductSlowMoving := percent(float64(totalSlowMoving), float64(totalAll))
	percentageProductSlowMovingPrice := percent(float64(priceSlowMoving), float64(totalPriceAll))
	percentageProductDump := percent(float64(totalDump), float64(totalAll))
	percentageProductDumpPrice := percent(float64(priceDump), float64(totalPriceAll))
	percentageProductScrap := percent(float64(totalScrap), float64(totalAll))
	percentageProductScrapPrice := percent(float64(priceScrap), float64(totalPriceAll))
	tagProducts := mapColorTagReport(colorTags, totalAll, totalPriceAll)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Laporan Data Perkategori",
		"data": gin.H{
			"month": gin.H{
				"month": now.Format("January"),
				"year":  now.Year(),
			},
			"chart": gin.H{
				"inventory":    displayMain,
				"staging": displayStaging,
				"slow_moving": slowMoving,
				"dump":    dumpProducts,
				"scrap":   scrapProducts,
			},
			"color_tags": tagProducts,
			"total_all_product": totalAll,
			"total_all_price":   totalPriceAll,
			"total_percentage_product":  percent(float64(totalAll), float64(totalAll)),
			"total_percentage_price":   percent(float64(totalPriceAll), float64(totalPriceAll)),
			"total_display": totalInventory,
			"total_display_price": priceInventory,
			"percentage_display":   percentageProductDisplay,
			"percentage_display_price": percentageProductDisplayPrice,
			"total_staging": totalStaging,
			"total_staging_price": priceStaging,
			"percentage_staging":   percentageProductStaging,
			"percentage_staging_price": percentageProductStagingPrice,
			"total_slow_moving": totalSlowMoving,
			"total_slow_moving_price": priceSlowMoving,
			"percentage_slow_moving":   percentageProductSlowMoving,
			"percentage_slow_moving_price": percentageProductSlowMovingPrice,
			"total_dump": totalDump,
			"total_dump_price": priceDump,
			"percentage_dump":   percentageProductDump,
			"percentage_dump_price": percentageProductDumpPrice,
			"total_scrap": totalScrap,
			"total_scrap_price": priceScrap,
			"percentage_scrap":   percentageProductScrap,
			"percentage_scrap_price": percentageProductScrapPrice,
		},
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
        fromDate, err = parseFlexibleDate(fromInput, loc)
    	if err != nil { 
			c.JSON(500, gin.H{"message": "Gagal memparsing tanggal", "error": fmt.Sprintf("%v", err)})
			return
		}
    } else {
        fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
    }

    if toInput != "" {
		toDate, err = parseFlexibleDate(toInput, loc)
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

//summary-report
func SummaryBeginBalance(c *gin.Context) {

    loc, err := time.LoadLocation("Asia/Jakarta")
    if err != nil {
        c.JSON(500, gin.H{"message": "gagal load timezone"})
        return
    }

    // Ambil input date, default hari ini
    inputDate := c.DefaultQuery("date", time.Now().In(loc).Format("2006-01-02"))

    // Parse tanggal
    parsed, err := parseFlexibleDate(inputDate, loc)
    if err != nil {
        c.JSON(400, gin.H{
			"success": false,
            "error": err.Error(),
        })
        return
    }

    // Target = H-1
    targetDate := parsed.AddDate(0, 0, -1)

    var snapshot models.DailyInventorySnapshot

    err = config.DB.
        Where("snapshot_date = ?", targetDate.Format("2006-01-02")).
        First(&snapshot).Error

    var response gin.H

    if errors.Is(err, gorm.ErrRecordNotFound) {
        response = gin.H{
            "date_snapshot":     targetDate.Format("2006-01-02"),
            "total_all_product": 0,
            "total_all_price":   0,
            "message": "Data saldo awal belum tersedia (cronjob belum berjalan kemarin)",
        }
    } else if err != nil {
        c.JSON(500, gin.H{"message": err.Error()})
        return
    } else {
        response = gin.H{
            "date_snapshot":     targetDate.Format("2006-01-02"),
            "total_all_product": snapshot.TotalQty,
            "total_all_price":   snapshot.TotalPrice,
        }
    }

    c.JSON(200, gin.H{
        "success":  true,
        "message": "Summary Saldo Awal",
        "resource":    response,
    })
}
func SummaryEndingBalance(c *gin.Context) {

    loc, _ := time.LoadLocation("Asia/Jakarta")

    filterDateStr := c.DefaultQuery("date", time.Now().In(loc).Format("2006-01-02"))
    todayStr := time.Now().In(loc).Format("2006-01-02")

	filterDate, err := parseFlexibleDate(filterDateStr, loc)
    if err != nil {
        c.JSON(400, gin.H{"success": false, "message": err.Error()})
        return
    }

	today, _ := time.ParseInLocation("2006-01-02", todayStr, loc)

    // ========== 1. JIKA TANGGAL LAMA => AMBIL SNAPSHOT ==========
    if filterDate.Before(today) {

        var snapshot models.DailyInventorySnapshot

        err := config.DB.
            Where("snapshot_date = ?", filterDate).
            First(&snapshot).Error

        if errors.Is(err, gorm.ErrRecordNotFound) {

            c.JSON(200, gin.H{
                "success":  true,
                "message": "Summary Saldo Akhir (Data History)",
                "resource": gin.H{
					"date_current": filterDate.Format("2006-01-02"),
					"total_all_product": 0,
					"total_all_price": 0,
					"note": "Data history tidak ditemukan",
				},
			})
            return
        }

        c.JSON(200, gin.H{
            "status":  true,
            "message": "Summary Saldo Akhir (Data History)",
            "data": gin.H{
				"date_current": filterDate.Format("2006-01-02"),
				"total_all_product": snapshot.TotalQty,
				"total_all_price": snapshot.TotalPrice,
			},
		})
        return
    }

    // ========== 2. JIKA HARI INI => HITUNG REALTIME ==========
    totalQty, totalPrice, err := helpers.CalculateCurrentBalance()
    if err != nil {
        c.JSON(500, gin.H{"status": false, "message": err.Error()})
        return
    }

    c.JSON(200, gin.H{
        "status":  true,
        "message": "Summary Saldo Akhir",
        "data": gin.H{
			"date_current": filterDate.Format("2006-01-02"),
			"total_all_product": totalQty,
			"total_all_price": totalPrice,
		},
    })
}



//======================== helper =================================

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
        fromDate, err = parseFlexibleDate(from, loc)
    	if err != nil { 
			return monthlyAnalyticSaleResponse{}, err
		}
    } else {
		//mencari tanggal 1 di bulan ini
        fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
    }

    if to != "" {
		toDate, err = parseFlexibleDate(to, loc)
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
			SUM(product_old_price) as display_price_sale,
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
			SUM(product_old_price) as display_price_sale,
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
            SUM(product_old_price) as display_price_sale,
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

func sumAggCategory(data []categoryAggregate) (int64, float64) {
	var total int64
	var price float64
	for _, v := range data {
		total += v.TotalProduct
		price += v.TotalPrice
	}
	return total, price
}

func sumAggColor(data []colorTagAggregate) (int64, float64) {
	var total int64
	var price float64
	for _, v := range data {
		total += v.TotalProduct
		price += v.TotalPrice
	}
	return total, price
}

func mapColorTagReport(
	data []colorTagAggregate,
	totalAllProduct int64,
	totalAllPrice float64,
) []colorTagAggregate {

	result := make([]colorTagAggregate, 0, len(data))

	for _, tag := range data {
		item := colorTagAggregate{
			TagColorID:            tag.TagColorID,
			NameColor:             tag.NameColor,
			TotalProduct:       tag.TotalProduct,
			TotalPrice:  tag.TotalPrice,
			PercentageTagProduct: percent(
				float64(tag.TotalProduct),
				float64(totalAllProduct),
			),
			PercentagePriceTagProduct: percent(
				tag.TotalPrice,
				totalAllPrice,
			),
		}
		result = append(result, item)
	}

	return result
}


func queryColorTagAggregate(db *gorm.DB, quality string) ([]colorTagAggregate, error) {
	var result []colorTagAggregate

	err := db.
		Table("products p").
		Select(`
			ct.id AS tag_color_id,
			ct.name_color,
			COUNT(p.id) AS total_product,
			COALESCE(SUM(p.price),0) AS total_price
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

func queryCategoryAggregate(
	db *gorm.DB,
	status []string,
	quality string,
	locationType *string, // main / staging / nil
) ([]categoryAggregate, error) {

	var result []categoryAggregate

	q := db.
		Table("products p").
		Select(`
			c.id AS category_id,
			c.name_category AS category_name,
			COUNT(p.id) AS total_product,
			COALESCE(SUM(p.price),0) AS total_price
		`).
		Joins("JOIN categories c ON c.id = p.category_id").
		Where("p.category_id IS NOT NULL").
		Where("p.tag_color_id IS NULL").
		Where("p.quality = ?", quality).
		Where("p.status IN ?", status)

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
			category_id,
			category_name,
			SUM(total_product) AS total_product,
			SUM(total_price) AS total_price
		FROM (
			SELECT
				c.id AS category_id,
				c.name_category AS category_name,
				COUNT(p.id) AS total_product,
				COALESCE(SUM(p.price),0) AS total_price
			FROM products p
			LEFT JOIN categories c ON c.id = p.category_id
			WHERE p.tag_color_id IS NULL
				AND p.category_id IS NOT NULL
				AND p.status IN ('display','expired')
				AND p.location_type = 'main'
				AND p.quality = 'lolos'
			GROUP BY c.id, c.name_category

			UNION ALL

			SELECT
				c.id AS category_id,
				c.name_category AS category_name,
				COUNT(b.id) AS total_product,
				COALESCE(SUM(b.total_price_custom),0) AS total_price
			FROM bundles b
			LEFT JOIN categories c ON c.id = b.category_id
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

func parseFlexibleDate(input string, loc *time.Location) (time.Time, error) {
    layouts := []string{
        "2006-01-02", // standar API
        "02-01-2006", // format indo
        "02/01/2006", // alternatif
    }

    for _, layout := range layouts {
        if t, err := time.ParseInLocation(layout, input, loc); err == nil {
            return t, nil
        }
    }

     return time.Time{}, fmt.Errorf(
        "format tanggal '%s' tidak dikenali. Gunakan format: YYYY-MM-DD atau DD-MM-YYYY",
        input,
    )
}

