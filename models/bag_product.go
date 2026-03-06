package models

import "time"

type BagProduct struct {
	ID              uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BarcodeBag      string    `gorm:"column:barcode_bag;type:varchar(15);unique;not null" json:"barcode_bag"`
	NameBag         string    `gorm:"column:name_bag;type:varchar(7);not null" json:"name_bag"`
	CategoryBag		string	`gorm:"column:category_bag;type:varchar(50);not null" json:"category_bag"`
	UserID          uint64     `gorm:"column:user_id;not null;index" json:"user_id"`
	BulkyDocumentID uint64     `gorm:"column:bulky_document_id;not null;index" json:"bulky_document_id"`
	CategoryID	  	*uint64     `gorm:"column:category_id;index" json:"category_id"`
	Type			string		`gorm:"column:type;type:enum('category','color');not null" json:"type"`
	TotalProduct    int64      `gorm:"column:total_product;default:0" json:"total_product"`
	Status          string    `gorm:"column:status;type:enum('proses','done');not null" json:"status"`
	CreatedAt       *time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       *time.Time `gorm:"column:updated_at" json:"updated_at"`
	
	//Relation
	User *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	BulkySales []BulkySale `gorm:"foreignKey:BagProductID;references:ID" json:"bulky_sales"`
}
