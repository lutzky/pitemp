package main

import (
	"context"
	_ "embed"
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

	"github.com/lutzky/pitemp/lcd"
	"github.com/lutzky/pitemp/pioled"
	"github.com/lutzky/pitemp/state"
)

var (
	ipIface   = flag.String("ip_iface", "wlan0", "Network interface for IP address")
	quitAfter = flag.Duration("quit_after", 0, "Automatically quit after this many seconds (0 for never)")

	dhtDelay   = flag.Duration("dht11_delay", time.Minute, "Frequency of DHT11 measurement")
	dhtPin     = flag.Int("dht11_pin", 4, "GPIO pin to which DHT11 data pin is connected")
	dhtRetries = flag.Int("dht11_retries", 10, "Retries for DHT11")

	lcdEnabled = flag.Bool("lcd_enabled", false, "Whether or not to use an HD44780 LCD")

	piOLEDEnabled      = flag.Bool("pioled_enabled", true, "Whether or not to use a PiOLED display")
	piOLEDRefreshDelay = flag.Duration("pioled_refresh_delay", 500*time.Millisecond, "How often to refresh PiOLED Display")

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
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()
	logger.ChangePackageLogLevel("i2c", logger.InfoLevel)
	logger.ChangePackageLogLevel("dht", logger.InfoLevel)

	http.HandleFunc("/", serveHTTP)
	http.HandleFunc("/pioled", pioled.HTTPResponse)
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(fmt.Sprintf(":%d", *flagPort), nil)

	if *lcdEnabled {
		lcd.IPIface = *ipIface
		if err := lcd.Initialize(); err != nil {
			log.Fatal(err)
		}
	}

	if *piOLEDEnabled {
		pioled.Initialize()
	}

	ctx, cancel := context.WithCancel(context.Background())

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, syscall.SIGTERM, syscall.SIGINT)

	if *quitAfter > 0 {
		go func() {
			time.Sleep(*quitAfter)
			interrupted <- syscall.SIGINT
		}()
	}

	go dhtUpdater(ctx)
	if *lcdEnabled {
		go lcd.Updater(ctx)
	}
	if *piOLEDEnabled {
		go pioled.Updater(ctx, *piOLEDRefreshDelay)
	}

	select {
	case <-interrupted:
		cancel()
	}
}

func dhtUpdater(ctx context.Context) {
	for {
		temperature, humidity, _, err := dht.ReadDHTxxWithRetry(dht.DHT11, *dhtPin, false, *dhtRetries)
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

		{
			t := time.NewTimer(*dhtDelay)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}
}
