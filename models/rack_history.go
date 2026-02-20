package models

import "time"

type RackHistory struct {
	ID          uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint64        `gorm:"index; not null" json:"user_id"`
	RackID      uint64        `gorm:"index; not null" json:"rack_id"`
	ProductID   uint64        `gorm:"index; not null" json:"product_id"`
	Barcode     string         `gorm:"type:varchar(50);not null;index" json:"barcode"`
	ProductName *string        `gorm:"type:varchar(255)" json:"product_name"`
	Action      string     	   `gorm:"type:enum('IN','OUT','MOVE');not null" json:"action"`
	Source      *string        `gorm:"type:varchar(255)" json:"source"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (RackHistory) TableName() string {
	return "rack_histories"
}
