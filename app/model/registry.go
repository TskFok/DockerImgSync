package model

import "time"

type Registry struct {
	ID           int32     `gorm:"column:id;primaryKey;autoIncrement"`
	Name         string    `gorm:"column:name;type:varchar(255);not null"`
	Address      string    `gorm:"column:address;type:varchar(255);not null"`
	Namespace    string    `gorm:"column:namespace;type:varchar(255);not null"`
	CredentialID int32     `gorm:"column:credential_id;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}
