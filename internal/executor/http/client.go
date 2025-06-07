package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"chatops-bot/internal/executor/mock"
	"chatops-bot/internal/models"
	"chatops-bot/internal/service"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{},
		baseURL:    baseURL,
	}
}

func (c *Client) GetResourceDetails(req models.ResourceDetailsRequest) (*models.ResourceDetails, error) {
	var respData ResourceDetailsResponse
	err := c.doRequest(context.Background(), "POST", "/resource-details", req, &respData)
	if err != nil {
		return nil, err
	}
	return respData.Details, nil
}

func (c *Client) ExecuteAction(req models.ActionRequest) models.ActionResult {
	var respData ExecuteActionResponse
	err := c.doRequest(context.Background(), "POST", "/execute-action", req, &respData)
	if err != nil {
		return models.ActionResult{Error: err.Error()}
	}
	return respData.Result
}

func (c *Client) GetAvailableResources() (*models.AvailableResources, error) {
	var respData AvailableResourcesResponse
	err := c.doRequest(context.Background(), "GET", "/available-resources", nil, &respData)
	if err != nil {
		return nil, err
	}
	return respData.Resources, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body, target interface{}) error {
	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// NewExecutorClient returns a real or mock client based on a flag.
func NewExecutorClient(useMock bool, baseURL string) service.ExecutorClient {
	if useMock {
		return mock.NewExecutorClientMock()
	}
	return NewClient(baseURL)
}
