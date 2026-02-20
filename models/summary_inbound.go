package models

import "time"

type SummaryInbound struct {
	ID                  uint64          `gorm:"primaryKey;autoIncrement" json:"id"`
	Qty                 int64           `gorm:"default:0" json:"qty"`
	OldPriceProduct     float64 		`gorm:"type:decimal(15,2);not null" json:"old_price_product"`
	NewPriceProduct     float64 		`gorm:"type:decimal(15,2);not null" json:"new_price_product"`
	DisplayPriceProduct float64 		`gorm:"type:decimal(15,2);not null" json:"display_price_product"`
	InboundDate      	time.Time       `gorm:"type:date;not null;uniqueIndex" json:"inbound_date"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}
