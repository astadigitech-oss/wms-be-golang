package models

import (
    "time"
)

// Notification mewakili tabel 'notifications'
type Notification struct {
    // GORM Fields
    ID        uint `gorm:"primaryKey" json:"id"`
    UserID uint `gorm:"column:user_id;not null;index" json:"user_id"`
    NotificationName string `json:"notification_name"`
    Status           string `gorm:"type:enum('pending','done','staging','display');default:'pending'" json:"status"`
    Role             string `json:"role"`
    RiwayatCheckID *uint `gorm:"index;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"riwayat_check_id"`
    ReadAt           *time.Time `json:"read_at"`
    ExternalID       *uint `json:"external_id"`
	Approved		*string `gorm:"type:enum('0','1','2');default:'0'" json:"approved"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    
    User   User `gorm:"foreignKey:UserID;references:ID"` 
    RiwayatCheck   RiwayatCheck `gorm:"foreignKey:RiwayatCheckID;references:ID"` // Definisi Relasi
}