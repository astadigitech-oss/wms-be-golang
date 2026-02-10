package models

import "time"

type Palet struct {
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`

	IsBulky 			*string `gorm:"column:is_bulky;type:enum('done','waiting_list','waiting_approve')" json:"is_bulky"`

	NamePalet        	string  `gorm:"column:name_palet;not null" json:"name_palet"`
	CategoryPalet    	string  `gorm:"column:category_palet;not null" json:"category_palet"`
	TotalPricePalet  	float64 `gorm:"column:total_price_palet;type:decimal(15,2);not null" json:"total_price_palet"`
	TotalProductPalet 	int     `gorm:"column:total_product_palet;not null" json:"total_product_palet"`

	PaletBarcode string `gorm:"column:palet_barcode;not null" json:"palet_barcode"`

	CreatedAt *time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt *time.Time `gorm:"column:updated_at" json:"updated_at"`

	FilePDF     *string `gorm:"column:file_pdf" json:"file_pdf"`
	Description *string `gorm:"column:description;type:text" json:"description"`

	IsActive bool `gorm:"column:is_active;default:0" json:"is_active"`
	IsSale   bool `gorm:"column:is_sale;default:0" json:"is_sale"`

	CategoryID *uint64 `gorm:"column:category_id" json:"category_id"`

	WarehouseID   *string `gorm:"column:warehouse_id;type:char(36)" json:"warehouse_id"`
	WarehouseName string  `gorm:"column:warehouse_name;not null" json:"warehouse_name"`

	ProductConditionID   *string `gorm:"column:product_condition_id;type:char(36)" json:"product_condition_id"`
	ProductConditionName string  `gorm:"column:product_condition_name;not null" json:"product_condition_name"`

	ProductStatusID   *string `gorm:"column:product_status_id;type:char(36)" json:"product_status_id"`
	ProductStatusName string  `gorm:"column:product_status_name;not null" json:"product_status_name"`

	Discount *float64 `gorm:"column:discount;type:decimal(5,2)" json:"discount"`

	CategoryPaletID *string `gorm:"column:category_palet_id;type:char(36)" json:"category_palet_id"`

	// JSON field
	BrandIDs   []string `gorm:"column:brand_ids;type:json" json:"brand_ids"`
	BrandNames []string `gorm:"column:brand_names;type:json" json:"brand_names"`

	UserID *uint64 `gorm:"column:user_id" json:"user_id"`
}

// func (Palet) TableName() string {
// 	return "palets"
// }
