package models

import (
	"time"
)

type SoColor struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	SummarySoColorID   uint64    `gorm:"not null; index" json:"summary_so_color_id"`
	ProductDamaged     int      `gorm:"not null;default:0" json:"product_damaged"`  // Pointer agar bisa menampung nilai null
	ProductAbnormal    int      `gorm:"not null;default:0" json:"product_abnormal"`
	ProductLost        int      `gorm:"not null;default:0" json:"product_lost"`
	ProductAddition    int      `gorm:"not null;default:0" json:"product_addition"`
	Color              string   `gorm:"not null;size:100" json:"color"`
	TotalColor         int      `gorm:"not null;default:0" json:"total_color"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	
	// Optional: BelongsTo relation back to parent
	// SummarySoColor SummarySoColor `gorm:"foreignKey:SummarySoColorID" json:"-"`
}