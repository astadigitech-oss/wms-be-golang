package models

import "time"

type BklItem struct {
	ID             	uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	BklDocumentID  	uint64    `gorm:"index;not null" json:"bkl_document_id"`
	TagColorID     	*uint64   `gorm:"index" json:"tag_color_id"`
	Qty            	int       `gorm:"default:0;not null" json:"qty"`
	Type      		string    `gorm:"size:5;not null;type:enum('in', 'out')" json:"type"`
	IsDamaged		bool      `gorm:"default:false;not null" json:"is_damaged"`
	CreatedAt      	time.Time `json:"created_at"`
	UpdatedAt      	time.Time `json:"updated_at"`

	BklDocument  	*BklDocument `gorm:"foreignKey:BklDocumentID;references:ID" json:"bkl_document,omitempty"`
	ColorTag     	*ColorTag  `gorm:"foreignKey:TagColorID;references:ID" json:"color_tag,omitempty"`
}