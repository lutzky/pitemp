package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/lutzky/pitemp/internal/app/client"
	"github.com/lutzky/pitemp/internal/lcd"
	"github.com/lutzky/pitemp/internal/pioled"
)

var (
	server = flag.String("server", "", "URL for pitemp API server (including /api)")
	port   = flag.Int("port", 8081, "HTTP Serving port")

	fetchInterval  = flag.Duration("fetch_interval", 1*time.Minute, "How often to poll the API server")
	updateInterval = flag.Duration("update_interval", 2*time.Second, "How often to update the screen")

	ipIface = flag.String("ip_iface", "wlan0", "Network interface for IP address")
)

func main() {
	flag.Parse()

	if *server == "" {
		log.Print("--server not provided")
		os.Exit(1)
	}

	lcd.IPIface = *ipIface
	if err := lcd.Initialize(); err != nil {
		log.Printf("Failed to initialize pioled: %v", err)
		os.Exit(1)
	}
	defer lcd.Cleanup()

	http.HandleFunc("/", pioled.HTTPResponse)
	srv := http.Server{Addr: fmt.Sprintf(":%d", *port)}
	go srv.ListenAndServe()
	defer srv.Shutdown(context.Background())

	log.Print("Starting client")
	client.Run(
		context.Background(),
		*server, lcd.Display,
		*fetchInterval, *updateInterval)
}
