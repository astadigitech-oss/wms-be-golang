package models

import "time"

type RiwayatCheck struct {
	ID                      uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID                  uint      `gorm:"column:user_id"`
	CodeDocument            string    `gorm:"column:code_document;size:255;not null" json:"code_document"`
	NameDocument            string    `gorm:"size:255;not null" json:"base_document"`
	TotalData               int       `gorm:"column:total_data"`
	TotalDataIn             int       `gorm:"column:total_data_in"`
	TotalPriceIn            *float64   `gorm:"type:decimal(18,2);" json:"total_price_in"`
	TotalDataLolos          int       `gorm:"column:total_data_lolos"`
	TotalDataDamaged        int       `gorm:"column:total_data_damaged"`
	TotalDataAbnormal       int       `gorm:"column:total_data_abnormal"`
	TotalDiscrepancy        int       `gorm:"column:total_discrepancy"`
	StatusApprove           string    `gorm:"type:enum('pending','staging','done','display');default:'pending'" json:"status_approve"`
	PrecentageTotalData     *float64  `gorm:"type:decimal(5,2);" json:"precentage_total_data"`
	PercentageIn            *float64  `gorm:"type:decimal(5,2);" json:"percentage_in"`
	PercentageLolos         *float64  `gorm:"type:decimal(5,2);" json:"percentage_lolos"`
	PercentageDamaged       *float64  `gorm:"type:decimal(5,2);" json:"percentage_damaged"`
	PercentageAbnormal      *float64  `gorm:"type:decimal(5,2);" json:"precentage_abnormal"`
	PercentageDiscrepancy   *float64  `gorm:"type:decimal(5,2);" json:"percentage_discrepancy"`
	TotalPrice              *float64  `gorm:"type:decimal(13,2);" json:"total_price"`
	ValueDataLolos          *float64  `gorm:"type:decimal(13,2);" json:"value_data_lolos"`
	ValueDataDamaged        *float64  `gorm:"type:decimal(13,2);" json:"value_data_damaged"`
	ValueDataAbnormal       *float64  `gorm:"type:decimal(13,2);" json:"value_data_abnormal"`
	ValueDataDiscrepancy    *float64  `gorm:"type:decimal(13,2);" json:"value_data_discrepancy"`
	StatusFile              bool      `gorm:"default:false" json:"status_file"`
	CreatedAt               time.Time
	UpdatedAt               time.Time
}