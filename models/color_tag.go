package models

import "time"

type ColorTag struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	HexaCodeColor  string   `gorm:"size:50;not null" json:"hexa_code_color"`
	NameColor      string   `gorm:"size:255;not null" json:"name_color"`
	MinPriceColor  float64  `gorm:"type:decimal(18,2);not null" json:"min_price_color"`
	MaxPriceColor  float64  `gorm:"type:decimal(18,2);not null" json:"max_price_color"`
	FixedPriceColor float64 `gorm:"type:decimal(18,2);not null" json:"fixed_price_color"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	Products []Product `gorm:"foreignKey:TagColorID" json:"products,omitempty"`
	Bundles  []Bundle  `gorm:"foreignKey:TagColorID" json:"bundles,omitempty"`
}