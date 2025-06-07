#!/bin/bash

# Скрипт для отправки тестового вебхука от Alertmanager на локальный сервер.
# Позволяет имитировать создание инцидента без настройки реального Alertmanager.

# --- Конфигурация ---
HOST="localhost"
PORT=${ALERT_PORT:-8081} # Используем переменную окружения ALERT_PORT или 8081 по умолчанию
TOKEN=${WEBHOOK_TOKEN:-"secret"} # Используем WEBHOOK_TOKEN или "secret" по умолчанию

# --- JSON Payload ---
# Этот JSON имитирует алерт о несоответствии количества реплик деплоймента.
# Он должен триггерить ActionSuggester и создать инцидент с предложенными действиями.
JSON_PAYLOAD=$(cat <<EOF
{
  "version": "4",
  "status": "firing",
  "alerts": [
    {
      "status": "firing",
      "fingerprint": "$(date +%s)-${RANDOM}",
      "labels": {
        "alertname": "KubeDeploymentReplicasMismatch",
        "deployment": "payment-gateway",
        "namespace": "production",
        "severity": "critical"
      },
      "annotations": {
        "summary": "Deployment replicas mismatch for payment-gateway",
        "description": "Deployment production/payment-gateway has 0/1 ready replicas."
      },
      "startsAt": "$(date -u +"%Y-%m-%dT%H:%M:%S.%NZ")",
      "endsAt": "0001-01-01T00:00:00Z"
    }
  ]
}
EOF
)

# --- Выполнение запроса ---
echo "Sending test alert to http://${HOST}:${PORT}/api/v1/alertmanager"
echo "Using token: ${TOKEN}"
echo "Payload:"
echo "${JSON_PAYLOAD}" | jq . # Выводим для наглядности

curl -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "${JSON_PAYLOAD}" \
  http://${HOST}:${PORT}/api/v1/alertmanager

echo -e "\nDone."
