package models

import "time"

type SaleDocument struct {
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID uint64 `gorm:"column:user_id;not null" json:"user_id"`
	CodeDocumentSale string `gorm:"column:code_document_sale;type:varchar(255);not null;unique" json:"code_document_sale"`

	BuyerIDDocumentSale      uint64 `gorm:"column:buyer_id_document_sale;not null" json:"buyer_id_document_sale"`
	BuyerNameDocumentSale    string `gorm:"column:buyer_name_document_sale;type:varchar(255);not null" json:"buyer_name_document_sale"`
	BuyerPhoneDocumentSale   string `gorm:"column:buyer_phone_document_sale;type:varchar(255);not null" json:"buyer_phone_document_sale"`
	BuyerAddressDocumentSale string `gorm:"column:buyer_address_document_sale;type:varchar(255);not null" json:"buyer_address_document_sale"`
	BuyerPointDocumentSale   int64  `gorm:"column:buyer_point_document_sale;not null" json:"buyer_point_document_sale"`

	NewDiscountSale *float64 `gorm:"column:new_discount_sale;type:double(15,2)" json:"new_discount_sale,omitempty"`
	TypeDiscount    *string  `gorm:"column:type_discount;type:enum('new','old')" json:"type_discount,omitempty"`

	TotalProductDocumentSale      int64   `gorm:"column:total_product_document_sale;not null" json:"total_product_document_sale"`
	TotalOldPriceDocumentSale     float64 `gorm:"column:total_old_price_document_sale;type:decimal(15,2);not null" json:"total_old_price_document_sale"`
	TotalPriceDocumentSale        float64 `gorm:"column:total_price_document_sale;type:decimal(15,2);not null" json:"total_price_document_sale"`
	TotalDisplayDocumentSale      float64 `gorm:"column:total_display_document_sale;type:decimal(15,2);not null" json:"total_display_document_sale"`

	StatusDocumentSale string `gorm:"column:status_document_sale;type:enum('proses','selesai');not null;default:'proses'" json:"status_document_sale"`

	CardboxQty        *int     `gorm:"column:cardbox_qty;type:int" json:"cardbox_qty,omitempty"`
	CardboxUnitPrice  *float64 `gorm:"column:cardbox_unit_price;type:decimal(15,2)" json:"cardbox_unit_price,omitempty"`
	CardboxTotalPrice *float64 `gorm:"column:cardbox_total_price;type:decimal(15,2)" json:"cardbox_total_price,omitempty"`

	Voucher      *float64 `gorm:"column:voucher;type:decimal(15,2)" json:"voucher,omitempty"`
	CodeDocument *string  `gorm:"column:code_document;type:varchar(255)" json:"code_document,omitempty"`

	Approved string `gorm:"column:approved;type:enum('0','1','2');default:'0'" json:"approved"`

	IsTax bool `gorm:"column:is_tax;type:tinyint(1);default:0" json:"is_tax"`

	Tax           *float64 `gorm:"column:tax;type:decimal(5,2)" json:"tax,omitempty"`
	PriceAfterTax *float64 `gorm:"column:price_after_tax;type:decimal(15,2)" json:"price_after_tax,omitempty"`

	CreatedAt *time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt *time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`

	//relation
	User *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
}
