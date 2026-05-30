package flashduty

import (
	"context"

	flashduty "github.com/flashcatcloud/go-flashduty"

	sdk "github.com/flashcatcloud/flashduty-sdk"
)

// Clients bundles the two Flashduty API clients a tool handler may need.
//
// New is the typed go-flashduty client and backs every migrated tool. Legacy
// is the hand-written flashduty-sdk client, kept only for the handful of tools
// whose endpoints go-flashduty does not cover yet (query_changes,
// validate_template, query_status_pages).
//
// TODO: drop Legacy and the flashduty-sdk dependency once go-flashduty covers
// /change/list, /template/preview, and /status-page/list.
type Clients struct {
	New    *flashduty.Client
	Legacy *sdk.Client
}

// GetFlashdutyClientFn returns the Flashduty clients for the current request.
type GetFlashdutyClientFn func(context.Context) (context.Context, *Clients, error)
