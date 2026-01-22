package models

import "time"

type BundleItem struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    *uint    `gorm:"index" json:"user_id"`
	BundleID  *uint64    `gorm:"index" json:"bundle_id"`
	ProductID uint64    `gorm:"unique;not null" json:"product_id"`
	BundleStage *string    `gorm:"size:50;type:enum('bundle_filter', 'repair_filter', 'qcd_filter')" json:"bundle_stage"`
	Status 	string   `gorm:"type:enum('display','expired','promo','bundle','palet','dump','sale','migrate','bkl');size:50;not null" json:"status"`  
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	//relations
	User  *Bundle  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Bundle  *Bundle  `gorm:"foreignKey:BundleID" json:"bundle,omitempty"`
	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}