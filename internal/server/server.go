package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"chatops-bot/internal/models"
	"chatops-bot/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Start запускает оба HTTP-сервера: для API и для вебхуков Alertmanager.
func Start(ctx context.Context, service *service.IncidentService, userRepo service.UserRepository, appPort, alertPort, webhookToken string) {
	go func() {
		log.Printf("Starting main API server on port %s", appPort)
		router := newRouter(service, userRepo)
		if err := http.ListenAndServe(fmt.Sprintf(":%s", appPort), router); err != nil {
			log.Fatalf("Failed to start main API server: %v", err)
		}
	}()

	go func() {
		log.Printf("Starting Alertmanager webhook server on port %s", alertPort)
		router := newAlertmanagerRouter(service, webhookToken)
		if err := http.ListenAndServe(fmt.Sprintf(":%s", alertPort), router); err != nil {
			log.Fatalf("Failed to start Alertmanager server: %v", err)
		}
	}()
}

// newRouter создает роутер для основного API (для Mini App).
func newRouter(service *service.IncidentService, userRepo service.UserRepository) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authMiddleware(userRepo))
		r.Get("/incidents/{id}", handleGetIncident(service))
	})
	return r
}

// newAlertmanagerRouter создает роутер для вебхуков от Alertmanager.
func newAlertmanagerRouter(service *service.IncidentService, token string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(webhookAuthMiddleware(token))
		r.Post("/alertmanager", handleAlertmanagerWebhook(service))
	})
	return r
}

// --- Middlewares ---

func authMiddleware(userRepo service.UserRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Для API, используемого Mini App, можно использовать моковые данные или
			// реализовать полноценную аутентификацию через Telegram.
			// Пока используем мок.
			const mockTelegramID = 123456789
			const mockUsername = "api_user"
			user, err := userRepo.FindOrCreateByTelegramID(r.Context(), mockTelegramID, mockUsername, "API", "User")
			if err != nil {
				http.Error(w, "Authentication failed", http.StatusInternalServerError)
				return
			}
			ctx := context.WithValue(r.Context(), "user", user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func webhookAuthMiddleware(expectedToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expectedToken == "" { // Если токен не задан, пропускаем проверку
				next.ServeHTTP(w, r)
				return
			}
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
				return
			}
			if parts[1] != expectedToken {
				http.Error(w, "Invalid token", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- Handlers ---

func handleGetIncident(service *service.IncidentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			http.Error(w, "Invalid incident ID", http.StatusBadRequest)
			return
		}
		incident, err := service.GetIncidentByID(r.Context(), uint(id))
		if err != nil {
			http.Error(w, "Incident not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(incident)
	}
}

func handleAlertmanagerWebhook(service *service.IncidentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var msg models.AlertmanagerWebhookMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			http.Error(w, "Failed to decode alertmanager webhook", http.StatusBadRequest)
			return
		}
		if len(msg.Alerts) == 0 {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Webhook received and processed (no alerts)."))
			return
		}
		alert := msg.Alerts[0]
		incident, err := service.CreateIncidentFromAlert(r.Context(), alert)
		if err != nil {
			log.Printf("Error creating incident from alert: %v", err)
			http.Error(w, "Failed to create incident", http.StatusInternalServerError)
			return
		}
		log.Printf("New incident created from alert: %s (ID: %d)", incident.Summary, incident.ID)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("Incident created successfully"))
	}
}
