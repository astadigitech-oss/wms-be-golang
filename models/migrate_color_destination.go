package models

import "time"

type MigrateColorDestination struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ShopName    string     `gorm:"column:shop_name;type:varchar(255);not null" json:"shop_name"`
	PhoneNumber string     `gorm:"column:phone_number;type:varchar(15);not null" json:"phone_number"`
	Address      string     `gorm:"column:address;type:text;not null" json:"address"`
	CreatedAt   *time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   *time.Time `gorm:"column:updated_at" json:"updated_at"`
}
