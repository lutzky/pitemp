package client

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lutzky/pitemp/internal/state"
)

func repeatUntilCancelled(ctx context.Context, f func(), interval time.Duration) {
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

func Run(ctx context.Context, server string, updater func(), fetchInterval, updateInterval time.Duration) {
	ctx, cancel := context.WithCancel(ctx)

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, syscall.SIGTERM, syscall.SIGINT)

	go repeatUntilCancelled(ctx, func() { fetchState(server) }, fetchInterval)
	go repeatUntilCancelled(ctx, updater, updateInterval)

	<-interrupted
	cancel()
}

func fetchState(server string) {
	log.Print("Fetching state")
	resp, err := http.Get(server)
	if err != nil {
		log.Printf("ERROR: http GET on %q failed: %v", server, err)
	}

	var s state.State
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		log.Printf("failed to decode response: %v", err)
	}

	state.Set(&s)
}
