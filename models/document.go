package models

import "time"

type Document struct {
	ID                	uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Code      			string         `gorm:"size:15;uniqueIndex;not null" json:"code_document"`
	NameDocument      	string        `gorm:"size:255;not null" json:"name_document"`
	TotalColumnDocument int64       `gorm:"not null" json:"total_column_document"`
	TotalRowData 		int64       `gorm:"not null" json:"total_row_data"`
	StatusDocument    	string        `gorm:"type:enum('pending','inprogress','done');default:'pending';size:10;not null" json:"status_document"`
	CustomBarcode     	*string        `gorm:"size:20" json:"custom_barcode"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`

	// Relationships
	Generates         []Generate     `gorm:"foreignKey:CodeDocument;references:Code;" json:"generates,omitempty"`
	// ProductOlds []ProductOld `gorm:"foreignKey:CodeDocument;references:Code;" json:"product_olds,omitempty"`
}



