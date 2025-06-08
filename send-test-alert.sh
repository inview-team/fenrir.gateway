#!/bin/bash

# --- Configuration ---
HOST="localhost"
PORT=${ALERT_PORT:-8081}
TOKEN=${WEBHOOK_TOKEN:-"secret"}
DEPLOYMENT_NAME=${1:-"frontend"} # Default to "frontend" if no argument is provided

# --- JSON Payload ---
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
        "deployment": "${DEPLOYMENT_NAME}",
        "namespace": "guestbook",
        "severity": "critical"
      },
      "annotations": {
        "summary": "Deployment replicas mismatch for ${DEPLOYMENT_NAME}",
        "description": "Deployment guestbook/${DEPLOYMENT_NAME} has 0/1 ready replicas."
      },
      "startsAt": "$(date -u +"%Y-%m-%dT%H:%M:%S.%NZ")",
      "endsAt": "0001-01-01T00:00:00Z"
    }
  ]
}
EOF
)

# --- Execution ---
echo "Sending test alert for ${DEPLOYMENT_NAME} to http://${HOST}:${PORT}/api/v1/alertmanager"
echo "Using token: ${TOKEN}"
echo "Payload:"
echo "${JSON_PAYLOAD}" | jq .

curl -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "${JSON_PAYLOAD}" \
  http://${HOST}:${PORT}/api/v1/alertmanager

echo -e "\nDone."
