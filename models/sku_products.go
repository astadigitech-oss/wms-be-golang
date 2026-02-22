package models

import "time"

type SkuProduct struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	CodeDocument    string    `gorm:"type:varchar(15);not null;index" json:"code_document"`
	BarcodeProduct  string    `gorm:"type:varchar(50);not null;index" json:"barcode_product"`
	NameProduct     string    `gorm:"type:varchar(1024);not null" json:"name_product"`
	PriceProduct    float64   `gorm:"type:decimal(12,2);not null" json:"price_product"`
	QuantityProduct int64      `gorm:"default:0" json:"quantity_product"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	//relation
	Document *Document `gorm:"foreignKey:CodeDocument;references:Code;" json:"document,omitempty"`
}

func (SkuProduct) TableName() string {
	return "sku_products"
}
