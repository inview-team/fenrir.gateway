package main

import (
	"context"
	"flag"
	"log"
	"os"
	"sync"

	"chatops-bot/internal/bot"
	"chatops-bot/internal/executor/http"
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
	// --- Определение флагов командной строки ---
	useMockExecutor := flag.Bool("use-mock-executor", true, "Use the mock executor client instead of the real HTTP client")
	executorBaseURL := flag.String("executor-url", "http://localhost:8082", "Base URL for the executor service")
	flag.Parse()

	// --- Инициализация и миграция БД ---
	// Добавляем параметр `_time_format=sqlite` для корректной обработки временных меток драйвером.
	db, err := gorm.Open(sqlite.Open("chatops.db?_time_format=sqlite"), &gorm.Config{})
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

	executorClient := http.NewExecutorClient(*useMockExecutor, *executorBaseURL)
	actionSuggester := service.NewActionSuggester()

	// Канал для уведомлений о новых инцидентах
	notificationChan := make(chan *models.Incident, 10)
	updateChan := make(chan *models.Incident, 10)

	incidentService := service.NewIncidentService(incidentRepo, userRepo, executorClient, actionSuggester, notificationChan, updateChan)

	var wg sync.WaitGroup

	// --- Запуск серверов и бота ---
	appPort := getEnv("APP_PORT", "8080")
	alertPort := getEnv("ALERT_PORT", "8081")
	webhookToken := getEnv("WEBHOOK_TOKEN", "") // Рекомендуется установить в production

	server.Start(context.Background(), incidentService, userRepo, appPort, alertPort, webhookToken)

	// --- Запуск Telegram-бота ---
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Println("TELEGRAM_BOT_TOKEN is not set. Bot will not start.")
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			telegramBot, err := bot.NewBot(token, incidentService, userRepo, actionSuggester)
			if err != nil {
				log.Fatalf("Failed to create bot: %v", err)
			}
			telegramBot.Start(notificationChan, updateChan)
		}()
	}

	log.Println("Application started. Press Ctrl+C to exit.")
	wg.Wait()
}

// getEnv получает значение переменной окружения или возвращает значение по умолчанию.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
