package models

import "time"

type SkuBundleHistory struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID          uint64     `gorm:"not null;index" json:"user_id"`
	CodeDocument    string    `gorm:"type:varchar(15);not null;index" json:"code_document"`
	BarcodeProduct  string     `gorm:"type:varchar(50);not null;index" json:"barcode_product"`
	NameProduct     string     `gorm:"type:varchar(1024);not null" json:"name_product"`

	PriceBefore     float64    `gorm:"type:decimal(12,2);not null;default:0.00" json:"price_before"`
	PriceAfter      float64    `gorm:"type:decimal(12,2);not null;default:0.00" json:"price_after"`

	QtyBefore       int        `gorm:"not null;default:0" json:"qty_before"`
	QtyAfter        int        `gorm:"not null;default:0" json:"qty_after"`

	TotalQtyBundle  *int       `json:"total_qty_bundle"`
	ItemsPerBundle  *int       `json:"items_per_bundle"`

	Type            string     `gorm:"type:enum('bundling','damaged');not null;" json:"type"`

	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`

	//relation
	User *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	
}