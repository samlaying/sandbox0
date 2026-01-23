package utils

import (
	"context"
	"errors"
	"time"
)

// Sandbox describes the minimal sandbox data used in tests.
type Sandbox struct {
	ID         string
	TemplateID string
}

// WaitUntil polls a condition until it succeeds or times out.
func WaitUntil(ctx context.Context, timeout, interval time.Duration, condition func(context.Context) (bool, error)) error {
	if condition == nil {
		return errors.New("condition is required")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		ok, err := condition(ctx)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
