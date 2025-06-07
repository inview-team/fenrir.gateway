package main

import (
	"context"
	"flag"
	"log"
	"sync"
	"time"

	"chatops-bot/internal/bot"
	"chatops-bot/internal/config"
	"chatops-bot/internal/executor/http"
	"chatops-bot/internal/executor/mock"
	"chatops-bot/internal/models"
	"chatops-bot/internal/server"
	"chatops-bot/internal/service"
	storage_gorm "chatops-bot/internal/storage/gorm"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to the configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// --- Инициализация и миграция БД ---
	db, err := gorm.Open(sqlite.Open(cfg.DB.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get underlying sql.DB: %v", err)
	}

	driver, err := sqlite3.WithInstance(sqlDB, &sqlite3.Config{})
	if err != nil {
		log.Fatalf("Failed to create migrate driver: %v", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://migrations",
		"sqlite3",
		driver,
	)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Failed to apply migrations: %v", err)
	}
	log.Println("Database migrations applied successfully.")

	// --- Инициализация зависимостей (Dependency Injection) ---
	userRepo, err := storage_gorm.NewGormUserRepository(db)
	if err != nil {
		log.Fatalf("Failed to create user repository: %v", err)
	}

	incidentRepo, err := storage_gorm.NewGormIncidentRepository(db)
	if err != nil {
		log.Fatalf("Failed to create incident repository: %v", err)
	}

	var executorClient service.ExecutorClient
	if cfg.Executor.UseMock {
		executorClient = mock.NewExecutorClientMock()
	} else {
		executorClient = http.NewExecutorClient(cfg.Executor.BaseURL)
	}
	actionSuggester := service.NewActionSuggester()

	// Канал для уведомлений о новых инцидентах
	notificationChan := make(chan *models.Incident, 10)
	updateChan := make(chan *models.Incident, 10)
	topicDeletionChan := make(chan *models.Incident, 10)

	incidentService := service.NewIncidentService(incidentRepo, userRepo, executorClient, actionSuggester, notificationChan, updateChan, topicDeletionChan)

	var wg sync.WaitGroup

	// --- Запуск фонового процесса для удаления старых топиков ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(cfg.IncidentService.TopicDeletionInterval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Println("Running job to delete old incident topics...")
				incidentService.DeleteOldIncidentTopics(context.Background(), time.Duration(cfg.IncidentService.TopicMaxAge)*time.Second)
			case <-context.Background().Done():
				return
			}
		}
	}()

	// --- Запуск серверов и бота ---
	server.Start(context.Background(), incidentService, userRepo, cfg.Server.AppPort, cfg.Server.AlertPort, cfg.Server.WebhookToken)

	// --- Запуск Telegram-бота ---
	if cfg.Telegram.BotToken == "" {
		log.Println("Telegram bot token is not set. Bot will not start.")
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			telegramBot, err := bot.NewBot(cfg.Telegram.BotToken, incidentService, userRepo, actionSuggester, cfg.Telegram.AlertChannelID)
			if err != nil {
				log.Fatalf("Failed to create bot: %v", err)
			}
			telegramBot.Start(notificationChan, updateChan, topicDeletionChan)
		}()
	}

	log.Println("Application started. Press Ctrl+C to exit.")
	wg.Wait()
}
