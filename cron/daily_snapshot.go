package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"

	"gorm.io/gorm"
)

func main() {

    // init DB dulu
    config.InitDB()

    if err := runDailySnapshot(); err != nil {
        log.Fatal("Snapshot gagal:", err)
    }

    fmt.Println("Done.")
}

func runDailySnapshot() error {

    loc, _ := time.LoadLocation("Asia/Jakarta")
    today := time.Now().In(loc).Format("2006-01-02")

    fmt.Println("== Running Daily Inventory Snapshot ==")
    fmt.Println("Date:", today)

    // HITUNG SALDO SAAT INI
    totalQty, totalPrice, err := helpers.CalculateCurrentBalance()
    if err != nil {
        return err
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
    
        fmt.Println("Snapshot berhasil disimpan!")
        fmt.Printf("Qty: %d | Price: %.2f\n", totalQty, totalPrice)

        return config.DB.Create(&snapshot).Error
    }else {
        fmt.Println("Snapshot berhasil diperbarui!")
        fmt.Printf("Qty: %d | Price: %.2f\n", totalQty, totalPrice)

        return config.DB.Model(&exists_snapshot).Updates(map[string]interface{}{
            "total_qty":   totalQty,
            "total_price": totalPrice,
        }).Error
    }

}

func parseDate(d string, loc *time.Location) time.Time {
    t, _ := time.ParseInLocation("2006-01-02", d, loc)
    return t
}