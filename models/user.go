package models

import "time"

type User struct {
	ID              uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name            string     `json:"name" gorm:"size:255;not null"`
	Username        string     `json:"username" gorm:"size:255;unique;not null"`
	Email           string     `json:"email" gorm:"size:255;unique;not null"`
	EmailVerifiedAt *time.Time `json:"email_verified_at" gorm:"default:null"`
	Password        string     `json:"-" gorm:"size:255;not null"`
	RememberToken   string     `json:"-" gorm:"size:100"`
	RoleID          uint       `json:"role_id" gorm:"not null"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// realtions
	Role            *Role       `json:"role,omitempty" gorm:"foreignKey:RoleID"`
	UserLogs []UserLog `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE" json:"user_logs,omitempty"`
}

