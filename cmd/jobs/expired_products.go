package jobs

import (
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/models"
	"time"

	"github.com/sirupsen/logrus"
)

func RunExpireProducts(log *logrus.Logger) {
	db := config.DB
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	log.WithFields(logrus.Fields{
		"job":        "MarkProductAsExpired",
		"start_time": now.Format(time.RFC3339),
		"month":      now.Format("2006-01"),
	}).Info("===== Process expired products is started")
	ninetyOneDaysAgo := now.AddDate(0, 0, -91)

	result := db.Model(&models.Product{}).
		Where("created_at <= ?", ninetyOneDaysAgo).
		Where("status = ?", "display").
		Update("status", "expired")

	if result.Error != nil {
		log.WithError(result.Error).Error("Gagal mengambil data product")
		return
	}

	log.Info(fmt.Sprintf("===== Expire products success | affected_rows=%d | execution_time=%s",
		result.RowsAffected,
		time.Since(now),
	))
}

func RunSlowMovingProduct(log *logrus.Logger) {
	db := config.DB
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	log.WithFields(logrus.Fields{
		"job":        "MarkProductAsSlowMoving",
		"start_time": now.Format(time.RFC3339),
		"month":      now.Format("2006-01"),
	}).Info("===== Process slow moving products is started")
	expiredDate := now.AddDate(0, 0, -60)

	result := db.Model(&models.Product{}).
		Where("created_at <= ?", expiredDate).
		Where("status = ?", "display").
		Update("status", "expired")

	if result.Error != nil {
		log.WithError(result.Error).Error("Gagal mengambil data product")
		return
	}

	log.Info(fmt.Sprintf("===== Slow Moving products success | affected_rows=%d | execution_time=%s",
		result.RowsAffected,
		time.Since(now),
	))
}
