package models

import (
	"time"
)

type DailyInventorySnapshot struct {
    ID           uint      `gorm:"primaryKey;autoIncrement;" json:"id"`
    SnapshotDate time.Time `gorm:"column:snapshot_date;index;type:date" json:"snapshot_date"`
    TotalQty     int64     `gorm:"column:total_qty" json:"total_qty"`
    TotalPrice   float64   `gorm:"column:total_price" json:"total_price"`
	CreatedAt    *time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt    *time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

