package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/lutzky/pitemp/internal/pioled"
	"github.com/lutzky/pitemp/internal/state"
)

var (
	server        = flag.String("server", "", "URL for pitemp API server (including /api)")
	port          = flag.Int("port", 8081, "HTTP Serving port")
	fetchInterval = flag.Duration("fetch_interval", 1*time.Minute, "How often to poll the API server")
)

func main() {
	flag.Parse()

	if *server == "" {
		log.Print("--server not provided")
		os.Exit(1)
	}

	if err := pioled.Initialize(); err != nil {
		log.Printf("Failed to initialize pioled: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, syscall.SIGTERM, syscall.SIGINT)

	var wg sync.WaitGroup

	waitGroupGo := func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
	}

	waitGroupGo(func() { fetchState(ctx) })
	waitGroupGo(func() { pioled.Updater(ctx) })
	http.HandleFunc("/", pioled.HTTPResponse)
	go http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)

	select {
	case <-interrupted:
		cancel()
	}

	wg.Wait()
}

func fetchState(ctx context.Context) {
	for {
		resp, err := http.Get(*server)
		if err != nil {
			log.Printf("ERROR: http GET on %q failed: %v", *server, err)
		}

		var s state.State
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			log.Printf("failed to decode response: %v", err)
		}

		state.Set(&s)

		{
			t := time.NewTimer(*fetchInterval)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}
}
