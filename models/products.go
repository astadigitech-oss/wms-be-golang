package models

import (
	"time"
	"fmt"
)

type Product struct {
	ID            		uint64      `gorm:"primaryKey;autoIncrement" json:"id"`
	RackID  	  		*uint64     `json:"rack_id"`
	InboundType      	string   	`gorm:"size:20;not null" json:"inbound_type"`
	CodeDocument  		*string     `gorm:"size:15" json:"code_document"`

	OldBarcodeProduct 	*string  	`gorm:"size:50" json:"old_barcode_product"`
	OldNameProduct   	string  	`gorm:"size:255;not null" json:"old_name_product"`
	OldQuantityProduct 	int 		`gorm:"not null" json:"old_quantity_product"`
	OldPriceProduct  	float64		`gorm:"type:decimal(18,2);not null" json:"old_price_product"`
	ActualOldPrice		float64		`gorm:"type:decimal(18,2);not null" json:"actual_old_price"`
	ActualQuality       string     	`gorm:"type:enum('lolos','abnormal','damaged','non','migrate');default:'lolos';size:10;not null" json:"actual_quality"` 

	Barcode       		string     	`gorm:"size:50;unique;not null" json:"barcode"`
	Name          		string     	`gorm:"size:255;not null" json:"name"`
	Quantity      		int64      	`gorm:"not null" json:"quantity"`
	Price         		float64    	`gorm:"type:decimal(18,2); not null" json:"price"`
	Status        		string     	`gorm:"type:enum('display','expired','promo','bundle','repair','palet','dump','sale','migrate','bkl','scrap_qcd','slow_moving');size:15;not null" json:"status"`       // enum
	Quality       		string     	`gorm:"type:enum('lolos','abnormal','damaged','non','migrate');default:'lolos';size:10;not null" json:"quality"`      // enum
	QualityText   		*string     `gorm:"type:text" json:"quality_text"`
	CategoryID    		*uint64     `gorm:"index" json:"category_id"`
	TagColorID    		*uint64     `gorm:"index" json:"tag_color_id"`
	Discount      		*float64    `gorm:"type:decimal(18,2);" json:"discount"`
	DisplayPrice  		float64    	`gorm:"type:decimal(18,2);not null" json:"display_price"`
	LocationType  		*string     `gorm:"type:enum('main','staging');size:7" json:"location_type"` // enum
	StagingStage  		*string     `gorm:"type:enum('process','approve');size:7" json:"staging_stage"` // enum
	WarehouseType 		string      `gorm:"type:enum('type1', 'type2');default:'type1';size:5" json:"warehouse_type"`// enum
	IsSo          		*string     `gorm:"type:enum('check','done','lost','addition');size:8" json:"is_so"`
	IsExtraProduct    	bool     	`gorm:"default:false" json:"is_extra_product"`
	UserSo        		*uint64     `json:"user_so"`
	
	CreatedAt     		time.Time   `json:"created_at"`
	UpdatedAt     		time.Time   `json:"updated_at"`

	// Relations
	Document	*Document `gorm:"foreignKey:CodeDocument;references:Code" json:"document,omitempty"`
	Category   *Category   `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	ColorTag   *ColorTag   `gorm:"foreignKey:TagColorID" json:"color_tag,omitempty"`
	BundleItems []BundleItem `gorm:"foreignKey:ProductID" json:"bundle_items,omitempty"`
	Promo      *Promo      `gorm:"foreignKey:ProductID" json:"promo,omitempty"`
	Rack      *Rack      `gorm:"foreignKey:RackID" json:"rack,omitempty"`
}

func (p *Product) GetDaysSinceCreated() string {
	// Menghitung selisih waktu dari CreatedAt sampai sekarang
	duration := time.Since(p.CreatedAt)
	
	// Konversi durasi ke jam lalu bagi 24 untuk dapat jumlah hari
	days := int(duration.Hours() / 24)

	return fmt.Sprintf("%d Hari", days)
}
