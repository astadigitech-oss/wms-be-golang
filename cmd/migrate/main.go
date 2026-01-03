package main

import (
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/models"
	"os"

	"log"
)

func main() {
	// Cek Argumen
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  go run ./cmd/migrate/main.go -up     (Untuk migrasi tabel)")
		fmt.Println("  go run ./cmd/migrate/main.go -drop   (Untuk menghapus semua tabel)")
		return
	}

	command := os.Args[1]

	switch command {
	case "-up":
		config.InitDB()
		runMigrations()
	case "-drop":
		config.InitDB()
		dropMigrations()
	default:
		fmt.Printf("Perintah '%s' tidak dikenali.\n", command)
		fmt.Println("Gunakan -up atau -drop")
	}
}

func runMigrations() {
	log.Println("⏳ Menjalankan migrasi...")
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

	log.Println("✅ Migrasi berhasil, semua tabel siap!")
}

func dropMigrations() {
	log.Println("⚠️ Menghapus semua tabel...")
	// Matikan pengecekan foreign key
	config.DB.Exec("SET FOREIGN_KEY_CHECKS = 0;")
	err := config.DB.Migrator().DropTable(
		&models.SoColor{},
		&models.SummarySoColor{},
		&models.SummarySoCategory{},
		&models.Notification{},
		&models.ApproveQueue{},
		&models.UserScanWeb{},
		&models.Promo{},
		&models.BundleItem{},
		&models.Bundle{},
		&models.Product{}, // Hapus product sebelum category/rack/color_tag
		&models.ProductOld{},
		&models.RiwayatCheck{},
		&models.Generate{},
		&models.Document{},
		&models.UserLog{},
		&models.UserToken{},
		&models.User{},
		&models.Role{},
		&models.Category{},
		&models.ColorTag{},
		&models.Rack{},
	)
	// Hidupkan kembali pengecekan foreign key
	config.DB.Exec("SET FOREIGN_KEY_CHECKS = 1;")

	if err != nil {
		log.Fatalf("Gagal Drop Tabel: %v", err)
	}
	log.Println("Tabel Berhasil Dihapus!")
}