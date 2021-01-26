package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/d2r2/go-dht"
	device "github.com/d2r2/go-hd44780"
	"github.com/d2r2/go-i2c"
	"github.com/d2r2/go-logger"
)

var (
	ipIface      = flag.String("ip_iface", "wlan0", "Network interface for IP address")
	message      = flag.String("message", "^_^ LCD ACTIVE ^_^", "Message to display")
	backlightOff = flag.Bool("backlight_off_when_done", true, "Turn backlight off when done")
	delay        = flag.Duration("delay", 0, "Automatically quit after delay")

	dhtDelay   = flag.Duration("dht11_delay", time.Minute, "Frequency of DHT11 measurement")
	dhtPin     = flag.Int("dht11_pin", 4, "GPIO pin to which DHT11 data pin is connected")
	dhtRetries = flag.Int("dht11_retries", 10, "Retries for DHT11")

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

// TODO(lutzky): Once go1.16 is out, use embed package instead
const httpTemplateText = `
<html>
<head>
<title>PiTemp</title>
</head>
<body>
<h1>PiTemp</h1>
<p>IP address: {{.IP}}</p>
<p>{{.Temperature}}&deg;, {{.Humidity}}&percnt; humidity</p>
<p>Sensor last updated {{.LastSensorUpdate}}</p>
</body>
</html>
`

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
	go http.ListenAndServe(fmt.Sprintf(":%d", *flagPort), nil)

	check := func(err error) {
		if err != nil {
			log.Fatal(err)
		}
	}

	i2c, err := i2c.NewI2C(0x27, 1)
	check(err)
	defer i2c.Close()

	lcd, err := device.NewLcd(i2c, device.LCD_20x4)
	check(err)

	err = lcd.BacklightOn()
	check(err)

	ctx, cancel := context.WithCancel(context.Background())

	go dhtUpdater(ctx)

	ticker := time.NewTicker(*lcdRefreshDelay)
	defer ticker.Stop()
	done := make(chan bool)
	if *delay > 0 {
		go func() {
			time.Sleep(*delay)
			done <- true
		}()
	}

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)

MainLoop:
	for {
		select {
		case <-done:
			cancel()
			break MainLoop
		case <-interrupted:
			cancel()
			break MainLoop
		case <-ticker.C:
			update(lcd)
		}
	}

	if *backlightOff {
		err = lcd.BacklightOff()
		check(err)
	}
}

func update(lcd *device.Lcd) {
	var err error

	if !state.LastSensorUpdate.IsZero() {
		*message = fmt.Sprintf("Freshness: %s             ",
			time.Now().Sub(state.LastSensorUpdate).Round(time.Second))
	}

	err = lcd.ShowMessage(*message, device.SHOW_LINE_1)
	if err != nil {
		log.Printf("Failed to show message: %v\n", err)
	}

	ipaddr, err := getIP(*ipIface)
	if err != nil {
		ipaddr = err.Error()
	}

	err = lcd.ShowMessage(ipaddr, device.SHOW_LINE_2)
	if err != nil {
		log.Printf("Failed to show IP Address: %v\n", err)
	}

	dhtMessage := "[waiting for dht11]"
	if !state.LastSensorUpdate.IsZero() {
		dhtMessage = fmt.Sprintf("%2.1f%cC, %3.0f%% humid ",
			state.Temperature, *lcdDegreeSymbol, state.Humidity)
	}
	err = lcd.ShowMessage(dhtMessage, device.SHOW_LINE_3)
	if err != nil {
		log.Printf("Failed to show temperature: %v\n", err)
	}

	timeMessage := time.Now().Local().Format("Mon Jan 2 15:04:05")
	err = lcd.ShowMessage(timeMessage, device.SHOW_LINE_4)
	if err != nil {
		log.Printf("Failed to show time: %v\n", err)
	}
}

func dhtUpdater(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		temperature, humidity, _, err := dht.ReadDHTxxWithRetry(dht.DHT11, *dhtPin, false, *dhtRetries)
		if err != nil {
			log.Printf("Failed to read DHT11: %v", err)
		} else {
			state.Temperature = temperature
			state.Humidity = humidity
			state.LastSensorUpdate = time.Now()
		}

		time.Sleep(*dhtDelay)
	}
}
