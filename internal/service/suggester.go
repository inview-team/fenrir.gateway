package service

import (
	"fmt"
	"log"

	"chatops-bot/internal/models"
)

// ActionSuggester отвечает за генерацию предложений по действиям на основе данных инцидента.
type ActionSuggester struct{}

// NewActionSuggester создает новый экземпляр ActionSuggester.
func NewActionSuggester() *ActionSuggester {
	return &ActionSuggester{}
}

// SuggestActions генерирует список предлагаемых действий для "быстрого пути".
func (s *ActionSuggester) SuggestActions(incident *models.Incident) []models.SuggestedAction {
	var suggestions []models.SuggestedAction

	// Правило 1: Проблема с репликами деплоймента
	if alertName, ok := incident.Labels["alertname"]; ok && alertName == "KubeDeploymentReplicasMismatch" {
		if deploymentName, ok := incident.AffectedResources["deployment"]; ok {
			params := map[string]string{
				"deployment": incident.AffectedResources["deployment"],
				"namespace":  incident.AffectedResources["namespace"],
			}
			suggestions = append(suggestions, models.SuggestedAction{
				HumanReadable: fmt.Sprintf("⏪ Откатить %s", deploymentName),
				Action:        string(models.ActionRollbackDeployment),
				Parameters:    params,
			})
		}
	}

	// Правило 2: Pod в состоянии CrashLoopBackOff
	if alertName, ok := incident.Labels["alertname"]; ok && alertName == "KubePodCrashLooping" {
		if podName, ok := incident.AffectedResources["pod"]; ok {
			suggestions = append(suggestions, models.SuggestedAction{
				HumanReadable: fmt.Sprintf("📄 Логи пода %s", podName),
				Action:        string(models.ActionGetPodLogs),
				Parameters: map[string]string{
					"pod":       incident.AffectedResources["pod"],
					"namespace": incident.AffectedResources["namespace"],
				},
			})
		}
	}

	log.Printf("Generated %d suggestions for incident %d", len(suggestions), incident.ID)
	return suggestions
}

// SuggestActionsForResource генерирует действия для конкретного выбранного ресурса.
func (s *ActionSuggester) SuggestActionsForResource(incident *models.Incident, resourceType, resourceName string) []models.SuggestedAction {
	var suggestions []models.SuggestedAction
	namespace := incident.AffectedResources["namespace"]

	switch resourceType {
	case "deployment":
		params := map[string]string{"deployment": resourceName, "namespace": namespace}
		suggestions = append(suggestions,
			models.SuggestedAction{
				HumanReadable: "⏪ Откатить",
				Action:        string(models.ActionRollbackDeployment),
				Parameters:    params,
			},
			models.SuggestedAction{
				HumanReadable: "📦 Список подов",
				Action:        string(models.ActionListPodsForDeployment),
				Parameters:    params,
			},
			models.SuggestedAction{
				HumanReadable: "ℹ️ Описать (Describe)",
				Action:        string(models.ActionDescribeDeployment),
				Parameters:    params,
			},
		)
	case "pod":
		params := map[string]string{"pod_name": resourceName, "namespace": namespace}
		suggestions = append(suggestions,
			models.SuggestedAction{
				HumanReadable: "📄 Логи",
				Action:        string(models.ActionGetPodLogs),
				Parameters:    map[string]string{"pod": resourceName, "namespace": namespace},
			},
			models.SuggestedAction{
				HumanReadable: "ℹ️ Описать (Describe)",
				Action:        string(models.ActionDescribePod),
				Parameters:    map[string]string{"pod": resourceName, "namespace": namespace},
			},
			models.SuggestedAction{
				HumanReadable: "🗑️ Удалить",
				Action:        string(models.ActionDeletePod),
				Parameters:    params,
			},
		)
	}

	return suggestions
}
