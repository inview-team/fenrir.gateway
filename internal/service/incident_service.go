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

// IncidentService предоставляет бизнес-логику для управления инцидентами.
type IncidentService struct {
	repo             IncidentRepository
	userRepo         UserRepository
	executor         ExecutorClient
	suggester        *ActionSuggester
	notificationChan chan<- *models.Incident // Канал для отправки уведомлений
}

// NewIncidentService создает новый экземпляр IncidentService.
func NewIncidentService(repo IncidentRepository, userRepo UserRepository, executor ExecutorClient, suggester *ActionSuggester, notifChan chan<- *models.Incident) *IncidentService {
	return &IncidentService{
		repo:             repo,
		userRepo:         userRepo,
		executor:         executor,
		suggester:        suggester,
		notificationChan: notifChan,
	}
}

// GetIncidentByID находит инцидент по ID.
func (s *IncidentService) GetIncidentByID(ctx context.Context, id uint) (*models.Incident, error) {
	return s.repo.FindByID(ctx, id)
}

// ListActiveIncidents возвращает список активных инцидентов.
func (s *IncidentService) ListActiveIncidents(ctx context.Context) ([]*models.Incident, error) {
	return s.repo.ListActive(ctx)
}

// ListClosed возвращает список закрытых инцидентов.
func (s *IncidentService) ListClosed(ctx context.Context, limit int, offset int) ([]*models.Incident, error) {
	return s.repo.ListClosed(ctx, limit, offset)
}

// CreateIncidentFromAlert создает новый инцидент на основе данных из Alertmanager.
// Если активный инцидент с таким же fingerprint уже существует, он будет возвращен.
func (s *IncidentService) CreateIncidentFromAlert(ctx context.Context, alert models.Alert) (*models.Incident, error) {
	// Проверяем, нет ли уже инцидента с таким же fingerprint.
	existing, err := s.repo.FindByFingerprint(ctx, alert.Fingerprint)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Это какая-то другая ошибка базы данных, которую мы должны вернуть.
		return nil, err
	}

	// Если ошибка `nil`, значит инцидент найден.
	if err == nil && existing.Status == models.StatusActive {
		log.Printf("Incident with fingerprint %s already exists and is active. Skipping creation.", alert.Fingerprint)
		// Ничего не делаем, просто выходим. Можно было бы обновить существующий, но пока пропустим.
		return existing, nil
	}

	// Извлекаем ключевые ресурсы из лейблов для быстрого доступа.
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

	// Отправляем уведомление о новом инциденте асинхронно
	go func() {
		s.notificationChan <- incident
	}()

	return incident, nil
}

// ExecuteAction выполняет действие над инцидентом.
func (s *IncidentService) ExecuteAction(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	incident, err := s.repo.FindByID(ctx, req.IncidentID)
	if err != nil {
		return models.ActionResult{Error: "Incident not found"}, err
	}

	// Выполняем действие через executor
	result := s.executor.ExecuteAction(req)

	// Записываем в лог аудита
	entry := models.AuditRecord{
		IncidentID: req.IncidentID,
		UserID:     req.UserID,
		Action:     req.Action,
		Parameters: models.JSONBMap(req.Parameters),
		Timestamp:  time.Now(),
		Success:    result.Error == "",
		Result:     result.Message,
	}

	incident.AuditLog = append(incident.AuditLog, entry)

	if err := s.repo.Update(ctx, incident); err != nil {
		return result, err // Возвращаем результат действия, но и ошибку сохранения
	}

	return result, nil
}

// GetResourceDetails получает детали ресурса от Executor'а.
func (s *IncidentService) GetResourceDetails(ctx context.Context, req models.ResourceDetailsRequest) (*models.ResourceDetails, error) {
	// В будущем здесь может быть дополнительная логика,
	// например, проверка прав доступа пользователя к этому ресурсу.
	return s.executor.GetResourceDetails(req)
}

// GetAvailableResources получает доступные профили ресурсов от Executor'а.
func (s *IncidentService) GetAvailableResources(ctx context.Context) (*models.AvailableResources, error) {
	return s.executor.GetAvailableResources()
}

// UpdateStatus изменяет статус инцидента и добавляет запись в лог аудита.
func (s *IncidentService) UpdateStatus(ctx context.Context, userID, incidentID uint, status models.IncidentStatus, reason string) error {
	incident, err := s.repo.FindByID(ctx, incidentID)
	if err != nil {
		return err
	}

	incident.Status = status
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

	return s.repo.Update(ctx, incident)
}
