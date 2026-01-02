package controllers

import (
	"liquid8/wms/config"
	
	// "fmt"
	"time"
	"math"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
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


