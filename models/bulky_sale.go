package models

import (
	"time"
)

type BulkySale struct {
	ID                       uint64             `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BulkyDocumentID          uint64             `gorm:"column:bulky_document_id;not null;index" json:"bulky_document_id"`
	BagProductID             uint64             `gorm:"column:bag_product_id;not null;index" json:"bag_product_id"`

	ProductBarcode         		*string      `gorm:"size:255;index" json:"product_barcode"` // merujuk kepada data product
	BundleBarcode       		*string      `gorm:"size:255;index" json:"bundle_barcode"` // merujuk kepada data bundle
	
	ProductName       			string      `gorm:"size:255;not null" json:"product_name"`
	ProductCategory       		string      `gorm:"size:255;not null" json:"product_category"`
	ProductOldPrice       		float64      `gorm:"type:decimal(15,2);not null" json:"product_old_price"`
	ProductPrice       			float64      `gorm:"type:decimal(15,2);not null" json:"product_price"`
	ProductQuantity       		int64      `gorm:"not null" json:"product_quantity"`
	ProductStatusBefore         string     `gorm:"column:product_status_before;not null" json:"product_status_before"`

	AfterPriceBulkySale      float64      `gorm:"column:after_price_bulky_sale;type:decimal(15,2);not null" json:"after_price_bulky_sale"`
	DisplayPrice             float64      `gorm:"column:display_price;type:decimal(15,2);default:0.00" json:"display_price"`

	CreatedAt                *time.Time           `gorm:"column:created_at" json:"created_at"`
	UpdatedAt                *time.Time           `gorm:"column:updated_at" json:"updated_at"`

	//Relation
	Product *Product `gorm:"foreignKey:ProductBarcode;references:Barcode" json:"product,omitempty"`
	Bundle *Bundle `gorm:"foreignKey:BundleBarcode;references:Barcode" json:"bundle,omitempty"`
}
