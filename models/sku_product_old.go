package models

import "time"

type SkuProductOld struct {
	ID                     uint64          `gorm:"primaryKey;autoIncrement" json:"id"`
	CodeDocument           string          `gorm:"type:varchar(20);not null;index" json:"code_document"`
	OldBarcodeProduct      string          `gorm:"type:varchar(50);not null" json:"old_barcode_product"`
	OldNameProduct         string          `gorm:"type:varchar(1024);not null" json:"old_name_product"`
	OldPriceProduct        float64         `gorm:"type:decimal(12,2);not null" json:"old_price_product"`
	OldQuantityProduct     int64             `gorm:"default:0" json:"old_quantity_product"`
	ActualQuantityProduct  int64             `gorm:"default:0" json:"actual_quantity_product"`
	DamagedQuantityProduct int64             `gorm:"default:0" json:"damaged_quantity_product"`
	LostQuantityProduct    int64             `gorm:"default:0" json:"lost_quantity_product"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

func (SkuProductOld) TableName() string {
	return "sku_product_olds"
}
