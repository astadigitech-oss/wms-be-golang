package models

import (
	"time"

)

type Role struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	RoleName string `json:"role_name" gorm:"size:255;not null"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}