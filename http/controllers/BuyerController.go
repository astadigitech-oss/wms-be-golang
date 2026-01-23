package controllers

import (
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"math"
	"strconv"
	"time"

	// "strings"

	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	// "github.com/go-playground/validator/v10"
)

// ===================== OUTBOUND ====================
//sale
func GetBuyers(c *gin.Context) {
	q := c.Query("q")

	limit := 50
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	offset := (page - 1) * limit

	db := config.DB

	// month & year
	now := time.Now()
	month, _ := strconv.Atoi(c.DefaultQuery("month", strconv.Itoa(int(now.Month()))))
	year, _ := strconv.Atoi(c.DefaultQuery("year", strconv.Itoa(now.Year())))

	// BASE QUERY BUYER
	baseQuery := db.Model(&models.Buyer{}).
		Preload("Rank")

	if q != "" {
		baseQuery = baseQuery.Where(`
			name_buyer LIKE ?
			OR phone_buyer LIKE ?
			OR address_buyer LIKE ?
			OR type_buyer LIKE ?
		`,
			"%"+q+"%",
			"%"+q+"%",
			"%"+q+"%",
			"%"+q+"%",
		)
	}

	// COUNT TOTAL DATA
	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "gagal hitung total buyer", "error": err.Error()})
		return
	}

	// AMBIL BUYER + MONTHLY POINT
	type buyerRow struct {
		models.Buyer
		MonthlyPoint int64
	}

	var rows []buyerRow

	if err := baseQuery.
		Select(`
			buyers.*,
			COALESCE(SUM(sale_documents.buyer_point), 0) AS monthly_point
		`).
		Joins(`
			LEFT JOIN sale_documents
			ON sale_documents.buyer_id = buyers.id
			AND sale_documents.status = 'selesai'
			AND MONTH(sale_documents.created_at) = ?
			AND YEAR(sale_documents.created_at) = ?
		`, month, year).
		Group("buyers.id").
		Order("monthly_point DESC").
		Order("name_buyer ASC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error; err != nil {

		c.JSON(500, gin.H{"success": false, "message": "gagal ambil data buyer"})
		return
	}

	// HITUNG MONTHLY RANK
	type BuyerRankingDTO struct {
		models.Buyer
		MonthlyPoint         int64 `json:"monthly_point"`
		MonthlyRankPosition int64 `json:"monthly_rank_position"`
	}

	var result []BuyerRankingDTO

	for _, row := range rows {
		var higherRankCount int64

		db.Model(&models.SaleDocument{}).
			Select("buyer_id").
			Where(`
				status = 'selesai'
				AND MONTH(created_at) = ?
				AND YEAR(created_at) = ?
			`, month, year).
			Group("buyer_id").
			Having("SUM(buyer_point) > ?", row.MonthlyPoint).
			Count(&higherRankCount)

		result = append(result, BuyerRankingDTO{
			Buyer:                 row.Buyer,
			MonthlyPoint:          row.MonthlyPoint,
			MonthlyRankPosition: higherRankCount + 1,
		})
	}

	// PAGINATION META
	lastPage := int(math.Ceil(float64(total) / float64(limit)))
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	// RESPONSE
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("List data buyer ranking periode %d-%d", month, year),
		"data": gin.H{
			"current_page": page,
			"per_page":     limit,
			"total":        total,
			"last_page":    lastPage,
			"data":         result,
			"links":        links,
		},
	})
}

//buyer
func GetBuyerSummary(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Gagal memuat data",
				"error":   fmt.Sprintf("%v", r),
			})
		}
	}()

	db := config.DB

	// ===== Ambil month & year =====
	monthStr := c.DefaultQuery("month", time.Now().Format("01"))
	yearStr  := c.DefaultQuery("year", time.Now().Format("2006"))

	month, _ := strconv.Atoi(monthStr)
	year, _  := strconv.Atoi(yearStr)

	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	end   := start.AddDate(0, 1, 0)

	// ===== Total buyer =====
	var totalMasterBuyer int64
	if err := db.Model(&models.Buyer{}).Count(&totalMasterBuyer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	// ===== Total point =====
	var totalPoints int64
	if err := db.Model(&models.SaleDocument{}).
		Where("status_document_sale = ?", "selesai").
		Where("created_at >= ? AND created_at < ?", start, end).
		Select("COALESCE(SUM(buyer_point_document_sale), 0)").
		Scan(&totalPoints).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	// ===== New Buyer =====
	var newBuyer int64
	if err := db.
		Table("buyers b").
		Joins("JOIN loyalty_ranks lr ON lr.id = b.loyalty_rank_id").
		Where("b.created_at >= ? AND b.created_at < ?", start, end).
		Where("b.transaction_count IN (0,1)").
		Where("lr.rank = ?", "New Buyer").
		Count(&newBuyer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	// ===== Active Buyer =====
	var activeBuyerCount int64
	if err := db.Model(&models.SaleDocument{}).
		Where("status_document_sale = ?", "selesai").
		Where("created_at >= ? AND created_at < ?", start, end).
		Distinct("buyer_id_document_sale").  //hitung jumlah buyer berbeda yang pernah belanja
		Count(&activeBuyerCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	// ===== Inactive Buyer =====
	inactiveBuyerCount := totalMasterBuyer - activeBuyerCount
	if inactiveBuyerCount < 0 {
		inactiveBuyerCount = 0
	}

	// ===== Retention Rate =====
	var retentionRate string
	if totalMasterBuyer > 0 {
		retentionRate = fmt.Sprintf("%.1f%%",
			(float64(activeBuyerCount)/float64(totalMasterBuyer))*100,
		)
	} else {
		retentionRate = "0%"
	}

	// ===== Response =====
	data := gin.H{
		"period":                  fmt.Sprintf("%02d-%d", month, year),
		"total_registered_buyer":  totalMasterBuyer,
		"total_point_monthly":     totalPoints,
		"new_buyer_monthly":       newBuyer,
		"active_buyer_monthly":    activeBuyerCount,
		"inactive_buyer_monthly":  inactiveBuyerCount,
		"shopper_retention_rate":  retentionRate,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Data Summary Buyer Periode %02d-%d", month, year),
		"data":    data,
	})
}

func GetBuyerMonthlyPoints(c *gin.Context) {
	db := config.DB

	type BuyerMonthlyReport struct {
		ID                     uint    `json:"id"`
		NameBuyer              string  `json:"name_buyer"`
		Email                  *string `json:"email"`
		Phone                  string  `json:"no_hp"`
		Address                string  `json:"address"`
		Rank                   string  `json:"rank"`
		TotalTransaction       int64   `json:"total_transaction"`
		TotalPurchase          float64 `json:"total_purchase"`
		TotalPoints            int64   `json:"total_points"`
		MonthlyPoints          int64   `json:"monthly_points"`
		MonthlyTotalPurchase   float64 `json:"monthly_total_purchase"`
		MonthlyTransaction     int64   `json:"monthly_transaction"`
	}


	// ===== Params =====
	monthStr := c.DefaultQuery("month", time.Now().Format("01"))
	yearStr  := c.DefaultQuery("year", time.Now().Format("2006"))
	search   := c.Query("q")

	month, _ := strconv.Atoi(monthStr)
	year, _  := strconv.Atoi(yearStr)
	

	// date range (INDEX FRIENDLY)
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate   := startDate.AddDate(0, 1, 0)

	// ===== Pagination =====
	page, _  := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit    := 10
	offset   := (page - 1) * limit

	var results []BuyerMonthlyReport
	var totalRows int64

	// ===== Base Query =====
	query := db.Table("buyers b").
		Select(`
			b.id,
			b.name_buyer,
			b.email,
			b.phone_buyer,
			b.address_buyer,
			COALESCE(lr.rank, '-') AS rank,
			b.amount_transaction_buyer AS total_transaction,
			b.amount_purchase_buyer AS total_purchase,
			b.point_buyer AS total_points,

			COALESCE(SUM(
				CASE 
					WHEN sd.created_at >= ? 
					AND sd.created_at < ?
					AND sd.status_document_sale = 'selesai'
					THEN sd.buyer_point_document_sale
					ELSE 0
				END
			), 0) AS monthly_points,

			COALESCE(SUM(
				CASE 
					WHEN sd.created_at >= ? 
					AND sd.created_at < ?
					AND sd.status_document_sale = 'selesai'
					THEN sd.total_price_document_sale
					ELSE 0
				END
			), 0) AS monthly_total_purchase,

			COUNT(
				DISTINCT CASE 
					WHEN sd.created_at >= ? 
					AND sd.created_at < ?
					AND sd.status_document_sale = 'selesai'
					THEN sd.id
				END
			) AS monthly_transaction
		`, startDate, endDate, startDate, endDate, startDate, endDate).
		Joins("LEFT JOIN sale_documents sd ON sd.buyer_id_document_sale = b.id").
		Joins("LEFT JOIN loyalty_ranks lr ON lr.id = b.loyalty_rank_id").
		Group("b.id")

	if search != "" {
		query = query.Where("b.name_buyer LIKE ?", "%"+search+"%")
	}

	// ===== Count total =====
	if err := query.Session(&gorm.Session{}).Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	// ===== Fetch data =====
	if err := query.
		Limit(limit).
		Offset(offset).
		Scan(&results).Error; err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	lastPage := int(math.Ceil(float64(totalRows) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)	

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Data Buyer Periode %02d-%d", month, year),
		"resource": gin.H{
			"current_page": page,
			"data": results,
			"per_page": limit,
			"links": links,
			"from": offset + 1,
			"to": offset + int(totalRows),
			"total": totalRows,
		},
	})
}

