package model

import "time"

type Credential struct {
	ID                int32     `gorm:"column:id;primaryKey;autoIncrement"`
	Name              string    `gorm:"column:name;type:varchar(255);not null"`
	Username          string    `gorm:"column:username;type:varchar(255);not null"`
	PasswordEncrypted string    `gorm:"column:password_encrypted;type:text;not null"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime"`
}
