package models

import (
	"time"
)

type SummarySoCategory struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	Type      		   string    `gorm:"not null;size:20;type:enum('done', 'process');default:'process'" json:"type"` // Menggunakan pointer untuk mendukung nullable
	StartDate 		   Date `gorm:"type:date;not null" json:"start_date"` //Struct Date ngambil di user_scan_web.go
	EndDate   		   *Date `gorm:"type:date" json:"end_date"`
	ProductBundle     int      `gorm:"not null;default:0" json:"product_bundle"`  // Pointer agar bisa menampung nilai null
	ProductStaging     int      `gorm:"not null;default:0" json:"product_staging"`  // Pointer agar bisa menampung nilai null
	ProductInventory     int      `gorm:"not null;default:0" json:"product_inventory"`  // Pointer agar bisa menampung nilai null
	ProductDamaged     int      `gorm:"not null;default:0" json:"product_damaged"`  // Pointer agar bisa menampung nilai null
	ProductAbnormal    int      `gorm:"not null;default:0" json:"product_abnormal"`
	ProductNon    		int      `gorm:"not null;default:0" json:"product_non"`
	ProductLost        int      `gorm:"not null;default:0" json:"product_lost"`
	ProductAddition    int      `gorm:"not null;default:0" json:"product_addition"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}