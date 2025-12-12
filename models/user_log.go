package models

import "time"

type UserLog struct {
	ID       uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID   uint      `gorm:"not null;" json:"user_id"`
	NameUser string    `gorm:"size:255;not null" json:"name_user"`
	Action 	string    `gorm:"size:255;not null" json:"action"`
	Page     string    `gorm:"size:255;not null" json:"page"`
	Info     string    `gorm:"type:text;not null" json:"info"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationship
	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;" json:"user,omitempty"`
}