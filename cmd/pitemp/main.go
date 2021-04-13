package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/d2r2/go-dht"
	"github.com/d2r2/go-logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/lutzky/pitemp/internal/state"
	"github.com/lutzky/pitemp/internal/sync"
)

var (
	dhtDelay   = flag.Duration("dht11_delay", time.Minute, "Frequency of DHT11 measurement")
	dhtPin     = flag.Int("dht11_pin", 4, "GPIO pin to which DHT11 data pin is connected")
	dhtRetries = flag.Int("dht11_retries", 10, "Retries for DHT11")

	flagPort = flag.Int("port", 8080, "HTTP listening port")
)

var (
	tempGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pitemp_temperature_celsius",
		Help: "Current temperature as measured by DHT11",
	})
	humidityGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pitemp_humidity_percent",
		Help: "Current humidity as measured by DHT11",
	})
	lastUpdateGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pitemp_last_update",
		Help: "Last update time from DHT11",
	})
)

func init() {
	prometheus.MustRegister(tempGauge)
	prometheus.MustRegister(humidityGauge)
	prometheus.MustRegister(lastUpdateGauge)
}

//go:embed template.html
var httpTemplateText string

var httpTemplate = template.Must(template.New("root").Parse(httpTemplateText))

func serveHTTP(w http.ResponseWriter, r *http.Request) {
	err := httpTemplate.Execute(w, state.Get())
	if err != nil {
		log.Printf("Error executing HTTP template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func serveJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(state.Get()); err != nil {
		log.Printf("Error encoding JSON: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()
	logger.ChangePackageLogLevel("i2c", logger.InfoLevel)
	logger.ChangePackageLogLevel("dht", logger.InfoLevel)

	srv := &http.Server{Addr: fmt.Sprintf(":%d", *flagPort)}
	http.HandleFunc("/", serveHTTP)
	http.HandleFunc("/api", serveJSON)
	http.Handle("/metrics", promhttp.Handler())
	go srv.ListenAndServe()

	ctx, cancel := context.WithCancel(context.Background())

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-interrupted
		cancel()
	}()

	sync.RepeatUntilCancelled(ctx, func() { dhtUpdater(ctx) }, *dhtDelay)

	if err := srv.Shutdown(context.Background()); err != nil {
		log.Println("Failed to cleanly shut down HTTP server")
		panic(err)
	}
}

func dhtUpdater(ctx context.Context) {
	temperature, humidity, _, err := dht.ReadDHTxxWithContextAndRetry(ctx, dht.DHT11, *dhtPin, false, *dhtRetries)
	if err != nil {
		log.Printf("Failed to read DHT11: %v", err)
	} else {
		state.Set(&state.State{
			Temperature:      temperature,
			Humidity:         humidity,
			LastSensorUpdate: time.Now(),
		})

		tempGauge.Set(float64(temperature))
		humidityGauge.Set(float64(humidity))
		lastUpdateGauge.Set(float64(time.Now().Unix()))
	}
}
