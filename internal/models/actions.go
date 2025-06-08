package models

// ActionType определяет тип действия, которое может быть выполнено.
type ActionType string

// Константы для всех поддерживаемых действий в системе.
const (
	// Действия уровня деплоймента
	ActionRollbackDeployment ActionType = "rollback_deployment"
	ActionScaleDeployment    ActionType = "scale_deployment"
	ActionDescribeDeployment ActionType = "describe_deployment"

	// Действия уровня пода
	ActionGetPodLogs  ActionType = "get_pod_logs"
	ActionDescribePod ActionType = "describe_pod"
	ActionDeletePod   ActionType = "delete_pod"

	// Действия для получения списков
	ActionListPodsForDeployment ActionType = "list_pods_for_deployment"

	// Действия по управлению ресурсами
	ActionAllocateHardware  ActionType = "allocate_hardware"
	ActionGetDeploymentInfo ActionType = "get_deployment_info"
)

// ActionResult представляет стандартизированный результат выполнения действия.
type ActionResult struct {
	// Message - это человекочитаемое сообщение о результате, которое будет показано пользователю.
	Message string `json:"message"`
	// Error - сообщение об ошибке, если действие не удалось.
	Error string `json:"error,omitempty"`
	// ResultData - это опциональные структурированные данные, которые могут быть использованы для построения дальнейшего UI.
	// Например, для действия "list_pods" здесь будет список имен подов.
	ResultData *ResultData `json:"result_data,omitempty"`
}

// ResourceInfo содержит краткую информацию о ресурсе.
type ResourceInfo struct {
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
}

// ResultData содержит структурированные данные, возвращаемые действием.
type ResultData struct {
	// Type указывает на тип данных, например "list".
	Type string `json:"type"`
	// Items - это список элементов, например, информация о подах или деплойментах.
	Items []ResourceInfo `json:"items"`
	// ItemType - это тип элементов в списке, например, "pod" или "deployment".
	// Это поможет боту понять, какие действия можно предложить для каждого элемента.
	ItemType string `json:"item_type,omitempty"`
}

// ActionRequest описывает запрос на выполнение действия.
type ActionRequest struct {
	Action     string            `json:"action"`
	IncidentID uint              `json:"incident_id"`
	UserID     uint              `json:"user_id"`
	Parameters map[string]string `json:"parameters"`
}

// SuggestedAction представляет собой действие, предложенное пользователю.
type SuggestedAction struct {
	// HumanReadable - это текст, который будет на кнопке.
	HumanReadable string `json:"human_readable"`
	// Action - это идентификатор действия, который будет отправлен обратно при нажатии.
	Action string `json:"action"`
	// Parameters - это параметры, необходимые для выполнения действия.
	Parameters map[string]string `json:"parameters"`
}

// ActionTarget определяет ресурс, к которому применяется действие.
type ActionTarget struct {
	Type string // e.g., "deployment", "pod"
	Name string // e.g., "my-app-deployment", "my-app-pod-12345"
}
