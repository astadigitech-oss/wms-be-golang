package main

import (
	"fmt"
	"log"
	"os"
	"strings"

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

type SeederConfig struct {
	Table string
	Run   func() error
}

var seederRegistry = map[string]SeederConfig{
	"role": {
		Table: "roles",
		Run:   func() error { return seedRoles(config.DB) },
	},
	"user": {
		Table: "users",
		Run:   func() error { return seedUsers(config.DB) },
	},
	"color": {
		Table: "color_tags",
		Run:   func() error { return seedColorTags(config.DB) },
	},
	"category": {
		Table: "categories",
		Run:   func() error { return seedCategories(config.DB) },
	},
	"rack": {
		Table: "racks",
		Run:   func() error { return seedRacks(config.DB) },
	},
	"loyalty": {
		Table: "loyalty_ranks",
		Run:   func() error { return seedLoyaltyRanks(config.DB) },
	},
	"buyer": {
		Table: "buyers",
		Run:   func() error { return seedBuyer(config.DB, 10) },
	},
	"destination": {
		Table: "migrate_color_destinations",
		Run:   func() error { return seedDestionationOlsera(config.DB) },
	},
}

var seederOrder = []string{
	"role",
	"user",
	"color",
	"category",
	"rack",
	"loyalty",
	"buyer",
	"destination",
}

func main() {
	app_env := os.Getenv("APP_ENV")
	if app_env == "production" {
		log.Fatal("❌ Gagal: sistem saat ini dalam mode production")
	}

	// Cek argumen
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  go run ./cmd/seeder/main.go --all 		(untuk seed semua table)")
		fmt.Println("  go run ./cmd/seeder/main.go --class=name 	(untuk seed table tertentu)")
		fmt.Println("  go run ./cmd/seeder/main.go --list 		(untuk melihat list name class)")
		return
	}

	command := os.Args[1]

	if command == "--all"{
		config.InitDB()
		runAllSeeders()
		return

	}else if strings.HasPrefix(command, "--class=") {
		parts := strings.SplitN(command, "=", 2)
		if len(parts) != 2 || parts[1] == "" {
			log.Fatal("❌ Format salah. Gunakan --class=namaSeeder")
		}
		className := parts[1]
		config.InitDB()
		runSingleSeeder(className)
		return

	}else if command == "--list" {
		fmt.Println("📦 Available Seeder Classes:")
		fmt.Println("--------------------------------")
		fmt.Printf("  %-18s %s\n", "[class_name]", "[table_name]")
		for name, config := range seederRegistry {
			fmt.Printf("  - %-15s → %s\n", name, config.Table)
		}
		fmt.Println("--------------------------------")
		fmt.Println("Usage: go run ./cmd/seeder/main.go --class=[class_name]")

		return
	}else{
		fmt.Printf("Perintah '%s' tidak dikenali.\n", command)
		fmt.Println("Gunakan -all atau -class")
	}
}

func runAllSeeders() {
	if err := truncateTables(); err != nil  {
		log.Fatal("❌ Gagal :", err)
	}

	log.Println("🚀 Menjalankan semua seeder...")

	for _, name := range seederOrder {
		config, exists := seederRegistry[name]
		if !exists {
			log.Fatalf("Seeder %s tidak ditemukan", name)
		}
		
		if err := config.Run(); err != nil {
			log.Printf("→ %s ❌\n", name)
			log.Fatal("❌ Gagal:", err)
		}
		log.Printf("→ %s ✅\n", name)
	}

	log.Println("✅ Semua seeder selesai")
}

func runSingleSeeder(name string) {
	config, exists := seederRegistry[name]
	if !exists {
		log.Fatalf("❌ Seeder '%s' tidak ditemukan", name)
	}

	if err := truncateTableByClass(name); err != nil  {
		log.Fatal(err.Error())
	}

	log.Printf("🚀 Menjalankan seeder: %s\n", name)

	if err := config.Run(); err != nil {
		log.Fatal("❌ Gagal:", err)
	}

	log.Println("✅ Seeder selesai")
}

//==================================================================
// Seeder
//==================================================================

func seedRoles(db *gorm.DB) error {

	roles := []models.Role{
		{RoleName: "Admin"},
		{RoleName: "Spv"},
		{RoleName: "Team leader"},
		{RoleName: "Crew"},
		{RoleName: "Admin Kasir"},
		{RoleName: "Reparasi"},
		{RoleName: "Developer"},
		{RoleName: "Kasir leader"},
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
		{Name: "kasir_leader", Username: "kasir_leader", Email: "kasir_leader@gmail.com", Password: string(password), RoleID: 8},
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

func seedDestionationOlsera(db *gorm.DB) error {
	destinations := []models.MigrateColorDestination{
		{
			ShopName:           "Diskonter Proklamasi",
			IsOlseraIntegreted: true,
			OlseraAppID:        "IFRcKWdLuuB2Gk26q4l0",
			OlseraSecretKey:    "ClHhvj03NRVYg8oln8T97b0OMy4NR5dX" ,
			PhoneNumber:        "08",
			Address:            "Diskonter Proklamasi",
		},
		{
			ShopName:           "Diskonter Pinang",
			IsOlseraIntegreted: true,
			OlseraAppID:        "YtqfBLJuDvku0eE45aBu",
			OlseraSecretKey:    "X3uYTOgakrVtDosLMStNdtV4UjSZHXA9",
			PhoneNumber:        "08",
			Address:            "Diskonter Pinang",
		},
		{
			ShopName:           "Diskonter Cinere",
			IsOlseraIntegreted: true,
			OlseraAppID:        "amg8Zh4TnQfq8GxPJCoz",
			OlseraSecretKey:    "Y4OTBpdEEbPcmzI4nqcHqjBe1tEi9cTT",
			PhoneNumber:        "08",
			Address:            "Diskonter Cinere",
		},
		{
			ShopName:           "Diskonter Kayu Manis",
			IsOlseraIntegreted: true,
			OlseraAppID:        "LTlexJCQvVblHP5p6d0X",
			OlseraSecretKey:    "Yf5F6KVzGoEg3zcpVFmM62ROoLclc8P8",
			PhoneNumber:        "08",
			Address:            "Diskonter Kayu Manis",
		},
		{
			ShopName:           "Diskonter Zambrud",
			IsOlseraIntegreted: true,
			OlseraAppID:        "CP3CsneLGEQg9WAfnslW",
			OlseraSecretKey:    "I5Sx2Wg6B6zqCrcOjGSvMfWORWayYBJg",
			PhoneNumber:        "08",
			Address:            "Diskonter Zambrud",
		},
		{
			ShopName:           "Diskonter Bintaro",
			IsOlseraIntegreted: true,
			OlseraAppID:        "ZCIqFazJfFlk20bGib4a",
			OlseraSecretKey:    "33EwuVVrjS5bQ0yCpUomQBM8LAeheons",
			PhoneNumber:        "08",
			Address:            "Diskonter Bintaro",
		},
		{
			ShopName:           "Diskonter Pekayon",
			IsOlseraIntegreted: true,
			OlseraAppID:        "tA8qzA7aEynOhDTo3Avp",
			OlseraSecretKey:    "efoGeHDTgytlJrABpVrTl3Ir1CqkpUi2",
			PhoneNumber:        "08",
			Address:            "Diskonter Pekayon",
		},
		{
			ShopName:           "Diskonter Harapan",
			IsOlseraIntegreted: true,
			OlseraAppID:        "mMiX1POMu82ucxoLpHmU",
			OlseraSecretKey:    "bhKKotSdbfDqScvLsWHW2aoZkKog8rAW",
			PhoneNumber:        "08",
			Address:            "Diskonter Harapan",
		},
		{
			ShopName:           "Diskonter Loji",
			IsOlseraIntegreted: true,
			OlseraAppID:        "Aq3xc2bgkMCuXQvLg2Vf",
			OlseraSecretKey:    "7mU2C3ilNtAAG4Ftp21WFsasBlpvtflb",
			PhoneNumber:        "08",
			Address:            "Diskonter Loji",
		},
		{
			ShopName:           "Diskonter Mayor Oking",
			IsOlseraIntegreted: true,
			OlseraAppID:        "IdGByAN35lZvdoBsoAkn",
			OlseraSecretKey:    "DLrPRTvX9W0tyEXjulDhVf18jXa40eIe",
			PhoneNumber:        "08",
			Address:            "Diskonter Mayor Oking",
		},
	}

	for _,destination := range destinations {
		secret_key, err := helpers.Encrypt(destination.OlseraSecretKey)
		if err != nil {
			return err
		}

		destination.OlseraSecretKey = secret_key
		if err := db.Create(&destination).Error; err != nil {
			return err
		}
	}

	return nil
}

func seedCategories(db *gorm.DB) error {
	categories := []models.Category{
		{
			CategorySlug:     helpers.CreateCategorySlug("TOYS HOBBIES (200-699)"),
			NameCategory:     "TOYS HOBBIES (200-699)",
			DiscountCategory: 50,
			MaxPriceCategory: 699999,
			MinPriceCategory: 0,
		},
		{
			CategorySlug:    helpers.CreateCategorySlug("TOYS HOBBIES (>700)"),
			NameCategory:     "TOYS HOBBIES (>700)",
			DiscountCategory: 40,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 700000,
		},
		{
			CategorySlug:    helpers.CreateCategorySlug("FMCG"),
			NameCategory:     "FMCG",
			DiscountCategory: 50,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			CategorySlug:   helpers.CreateCategorySlug("BABY PRODUCT"),
			NameCategory:     "BABY PRODUCT",
			DiscountCategory: 40,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("OTOMOTIF MOTOR"),
			NameCategory:     "OTOMOTIF MOTOR",
			DiscountCategory: 60,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("OTOMOTIF MOBIL"),
			NameCategory:     "OTOMOTIF MOBIL",
			DiscountCategory: 60,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("ELEKTRONIK HV"),
			NameCategory:     "ELEKTRONIK HV",
			DiscountCategory: 30,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("ELEKTRONIK ART"),
			NameCategory:     "ELEKTRONIK ART",
			DiscountCategory: 40,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("ACC (0-499)"),
			NameCategory:     "ACC (0-499)",
			DiscountCategory: 60,
			MaxPriceCategory: 499999,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("ACC (>500)"),
			NameCategory:     "ACC (>500)",
			DiscountCategory: 50,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 500000,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("FASHION"),
			NameCategory:     "FASHION",
			DiscountCategory: 60,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("ATK"),
			NameCategory:     "ATK",
			DiscountCategory: 50,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("ART HV"),
			NameCategory:     "ART HV",
			DiscountCategory: 40,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("TOYS HOBBIES (0-199)"),
			NameCategory:     "TOYS HOBBIES (0-199)",
			DiscountCategory: 60,
			MaxPriceCategory: 199999,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("ART"),
			NameCategory:     "ART",
			DiscountCategory: 50,
			MaxPriceCategory: 10000000,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("OTHER"),
			NameCategory:     "OTHER",
			DiscountCategory: 50,
			MaxPriceCategory: 2000000,
			MinPriceCategory: 0,
		},
		{
			CategorySlug: helpers.CreateCategorySlug("TOOLS"),
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

func truncateTableByClass(className string) error {

	conf, exists := seederRegistry[className]
	if !exists {
		return fmt.Errorf("seeder '%s' tidak memiliki mapping table", className)
	}

	// Matikan FK
	if err := config.DB.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
		return err
	}

	if err := config.DB.Exec("TRUNCATE TABLE " + conf.Table).Error; err != nil {
		config.DB.Exec("SET FOREIGN_KEY_CHECKS = 1")
		return err
	}

	// Hidupkan FK
	return config.DB.Exec("SET FOREIGN_KEY_CHECKS = 1").Error
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

