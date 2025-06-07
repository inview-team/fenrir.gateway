package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"chatops-bot/internal/models"
	"chatops-bot/internal/service"
	"chatops-bot/internal/storage/inmemory"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Setup ---

type serverTestKit struct {
	incidentRepo service.IncidentRepository
	userRepo     service.UserRepository
	service      *service.IncidentService
	router       http.Handler
}

func setupServerTest(t *testing.T) *serverTestKit {
	incidentRepo := inmemory.NewMockIncidentRepository()
	userRepo := inmemory.NewMockUserRepository()
	// For server tests, we don't need a real executor or suggester
	incidentService := service.NewIncidentService(incidentRepo, userRepo, nil, nil, make(chan *models.Incident, 1))

	// We test the main API router here
	router := newRouter(incidentService, userRepo)

	return &serverTestKit{
		incidentRepo: incidentRepo,
		userRepo:     userRepo,
		service:      incidentService,
		router:       router,
	}
}

// --- Handler Tests ---

func TestHandleGetIncident(t *testing.T) {
	kit := setupServerTest(t)

	// Pre-populate an incident
	testIncident := &models.Incident{Summary: "Test Incident"}
	err := kit.incidentRepo.Create(context.Background(), testIncident)
	require.NoError(t, err)

	// Используем ID, который был присвоен при создании, а не жестко заданный "1"
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/incidents/%d", testIncident.ID), nil)
	rr := httptest.NewRecorder()

	kit.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var returnedIncident models.Incident
	err = json.NewDecoder(rr.Body).Decode(&returnedIncident)
	require.NoError(t, err)
	assert.Equal(t, "Test Incident", returnedIncident.Summary)
	assert.Equal(t, testIncident.ID, returnedIncident.ID)
}

func TestHandleAlertmanagerWebhook(t *testing.T) {
	// Для этого теста создаем чистый репозиторий без начальных данных (seed)
	incidentRepo := inmemory.NewMockIncidentRepository()
	// Очищаем его от данных, созданных в NewMockIncidentRepository
	active, _ := incidentRepo.ListActive(context.Background())
	for _, inc := range active {
		inc.Status = models.StatusResolved
		incidentRepo.Update(context.Background(), inc)
	}

	incidentService := service.NewIncidentService(incidentRepo, nil, nil, nil, make(chan *models.Incident, 1))
	router := newAlertmanagerRouter(incidentService, "test-token")

	webhookBody := models.AlertmanagerWebhookMessage{
		Alerts: []models.Alert{
			{
				Status:      "firing",
				Labels:      models.Labels{"alertname": "TestAlert"},
				Annotations: models.Annotations{"summary": "Webhook test summary"},
				StartsAt:    time.Now(),
			},
		},
	}
	body, _ := json.Marshal(webhookBody)

	req := httptest.NewRequest("POST", "/api/v1/alertmanager", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)

	// Verify that the incident was created in the repo
	activeIncidents, err := incidentRepo.ListActive(context.Background())
	require.NoError(t, err)
	assert.Len(t, activeIncidents, 1)
	assert.Equal(t, "Webhook test summary", activeIncidents[0].Summary)
}

// --- Middleware Tests ---

func TestWebhookAuthMiddleware(t *testing.T) {
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	testCases := []struct {
		name               string
		requiredToken      string
		requestHeaderValue string
		expectedStatusCode int
	}{
		{
			name:               "Correct Token Provided",
			requiredToken:      "secret-token",
			requestHeaderValue: "Bearer secret-token",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "No Token Provided when Required",
			requiredToken:      "secret-token",
			requestHeaderValue: "",
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "Wrong Token Provided",
			requiredToken:      "secret-token",
			requestHeaderValue: "Bearer wrong-token",
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "Invalid Header Format (No Bearer)",
			requiredToken:      "secret-token",
			requestHeaderValue: "secret-token",
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "Token Not Required",
			requiredToken:      "",
			requestHeaderValue: "",
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/v1/alertmanager", nil)
			if tc.requestHeaderValue != "" {
				req.Header.Set("Authorization", tc.requestHeaderValue)
			}

			middleware := webhookAuthMiddleware(tc.requiredToken)
			handler := middleware(mockHandler)
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatusCode, rr.Code)
		})
	}
}
