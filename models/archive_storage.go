package models

import (
	"time"
	"gorm.io/gorm"
)

type ArchiveStorage struct {
	ID              uint64          `gorm:"primaryKey;autoIncrement" json:"id"`
	CategoryProduct *string         `gorm:"type:varchar(100)" json:"category_product"`
	Color           *string         `gorm:"type:varchar(25)" json:"color"`
	TotalCategory   int64           `gorm:"default:0" json:"total_category"`
	TotalColor      int64           `gorm:"default:0" json:"total_color"`
	ValueProduct    float64			`gorm:"type:decimal(15,2);not null;default:0" json:"value_product"`
	Month           string          `gorm:"type:varchar(15);not null" json:"month"`
	Year            string          `gorm:"type:varchar(5);not null" json:"year"`
	Type            *string         `gorm:"type:varchar(15)" json:"type"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	DeletedAt       gorm.DeletedAt  `gorm:"index" json:"-"`
}

// func (ArchiveStorage) TableName() string {
// 	return "archive_storages"
// }
