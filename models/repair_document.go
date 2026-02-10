package models

import "time"

type RepairDocument struct {
    ID                 uint      `gorm:"primaryKey;column:id" json:"id"`
    CodeDocument       string    `gorm:"column:code_document;size:15;not null" json:"code_document"`
    UserID             uint      `gorm:"column:user_id;not null" json:"user_id"`
    TypeDocument       string    `gorm:"column:type_document;type:enum('damaged','non');not null" json:"type_document"`
    TotalProduct       int       `gorm:"column:total_product;default:0" json:"total_product"`
    TotalNewPrice      float64   `gorm:"column:total_new_price;type:decimal(15,2);default:0" json:"total_new_price"`
    TotalOldPrice      float64   `gorm:"column:total_old_price;type:decimal(15,2);default:0" json:"total_old_price"`
    Status             string    `gorm:"column:status;type:enum('proses','lock','selesai');default:'proses'" json:"status"`
    CreatedAt          *time.Time `gorm:"column:created_at" json:"created_at"`
    UpdatedAt          *time.Time `gorm:"column:updated_at" json:"updated_at"`

    // Relasi ke items
    Items []RepairDocumentItem `gorm:"foreignKey:RepairDocumentID" json:"items,omitempty"`
	User   *User                 `gorm:"foreignKey:UserID" json:"user,omitempty"`
}
