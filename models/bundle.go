package models

import "time"

type Bundle struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID          *uint64    `json:"user_id"`
	NameBundle      string    `gorm:"size:255;not null" json:"name_bundle"`
	TotalPrice      float64   `gorm:"type:decimal(18,2);default:0" json:"total_price"`
	TotalPriceCustom float64  `gorm:"type:decimal(18,2);default:0" json:"total_price_custom"`
	TotalProduct    int64     `gorm:"default:0" json:"total_product"`
	Status          string    `gorm:"size:50;type:enum('not sale', 'sale', 'bundle', 'draft');default:'not sale'" json:"status"`
	Barcode         string    `gorm:"size:255;unique;not null" json:"barcode"`
	CategoryID      *uint64    `json:"category_id"`
	TagColorID      *uint64    `json:"tag_color_id"`
	WarehouseType   string    `gorm:"type:enum('type1', 'type2');default:'type1';size:50" json:"warehouse_type"`
	BundleType      string    `gorm:"type:enum('bundle','qcd','repair');default:'bundle';size:50" json:"bundle_type"`
	IsSo            *string      `gorm:"type:enum('done','check','lost','addition');" json:"is_so"`
	UserSo          *uint64    `json:"user_so"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	//relations
	Category *Category  `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	ColorTag *ColorTag  `gorm:"foreignKey:TagColorID" json:"color_tag,omitempty"`
	Items    []BundleItem `gorm:"foreignKey:BundleID" json:"items"`
}