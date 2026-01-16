package main

import (
	"fmt"
	"log"
	"os"

	// "os"
	"time"

	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"

	"github.com/go-faker/faker/v4"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func main() {
	config.InitDB()
	
	app_env := os.Getenv("APP_ENV")
	if app_env == "production" {
		log.Fatal("❌ Gagal: sistem saat ini dalam mode production")
	}

	log.Println("⏳ Memulai seeder...")
	
	// truncateTables()
	if err := truncateTables(); err != nil {
		log.Fatal("❌ Gagal :", err)
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

	if err := seedLoyaltyRanks(config.DB); err != nil {
		log.Fatal("❌ Gagal :", err)
	}

	if err := seedBuyer(config.DB, 10); err != nil {
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

func seedBuyer(db *gorm.DB, total int) error {
	for i := 0; i < total; i++ {
		buyer := models.Buyer{
			NameBuyer:              faker.Name(),
			LoyaltyRankID: 			1,	
			PhoneBuyer:             faker.Phonenumber(),
			AddressBuyer:           faker.GetRealAddress().Address,
			TypeBuyer:              "Biasa",
			AmountTransactionBuyer: 1000.0,
			AmountPurchaseBuyer:    1000.0,
			AvgPurchaseBuyer:       1000.0,
		}

		if err := db.Create(&buyer).Error; err != nil {
			return err
		}
	}

	return nil
}

func seedLoyaltyRanks(db *gorm.DB) error {
	// now := time.Now()

	ranks := []models.LoyaltyRank{
		{
			Rank:                 "New Buyer",
			MinTransactions:      0,
			MinAmountTransaction: 5000000,
			PercentageDiscount:   0,
			ExpiredWeeks:         0,
		},
		{
			Rank:                 "Bronze",
			MinTransactions:      1,
			MinAmountTransaction: 5000000,
			PercentageDiscount:   1,
			ExpiredWeeks:         5,
		},
		{
			Rank:                 "Silver",
			MinTransactions:      3,
			MinAmountTransaction: 5000000,
			PercentageDiscount:   2,
			ExpiredWeeks:         4,
		},
		{
			Rank:                 "Gold",
			MinTransactions:      6,
			MinAmountTransaction: 5000000,
			PercentageDiscount:   4,
			ExpiredWeeks:         3,
		},
		{
			Rank:                 "Platinum",
			MinTransactions:      12,
			MinAmountTransaction: 5000000,
			PercentageDiscount:   8,
			ExpiredWeeks:         2,
		},
	}

	return db.Create(&ranks).Error
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
		"OBAT & SUPLEMEN",
		"ORGANIK, HEWAN, PESTISIDA",
		"SERVICE & SANITASI, HOME INDUSTRI",
		"ALAT KESEHATAN",
		"ATK",
		"TOOLS",
		"BABY PRODUCT",
		"FASHION",
		"REFURBISHED",
	}

	var racks []models.Rack

	for _, name := range displayRacks {
		racks = append(racks, models.Rack{
			Name:    name,
			Source:  "display",
			Barcode: fmt.Sprintf("DIS-%s", helpers.RandomString(8)),
			CreatedAt: time.Now(),
            UpdatedAt: time.Now(),
		})
	}

	// Lakukan satu kali hit ke database untuk semua data
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}, {Name: "source"}},
		DoNothing: true,
	}).Create(&racks).Error
}

func truncateTables() error {
	tables := []string{
		"roles",
		"users",
		"color_tags",
		"categories",
		"racks",
		"loyalty_ranks",
		"buyers",
	}
	// Matikan foreign key check
	config.DB.Exec("SET FOREIGN_KEY_CHECKS = 0")

	for _, table := range tables {
		if err := config.DB.Exec("TRUNCATE TABLE " + table).Error; err != nil {
			config.DB.Exec("SET FOREIGN_KEY_CHECKS = 1")
			return err
		}
	}

	// Hidupkan lagi
	config.DB.Exec("SET FOREIGN_KEY_CHECKS = 1")
	return nil
}

