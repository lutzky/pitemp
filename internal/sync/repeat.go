package sync

import (
	"context"
	"time"
)

// RepeatUntilCancelled runs f every interval until ctx is cancelled.
func RepeatUntilCancelled(ctx context.Context, f func(), interval time.Duration) {
	for {
		f()
		{
			t := time.NewTimer(interval)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}
}
