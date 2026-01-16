package models

import "time"

type ScrapItem struct {
	ID             		uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ScrapDocumentID  	uint64    `gorm:"index;not null;" json:"scrap_document_id"`
	ProductID     		uint64   `gorm:"uniqueIndex;not null" json:"product_id"`
	CreatedAt      	time.Time `json:"created_at"`
	UpdatedAt      	time.Time `json:"updated_at"`

	ScrapDocument  	*ScrapDocument `gorm:"foreignKey:ScrapDocumentID;references:ID" json:"scrap_document,omitempty"`
	Product     	*Product  `gorm:"foreignKey:ProductID;references:ID" json:"product,omitempty"`
}