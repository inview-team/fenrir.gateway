package models

import (
	"database/sql"
	"time"

	"gorm.io/gorm"
)

type IncidentStatus string

const (
	StatusActive   IncidentStatus = "active"
	StatusResolved IncidentStatus = "resolved"
	StatusRejected IncidentStatus = "rejected"
)

type User struct {
	gorm.Model
	TelegramID int64  `gorm:"uniqueIndex;not null"`
	Username   string `gorm:"uniqueIndex"`
	FirstName  string
	LastName   string
	IsAdmin    bool `gorm:"default:true"`
}

type Incident struct {
	gorm.Model
	ID                uint           `gorm:"primarykey"`
	Fingerprint       string         `gorm:"uniqueIndex;not null"`
	Status            IncidentStatus `gorm:"index;not null"`
	StartsAt          time.Time
	EndsAt            *time.Time
	Summary           string
	Description       string
	Labels            JSONBMap
	AffectedResources JSONBMap
	AuditLog          []AuditRecord `gorm:"foreignKey:IncidentID"`
	ResolvedBy        *uint
	ResolvedByUser    User `gorm:"foreignKey:ResolvedBy"`
	RejectionReason   string

	TelegramChatID    sql.NullInt64 `gorm:"index"`
	TelegramMessageID sql.NullInt64 `gorm:"index"`
	TelegramTopicID   sql.NullInt64 `gorm:"index"`
}

type AuditRecord struct {
	gorm.Model
	IncidentID uint `gorm:"index;not null"`
	UserID     uint `gorm:"not null"`
	User       User `gorm:"foreignKey:UserID"`
	Action     string
	Parameters JSONBMap
	Timestamp  time.Time `gorm:"not null"`
	Success    bool
	Result     string `gorm:"type:text"`
}
