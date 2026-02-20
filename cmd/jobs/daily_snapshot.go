package jobs

import (
	"errors"
	"fmt"
	"time"

	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func RunDailySnapshot(log *logrus.Logger) {
    loc, _ := time.LoadLocation("Asia/Jakarta")
    today := time.Now().In(loc).Format("2006-01-02")

    log.Info("===== Running Daily Inventory Snapshot")

    // HITUNG SALDO SAAT INI
    totalQty, totalPrice, err := helpers.CalculateCurrentBalance()
    if err != nil {
        log.WithError(err).Error("failed to calculate current balance")
        return
    }

    // Cek apakah sudah ada snapshot hari ini
    var exists_snapshot models.DailyInventorySnapshot
    errCheck := config.DB.Where("snapshot_date = ?", today).
        First(&exists_snapshot).Error

    if errors.Is(errCheck, gorm.ErrRecordNotFound) {
        snapshot := models.DailyInventorySnapshot{
            SnapshotDate: parseDate(today, loc),
            TotalQty:     totalQty,
            TotalPrice:   totalPrice,
        }
    
        log.Info(fmt.Printf("Qty: %d | Price: %.2f", totalQty, totalPrice))
        log.Info("===== Snapshot berhasil disimpan!")

        err := config.DB.Create(&snapshot).Error
        if err != nil {
            log.WithError(err).Error("failed to create snapshot")
        }
        return
    }else {
        log.Info(fmt.Printf("Qty: %d | Price: %.2f", totalQty, totalPrice))
        log.Info("===== Snapshot berhasil disimpan!")

        err := config.DB.Model(&exists_snapshot).Updates(map[string]interface{}{
            "total_qty":   totalQty,
            "total_price": totalPrice,
        }).Error

        if err != nil {
            log.WithError(err).Error("failed to update snapshot")
        }
        return
    }

}

func parseDate(d string, loc *time.Location) time.Time {
    t, _ := time.ParseInLocation("2006-01-02", d, loc)
    return t
}