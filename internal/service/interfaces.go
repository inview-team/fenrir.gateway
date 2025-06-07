package service

import (
	"context"
	"time"

	"chatops-bot/internal/models"
)

// IncidentRepository определяет интерфейс для хранения и получения инцидентов.
type IncidentRepository interface {
	Create(ctx context.Context, incident *models.Incident) error
	FindByID(ctx context.Context, id uint) (*models.Incident, error)
	FindByFingerprint(ctx context.Context, fingerprint string) (*models.Incident, error)
	Update(ctx context.Context, incident *models.Incident) error
	ListActive(ctx context.Context) ([]*models.Incident, error)
	ListClosed(ctx context.Context, limit int, offset int) ([]*models.Incident, error)
	SetTelegramMessageID(ctx context.Context, incidentID uint, chatID, messageID int64) error
	SetTelegramTopicID(ctx context.Context, incidentID uint, topicID int64) error
	FindClosedBefore(ctx context.Context, t time.Time) ([]*models.Incident, error)
}

// UserRepository определяет интерфейс для работы с пользователями.
type UserRepository interface {
	FindOrCreateByTelegramID(ctx context.Context, telegramID int64, username, firstName, lastName string) (*models.User, error)
	ListAll(ctx context.Context) ([]*models.User, error)
	FindByID(ctx context.Context, id uint) (*models.User, error)
}

// ExecutorClient определяет интерфейс для взаимодействия с воркером,
// который выполняет действия в Kubernetes.
type ExecutorClient interface {
	ExecuteAction(req models.ActionRequest) models.ActionResult
	GetResourceDetails(req models.ResourceDetailsRequest) (*models.ResourceDetails, error)
	GetAvailableResources() (*models.AvailableResources, error)
}
