package jobs

import (
	"liquid8/wms/config"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func RunSummaryDaily(log *logrus.Logger) {
	defer func() {
        if r := recover(); r != nil {
            log.WithField("panic", r).Error("===== SummaryDailyReport PANIC")
        }
    }()

	start := time.Now()

	log.WithFields(logrus.Fields{
		"job":        "SummaryDailyReport",
		"start_time": start.Format("2006-01-02 15:04:05"),
	}).Info("===== Summary Daily Job Started")

	err := processSummaryDaily()

	if err != nil {
		log.WithError(err).Error("===== Summary Daily Job Failed")
		return
	}

	log.WithFields(logrus.Fields{
		"execution_seconds": time.Since(start).Seconds(),
	}).Info("===== Summary Daily Job Completed")
}

func processSummaryDaily() error {
	db := config.DB
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	startOfDay := now.Truncate(24 * time.Hour)
	endOfDay := startOfDay.Add(24 * time.Hour)
	date := now.Format("2006-01-02")

	// INBOUND
	inbound, err := calculateInbound(tx, startOfDay, endOfDay)
	if err != nil {
		tx.Rollback()
		return err
	}

	err = UpsertSummaryInbound(tx, date, inbound)
	if err != nil {
		tx.Rollback()
		return err
	}

	// OUTBOUND
	outbound, err := calculateOutbound(tx, startOfDay, endOfDay)
	if err != nil {
		tx.Rollback()
		return err
	}

	err = UpsertSummaryOutbound(tx, date, outbound)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}


type SummaryResult struct {
	Qty          int64
	OldPrice     float64
	NewPrice     float64
	DisplayPrice float64
	Discount 	float64
}

//calculate
func calculateInbound(tx *gorm.DB, start, end time.Time) (SummaryResult, error) {
	var total SummaryResult

	// PRODUCTS (main + staging)
	var products SummaryResult
	err := tx.Table("products").
		Select(`
			COUNT(id) as qty,
			COALESCE(SUM(old_price_product),0) as old_price,
			COALESCE(SUM(price),0) as new_price,
			COALESCE(SUM(display_price),0) as display_price
		`).
		Where("location_type IN ?", []string{"main", "staging"}).
		Where("status != ?", "scrap_qcd").
		Where("quality != ?", "damaged").
		Where("created_at >= ? AND created_at < ?", start, end).
		Scan(&products).Error
	if err != nil {
		return total, err
	}

	// BUNDLES
	var bundles SummaryResult
	err = tx.Table("bundles").
		Select(`
			COUNT(id) as qty,
			COALESCE(SUM(total_price),0) as old_price,
			COALESCE(SUM(total_price_custom),0) as new_price,
			COALESCE(SUM(total_price_custom),0) as display_price
		`).
		Where("created_at >= ? AND created_at < ?", start, end).
		Scan(&bundles).Error
	if err != nil {
		return total, err
	}

	// SKU
	var sku SummaryResult
	err = tx.Table("sku_products").
		Select(`
			COUNT(quantity_product) as qty,
			COALESCE(SUM(total_price),0) as old_price,
			COALESCE(SUM(price_product * quantity_product),0) as old_price,
			COALESCE(SUM(price_product * quantity_product),0) as new_price,
			COALESCE(SUM(price_product * quantity_product),0) as display_price
		`).
		Where("created_at >= ? AND created_at < ?", start, end).
		Scan(&sku).Error
	if err != nil {
		return total, err
	}

	total.Qty = products.Qty + bundles.Qty + sku.Qty
	total.OldPrice = products.OldPrice + bundles.OldPrice + sku.OldPrice
	total.NewPrice = products.NewPrice + bundles.NewPrice + sku.NewPrice
	total.DisplayPrice = products.DisplayPrice + bundles.DisplayPrice + sku.DisplayPrice
	total.Discount = 0

	return total, nil
}

func calculateOutbound(tx *gorm.DB, start, end time.Time) (SummaryResult, error) {
	var total SummaryResult

	// // PALET
	// var bulky SummaryResult
	// err := tx.Table("bulky_sales").
	// 	Select(`
	// 		COUNT(id) as qty,
	// 		COALESCE(SUM(product_old_price),0) as old_price,
	// 		COALESCE(SUM(after_price_bulky_sale),0) as new_price,
	// 		COALESCE(SUM(display_price),0) as display_price
	// 	`).
	// 	Where("created_at >= ? AND created_at < ?", start, end).
	// 	Scan(&bulky).Error
	// if err != nil {
	// 	return total, err
	// }

	// BULKY SALES
	var bulky SummaryResult
	err := tx.Table("bulky_sales").
		Select(`
			COUNT(id) as qty,
			COALESCE(SUM(product_old_price),0) as old_price,
			COALESCE(SUM(after_price_bulky_sale),0) as new_price,
			COALESCE(SUM(display_price),0) as display_price
		`).
		Where("created_at >= ? AND created_at < ?", start, end).
		Scan(&bulky).Error
	if err != nil {
		return total, err
	}

	// SALES
	var sales SummaryResult
	err = tx.Table("sales").
		Select(`
			COUNT(id) as qty,
			COALESCE(SUM(product_old_price),0) as old_price,
			COALESCE(SUM(product_price_sale),0) as new_price,
			COALESCE(SUM(base_price),0) as display_price
		`).
		Where("created_at >= ? AND created_at < ?", start, end).
		Scan(&sales).Error
	if err != nil {
		return total, err
	}

	// MIGRATE PRODUCTS
	var migrate SummaryResult
	err = tx.Table("products").
		Select(`
			COUNT(id) as qty,
			COALESCE(SUM(old_price_product),0) as old_price,
			COALESCE(SUM(price),0) as new_price,
			COALESCE(SUM(display_price),0) as display_price
		`).
		Where("status = ?", "migrate").
		Where("quality = ?", "lolos").
		Where("created_at >= ? AND created_at < ?", start, end).
		Scan(&migrate).Error
	if err != nil {
		return total, err
	}

	total.Qty = bulky.Qty + sales.Qty + migrate.Qty
	total.OldPrice = bulky.OldPrice + sales.OldPrice + migrate.OldPrice
	total.NewPrice = bulky.NewPrice + sales.NewPrice + migrate.NewPrice
	total.DisplayPrice = bulky.DisplayPrice + sales.DisplayPrice + migrate.DisplayPrice

	// DISCOUNT
	discountBs := bulky.DisplayPrice - bulky.NewPrice
	discountS := sales.DisplayPrice - bulky.NewPrice
	total.Discount = discountBs + discountS

	return total, nil
}

//upsert
func UpsertSummaryInbound(tx *gorm.DB, date string, data SummaryResult) error {
	return tx.Exec(`
		INSERT INTO summary_inbounds 
		(inbound_date, qty, new_price_product, old_price_product, display_price_product, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
		qty = VALUES(qty),
		new_price_product = VALUES(new_price_product),
		old_price_product = VALUES(old_price_product),
		display_price_product = VALUES(display_price_product),
		updated_at = NOW()
	`,
		date,
		data.Qty,
		data.NewPrice,
		data.OldPrice,
		data.DisplayPrice,
	).Error
}
func UpsertSummaryOutbound(tx *gorm.DB, date string, data SummaryResult) error {
	return tx.Exec(`
		INSERT INTO summary_outbounds 
		(outbound_date, qty, old_price_product, display_price_product, price_sale, discount, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
		qty = VALUES(qty),
		old_price_product = VALUES(old_price_product),
		display_price_product = VALUES(display_price_product),
		price_sale = VALUES(price_sale),
		discount = VALUES(discount),
		updated_at = NOW()
	`,
		date,
		data.Qty,
		data.OldPrice,
		data.DisplayPrice,
		data.NewPrice,
		data.Discount,
	).Error
}





