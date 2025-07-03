package flashduty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelInfos(t *testing.T) {
	responses := map[string]interface{}{
		"/channel/infos": map[string]interface{}{
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"channel_id": 1, "name": "Slack Channel", "type": "slack"},
					map[string]interface{}{"channel_id": 2, "name": "Teams Channel", "type": "teams"},
				},
			},
		},
	}
	getClient, translator := testSetup(t, responses)

	tool, handler := ChannelInfos(getClient, translator)
	assert.Equal(t, "flashduty_channels_infos", tool.Name)

	request := createMCPRequest(map[string]interface{}{
		"channel_ids": "1,2",
	})
	ctx := context.Background()
	result, err := handler(ctx, request)

	assert.NoError(t, err)
	textResult := getTextResult(t, result)
	assert.Contains(t, textResult.Text, "Slack Channel")
	assert.Contains(t, textResult.Text, "Teams Channel")
}
