package models

import (
	"time"
)

type Category struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	CategorySlug     string    `gorm:"size:255;not null" json:"category_slug"`
	NameCategory     string    `gorm:"size:255;not null" json:"name_category"`
	DiscountCategory int      `gorm:"not null" json:"discount_category"`
	MaxPriceCategory float64  `gorm:"type:decimal(18,2);not null" json:"max_price_category"`
	MinPriceCategory float64  `gorm:"type:decimal(18,2);not null" json:"min_price_category"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	Products []Product `gorm:"foreignKey:CategoryID" json:"products,omitempty"`
	Bundles  []Bundle  `gorm:"foreignKey:CategoryID" json:"bundles,omitempty"`
}