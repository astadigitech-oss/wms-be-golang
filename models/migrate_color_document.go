package models

import "time"

type MigrateColorDocument struct {
	ID                        uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	CodeDocument      		  string     `gorm:"column:code_document;type:varchar(100);not null;unique" json:"code_document"`
	DestinyDocument		      string     `gorm:"column:destiny_document;type:varchar(50);not null" json:"destiny_document"`
	OlseraPurcaseID		      uint64     `gorm:"column:olsera_purchase_id;" json:"olsera_purchase_id"`
	OlseraResponseLog		  string     `gorm:"column:olsera_response_log;type:text;" json:"olsera_response_log"`
	TotalProductDocument	  int64    `gorm:"column:total_product_document;not null" json:"total_product_document"`
	StatusDocument		      string     `gorm:"column:status_document;type:enum('proses','selesai');not null" json:"status_document"`
	UserID                    uint64     `gorm:"column:user_id;not null;index" json:"user_id"`
	CreatedAt                 *time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt                 *time.Time `gorm:"column:updated_at" json:"updated_at"`

	//Relation
	User *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	Migrates []MigrateColorItem `gorm:"foreignKey:CodeDocumentMigrate;references:CodeDocument" json:"migrates,omitempty"`
}