package flashduty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListIncidents(t *testing.T) {
	responses := map[string]interface{}{
		"/incident/list": map[string]interface{}{
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"incident_id": 1, "incident_title": "Database Connection Issue"},
					map[string]interface{}{"incident_id": 2, "incident_title": "API Response Time High"},
				},
			},
		},
	}
	getClient, translator := testSetup(t, responses)

	tool, handler := ListIncidents(getClient, translator)
	assert.Equal(t, "flashduty_list_incidents", tool.Name)

	request := createMCPRequest(map[string]interface{}{
		"start_time": 1640995200.0, // 2022-01-01 00:00:00 UTC
		"end_time":   1672531200.0, // 2023-01-01 00:00:00 UTC
	})
	ctx := context.Background()
	result, err := handler(ctx, request)

	assert.NoError(t, err)
	textResult := getTextResult(t, result)
	assert.Contains(t, textResult.Text, "Database Connection Issue")
	assert.Contains(t, textResult.Text, "API Response Time High")
}

func TestIncidentInfos(t *testing.T) {
	responses := map[string]interface{}{
		"/incident/list-by-ids": map[string]interface{}{
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"incident_id": "1", "incident_title": "Database Connection Issue", "status": "triggered"},
					map[string]interface{}{"incident_id": "2", "incident_title": "API Response Time High", "status": "acknowledged"},
				},
			},
		},
	}
	getClient, translator := testSetup(t, responses)

	tool, handler := IncidentInfos(getClient, translator)
	assert.Equal(t, "flashduty_incidents_infos", tool.Name)

	request := createMCPRequest(map[string]interface{}{
		"incident_ids": "1,2",
	})
	ctx := context.Background()
	result, err := handler(ctx, request)

	assert.NoError(t, err)
	textResult := getTextResult(t, result)
	assert.Contains(t, textResult.Text, "Database Connection Issue")
	assert.Contains(t, textResult.Text, "triggered")
}

func TestCreateIncident(t *testing.T) {
	responses := map[string]interface{}{
		"/incident/create": map[string]interface{}{
			"data": map[string]interface{}{
				"incident_id": 3, "incident_title": "New Critical Issue",
			},
		},
	}
	getClient, translator := testSetup(t, responses)

	tool, handler := CreateIncident(getClient, translator)
	assert.Equal(t, "flashduty_create_incident", tool.Name)

	request := createMCPRequest(map[string]interface{}{
		"title":             "New Critical Issue",
		"incident_severity": "Critical",
	})
	ctx := context.Background()
	result, err := handler(ctx, request)

	assert.NoError(t, err)
	textResult := getTextResult(t, result)
	assert.Contains(t, textResult.Text, "New Critical Issue")
}

func TestAckIncident(t *testing.T) {
	responses := map[string]interface{}{
		"/incident/ack": map[string]interface{}{
			"data": map[string]interface{}{
				"incident_id": 1, "status": "acknowledged",
			},
		},
	}
	getClient, translator := testSetup(t, responses)

	tool, handler := AckIncident(getClient, translator)
	assert.Equal(t, "flashduty_ack_incident", tool.Name)

	request := createMCPRequest(map[string]interface{}{
		"incident_ids": "1",
	})
	ctx := context.Background()
	result, err := handler(ctx, request)

	assert.NoError(t, err)
	textResult := getTextResult(t, result)
	assert.Contains(t, textResult.Text, "success")
}

func TestResolveIncident(t *testing.T) {
	responses := map[string]interface{}{
		"/incident/resolve": map[string]interface{}{
			"data": map[string]interface{}{
				"incident_id": 1, "status": "resolved",
			},
		},
	}
	getClient, translator := testSetup(t, responses)

	tool, handler := ResolveIncident(getClient, translator)
	assert.Equal(t, "flashduty_resolve_incident", tool.Name)

	request := createMCPRequest(map[string]interface{}{
		"incident_ids": "1",
	})
	ctx := context.Background()
	result, err := handler(ctx, request)

	assert.NoError(t, err)
	textResult := getTextResult(t, result)
	assert.Contains(t, textResult.Text, "success")
}
