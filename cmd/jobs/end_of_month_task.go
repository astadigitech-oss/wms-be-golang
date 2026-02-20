package jobs

import (
	"liquid8/wms/config"
	"liquid8/wms/http/controllers"
	"liquid8/wms/models"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func RunEndOfMonthTask(log *logrus.Logger) {
	defer func() {
        if r := recover(); r != nil {
            log.WithField("panic", r).Error("===== EndOfMonthTask PANIC")
        }
    }()

	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	tomorrow := now.AddDate(0, 0, 1)

	// kalau bulan besok sama, berarti hari ini bukan hari terakhir bulan
	if now.Month() == tomorrow.Month() {
		return
	}

	startTime := now

	log.WithFields(logrus.Fields{
		"job":        "EndOfMonthTask",
		"start_time": startTime.Format(time.RFC3339),
		"month":      startTime.Format("2006-01"),
	}).Info("===== EndOfMonthTask Started")

	err := archiveMonthlyReport()
	if err != nil {
		log.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Error("===== EndOfMonthTask Failed")
		return
	}

	endTime := time.Now()

	log.WithFields(logrus.Fields{
		"execution_seconds": endTime.Sub(startTime).Seconds(),
		"month":             startTime.Format("2006-01"),
	}).Info("===== EndOfMonthTask Completed Successfully")
}

func archiveMonthlyReport() error {
	db := config.DB
	now := time.Now()
	month := now.Format("January")
	year := now.Format("2006")

	// Ambil data report (gantikan dengan function real kamu)
	report, err := controllers.StorageReportForArchive()
	if err != nil {
		return err
	}

	var archives []models.ArchiveStorage
	type categoryAggregate struct {
		TotalProduct int64
		TotalPrice   float64
	}

	type1 := "type1"
	type2 := "type2"
	color := "color"

	categoryMap := make(map[string]*categoryAggregate)

	// Ambil dari Inventories
	for _, d := range report.Inventories {
		if _, exists := categoryMap[d.CategoryName]; !exists {
			categoryMap[d.CategoryName] = &categoryAggregate{}
		}

		categoryMap[d.CategoryName].TotalProduct += d.TotalProduct
		categoryMap[d.CategoryName].TotalPrice += d.TotalPrice
	}

	// Ambil dari SlowMoving
	for _, d := range report.SlowMoving {
		if _, exists := categoryMap[d.CategoryName]; !exists {
			categoryMap[d.CategoryName] = &categoryAggregate{}
		}

		categoryMap[d.CategoryName].TotalProduct += d.TotalProduct
		categoryMap[d.CategoryName].TotalPrice += d.TotalPrice
	}

	//TYPE 1
	for categoryName, agg := range categoryMap {

		category := categoryName
		total := agg.TotalProduct
		value := agg.TotalPrice

		archives = append(archives, models.ArchiveStorage{
			CategoryProduct: &category,
			TotalCategory:   total,
			ValueProduct:    value,
			Month:           month,
			Year:            year,
			Type:            &type1,
		})
	}


	// TYPE 2
	for _, d := range report.Staging {
		archives = append(archives, models.ArchiveStorage{
			CategoryProduct: &d.CategoryName,
			TotalCategory:   d.TotalProduct,
			ValueProduct:    d.TotalPrice,
			Month:           month,
			Year:            year,
			Type:            &type2,
		})
	}

	// COLOR
	for _, d := range report.TagProducts {
		archives = append(archives, models.ArchiveStorage{
			Color:        &d.NameColor,
			TotalColor:   d.TotalProduct,
			ValueProduct: d.TotalPrice,
			Month:        month,
			Year:         year,
			Type:         &color,
		})
	}

	// BULK INSERT (INI YANG BIKIN EFISIEN)
	return db.Transaction(func(tx *gorm.DB) error {
		if len(archives) > 0 {
			if err := tx.CreateInBatches(&archives, 500).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
