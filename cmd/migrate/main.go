package main

import (
	"liquid8/wms/config"
	"liquid8/wms/models"

	"log"
)

func main() {
	// set koneksi database
	config.InitDB()

	// Jalankan AutoMigrate
	err := config.DB.AutoMigrate(
		&models.Role{},
		&models.User{},
		&models.UserToken{},
		&models.UserLog{},
		&models.Document{},
		&models.Generate{},
		&models.RiwayatCheck{},
		&models.ProductOld{},
		&models.Category{},
		&models.ColorTag{},
		&models.Product{},
		&models.Rack{},
		&models.Bundle{},
		&models.BundleItem{},
		&models.Promo{},
		&models.UserScanWeb{},
		&models.ApproveQueue{},
		&models.Notification{},
		&models.SummarySoCategory{},
		&models.SummarySoColor{},
		&models.SoColor{},
	)

	if err != nil {
		log.Fatal("❌ Gagal migrate:", err)
	}

	log.Println("✅ AutoMigrate selesai, semua tabel siap!")
}