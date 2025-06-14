package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	case models.ActionGetDeploymentInfo:
		res, _ := c.getDeploymentInfo(context.Background(), req)
		return res
	case models.ActionDeletePod:
		res, _ := c.restartPod(context.Background(), req)
		return res
	case models.ActionScaleDeployment:
		res, _ := c.scaleDeployment(context.Background(), req)
		return res
	case models.ActionListPodsForDeployment:
		res, _ := c.listPodsByDeployment(context.Background(), req)
		return res
	case models.ActionGetPodLogs:
		res, _ := c.getPodLogs(context.Background(), req)
		return res
	case models.ActionDescribePod:
		res, _ := c.describePod(context.Background(), req)
		return res
	case models.ActionDescribeDeployment:
		res, _ := c.describeDeployment(context.Background(), req)
		return res
	case models.ActionRollbackDeployment:
		res, _ := c.rollbackDeployment(context.Background(), req)
		return res
	default:
		return models.ActionResult{Error: "unsupported action"}
	}
}

func (c *ExecutorClient) restartPod(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	url := fmt.Sprintf("%s/api/kubernetes/%s/pods/%s", c.baseURL, req.Parameters["namespace"], req.Parameters["pod_name"])
	log.Printf("ExecutorClient: restarting pod with URL: %s", url)
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
	log.Printf("ExecutorClient: scaling deployment with URL: %s", url)
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

func (c *ExecutorClient) getPodInfo(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	url := fmt.Sprintf("%s/api/kubernetes/%s/pods/%s", c.baseURL, req.Parameters["namespace"], req.Parameters["pod_name"])
	log.Printf("ExecutorClient: getting pod info with URL: %s", url)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.ActionResult{}, err
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return models.ActionResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.ActionResult{Error: fmt.Sprintf("failed to get pod info: status code %d", resp.StatusCode)}, nil
	}

	var podInfo Pod
	if err := json.NewDecoder(resp.Body).Decode(&podInfo); err != nil {
		return models.ActionResult{}, err
	}

	return models.ActionResult{
		Message: "Pod info retrieved successfully",
		ResultData: &models.ResultData{
			Type:     "pod_info",
			ItemType: "pod_info",
			Items: []models.ResourceInfo{
				{
					Name:      podInfo.Name,
					Status:    podInfo.Status,
					Resources: convertResources(podInfo.Resources),
				},
			},
		},
	}, nil
}

func (c *ExecutorClient) listPodsByDeployment(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	url := fmt.Sprintf("%s/api/kubernetes/%s/pods?deployment=%s", c.baseURL, req.Parameters["namespace"], req.Parameters["deployment"])
	log.Printf("ExecutorClient: listing pods with URL: %s", url)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.ActionResult{}, err
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return models.ActionResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.ActionResult{Error: fmt.Sprintf("failed to list pods: status code %d", resp.StatusCode)}, nil
	}

	var listPodsResponse Pods
	if err := json.NewDecoder(resp.Body).Decode(&listPodsResponse); err != nil {
		return models.ActionResult{}, err
	}

	var resourceInfos []models.ResourceInfo
	for _, p := range listPodsResponse.Pods {
		resourceInfos = append(resourceInfos, models.ResourceInfo{Name: p.Name, Status: p.Status})
	}

	return models.ActionResult{
		Message:    "Pods listed successfully",
		ResultData: &models.ResultData{Type: "list", ItemType: "pod", Items: resourceInfos},
	}, nil
}

func (c *ExecutorClient) GetResourceDetails(req models.ResourceDetailsRequest) (*models.ResourceDetails, error) {
	var url string
	if req.ResourceType == "pod" {
		url = fmt.Sprintf("%s/api/kubernetes/%s/pods/%s", c.baseURL, req.Labels["namespace"], req.ResourceName)
	} else if req.ResourceType == "deployment" {
		url = fmt.Sprintf("%s/api/kubernetes/%s/deployments/%s", c.baseURL, req.Labels["namespace"], req.ResourceName)
	} else {
		return nil, fmt.Errorf("unsupported resource type: %s", req.ResourceType)
	}

	log.Printf("ExecutorClient: getting resource details with URL: %s", url)
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get resource details: status code %d", resp.StatusCode)
	}

	if req.ResourceType == "pod" {
		var pod Pod
		if err := json.NewDecoder(resp.Body).Decode(&pod); err != nil {
			return nil, err
		}
		return &models.ResourceDetails{
			Status:    pod.Status,
			Restarts:  pod.Restarts,
			Age:       pod.Age,
			Resources: convertResources(pod.Resources),
		}, nil
	}

	if req.ResourceType == "deployment" {
		var deployment Deployment
		if err := json.NewDecoder(resp.Body).Decode(&deployment); err != nil {
			return nil, err
		}
		return &models.ResourceDetails{
			Status:       "active", // Or some other status, as it's not in the response
			ReplicasInfo: fmt.Sprintf("%d replicas", deployment.Replicas),
		}, nil
	}

	return nil, fmt.Errorf("unsupported resource type: %s", req.ResourceType)
}

func (c *ExecutorClient) getDeploymentInfo(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	url := fmt.Sprintf("%s/api/kubernetes/%s/deployments/%s", c.baseURL, req.Parameters["namespace"], req.Parameters["deployment"])
	log.Printf("ExecutorClient: getting deployment info with URL: %s", url)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.ActionResult{}, err
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return models.ActionResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.ActionResult{Error: fmt.Sprintf("failed to get deployment info: status code %d", resp.StatusCode)}, nil
	}

	var deploymentInfo Deployment
	if err := json.NewDecoder(resp.Body).Decode(&deploymentInfo); err != nil {
		return models.ActionResult{}, err
	}

	return models.ActionResult{
		Message: "Deployment info retrieved successfully",
		ResultData: &models.ResultData{
			Type:     "deployment_info",
			ItemType: "deployment_info",
			Items: []models.ResourceInfo{
				{
					Name:   deploymentInfo.Name,
					Status: fmt.Sprintf("%d replicas", deploymentInfo.Replicas),
				},
			},
		},
	}, nil
}

func (c *ExecutorClient) getPodLogs(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	url := fmt.Sprintf("%s/api/kubernetes/%s/pods/%s/logs?container=%s&tail=%s", c.baseURL, req.Parameters["namespace"], req.Parameters["pod_name"], req.Parameters["container"], req.Parameters["tail"])
	log.Printf("ExecutorClient: getting pod logs with URL: %s", url)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.ActionResult{}, err
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return models.ActionResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.ActionResult{Error: fmt.Sprintf("failed to get pod logs: status code %d", resp.StatusCode)}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.ActionResult{}, err
	}

	return models.ActionResult{
		Message: "Pod logs retrieved successfully",
		ResultData: &models.ResultData{
			Type:     "pod_logs",
			ItemType: "pod_logs",
			Items: []models.ResourceInfo{
				{
					Name:   "logs",
					Status: string(body),
				},
			},
		},
	}, nil
}

func (c *ExecutorClient) describePod(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	url := fmt.Sprintf("%s/api/kubernetes/%s/pods/%s/describe", c.baseURL, req.Parameters["namespace"], req.Parameters["pod_name"])
	log.Printf("ExecutorClient: describing pod with URL: %s", url)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.ActionResult{}, err
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return models.ActionResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.ActionResult{Error: fmt.Sprintf("failed to describe pod: status code %d", resp.StatusCode)}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.ActionResult{}, err
	}

	return models.ActionResult{
		Message: "Pod description retrieved successfully",
		ResultData: &models.ResultData{
			Type:     "pod_description",
			ItemType: "pod_description",
			Items: []models.ResourceInfo{
				{
					Name:   "description",
					Status: string(body),
				},
			},
		},
	}, nil
}

func (c *ExecutorClient) describeDeployment(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	url := fmt.Sprintf("%s/api/kubernetes/%s/deployments/%s/describe", c.baseURL, req.Parameters["namespace"], req.Parameters["deployment"])
	log.Printf("ExecutorClient: describing deployment with URL: %s", url)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.ActionResult{}, err
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return models.ActionResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.ActionResult{Error: fmt.Sprintf("failed to describe deployment: status code %d", resp.StatusCode)}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.ActionResult{}, err
	}

	return models.ActionResult{
		Message: "Deployment description retrieved successfully",
		ResultData: &models.ResultData{
			Type:     "deployment_description",
			ItemType: "deployment_description",
			Items: []models.ResourceInfo{
				{
					Name:   "description",
					Status: string(body),
				},
			},
		},
	}, nil
}

func (c *ExecutorClient) rollbackDeployment(ctx context.Context, req models.ActionRequest) (models.ActionResult, error) {
	url := fmt.Sprintf("%s/api/kubernetes/%s/deployments/%s/rollback", c.baseURL, req.Parameters["namespace"], req.Parameters["deployment"])
	log.Printf("ExecutorClient: rolling back deployment with URL: %s", url)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return models.ActionResult{}, err
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return models.ActionResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.ActionResult{Error: fmt.Sprintf("failed to rollback deployment: status code %d", resp.StatusCode)}, nil
	}

	return models.ActionResult{Message: "Deployment rolled back successfully"}, nil
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

func convertResources(res []*ContainerResources) []models.ContainerResources {
	if res == nil {
		return nil
	}
	result := make([]models.ContainerResources, len(res))
	for i, r := range res {
		result[i] = models.ContainerResources{
			Name:         r.Name,
			CpuUsage:     r.CpuUsage,
			MemoryUsage:  r.MemoryUsage,
			CpuLimits:    r.CpuLimits,
			MemoryLimits: r.MemoryLimits,
		}
	}
	return result
}
