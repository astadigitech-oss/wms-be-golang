package models

import "time"

type SummaryOutbound struct {
	ID                  uint64          `gorm:"primaryKey;autoIncrement" json:"id"`
	Qty                 int64           `gorm:"default:0" json:"qty"`
	OldPriceProduct     float64 		`gorm:"type:decimal(15,2);not null" json:"old_price_product"`
	DisplayPriceProduct float64 		`gorm:"type:decimal(15,2);not null" json:"display_price_product"`
	PriceSale           float64 		`gorm:"type:decimal(15,2);not null" json:"price_sale"`
	Discount            float64 		`gorm:"type:decimal(15,2);not null;default:0" json:"discount"`
	OutboundDate        time.Time       `gorm:"type:date;not null;uniqueIndex" json:"outbound_date"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}
