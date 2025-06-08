package mock

import (
	"fmt"

	"chatops-bot/internal/models"
)

// ExecutorClientMock имитирует клиент для взаимодействия с исполнителем (воркером).
// В реальном приложении здесь будут вызовы к K8s API.
type ExecutorClientMock struct {
	// FailNextCall используется для тестирования сценариев с ошибками.
	FailNextCall bool
}

// NewExecutorClientMock создает новый экземпляр мока.
func NewExecutorClientMock() *ExecutorClientMock {
	return &ExecutorClientMock{}
}

// ExecuteAction имитирует выполнение действия.
func (m *ExecutorClientMock) ExecuteAction(req models.ActionRequest) models.ActionResult {
	if m.FailNextCall {
		m.FailNextCall = false // Сбрасываем флаг после использования
		return models.ActionResult{
			Message: "Действие не удалось (симуляция ошибки)",
			Error:   "mock executor failed",
		}
	}

	// Извлекаем параметры для удобства
	deployment := req.Parameters["deployment"]
	pod := req.Parameters["pod"]
	namespace := req.Parameters["namespace"]

	switch models.ActionType(req.Action) {
	case models.ActionRestartDeployment:
		return models.ActionResult{Message: fmt.Sprintf("Деплоймент '%s' в неймспейсе '%s' успешно перезапущен.", deployment, namespace)}
	case models.ActionRollbackDeployment:
		return models.ActionResult{Message: fmt.Sprintf("Деплоймент '%s' в неймспейсе '%s' успешно откачен.", deployment, namespace)}
	case models.ActionScaleDeployment:
		replicas := req.Parameters["replicas"]
		return models.ActionResult{Message: fmt.Sprintf("Деплоймент '%s' в неймспейсе '%s' смасштабирован до %s реплик.", deployment, namespace, replicas)}
	case models.ActionDescribeDeployment:
		return models.ActionResult{Message: fmt.Sprintf("Описание для деплоймента '%s':\n... deployment description ...", deployment)}
	case models.ActionGetPodLogs:
		return models.ActionResult{Message: fmt.Sprintf("Логи для пода '%s':\n... some logs here ...", pod)}
	case models.ActionDescribePod:
		return models.ActionResult{Message: fmt.Sprintf("Описание для пода '%s':\n... pod description ...", pod)}
	case models.ActionDeletePod:
		return models.ActionResult{Message: fmt.Sprintf("Под '%s' в неймспейсе '%s' успешно удален.", pod, namespace)}
	case models.ActionListPodsForDeployment:
		pods := []models.ResourceInfo{
			{Name: fmt.Sprintf("%s-pod-1-abcde", deployment), Status: "Running"},
			{Name: fmt.Sprintf("%s-pod-2-fghij", deployment), Status: "Running"},
			{Name: fmt.Sprintf("%s-pod-3-klmno", deployment), Status: "Error"},
		}
		return models.ActionResult{
			Message: fmt.Sprintf("Поды для деплоймента `%s`:", deployment),
			ResultData: &models.ResultData{
				Type:     "list",
				Items:    pods,
				ItemType: "pod",
			},
		}
	case models.ActionAllocateHardware:
		profile := req.Parameters["profile"]
		return models.ActionResult{Message: fmt.Sprintf("Ресурсы по профилю '%s' успешно выделены для пода '%s'.", profile, pod)}
	default:
		return models.ActionResult{
			Message: fmt.Sprintf("Неизвестное действие: %s", req.Action),
			Error:   "unknown action",
		}
	}
}

// GetResourceDetails имитирует получение деталей о ресурсе.
func (m *ExecutorClientMock) GetResourceDetails(req models.ResourceDetailsRequest) (*models.ResourceDetails, error) {
	if m.FailNextCall {
		m.FailNextCall = false
		return nil, fmt.Errorf("failed to get resource details (simulation)")
	}

	switch req.ResourceType {
	case "deployment":
		return &models.ResourceDetails{
			Status:       "Active",
			ReplicasInfo: "3/3",
			Age:          "12d",
			RawOutput:    fmt.Sprintf("Details for deployment %s", req.ResourceName),
		}, nil
	case "pod":
		return &models.ResourceDetails{
			Status:    "Running",
			Restarts:  2,
			Age:       "4h",
			RawOutput: fmt.Sprintf("Details for pod %s", req.ResourceName),
		}, nil
	default:
		return nil, fmt.Errorf("unknown resource type: %s", req.ResourceType)
	}
}

// GetAvailableResources имитирует получение доступных профилей ресурсов.
func (m *ExecutorClientMock) GetAvailableResources() (*models.AvailableResources, error) {
	if m.FailNextCall {
		m.FailNextCall = false
		return nil, fmt.Errorf("failed to get available resources (simulation)")
	}

	return &models.AvailableResources{
		Profiles: []models.ResourceProfile{
			{Name: "small", Description: "1 CPU, 2Gi RAM", IsDefault: true},
			{Name: "medium", Description: "2 CPU, 4Gi RAM"},
			{Name: "large", Description: "4 CPU, 8Gi RAM"},
		},
	}, nil
}
