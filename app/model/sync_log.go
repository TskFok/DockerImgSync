package model

import "time"

type SyncLog struct {
	ID           int32     `gorm:"column:id;primaryKey;autoIncrement"`
	SyncTaskID   int32     `gorm:"column:sync_task_id;not null"`
	Trigger      string    `gorm:"column:trigger;type:varchar(16);not null"`
	Status       string    `gorm:"column:status;type:varchar(16);not null"`
	SourceDigest string    `gorm:"column:source_digest;type:varchar(255);not null"`
	Message      string    `gorm:"column:message;type:text;not null"`
	StartedAt    time.Time `gorm:"column:started_at;not null"`
	FinishedAt   time.Time `gorm:"column:finished_at;not null"`
}
