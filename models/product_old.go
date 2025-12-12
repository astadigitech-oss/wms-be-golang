package models

import ( 
	"time" 
)

type ProductOld struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	CodeDocument     *string    `gorm:"size:255;index" json:"code_document"`
	InboundType      string   `gorm:"size:100;not null" json:"inbound_type"`
	OldBarcodeProduct *string  `gorm:"size:255" json:"old_barcode_product"`
	OldNameProduct   string   `gorm:"size:255;not null" json:"old_name_product"`
	OldQuantityProduct int `gorm:"size:255;not null" json:"old_quantity_product"`
	OldPriceProduct  float64    `gorm:"type:decimal(18,2);not null" json:"old_price_product"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// Relations
	// Document	 *Document `gorm:"foreignKey:CodeDocument;references:Code" json:"document,omitempty"`
	Product        *Product `gorm:"foreignKey:ProductOldID;references:ID" json:"products,omitempty"`
}