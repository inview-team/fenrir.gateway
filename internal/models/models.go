package models

import (
	"database/sql"
	"time"

	"gorm.io/gorm"
)

// IncidentStatus определяет возможные состояния инцидента.
type IncidentStatus string

const (
	StatusActive   IncidentStatus = "active"
	StatusResolved IncidentStatus = "resolved"
	StatusRejected IncidentStatus = "rejected"
)

// User представляет пользователя системы (инженера).
// В MVP все пользователи имеют роль "админ".
type User struct {
	gorm.Model
	TelegramID int64  `gorm:"uniqueIndex;not null"`
	Username   string `gorm:"uniqueIndex"`
	FirstName  string
	LastName   string
	IsAdmin    bool `gorm:"default:true"`
}

// Incident представляет собой инцидент, созданный на основе алерта из Alertmanager.
type Incident struct {
	gorm.Model
	ID                uint           `gorm:"primarykey"`
	Fingerprint       string         `gorm:"uniqueIndex;not null"`
	Status            IncidentStatus `gorm:"index;not null"`
	StartsAt          time.Time
	EndsAt            *time.Time
	Summary           string
	Description       string
	Labels            JSONBMap      // Лейблы из алерта для контекстно-зависимых действий.
	AffectedResources JSONBMap      // Извлеченные из лейблов ресурсы (deployment, pod и т.д.).
	AuditLog          []AuditRecord `gorm:"foreignKey:IncidentID"`
	ResolvedBy        *uint
	ResolvedByUser    User `gorm:"foreignKey:ResolvedBy"`
	RejectionReason   string

	// Telegram-specific fields for message management
	TelegramChatID    sql.NullInt64 `gorm:"index"`
	TelegramMessageID sql.NullInt64 `gorm:"index"`
	TelegramTopicID   sql.NullInt64 `gorm:"index"`
}

// AuditRecord хранит запись о действии, выполненном над инцидентом.
// Является неизменяемой частью истории инцидента.
type AuditRecord struct {
	gorm.Model
	IncidentID uint      `gorm:"index;not null"`
	UserID     uint      `gorm:"not null"`
	User       User      `gorm:"foreignKey:UserID"`
	Action     string    // Например, "rollback_deployment"
	Parameters JSONBMap  // Параметры, с которыми было вызвано действие.
	Timestamp  time.Time `gorm:"not null"`
	Success    bool
	Result     string `gorm:"type:text"`
}
