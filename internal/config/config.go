package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	DB              DBConfig              `json:"db"`
	Server          ServerConfig          `json:"server"`
	Executor        ExecutorConfig        `json:"executor"`
	Telegram        TelegramConfig        `json:"telegram"`
	IncidentService IncidentServiceConfig `json:"incident_service"`
}

type DBConfig struct {
	DSN string `json:"dsn"`
}

type ServerConfig struct {
	AppPort      string `json:"app_port"`
	AlertPort    string `json:"alert_port"`
	WebhookToken string `json:"webhook_token"`
}

type ExecutorConfig struct {
	UseMock bool   `json:"use_mock"`
	BaseURL string `json:"base_url"`
}

type TelegramConfig struct {
	BotToken       string `json:"bot_token,omitempty"`
	AlertChannelID int64  `json:"alert_channel_id"`
}

type IncidentServiceConfig struct {
	TopicDeletionInterval int64 `json:"topic_deletion_interval"` // in seconds
	TopicMaxAge           int64 `json:"topic_max_age"`           // in seconds
}

func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return nil, err
	}

	if token := os.Getenv("TELEGRAM_BOT_TOKEN"); token != "" {
		cfg.Telegram.BotToken = token
	}

	return &cfg, nil
}
