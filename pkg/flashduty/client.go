package flashduty

import (
	"context"

	flashduty "github.com/flashcatcloud/go-flashduty"
)

// Clients bundles the Flashduty API clients a tool handler may need.
//
// New is the typed go-flashduty client and backs every tool.
type Clients struct {
	New *flashduty.Client
}

// GetFlashdutyClientFn returns the Flashduty clients for the current request.
type GetFlashdutyClientFn func(context.Context) (context.Context, *Clients, error)
