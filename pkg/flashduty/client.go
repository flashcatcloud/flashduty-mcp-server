package flashduty

import (
	"context"

	sdk "github.com/flashcatcloud/flashduty-sdk"
)

// GetFlashdutyClientFn is a function that returns a flashduty SDK client
type GetFlashdutyClientFn func(context.Context) (context.Context, *sdk.Client, error)
