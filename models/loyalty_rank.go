package models

import "time"

type LoyaltyRank struct {
	ID                   uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Rank                 string     `gorm:"column:rank;type:varchar(255);not null" json:"rank"`
	MinTransactions      int        `gorm:"column:min_transactions;type:int;default:0" json:"min_transactions"`
	MinAmountTransaction float64    `gorm:"column:min_amount_transaction;type:decimal(15,2);default:0.00" json:"min_amount_transaction"`
	PercentageDiscount   float64    `gorm:"column:percentage_discount;type:decimal(5,2);default:0.00" json:"percentage_discount"`
	ExpiredWeeks         int        `gorm:"column:expired_weeks;type:int;default:0" json:"expired_weeks"`
	CreatedAt            *time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt            *time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}
