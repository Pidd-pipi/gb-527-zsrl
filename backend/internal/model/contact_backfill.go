package model

import "time"

type ContactBackfill struct {
	ID                   uint      `gorm:"primaryKey"`
	ResolutionID         uint      `gorm:"index;not null"`
	WindowID             uint      `gorm:"index;not null"`
	ActualStartAt        time.Time `gorm:"not null"`
	ActualEndAt          time.Time `gorm:"not null"`
	ActualElevationDeg   float64   `gorm:"not null"`
	Outcome              string    `gorm:"size:24;not null"`
	ResultNote           string    `gorm:"size:500;not null"`
	StartDeviationSec    int       `gorm:"not null"`
	DurationDeviationSec int       `gorm:"not null"`
	ReviewStatus         string    `gorm:"size:24;index;not null"`
	RecordedBy           string    `gorm:"size:80;not null"`
	CreatedAt            time.Time `gorm:"not null"`
}

func (ContactBackfill) TableName() string { return "contact_backfills" }
