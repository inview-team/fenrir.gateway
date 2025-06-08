package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"chatops-bot/internal/models"

	"gorm.io/gorm"
)

type IncidentService struct {
	repo              IncidentRepository
	userRepo          UserRepository
	executor          ExecutorClient
	suggester         *ActionSuggester
	notificationChan  chan<- *models.Incident
	updateChan        chan<- *models.Incident
	topicDeletionChan chan<- *models.Incident
}

func NewIncidentService(repo IncidentRepository, userRepo UserRepository, executor ExecutorClient, suggester *ActionSuggester, notifChan, updateChan, topicDeletionChan chan<- *models.Incident) *IncidentService {
	return &IncidentService{
		repo:              repo,
		userRepo:          userRepo,
		executor:          executor,
		suggester:         suggester,
		notificationChan:  notifChan,
		updateChan:        updateChan,
		topicDeletionChan: topicDeletionChan,
	}
}

func (s *IncidentService) GetIncidentByID(ctx context.Context, id uint) (*models.Incident, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *IncidentService) ListActiveIncidents(ctx context.Context) ([]*models.Incident, error) {
	return s.repo.ListActive(ctx)
}

func (s *IncidentService) ListClosed(ctx context.Context, limit int, offset int) ([]*models.Incident, error) {
	return s.repo.ListClosed(ctx, limit, offset)
}

func (s *IncidentService) CreateIncidentFromAlert(ctx context.Context, alert models.Alert) (*models.Incident, error) {
	existing, err := s.repo.FindByFingerprint(ctx, alert.Fingerprint)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if err == nil && existing.Status == models.StatusActive {
		log.Printf("Incident with fingerprint %s already exists and is active. Skipping creation.", alert.Fingerprint)
		return existing, nil
	}

	affectedResources := make(models.JSONBMap)
	if val, ok := alert.Labels["deployment"]; ok {
		affectedResources["deployment"] = val
	}
	if val, ok := alert.Labels["pod"]; ok {
		affectedResources["pod"] = val
	}
	if val, ok := alert.Labels["namespace"]; ok {
		affectedResources["namespace"] = val
	}

	incident := &models.Incident{
		Fingerprint:       alert.Fingerprint,
		Status:            models.StatusActive,
		StartsAt:          alert.StartsAt,
		Summary:           alert.Annotations["summary"],
		Description:       alert.Annotations["description"],
		Labels:            models.JSONBMap(alert.Labels),
		AffectedResources: affectedResources,
		AuditLog:          []models.AuditRecord{},
	}

	err = s.repo.Create(ctx, incident)
	if err != nil {
		return nil, err
	}

	go func() {
		s.notificationChan <- incident
	}()

	return incident, nil
}

func (s *IncidentService) SetTelegramMessageID(ctx context.Context, incidentID uint, chatID, messageID int64) error {
	return s.repo.SetTelegramMessageID(ctx, incidentID, chatID, messageID)
}

func (s *IncidentService) SetTelegramTopicID(ctx context.Context, incidentID uint, topicID int64) error {
	return s.repo.SetTelegramTopicID(ctx, incidentID, topicID)
}

func (s *IncidentService) ExecuteAction(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	incident, err := s.repo.FindByID(ctx, req.IncidentID)
	if err != nil {
		return models.ActionResult{Error: "Incident not found"}, err
	}

	result := s.executor.ExecuteAction(req)

	entry := models.AuditRecord{
		IncidentID: req.IncidentID,
		UserID:     req.UserID,
		Action:     req.Action,
		Parameters: models.JSONBMap(req.Parameters),
		Timestamp:  time.Now(),
		Success:    result.Error == "",
		Result:     result.Message,
	}

	addAffectedResourceToAudit(&entry, req)

	incident.AuditLog = append(incident.AuditLog, entry)

	if err := s.repo.Update(ctx, incident); err != nil {
		return result, err
	}

	s.updateChan <- incident

	return result, nil
}

func (s *IncidentService) GetResourceDetails(ctx context.Context, req models.ResourceDetailsRequest) (*models.ResourceDetails, error) {
	return s.executor.GetResourceDetails(req)
}

func (s *IncidentService) GetAvailableResources(ctx context.Context) (*models.AvailableResources, error) {
	return s.executor.GetAvailableResources()
}

func (s *IncidentService) DeleteOldIncidentTopics(ctx context.Context, retention time.Duration) {
	threshold := time.Now().Add(-retention)
	incidents, err := s.repo.FindClosedBefore(ctx, threshold)
	if err != nil {
		log.Printf("Error finding old incidents to delete topics: %v", err)
		return
	}

	for _, incident := range incidents {
		if incident.TelegramTopicID.Valid {
			log.Printf("Scheduling topic deletion for incident #%d", incident.ID)
			s.topicDeletionChan <- incident
		}
	}
}

func (s *IncidentService) UpdateStatus(ctx context.Context, userID, incidentID uint, status models.IncidentStatus, reason string) error {
	incident, err := s.repo.FindByID(ctx, incidentID)
	if err != nil {
		return err
	}

	incident.Status = status
	now := time.Now()
	if status == models.StatusResolved || status == models.StatusRejected {
		incident.EndsAt = &now
	}

	if status == models.StatusResolved {
		incident.ResolvedBy = &userID
	}
	if status == models.StatusRejected {
		incident.RejectionReason = reason
	}

	entry := models.AuditRecord{
		IncidentID: incidentID,
		UserID:     userID,
		Action:     "update_status",
		Parameters: map[string]string{
			"new_status": string(status),
			"reason":     reason,
		},
		Timestamp: time.Now(),
		Success:   true,
		Result:    fmt.Sprintf("Status updated to %s", status),
	}
	incident.AuditLog = append(incident.AuditLog, entry)

	err = s.repo.Update(ctx, incident)
	if err == nil {
		s.updateChan <- incident
	}
	return err
}

func addAffectedResourceToAudit(entry *models.AuditRecord, req models.ActionRequest) {
	resourceIdentifier := ""
	if pod, ok := req.Parameters["pod"]; ok {
		resourceIdentifier = fmt.Sprintf("pod: %s", pod)
	} else if deployment, ok := req.Parameters["deployment"]; ok {
		resourceIdentifier = fmt.Sprintf("deployment: %s", deployment)
	}

	if resourceIdentifier != "" {
		if entry.Parameters == nil {
			entry.Parameters = make(models.JSONBMap)
		}
		entry.Parameters["affected_resource"] = resourceIdentifier
	}
}
