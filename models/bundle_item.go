package models

import "time"

type BundleItem struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	BundleID  uint64    `json:"bundle_id"`
	ProductID uint64    `json:"product_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	//relations
	Bundle  *Bundle  `gorm:"foreignKey:BundleID" json:"bundle,omitempty"`
	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}