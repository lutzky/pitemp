package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/d2r2/go-dht"
	"github.com/d2r2/go-hd44780"
	"github.com/d2r2/go-i2c"
	"github.com/d2r2/go-logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ipIface      = flag.String("ip_iface", "wlan0", "Network interface for IP address")
	message      = flag.String("message", "^_^ LCD ACTIVE ^_^", "Message to display")
	backlightOff = flag.Bool("backlight_off_when_done", true, "Turn backlight off when done")
	quitAfter    = flag.Duration("quit_after", 0, "Automatically quit after this many seconds (0 for never)")

	dhtDelay   = flag.Duration("dht11_delay", time.Minute, "Frequency of DHT11 measurement")
	dhtPin     = flag.Int("dht11_pin", 4, "GPIO pin to which DHT11 data pin is connected")
	dhtRetries = flag.Int("dht11_retries", 10, "Retries for DHT11")

	useLCD          = flag.Bool("lcd_enabled", false, "Whether or not to use an HD44780 LCD")
	lcdDegreeSymbol = flag.Int("lcd_degree_symbol", LCDDegreeSymbol, "Character code for degree symbol for LCD")
	lcdRefreshDelay = flag.Duration("lcd_refresh_delay", 2*time.Second, "How often to refresh LCD display")

	flagPort = flag.Int("port", 8080, "HTTP listening port")
)

// LCDDegreeSymbol is the character code used for displaying the degrees
// symbol (normally "°"). We're using the Japanese handakuten (゜).
const LCDDegreeSymbol = 0xdf

func getIP(iface string) (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to get interfaces: %w", err)
	}
	for _, i := range ifaces {
		if i.Name != iface {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			return "", fmt.Errorf("failed to get addrs for %q: %w", iface, err)
		}
		for _, addr := range addrs {
			return addr.String(), nil
		}
	}
	return "", fmt.Errorf("interface %q not found", iface)
}

var state = struct {
	Temperature, Humidity float32
	IP                    string
	LastSensorUpdate      time.Time
}{}

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
	err := httpTemplate.Execute(w, state)
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
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(fmt.Sprintf(":%d", *flagPort), nil)

	check := func(err error) {
		if err != nil {
			log.Fatal(err)
		}
	}

	var lcd *hd44780.Lcd

	if *useLCD {
		i2c, err := i2c.NewI2C(0x27, 1)
		check(err)
		defer i2c.Close()

		lcd, err = hd44780.NewLcd(i2c, hd44780.LCD_20x4)
		check(err)

		err = lcd.BacklightOn()
		check(err)
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
	go lcdUpdater(ctx, lcd)

	select {
	case <-interrupted:
		cancel()
	}

	if *useLCD && *backlightOff {
		err := lcd.BacklightOff()
		check(err)
	}
}

func lcdUpdater(ctx context.Context, lcd *hd44780.Lcd) {
	if lcd == nil {
		return
	}

	for {
		var err error

		if !state.LastSensorUpdate.IsZero() {
			*message = fmt.Sprintf("Freshness: %s",
				time.Now().Sub(state.LastSensorUpdate).Round(time.Second))
		}

		err = lcd.ShowMessage(*message, hd44780.SHOW_LINE_1|hd44780.SHOW_BLANK_PADDING)
		if err != nil {
			log.Printf("Failed to show message: %v\n", err)
		}

		ipaddr, err := getIP(*ipIface)
		if err != nil {
			ipaddr = err.Error()
		}

		err = lcd.ShowMessage(ipaddr, hd44780.SHOW_LINE_2|hd44780.SHOW_BLANK_PADDING)
		if err != nil {
			log.Printf("Failed to show IP Address: %v\n", err)
		}

		dhtMessage := "[waiting for dht11]"
		if !state.LastSensorUpdate.IsZero() {
			dhtMessage = fmt.Sprintf("%.0f%cC, %.0f%% humid",
				state.Temperature, *lcdDegreeSymbol, state.Humidity)
		}
		err = lcd.ShowMessage(dhtMessage, hd44780.SHOW_LINE_3|hd44780.SHOW_BLANK_PADDING)
		if err != nil {
			log.Printf("Failed to show temperature: %v\n", err)
		}

		timeMessage := time.Now().Local().Format("Mon Jan 2 15:04:05")
		err = lcd.ShowMessage(timeMessage, hd44780.SHOW_LINE_4|hd44780.SHOW_BLANK_PADDING)
		if err != nil {
			log.Printf("Failed to show time: %v\n", err)
		}

		{
			t := time.NewTimer(*lcdRefreshDelay)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}
}

func dhtUpdater(ctx context.Context) {
	for {
		temperature, humidity, _, err := dht.ReadDHTxxWithRetry(dht.DHT11, *dhtPin, false, *dhtRetries)
		if err != nil {
			log.Printf("Failed to read DHT11: %v", err)
		} else {
			state.Temperature = temperature
			state.Humidity = humidity
			state.LastSensorUpdate = time.Now()

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
