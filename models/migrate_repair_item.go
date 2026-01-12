package models

import "time"

type MigrateRepairItem struct {
	ID             		uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	RepairDocumentID  	uint64    `gorm:"index;not null" json:"repair_document_id"`
	ProductID     		uint64   `gorm:"uniqueIndex;not null" json:"product_id"`
	CreatedAt      	time.Time `json:"created_at"`
	UpdatedAt      	time.Time `json:"updated_at"`

	RepairDocument  	*MigrateRepairDocument `gorm:"foreignKey:RepairDocumentID;references:ID" json:"repair_document,omitempty"`
	Product     	*Product  `gorm:"foreignKey:ProductID;references:ID" json:"product,omitempty"`
}