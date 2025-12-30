package models

import (
	"time"
)

type SummarySoColor struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Type      string    `gorm:"not null;size:20;type:enum('done', 'process');default:'process'" json:"type"` // Menggunakan pointer untuk mendukung nullable
	StartDate Date `gorm:"type:date;not null" json:"start_date"` //Struct Date ngambil di user_scan_web.go
	EndDate   *Date `gorm:"type:date" json:"end_date"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	
	// Relasi HasMany ke SoColor
	SoColors  []SoColor  `gorm:"foreignKey:SummarySoColorID;constraint:OnDelete:CASCADE" json:"so_colors"`
}