package models

import "time"

type Promo struct {
	ID           uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ProductID    uint64     `gorm:"uniqueIndex;not null" json:"product_id"`
	NamePromo    string    `gorm:"size:255" json:"name_promo,omitempty"`
	DiscountPromo float64  `gorm:"type:decimal(18,2)" json:"discount_promo,omitempty"`
	PricePromo   float64   `gorm:"type:decimal(18,2)" json:"price_promo,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}