package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"chatops-bot/internal/models"
	"chatops-bot/internal/service"
)

// MockIncidentRepository - это in-memory реализация IncidentRepository для тестов.
type MockIncidentRepository struct {
	mu        sync.RWMutex
	incidents map[uint]*models.Incident
	nextID    uint
}

// NewMockIncidentRepository создает новый экземпляр мок-репозитория.
func NewMockIncidentRepository() service.IncidentRepository {
	repo := &MockIncidentRepository{
		incidents: make(map[uint]*models.Incident),
		nextID:    1,
	}
	repo.seed() // Заполняем начальными данными
	return repo
}

func (m *MockIncidentRepository) Create(ctx context.Context, incident *models.Incident) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	incident.ID = m.nextID
	m.incidents[incident.ID] = incident
	m.nextID++
	return nil
}

func (m *MockIncidentRepository) FindByID(ctx context.Context, id uint) (*models.Incident, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	incident, exists := m.incidents[id]
	if !exists {
		return nil, fmt.Errorf("incident with ID %d not found", id)
	}
	return incident, nil
}

func (m *MockIncidentRepository) FindByFingerprint(ctx context.Context, fingerprint string) (*models.Incident, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, incident := range m.incidents {
		if incident.Fingerprint == fingerprint {
			return incident, nil
		}
	}
	return nil, fmt.Errorf("incident with fingerprint %s not found", fingerprint)
}

func (m *MockIncidentRepository) Update(ctx context.Context, incident *models.Incident) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.incidents[incident.ID]; !exists {
		return fmt.Errorf("incident with ID %d not found", incident.ID)
	}
	m.incidents[incident.ID] = incident
	return nil
}

func (m *MockIncidentRepository) ListActive(ctx context.Context) ([]*models.Incident, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var activeIncidents []*models.Incident
	for _, incident := range m.incidents {
		if incident.Status == models.StatusActive {
			activeIncidents = append(activeIncidents, incident)
		}
	}
	return activeIncidents, nil
}

func (m *MockIncidentRepository) ListClosed(ctx context.Context, limit int, offset int) ([]*models.Incident, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var closedIncidents []*models.Incident
	for _, incident := range m.incidents {
		if incident.Status == models.StatusResolved || incident.Status == models.StatusRejected {
			closedIncidents = append(closedIncidents, incident)
		}
	}
	return closedIncidents, nil
}

// seed заполняет репозиторий тестовыми данными.
func (m *MockIncidentRepository) seed() {
	incident1 := &models.Incident{
		Fingerprint: "test-fingerprint-1",
		Status:      models.StatusActive,
		StartsAt:    time.Now(),
		Summary:     "API Gateway is down",
		Description: "The number of available replicas for the deployment 'api-gateway' is lower than desired.",
		Labels: map[string]string{
			"service":    "api-gateway",
			"severity":   "critical",
			"alertname":  "KubeDeploymentReplicasMismatch",
			"deployment": "api-gateway",
			"namespace":  "production",
		},
		AffectedResources: map[string]string{
			"deployment": "api-gateway",
			"namespace":  "production",
		},
		AuditLog: []models.AuditRecord{},
	}
	_ = m.Create(context.Background(), incident1)
}
