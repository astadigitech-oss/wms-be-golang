package models

import "time"

type Sale struct {
	ID                       uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID                   uint64     `gorm:"column:user_id;not null;index" json:"user_id"`

	SaleDocumentID         uint64      `gorm:"not null;index" json:"sale_document_id"`
	ProductBarcode         		*string      `gorm:"size:255;index" json:"product_barcode"` // merujuk kepada data product
	BundleBarcode       		*string      `gorm:"size:255;index" json:"bundle_barcode"` // merujuk kepada data bundle
	
	ProductName       			string      `gorm:"size:255;not null" json:"product_name"`
	ProductCategory       		string      `gorm:"size:255;not null" json:"product_category"`
	ProductOldPrice       		float64      `gorm:"type:decimal(15,2);not null" json:"product_old_price"`
	ProductPrice       			float64      `gorm:"type:decimal(15,2);not null" json:"product_price"`
	ProductQuantity       		int64      `gorm:"not null" json:"product_quantity"`
	ProductStatusBefore         string     `gorm:"column:product_status_before;not null" json:"product_status_before"`

	GaborSale                		*float64    `gorm:"column:gabor_sale;type:decimal(15,2)" json:"gabor_sale"`
	ProductPriceSale   		 		float64    `gorm:"type:decimal(15,2); not null" json:"product_price_sale"`
	BasePrice   	float64    `gorm:"type:decimal(15,2); not null" json:"BasePrice"` // price before loyalty discount
	// update price sale -> selisiih harga product price sale sebelumnya dengan harga update price yang diinput
	ProductUpdatePriceSale   *float64    `gorm:"column:product_update_price_sale;type:decimal(15,2)" json:"product_update_price_sale"`
	StatusSale               string      `gorm:"column:status_sale;type:enum('proses','selesai');default:'proses';not null" json:"status_sale"`  
	
	TotalDiscountSale   	float64    `gorm:"type:decimal(15,2); not null" json:"total_discount_sale"`
	DiscountSale   		 	float64    `gorm:"type:decimal(15,2); not null" json:"discount_sale"`
	TypeDiscount             *string     `gorm:"column:type_discount;type:enum('new','old')" json:"type_discount"`
	Approved                  string      `gorm:"column:approved;type:enum('0','1','2');default:'0'" json:"approved"`

	CreatedAt                 *time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt                 *time.Time `gorm:"column:updated_at" json:"updated_at"`

	//relation
	User *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	Product *Product `gorm:"foreignKey:ProductBarcode;references:Barcode" json:"product,omitempty"`
	Bundle *Bundle `gorm:"foreignKey:BundleBarcode;references:Barcode" json:"bundle,omitempty"`
	SaleDocument *SaleDocument `gorm:"foreignKey:SaleDocumentID;references:ID" json:"sale_document,omitempty"`
}
