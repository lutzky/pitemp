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
	"github.com/lutzky/pitemp/internal/sync"
)

// Run runs a client fetching state from server every fetchInterval, running
// update every updateInterval. It does so until the context is externally
// cancelled, or until receiving SIGTERM or SIGINT, which also cancels the
// context.
func Run(ctx context.Context, server string, updater func(), fetchInterval, updateInterval time.Duration) {
	ctx, cancel := context.WithCancel(ctx)

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, syscall.SIGTERM, syscall.SIGINT)

	go sync.RepeatUntilCancelled(ctx, func() { fetchState(server) }, fetchInterval)
	go sync.RepeatUntilCancelled(ctx, updater, updateInterval)

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
