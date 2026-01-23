package models

import "time"

type Buyer struct {
	ID                     	uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	LoyaltyRankID    		*uint64     `gorm:"column:loyalty_rank_id;index" json:"loyalty_rank_id"`
	NameBuyer              string     `gorm:"column:name_buyer;type:varchar(125);not null" json:"name_buyer"`
	PhoneBuyer             string     `gorm:"column:phone_buyer;type:varchar(20);not null" json:"phone_buyer"`
	AddressBuyer           string     `gorm:"column:address_buyer;type:varchar(255);not null" json:"address_buyer"`
	TypeBuyer              string     `gorm:"column:type_buyer;type:enum('Biasa','Repeat','Reguler');not null" json:"type_buyer"`

	AmountTransactionBuyer int64      `gorm:"column:amount_transaction_buyer;type:bigint;not null" json:"amount_transaction_buyer"`
	AmountPurchaseBuyer    float64    `gorm:"column:amount_purchase_buyer;type:decimal(15,2);not null" json:"amount_purchase_buyer"`
	AvgPurchaseBuyer       float64    `gorm:"column:avg_purchase_buyer;type:decimal(15,2);not null" json:"avg_purchase_buyer"`
	PointBuyer             int64      `gorm:"column:point_buyer;type:bigint;default:0" json:"point_buyer"`

	TransactionCount 	int        `gorm:"column:transaction_count;type:int;not null;default:0" json:"transaction_count"`
	LastUpgradeDate  	*time.Time `gorm:"column:last_upgrade_date" json:"last_upgrade_date,omitempty"`
	ExpireDate       	*time.Time `gorm:"column:expire_date" json:"expire_date,omitempty"`

	Email                  *string    `gorm:"column:email;type:varchar(255)" json:"email,omitempty"`
	Password               *string    `gorm:"column:password;type:varchar(255)" json:"-"`

	LoyaltyUpdatedAt             *time.Time `gorm:"column:loyalty_updated_at" json:"loyalty_updated_at"`
	CreatedAt              *time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt              *time.Time `gorm:"column:updated_at" json:"updated_at"`

	//relation
	Rank *LoyaltyRank `gorm:"foreignKey:LoyaltyRankID;references:ID" json:"rank,omitempty"`
}
