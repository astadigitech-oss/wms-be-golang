package models

import "time"

type ScrapDocument struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CodeDocument   string     `gorm:"column:code_document;type:varchar(255);not null;unique" json:"code_document"`
	UserID         uint64     `gorm:"column:user_id;not null;index" json:"user_id"`
	TotalProduct   int        `gorm:"column:total_product;not null;default:0" json:"total_product"`
	TotalNewPrice  float64    `gorm:"column:total_new_price;type:decimal(15,2);not null;default:0.00" json:"total_new_price"`
	TotalOldPrice  float64    `gorm:"column:total_old_price;type:decimal(15,2);not null;default:0.00" json:"total_old_price"`
	Status         string     `gorm:"column:status;type:enum('proses','lock','selesai');not null;default:'proses'" json:"status"`
	CreatedAt      *time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt      *time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`

	//relation
	ScrapItem []ScrapItem `gorm:"foreignKey:ScrapDocumentID;references:ID" json:"scrap_item,omitempty"`
	User *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
}
