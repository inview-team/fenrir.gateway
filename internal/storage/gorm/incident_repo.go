package gorm

import (
	"context"
	"time"

	"chatops-bot/internal/models"
	"chatops-bot/internal/service"

	"gorm.io/gorm"
)

type GormIncidentRepository struct {
	db *gorm.DB
}

func NewGormIncidentRepository(db *gorm.DB) (service.IncidentRepository, error) {
	return &GormIncidentRepository{db: db}, nil
}

func (r *GormIncidentRepository) Create(ctx context.Context, incident *models.Incident) error {
	return r.db.WithContext(ctx).Create(incident).Error
}

func (r *GormIncidentRepository) FindByID(ctx context.Context, id uint) (*models.Incident, error) {
	var incident models.Incident
	err := r.db.WithContext(ctx).Preload("AuditLog.User").Preload("ResolvedByUser").First(&incident, id).Error
	return &incident, err
}

func (r *GormIncidentRepository) FindByFingerprint(ctx context.Context, fingerprint string) (*models.Incident, error) {
	var incident models.Incident
	err := r.db.WithContext(ctx).Where("fingerprint = ?", fingerprint).First(&incident).Error
	return &incident, err
}

func (r *GormIncidentRepository) Update(ctx context.Context, incident *models.Incident) error {
	return r.db.WithContext(ctx).Save(incident).Error
}

func (r *GormIncidentRepository) ListActive(ctx context.Context) ([]*models.Incident, error) {
	var incidents []*models.Incident
	err := r.db.WithContext(ctx).Where("status = ?", models.StatusActive).Order("starts_at desc").Find(&incidents).Error
	return incidents, err
}

func (r *GormIncidentRepository) ListClosed(ctx context.Context, limit int, offset int) ([]*models.Incident, error) {
	var incidents []*models.Incident
	err := r.db.WithContext(ctx).
		Where("status IN (?, ?)", models.StatusResolved, models.StatusRejected).
		Order("created_at desc").
		Limit(limit).
		Offset(offset).
		Find(&incidents).Error
	return incidents, err
}

func (r *GormIncidentRepository) SetTelegramMessageID(ctx context.Context, incidentID uint, chatID, messageID int64) error {
	return r.db.WithContext(ctx).Model(&models.Incident{}).Where("id = ?", incidentID).Updates(map[string]interface{}{
		"telegram_chat_id":    chatID,
		"telegram_message_id": messageID,
	}).Error
}

func (r *GormIncidentRepository) SetTelegramTopicID(ctx context.Context, incidentID uint, topicID int64) error {
	return r.db.WithContext(ctx).Model(&models.Incident{}).Where("id = ?", incidentID).Update("telegram_topic_id", topicID).Error
}

func (r *GormIncidentRepository) FindClosedBefore(ctx context.Context, t time.Time) ([]*models.Incident, error) {
	var incidents []*models.Incident
	err := r.db.WithContext(ctx).
		Where("status IN (?, ?) AND ends_at < ?", models.StatusResolved, models.StatusRejected, t).
		Find(&incidents).Error
	return incidents, err
}
