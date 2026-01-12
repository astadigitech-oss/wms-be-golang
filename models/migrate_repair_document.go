package models

import "time"

type MigrateRepairDocument struct {
	ID             	uint64    	`gorm:"primaryKey;autoIncrement" json:"id"`
	UserID	   		uint64    	`gorm:"index;not null" json:"user_id"`	
	NameUser       	string   	`gorm:"size:50;not null;" json:"name_user"`
	Code         	string   	`gorm:"size:15;not null;unique" json:"code"`
	Status         	string   	`gorm:"size:15;not null;type:enum('process','added');default:'process'" json:"status"`
	CreatedAt      	time.Time 	`json:"created_at"`
	UpdatedAt      	time.Time 	`json:"updated_at"`

	User      		*User  		`gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	MigrateRepairItem      	[]MigrateRepairItem  	`gorm:"foreignKey:RepairDocumentID;references:ID" json:"repair_item,omitempty"`
}