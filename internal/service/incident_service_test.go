package service_test

import (
	"context"
	"testing"
	"time"

	"chatops-bot/internal/executor/mock"
	"chatops-bot/internal/models"
	"chatops-bot/internal/service"
	"chatops-bot/internal/storage/inmemory"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testKit struct {
	incidentRepo   service.IncidentRepository
	userRepo       service.UserRepository
	executorClient *mock.ExecutorClientMock
	suggester      *service.ActionSuggester
	service        *service.IncidentService
	notifChan      chan *models.Incident
	updateChan     chan *models.Incident
}

func setupService(t *testing.T) *testKit {
	incidentRepo := inmemory.NewMockIncidentRepository()
	userRepo := inmemory.NewMockUserRepository()
	executorClient := mock.NewExecutorClientMock()
	suggester := service.NewActionSuggester()
	notifChan := make(chan *models.Incident, 1)
	updateChan := make(chan *models.Incident, 1)

	incidentService := service.NewIncidentService(incidentRepo, userRepo, executorClient, suggester, notifChan, updateChan)

	return &testKit{
		incidentRepo:   incidentRepo,
		userRepo:       userRepo,
		executorClient: executorClient,
		suggester:      suggester,
		service:        incidentService,
		notifChan:      notifChan,
		updateChan:     updateChan,
	}
}

func TestIncidentService_CreateIncidentFromAlert(t *testing.T) {
	kit := setupService(t)
	ctx := context.Background()

	alert := models.Alert{
		Fingerprint: "alert-fingerprint",
		StartsAt:    time.Now(),
		Annotations: models.Annotations{"summary": "Test Summary", "description": "Test Description"},
		Labels:      models.Labels{"deployment": "test-deploy", "namespace": "test-ns", "severity": "critical"},
	}

	incident, err := kit.service.CreateIncidentFromAlert(ctx, alert)
	require.NoError(t, err)
	require.NotNil(t, incident)

	assert.Equal(t, "alert-fingerprint", incident.Fingerprint)
	assert.Equal(t, models.StatusActive, incident.Status)
	assert.Equal(t, "Test Summary", incident.Summary)
	assert.Equal(t, "Test Description", incident.Description)
	assert.Equal(t, "test-deploy", incident.AffectedResources["deployment"])
	assert.Equal(t, "test-ns", incident.AffectedResources["namespace"])

	// Проверяем, что уведомление было отправлено
	select {
	case notifiedIncident := <-kit.notifChan:
		assert.Equal(t, incident.ID, notifiedIncident.ID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("did not receive notification")
	}
}

func TestIncidentService_ExecuteAction_ClosingAction(t *testing.T) {
	kit := setupService(t)
	ctx := context.Background()

	// Создаем инцидент и пользователя
	incident, _ := kit.service.CreateIncidentFromAlert(ctx, models.Alert{Fingerprint: "test"})
	user, _ := kit.userRepo.FindByID(ctx, 1)

	req := models.ActionRequest{
		IncidentID: incident.ID,
		UserID:     user.ID,
		Action:     string(models.ActionRollbackDeployment),
		Parameters: map[string]string{"deployment": "test-deploy"},
	}

	result, err := kit.service.ExecuteAction(ctx, req)
	require.NoError(t, err)
	assert.Empty(t, result.Error)

	// Проверяем, что обновление было отправлено
	select {
	case updated := <-kit.updateChan:
		assert.Equal(t, incident.ID, updated.ID)
		assert.Equal(t, models.StatusResolved, updated.Status)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("did not receive update notification")
	}

	updatedIncident, _ := kit.incidentRepo.FindByID(ctx, incident.ID)
	require.NotNil(t, updatedIncident)

	assert.Equal(t, models.StatusResolved, updatedIncident.Status)
	require.NotNil(t, updatedIncident.ResolvedBy)
	assert.Equal(t, user.ID, *updatedIncident.ResolvedBy)
	require.Len(t, updatedIncident.AuditLog, 1)
	assert.True(t, updatedIncident.AuditLog[0].Success)
	assert.Equal(t, string(models.ActionRollbackDeployment), updatedIncident.AuditLog[0].Action)
}

func TestIncidentService_ExecuteAction_NonClosingAction(t *testing.T) {
	kit := setupService(t)
	ctx := context.Background()

	incident, _ := kit.service.CreateIncidentFromAlert(ctx, models.Alert{Fingerprint: "test"})
	user, _ := kit.userRepo.FindByID(ctx, 1)

	req := models.ActionRequest{
		IncidentID: incident.ID,
		UserID:     user.ID,
		Action:     string(models.ActionGetPodLogs),
		Parameters: map[string]string{"pod": "test-pod"},
	}

	_, err := kit.service.ExecuteAction(ctx, req)
	require.NoError(t, err)

	updatedIncident, _ := kit.incidentRepo.FindByID(ctx, incident.ID)
	require.NotNil(t, updatedIncident)

	assert.Equal(t, models.StatusActive, updatedIncident.Status)
	require.Len(t, updatedIncident.AuditLog, 1)
	assert.True(t, updatedIncident.AuditLog[0].Success)
}

func TestIncidentService_ExecuteAction_Fails(t *testing.T) {
	kit := setupService(t)
	ctx := context.Background()

	incident, _ := kit.service.CreateIncidentFromAlert(ctx, models.Alert{Fingerprint: "test"})
	user, _ := kit.userRepo.FindByID(ctx, 1)

	kit.executorClient.FailNextCall = true

	req := models.ActionRequest{
		IncidentID: incident.ID,
		UserID:     user.ID,
		Action:     string(models.ActionRestartDeployment),
	}

	result, err := kit.service.ExecuteAction(ctx, req)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Error)

	updatedIncident, _ := kit.incidentRepo.FindByID(ctx, incident.ID)
	require.NotNil(t, updatedIncident)

	assert.Equal(t, models.StatusActive, updatedIncident.Status)
	require.Len(t, updatedIncident.AuditLog, 1)
	assert.False(t, updatedIncident.AuditLog[0].Success)
	assert.Equal(t, "mock executor failed", result.Error)
}
