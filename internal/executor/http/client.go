package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"chatops-bot/internal/models"
)

type ExecutorClient struct {
	client  *http.Client
	baseURL string
}

func NewExecutorClient(baseURL string) *ExecutorClient {
	return &ExecutorClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: baseURL,
	}
}

func (c *ExecutorClient) ExecuteAction(req models.ActionRequest) models.ActionResult {
	switch models.ActionType(req.Action) {
	case models.ActionRestartDeployment:
		res, _ := c.restartDeployment(context.Background(), req)
		return res
	case models.ActionScaleDeployment:
		res, _ := c.scaleDeployment(context.Background(), req)
		return res
	default:
		return models.ActionResult{Error: "unsupported action"}
	}
}

func (c *ExecutorClient) restartDeployment(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	url := fmt.Sprintf("%s/api/kubernetes/%s/pods/%s", c.baseURL, req.Parameters["namespace"], req.Parameters["pod"])
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return models.ActionResult{}, err
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return models.ActionResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.ActionResult{Error: fmt.Sprintf("failed to restart pod: status code %d", resp.StatusCode)}, nil
	}

	return models.ActionResult{Message: "Pod restarted successfully"}, nil
}

func (c *ExecutorClient) scaleDeployment(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	url := fmt.Sprintf("%s/api/kubernetes/%s/deployments/%s?replicas=%s", c.baseURL, req.Parameters["namespace"], req.Parameters["deployment"], req.Parameters["replicas"])
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	if err != nil {
		return models.ActionResult{}, err
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return models.ActionResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.ActionResult{Error: fmt.Sprintf("failed to scale deployment: status code %d", resp.StatusCode)}, nil
	}

	return models.ActionResult{Message: "Deployment scaled successfully"}, nil
}

func (c *ExecutorClient) GetResourceDetails(req models.ResourceDetailsRequest) (*models.ResourceDetails, error) {
	// This is a mock implementation.
	// In a real scenario, you would make an HTTP call to the executor to get resource details.
	return &models.ResourceDetails{
		Status:       "Unknown",
		ReplicasInfo: "N/A",
		Age:          "N/A",
		RawOutput:    "Details not available via HTTP client yet.",
	}, nil
}

func (c *ExecutorClient) GetAvailableResources() (*models.AvailableResources, error) {
	// This is a mock implementation.
	return &models.AvailableResources{
		Profiles: []models.ResourceProfile{
			{Name: "small", Description: "1 CPU, 2Gi RAM", IsDefault: true},
			{Name: "medium", Description: "2 CPU, 4Gi RAM"},
			{Name: "large", Description: "4 CPU, 8Gi RAM"},
		},
	}, nil
}
