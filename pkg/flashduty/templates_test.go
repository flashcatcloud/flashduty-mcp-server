package flashduty

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
	sdk "github.com/flashcatcloud/flashduty-sdk"
)

func TestGetPresetTemplateSchemaDoesNotExposeLocale(t *testing.T) {
	t.Parallel()

	tool, _ := GetPresetTemplate(func(ctx context.Context) (context.Context, *sdk.Client, error) {
		return ctx, nil, nil
	}, translations.NullTranslationHelper)

	raw, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("marshal tool: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal tool: %v", err)
	}

	schema, ok := payload["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("expected inputSchema object, got %#v", payload["inputSchema"])
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties object, got %#v", schema["properties"])
	}
	if _, ok := props["locale"]; ok {
		t.Fatal("expected locale to be absent from get_preset_template schema")
	}

	channelSchema, ok := props["channel"].(map[string]any)
	if !ok {
		t.Fatalf("expected channel schema, got %#v", props["channel"])
	}
	enumVals, ok := channelSchema["enum"].([]any)
	if !ok {
		t.Fatalf("expected channel enum values, got %#v", channelSchema["enum"])
	}
	want := sortedChannelEnumValues()
	if len(enumVals) != len(want) {
		t.Fatalf("expected %d channel enum values, got %d", len(want), len(enumVals))
	}
	for i, got := range enumVals {
		if got.(string) != want[i] {
			t.Fatalf("channel enum[%d] = %q, want %q", i, got.(string), want[i])
		}
	}
}
