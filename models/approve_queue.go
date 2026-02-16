package models

import (
    "time"
    "gorm.io/gorm"
)

// ApproveQueue mewakili tabel 'approve_queues'
type ApproveQueue struct {
    ID        uint `gorm:"primaryKey" json:"id"`
    UserID 			  *uint `gorm:"index;constraint:OnDelete:SET NULL;" json:"user_id"`
    ProductID 		  *uint `json:"product_id"`
    Type              *string `gorm:"column:type;size:20" json:"type"`
    CodeDocument      *string `gorm:"column:code_document;size:15" json:"code_document"`
    OldPriceProduct   *float64 `gorm:"type:decimal(15,2)" json:"old_price_product"`
	NewNameProduct    *string `gorm:"size:255" json:"new_name_product"`
	NewQuantityProduct *int ` json:"new_quantity_product"`
	NewPriceProduct   *float64 `gorm:"type:decimal(15,2)" json:"new_price_product"`
	NewDiscount       *float64 `gorm:"type:decimal(15,2)" json:"new_discount"`
	TagColorID       *uint64 `json:"tag_color_id"`
	CategoryID      *uint64 `json:"category_id"`
	Status		  string `gorm:"type:enum('0','1');size:2;not null" json:"status"`
	CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
    
	// relations
	User   *User  `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	Category   *Category   `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	ColorTag   *ColorTag   `gorm:"foreignKey:TagColorID" json:"color_tag,omitempty"`
}