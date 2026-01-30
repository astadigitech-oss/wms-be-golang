package models

import "time"

type BulkyDocument struct {
	ID                   uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NameDocument         string    `gorm:"column:name_document;size:25" json:"name_document,omitempty"`
	CodeDocument	     string     `gorm:"column:code_document;size:25;not null;unique" json:"code_document"`
	UserID               uint64    `gorm:"column:user_id;index;not null" json:"user_id,omitempty"`
	NameUser             string    `gorm:"column:name_user;size:25;not null" json:"name_user,omitempty"`
	TotalProduct    	int64      `gorm:"column:total_product;not null" json:"total_product"`
	TotalOldPrice   	float64    `gorm:"column:total_old_price;type:decimal(15,2);not null" json:"total_old_price"`
	BuyerID              *uint64    `gorm:"column:buyer_id;index" json:"buyer_id,omitempty"`
	NameBuyer            *string    `gorm:"column:name_buyer;size:25" json:"name_buyer,omitempty"`
	StatusBulky          string     `gorm:"column:status_bulky;type:enum('proses','selesai');not null" json:"status_bulky"`
	DiscountBulky        float64       `gorm:"column:discount_bulky;not null;default:0" json:"discount_bulky"`
	AfterPriceBulky      float64    `gorm:"column:after_price_bulky;type:decimal(15,2);not null;default:0" json:"after_price_bulky"`
	CategoryBulky        *string    `gorm:"column:category_bulky;size:255" json:"category_bulky,omitempty"`
	IsSo            *string      `gorm:"type:enum('done','check','lost','addition');" json:"is_so"`
	UserSo          *uint64    `json:"user_so"`
	CreatedAt            *time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt            *time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`

	//Relation
	User *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	Buyer *Buyer `gorm:"foreignKey:BuyerID;references:ID" json:"buyer,omitempty"`
	BagProducts []BagProduct `gorm:"foreignKey:BulkyDocumentID;references:ID" json:"bag_products,omitempty"`
}
