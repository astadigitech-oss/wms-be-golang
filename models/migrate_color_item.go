package models

import "time"

type MigrateColorItem struct {
	ID                  uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	CodeDocumentMigrate string     `gorm:"column:code_document_migrate;type:varchar(100);not null;index" json:"code_document_migrate"`
	ProductColor        string     `gorm:"column:product_color;type:varchar(20);not null" json:"product_color"`
	ProductTotal        int        `gorm:"column:product_total;not null;default:0" json:"product_total"`
	Status		        string     `gorm:"column:status;type:enum('proses','selesai');not null" json:"status"`
	UserID              uint64     `gorm:"column:user_id;not null;index" json:"user_id"`
	CreatedAt           *time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           *time.Time `gorm:"column:updated_at" json:"updated_at"`

	//Relation
	User *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
}
