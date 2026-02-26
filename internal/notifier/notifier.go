package notifier

import (
	"context"

	"github.com/ningw42/nixpkgs-pr-tracker/internal/event"
)

type Notifier interface {
	Name() string
	Notify(ctx context.Context, e event.Event) error
}
