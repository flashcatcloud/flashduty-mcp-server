// This is a new file
package flashduty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTeamInfos(t *testing.T) {
	responses := map[string]interface{}{
		"/team/infos": map[string]interface{}{
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"team_id": 1, "team_name": "Engineering Team", "person_ids": []interface{}{1, 2, 3}},
					map[string]interface{}{"team_id": 2, "team_name": "Operations Team", "person_ids": []interface{}{4, 5}},
				},
			},
		},
	}
	getClient, translator := testSetup(t, responses)

	tool, handler := TeamInfos(getClient, translator)
	assert.Equal(t, "flashduty_teams_infos", tool.Name)

	request := createMCPRequest(map[string]interface{}{
		"team_ids": "1,2",
	})
	ctx := context.Background()
	result, err := handler(ctx, request)

	assert.NoError(t, err)
	textResult := getTextResult(t, result)
	assert.Contains(t, textResult.Text, "Engineering Team")
	assert.Contains(t, textResult.Text, "Operations Team")
}
