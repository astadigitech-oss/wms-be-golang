package models

import "time"

type Generate struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	CodeDocument string    `gorm:"size:255;not null" json:"code_document"`
	Data         string   `gorm:"type:longtext;not null" json:"data"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	
	// relations
	Document     *Document  `json:"document,omitempty" gorm:"foreignKey:CodeDocument;references:Code"`
}