package service_test

import (
	"testing"

	"chatops-bot/internal/models"
	"chatops-bot/internal/service"

	"github.com/stretchr/testify/assert"
)

func TestActionSuggester_SuggestActions(t *testing.T) {
	suggester := service.NewActionSuggester()

	testCases := []struct {
		name            string
		incident        *models.Incident
		expectedActions int
		assertFunc      func(t *testing.T, actions []models.SuggestedAction)
	}{
		{
			name: "Should suggest restart and rollback for KubeDeploymentReplicasMismatch",
			incident: &models.Incident{
				Labels:            models.JSONBMap{"alertname": "KubeDeploymentReplicasMismatch"},
				AffectedResources: models.JSONBMap{"deployment": "api-gateway", "namespace": "prod"},
			},
			expectedActions: 2,
			assertFunc: func(t *testing.T, actions []models.SuggestedAction) {
				assert.Equal(t, string(models.ActionRestartDeployment), actions[0].Action)
				assert.Equal(t, "api-gateway", actions[0].Parameters["deployment"])
				assert.Equal(t, "prod", actions[0].Parameters["namespace"])
				assert.Equal(t, string(models.ActionRollbackDeployment), actions[1].Action)
			},
		},
		{
			name: "Should suggest get logs for KubePodCrashLooping",
			incident: &models.Incident{
				Labels:            models.JSONBMap{"alertname": "KubePodCrashLooping"},
				AffectedResources: models.JSONBMap{"pod": "api-gateway-123", "namespace": "prod"},
			},
			expectedActions: 1,
			assertFunc: func(t *testing.T, actions []models.SuggestedAction) {
				assert.Equal(t, string(models.ActionGetPodLogs), actions[0].Action)
				assert.Equal(t, "api-gateway-123", actions[0].Parameters["pod"])
			},
		},
		{
			name: "Should suggest nothing for unknown alert",
			incident: &models.Incident{
				Labels: models.JSONBMap{"alertname": "SomeOtherAlert"},
			},
			expectedActions: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actions := suggester.SuggestActions(tc.incident)
			assert.Len(t, actions, tc.expectedActions)
			if tc.assertFunc != nil {
				tc.assertFunc(t, actions)
			}
		})
	}
}

func TestActionSuggester_SuggestActionsForResource(t *testing.T) {
	suggester := service.NewActionSuggester()
	incident := &models.Incident{
		AffectedResources: models.JSONBMap{"namespace": "prod"},
	}

	t.Run("Should suggest actions for deployment", func(t *testing.T) {
		actions := suggester.SuggestActionsForResource(incident, "deployment", "my-deploy")
		assert.Len(t, actions, 3)
		assert.Equal(t, string(models.ActionRestartDeployment), actions[0].Action)
		assert.Equal(t, "my-deploy", actions[0].Parameters["deployment"])
		assert.Equal(t, "prod", actions[0].Parameters["namespace"])
	})

	t.Run("Should suggest actions for pod", func(t *testing.T) {
		actions := suggester.SuggestActionsForResource(incident, "pod", "my-pod-xyz")
		assert.Len(t, actions, 3)
		assert.Equal(t, string(models.ActionGetPodLogs), actions[0].Action)
		assert.Equal(t, "my-pod-xyz", actions[0].Parameters["pod"])
		assert.Equal(t, "prod", actions[0].Parameters["namespace"])
	})
}
