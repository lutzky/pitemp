package lcd

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/d2r2/go-hd44780"
	"github.com/d2r2/go-i2c"
	"github.com/lutzky/pitemp/internal/state"
)

var i2cCloser *i2c.I2C

// DegreeSymbol is the character code used for displaying the degrees
// symbol (normally "°"). We're using the Japanese handakuten (゜).
const DegreeSymbol = 0xdf

// IPIface determines which interface (if any) the IP address will be read from
var IPIface string

// RefreshDelay controls how often the LCD is refreshed
var RefreshDelay = 2 * time.Second

// BacklightOff determines if the backlight should be turned off when done
var BacklightOff = true

var lcd *hd44780.Lcd

// Initialize the HD44780 LCD
func Initialize() error {
	var err error
	i2cCloser, err = i2c.NewI2C(0x27, 1)
	if err != nil {
		return fmt.Errorf("failed to initialize I2C: %w", err)
	}

	lcd, err = hd44780.NewLcd(i2cCloser, hd44780.LCD_20x4)
	if err != nil {
		return fmt.Errorf("failed to initialize LCD: %w", err)
	}

	err = lcd.BacklightOn()
	if err != nil {
		return fmt.Errorf("failed to turn backlight on: %w", err)
	}

	return nil
}

func Updater(ctx context.Context) {
	for {
		var err error

		s := state.Get()

		message := "[LCD live]"

		if !s.LastSensorUpdate.IsZero() {
			message = fmt.Sprintf("Freshness: %s",
				time.Now().Sub(s.LastSensorUpdate).Round(time.Second))
		}

		err = lcd.ShowMessage(message, hd44780.SHOW_LINE_1|hd44780.SHOW_BLANK_PADDING)
		if err != nil {
			log.Printf("Failed to show message: %v\n", err)
		}

		if IPIface != "" {
			ipaddr, err := getIP(IPIface)
			if err != nil {
				ipaddr = err.Error()
			}

			err = lcd.ShowMessage(ipaddr, hd44780.SHOW_LINE_2|hd44780.SHOW_BLANK_PADDING)
			if err != nil {
				log.Printf("Failed to show IP Address: %v\n", err)
			}
		}

		dhtMessage := "[waiting for dht11]"
		if !s.LastSensorUpdate.IsZero() {
			dhtMessage = fmt.Sprintf("%.0f%cC, %.0f%% humid",
				s.Temperature, DegreeSymbol, s.Humidity)
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
			t := time.NewTimer(RefreshDelay)
			defer t.Stop()
			select {
			case <-ctx.Done():
				cleanup()
				return
			case <-t.C:
			}
		}
	}
}

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

func cleanup() {
	if err := lcd.BacklightOff(); err != nil {
		log.Printf("ERROR: Failed to turn off backlight: %v", err)
	}
	i2cCloser.Close()
}
