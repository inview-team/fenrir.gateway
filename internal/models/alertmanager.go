package models

import "time"

// AlertmanagerWebhookMessage - это корневая структура сообщения от Alertmanager.
type AlertmanagerWebhookMessage struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	TruncatedAlerts   int               `json:"truncatedAlerts"`
	Status            string            `json:"status"` // "firing" or "resolved"
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []Alert           `json:"alerts"`
}

// Labels - это набор пар ключ-значение, идентифицирующих алерт.
type Labels map[string]string

// Annotations - это набор информационных пар ключ-значение.
type Annotations map[string]string

// Alert представляет собой отдельный алерт в сообщении.
type Alert struct {
	Status       string      `json:"status"`
	Labels       Labels      `json:"labels"`
	Annotations  Annotations `json:"annotations"`
	StartsAt     time.Time   `json:"startsAt"`
	EndsAt       time.Time   `json:"endsAt"`
	GeneratorURL string      `json:"generatorURL"`
	Fingerprint  string      `json:"fingerprint"`
}
