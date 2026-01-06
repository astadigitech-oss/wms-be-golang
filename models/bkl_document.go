package models

import "time"

type BklDocument struct {
	ID             	uint64    	`gorm:"primaryKey;autoIncrement" json:"id"`
	CodeBkl       	string   	`gorm:"size:50;not null;unique" json:"code_bkl"`
	Status         	string   	`gorm:"size:15;not null;type:enum('process', 'done')" json:"status"`
	UserID	   		uint64    	`gorm:"index;not null" json:"user_id"`	
	CreatedAt      	time.Time 	`json:"created_at"`
	UpdatedAt      	time.Time 	`json:"updated_at"`

	User      		*User  		`gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	BklItem      	[]BklItem  	`gorm:"foreignKey:BklDocumentID;references:ID" json:"bkl_item,omitempty"`
}