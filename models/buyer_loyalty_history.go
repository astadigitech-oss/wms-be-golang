package models

import "time"

type BuyerLoyaltyHistory struct {
	ID           uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BuyerID      uint64     `gorm:"column:buyer_id;not null;index" json:"buyer_id"`

	PreviousRankID uint64    `gorm:"index;not null" json:"previous_rank_id"`
	CurrentRankID  uint64    `gorm:"index;not null" json:"current_rank_id"`
	Note         *string    `gorm:"column:note;type:varchar(255)" json:"note,omitempty"`
	CreatedAt    *time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt    *time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`

	//relation
	Buyer *Buyer `gorm:"foreignKey:BuyerID;references:ID" json:"buyer,omitempty"`
	PreviousRank *LoyaltyRank `gorm:"foreignKey:PreviousRankID;references:ID" json:"previous_rank,omitempty"`
	CurrentRank *LoyaltyRank `gorm:"foreignKey:CurrentRankID;references:ID" json:"current_rank,omitempty"`
}
