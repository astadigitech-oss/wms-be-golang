package models

import "time"

type RepairDocumentItem struct {
    ID                uint       `gorm:"primaryKey;column:id" json:"id"`
    RepairDocumentID uint       `gorm:"column:repair_document_id;not null" json:"repair_document_id"`
    ProductID         uint       `gorm:"column:product_id;not null" json:"product_id"`
    CreatedAt         *time.Time `gorm:"column:created_at" json:"created_at"`
    UpdatedAt         *time.Time `gorm:"column:updated_at" json:"updated_at"`

    //Relasi
	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}
