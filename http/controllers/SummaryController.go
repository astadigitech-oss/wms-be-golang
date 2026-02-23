package controllers

import (
	"errors"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func SummaryBeginBalance(c *gin.Context) {

    loc, err := time.LoadLocation("Asia/Jakarta")
    if err != nil {
        c.JSON(500, gin.H{"message": "gagal load timezone"})
        return
    }

    // Ambil input date, default hari ini
    inputDate := c.DefaultQuery("date", time.Now().In(loc).Format("2006-01-02"))

    // Parse tanggal
    parsed, err := helpers.ParseFlexibleDate(inputDate)
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

	filterDate, err := helpers.ParseFlexibleDate(filterDateStr)
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
func ListSummaryBoth(c *gin.Context) {
    db := config.DB
    location, _ := time.LoadLocation("Asia/Jakarta")
    now := time.Now().In(location)

    dateFrom := c.Query("date_from")
    dateTo := c.Query("date_to")

    // logger umum
	log := helpers.NewLogger("./logs/app.log")
    log.WithFields(logrus.Fields{
        "date_from": dateFrom,
        "date_to": dateTo,
        "current_date": now.Format("2006-01-02"),
    }).Info("listSummaryBoth called")

    // =========================
    // VALIDATION
    // =========================
    from, errFrom := helpers.ParseFlexibleDate(dateFrom)
    if dateFrom != "" && errFrom != nil {     
        c.JSON(422, gin.H{
            "success": false,
            "message": "Format date_from harus Y-m-d atau d-m-Y",
            "error":    errFrom.Error(),
        })
        return
    }       
    to, errTo := helpers.ParseFlexibleDate(dateTo); 
    if dateTo != "" && errTo != nil {
        c.JSON(422, gin.H{
            "success": false,
            "message": "Format date_to harus Y-m-d atau d-m-Y",
            "error":    errTo.Error(),
        })
        return
    }
    if dateFrom != "" && dateTo != "" {
        if from.After(to) {
            c.JSON(422, gin.H{
                "success": false,
                "message": "date_from tidak boleh lebih besar dari date_to",
                "data":    nil,
            })
            return
        }
    }
    // =========================
    // DETERMINE REPORT DATE
    // =========================
    reportDate := now.Format("2006-01-02")
    if dateTo != "" {
        reportDate = dateTo
    } else if dateFrom != "" {
        reportDate = dateFrom
    }

    // =========================
    // CHECK SUMMARY TABLE
    // =========================
    var inbound models.SummaryInbound
    var outbound models.SummaryOutbound

    inboundErr := db.Where("inbound_date = ?", reportDate).First(&inbound).Error
    outboundErr := db.Where("outbound_date = ?", reportDate).First(&outbound).Error

    var summaryReport gin.H

    if inboundErr == nil && outboundErr == nil {
        summaryReport = gin.H{
            "begin_balance": inbound.OldPriceProduct,
            "end_balance":   outbound.DisplayPriceProduct,
            "qty_in":        inbound.Qty,
            "qty_out":       outbound.Qty,
            "price_in":      inbound.NewPriceProduct,
            "price_out":     outbound.DisplayPriceProduct,
        }
    } else {
        // =========================
        // REALTIME AGGREGATION
        // =========================
        summary, err := realtimeSummary(reportDate)
        if err != nil {
            helpers.ErrorResponse(c, 500, "Gagal menghitung realtime summary", err)
            return
        }

        summaryReport = gin.H{
            "begin_balance": summary["begin_balance"],
            "end_balance":   summary["end_balance"],
            "qty_in":        summary["qty_in"],
            "qty_out":       summary["qty_out"],
            "price_in":      summary["price_in"],
            "price_out":     summary["begin_balance"],
        }
    }

    // =========================
    // RANGE FILTER
    // =========================
    inboundQuery := db.Model(&models.SummaryInbound{})
    outboundQuery := db.Model(&models.SummaryOutbound{})

    if dateFrom != "" && dateTo != "" {
        inboundQuery.Where("inbound_date BETWEEN ? AND ?", from.Format("2006-01-02"), to.Format("2006-01-02"))
        outboundQuery.Where("outbound_date BETWEEN ? AND ?", from.Format("2006-01-02"), to.Format("2006-01-02"))
    } else if dateFrom != "" {
        inboundQuery.Where("inbound_date = ?", from.Format("2006-01-02"))
        outboundQuery.Where("outbound_date = ?", from.Format("2006-01-02"))
    } else if dateTo != "" {
        inboundQuery.Where("inbound_date <= ?", to.Format("2006-01-02"))
        outboundQuery.Where("outbound_date <= ?", to.Format("2006-01-02"))
    } else {
        today := now.Format("2006-01-02")
        inboundQuery.Where("inbound_date = ?", today)
        outboundQuery.Where("outbound_date = ?", today)
    }

    var inboundList []models.SummaryInbound
    var outboundList []models.SummaryOutbound

    if err := inboundQuery.Order("inbound_date ASC").Find(&inboundList).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Gagan mengambil data inbound", err)
        return
    }
    if err := outboundQuery.Order("outbound_date ASC").Find(&outboundList).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Gagal mengambil data outbound", err)
        return
    }

    // =========================
    // PREVIOUS DATA (skip Sunday)
    // =========================
    var prevInbound *models.SummaryInbound
    var prevOutbound *models.SummaryOutbound

    for i := 1; i <= 7; i++ {
        checkDate := now.AddDate(0, 0, -i)
        if checkDate.Weekday() == time.Sunday {
            continue
        }

        var temp models.SummaryInbound
        if err := db.Where("inbound_date = ?", checkDate.Format("2006-01-02")).First(&temp).Error; err == nil {
            prevInbound = &temp
            break
        }
    }

    for i := 1; i <= 7; i++ {
        checkDate := now.AddDate(0, 0, -i)
        if checkDate.Weekday() == time.Sunday {
            continue
        }

        var temp models.SummaryOutbound
        if err := db.Where("outbound_date = ?", checkDate.Format("2006-01-02")).First(&temp).Error; err == nil {
            prevOutbound = &temp
            break
        }
    }

    // =========================
    // RESPONSE
    // =========================
    responseData := gin.H{
        "date": gin.H{
            "current_date": now.Format("2006-01-02"),
            "date_from":    dateFrom,
            "date_to":      dateTo,
        },
        "summary_report": summaryReport,
        "inbound":        inboundList,
        "outbound":       outboundList,
        "data_before": gin.H{
            "inbound":  prevInbound,
            "outbound": prevOutbound,
        },
    }

    c.JSON(200, gin.H{
        "success": true,
        "message": "List of summary inbound and outbound",
        "data":    responseData,
    })
}

//================== Helper ================================
func realtimeSummary(reportDate string) (map[string]interface{}, error) {
	db := config.DB

    // startOfDay := now.Truncate(24 * time.Hour)
	// endOfDay := startOfDay.Add(24 * time.Hour)
    date,_ := time.Parse("2006-01-02", reportDate)
    startOfDay := date.Truncate(24 * time.Hour)
	endOfDay := startOfDay.Add(24 * time.Hour)

	type Agg struct {
		Qty          int64
		NewPrice     float64
		OldPrice     float64
		DealPrice     float64
		DisplayPrice float64
	}

	var (
		npIn, pbIn, skuIn Agg
		bsOut, saleOut, migrateOut  Agg
	)

	// ==========================
	// INBOUND
	// ==========================
	db.Model(&models.Product{}).
		Select("COUNT(id) as qty, COALESCE(SUM(price),0) as new_price, COALESCE(SUM(old_price_product),0) as old_price").
		Where("status NOT IN ?", []string{"scrap_qcd"}).
		Where("quality != ?", "damaged").
		Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
		Scan(&npIn)

	db.Model(&models.Bundle{}).
		Select("COUNT(id) as qty, COALESCE(SUM(total_price_custom),0) as new_price, COALESCE(SUM(total_price),0) as old_price").
		Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
		Scan(&pbIn)

	db.Model(&models.SkuProduct{}).
		Select(`
			COALESCE(SUM(quantity_product),0) as qty,
			COALESCE(SUM(price_product * quantity_product),0) as new_price,
			COALESCE(SUM(price_product * quantity_product),0) as old_price
		`).
		Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
		Scan(&skuIn)

	// ==========================
	// OUTBOUND
	// ==========================

	// db.Model(&PaletProduct{}).
	// 	Select("COUNT(id) as qty, COALESCE(SUM(display_price),0) as display_price").
	// 	Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
	// 	Scan(&palOut)

	db.Model(&models.BulkySale{}).
		Select("COUNT(id) as qty, COALESCE(SUM(after_price_bulky_sale), 0) AS deal_price, COALESCE(SUM(display_price),0) as display_price").
		Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
		Scan(&bsOut)

	db.Model(&models.Sale{}).
		Select("COUNT(id) as qty,  COALESCE(SUM(product_price_sale), 0) AS deal_price, COALESCE(SUM(display_price),0) as display_price").
		Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
		Scan(&saleOut)

	db.Model(&models.Product{}).
		Select("COUNT(id) as qty, COALESCE(SUM(price),0) as display_price").
		Where("status = ?", "migrate").
		Where("quality = ?", "lolos").
		Where("updated_at >= ? AND updated_at < ?", startOfDay, endOfDay).
		Scan(&migrateOut)

	// ==========================
	// TOTALING
	// ==========================

	realtimeQtyIn := npIn.Qty + pbIn.Qty + skuIn.Qty
	realtimePriceIn := npIn.NewPrice + pbIn.NewPrice + skuIn.NewPrice
	realtimeOldPriceIn := npIn.OldPrice + pbIn.OldPrice + skuIn.OldPrice

	realtimeQtyOut := bsOut.Qty + saleOut.Qty + migrateOut.Qty
	realtimeDisplayOut := bsOut.DisplayPrice + saleOut.DisplayPrice + migrateOut.DisplayPrice

	return map[string]interface{}{
		"begin_balance": realtimeOldPriceIn,
		"end_balance":   realtimeDisplayOut,
		"qty_in":        realtimeQtyIn,
		"qty_out":       realtimeQtyOut,
		"price_in":      realtimePriceIn,
		"price_out":     realtimeDisplayOut,
	}, nil
}