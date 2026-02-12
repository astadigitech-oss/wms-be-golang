package models

import "time"

type SaleDocument struct {
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID uint64 `gorm:"column:user_id;not null;index" json:"user_id"`
	CodeDocumentSale string `gorm:"column:code_document_sale;type:varchar(255);not null;unique" json:"code_document_sale"`

	BuyerID      uint64 `gorm:"not null;index" json:"buyer_id"`
	BuyerName    string `gorm:"type:varchar(100);not null" json:"buyer_name"`
	BuyerPhone   string `gorm:"type:varchar(25);not null" json:"buyer_phone"`
	BuyerAddress string `gorm:"type:varchar(255);not null" json:"buyer_address"`
	BuyerPoint   int64  `gorm:"not null" json:"buyer_point"`

	NewDiscountSale *float64 `gorm:"column:new_discount_sale;type:double(15,2)" json:"new_discount_sale,omitempty"`
	TypeDiscount    *string  `gorm:"column:type_discount;type:enum('new','old')" json:"type_discount,omitempty"`

	TotalProduct      int64   `gorm:"not null" json:"total_product"`
	TotalOldPrice     float64 `gorm:"type:decimal(15,2);not null" json:"total_old_price"`
	TotalPrice        float64 `gorm:"type:decimal(15,2);not null" json:"total_price"` //total price sale
	TotalDisplayPrice      float64 `gorm:"type:decimal(15,2);not null" json:"total_display_price"`

	Status string `gorm:"type:enum('proses','selesai');not null;default:'proses'" json:"status"`

	CardboxQty        int     `gorm:"column:cardbox_qty;type:int;default:0" json:"cardbox_qty,omitempty"`
	CardboxUnitPrice  float64 `gorm:"column:cardbox_unit_price;type:decimal(15,2);default:0" json:"cardbox_unit_price,omitempty"`
	CardboxTotalPrice float64 `gorm:"column:cardbox_total_price;type:decimal(15,2);default:0" json:"cardbox_total_price,omitempty"`

	Voucher      *float64 `gorm:"column:voucher;type:decimal(15,2)" json:"voucher,omitempty"`

	Approved string `gorm:"column:approved;type:enum('0','1','2');default:'0'" json:"approved"`

	IsTax bool `gorm:"column:is_tax;type:tinyint(1);default:0" json:"is_tax"`

	Tax           *float64 `gorm:"column:tax;type:decimal(5,2)" json:"tax,omitempty"`
	GrandTotalPrice float64 `gorm:"column:grand_total_price;type:decimal(15,2);default:0;" json:"grand_total_price"`
	PriceAfterTax float64 `gorm:"column:price_after_tax;type:decimal(15,2);default:0" json:"price_after_tax"`

	CreatedAt *time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt *time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`

	//relation
	User *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	Sales []Sale `gorm:"foreignKey:SaleDocumentID;references:ID" json:"sales,omitempty"`
	Buyer *Buyer `gorm:"foreignKey:BuyerID;references:ID" json:"buyer,omitempty"`
}
