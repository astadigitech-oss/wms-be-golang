package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func main() {
	config.InitDB()

	// empty tall table
	if err := truncateAllTables(config.DB, os.Getenv("DB_NAME")); err != nil {
		log.Fatal("❌ Empty Error :", err)
	}

	// start seed
	if err := seedRoles(config.DB); err != nil {
		log.Fatal("❌ Gagal :", err)
	}

	if err := seedUsers(config.DB); err != nil {
		log.Fatal("❌ Gagal :", err)
	}

	if err := seedColorTags(config.DB); err != nil {
		log.Fatal("❌ Gagal :", err)
	}

	if err := seedCategories(config.DB); err != nil {
		log.Fatal("❌ Gagal :", err)
	}

	if err := seedRacks(config.DB); err != nil {
		log.Fatal("❌ Gagal :", err)
	}

	// end seed

	log.Println("✅ Seeder selesai")
}

func seedRoles(db *gorm.DB) error {

	roles := []models.Role{
		{RoleName: "Admin"},
		{RoleName: "Spv"},
		{RoleName: "Team leader"},
		{RoleName: "Crew"},
		{RoleName: "Admin Kasir"},
		{RoleName: "Reparasi"},
		{RoleName: "Developer"},
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "role_name"}},
		DoNothing: true,
	}).Create(&roles).Error
}

func seedColorTags(db *gorm.DB) error {

	roles := []models.ColorTag{
		{HexaCodeColor: "#FF0000", NameColor: "merah", MinPriceColor:0, MaxPriceColor:49999, FixedPriceColor: 25000},
		{HexaCodeColor: "#0000FF", NameColor: "biru", MinPriceColor:50000, MaxPriceColor:99999, FixedPriceColor: 50000},
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "role_name"}},
		DoNothing: true,
	}).Create(&roles).Error
}

func seedUsers(db *gorm.DB) error {
	password, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)

	users := []models.User{
		{Name: "sugeng", Username: "sugeng1", Email: "sugeng@gmail.com", Password: string(password), RoleID: 1},
		{Name: "anas", Username: "anas1", Email: "anas@gmail.com", Password: string(password), RoleID: 2},
		{Name: "firdy", Username: "firdy1", Email: "isagagah3@gmail.com", Password: string(password), RoleID: 3},
		{Name: "freddy", Username: "freddy1", Email: "freddy@gmail.com", Password: string(password), RoleID: 4},
		{Name: "safrudin", Username: "safrudin1", Email: "gebus@gmail.com", Password: string(password), RoleID: 5},
		{Name: "hayyi", Username: "hayyi1", Email: "cok@gmail.com", Password: string(password), RoleID: 6},
		{Name: "developer", Username: "developer", Email: "developer@gmail.com", Password: string(password), RoleID: 7},
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "username"}},
		DoNothing: true,
	}).Create(&users).Error
}

func seedCategories(db *gorm.DB) error {
	categories := []models.Category{
		{
			NameCategory:     "TOYS HOBBIES (200-699)",
			DiscountCategory: 50,
			MaxPriceCategory: 699999,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "TOYS HOBBIES (>700)",
			DiscountCategory: 40,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 700000,
		},
		{
			NameCategory:     "FMCG",
			DiscountCategory: 50,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "BABY PRODUCT",
			DiscountCategory: 40,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "OTOMOTIF MOTOR",
			DiscountCategory: 60,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "OTOMOTIF MOBIL",
			DiscountCategory: 60,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "ELEKTRONIK HV",
			DiscountCategory: 30,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "ELEKTRONIK ART",
			DiscountCategory: 40,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "ACC (0-499)",
			DiscountCategory: 60,
			MaxPriceCategory: 499999,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "ACC (>500)",
			DiscountCategory: 50,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 500000,
		},
		{
			NameCategory:     "FASHION",
			DiscountCategory: 60,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "ATK",
			DiscountCategory: 50,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "ART HV",
			DiscountCategory: 40,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "TOYS HOBBIES (0-199)",
			DiscountCategory: 60,
			MaxPriceCategory: 199999,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "ART",
			DiscountCategory: 50,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "OTHER",
			DiscountCategory: 50,
			MaxPriceCategory: 2000000,
			MinPriceCategory: 0,
		},
		{
			NameCategory:     "TOOLS",
			DiscountCategory: 50,
			MaxPriceCategory: 2000000,
			MinPriceCategory: 0,
		},
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name_category"}},
		DoNothing: true,
	}).Create(&categories).Error
}

func seedRacks(db *gorm.DB) error {
	displayRacks := []string{
		"TOYS HOBBIES",
		"OTOMOTIF",
		"ELEKTRONIK",
		"ACC",
		"ACC GADGET",
		"HP, HV",
		"ART, KOMPOR KOPER",
		"F&B",
		"KOSMETIK, FMCG",
		"OBAT&SUPLEMEN",
		"ORGANIK, HEWAN, PESTISIDA",
		"SERVICE & SANITASI, HOME INDUSTRI",
		"ALAT KESEHATAN",
		"ATK",
		"TOOLS",
		"BABY PRODUCT",
		"FASHION",
		"REFURBISHED",
	}

	for _, name := range displayRacks {
		randomString := helpers.RandomString(8)
		barcodeValue := fmt.Sprintf("DIS-%s", randomString)

		// Implementasi FirstOrCreate menggunakan OnConflict
		return db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: name}, {Name: "display"}},
			DoNothing: true, // Jika sudah ada, jangan timpa (sama seperti firstOrCreate)
		}).Create(&models.Rack{
			Name:      name,
			Source:    "display",
			Barcode:   barcodeValue,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}).Error
	}

	return nil
}

func truncateAllTables(db *gorm.DB, dbName string) error {
	var tables []string

	err := db.Raw(`
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = ?
	`, dbName).Scan(&tables).Error
	if err != nil {
		return err
	}

	// Matikan foreign key check
	db.Exec("SET FOREIGN_KEY_CHECKS = 0")

	for _, table := range tables {
		db.Exec("TRUNCATE TABLE " + table)
	}

	// Hidupkan lagi
	db.Exec("SET FOREIGN_KEY_CHECKS = 1")

	return nil
}


