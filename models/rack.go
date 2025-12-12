package models

import (
    "time"
)

type Rack struct {
	ID uint `json:"id" gorm:"primaryKey"`
	Name string `json:"name" gorm:"type:varchar(255);not null;uniqueIndex:idx_rack_name_source"`
	Source *string `json:"source" gorm:"type:enum('staging','display');default:null;uniqueIndex:idx_rack_name_source"`
	CategoryID *uint64 `json:"category_id" gorm:"index"`
	TotalData int `json:"total_data" gorm:"default:0"`
	TotalNewPriceProduct float64 `json:"total_new_price_product" gorm:"type:decimal(15,2);default:0"`
	TotalOldPriceProduct float64 `json:"total_old_price_product" gorm:"type:decimal(15,2);default:0"`
	TotalDisplayPriceProduct float64 `json:"total_display_price_product" gorm:"type:decimal(15,2);default:0"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relations
	Category *Category `json:"category,omitempty" gorm:"foreignKey:CategoryID;"`
	Product []Product `json:"products,omitempty" gorm:"foreignKey:RackID"`
}