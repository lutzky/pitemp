package state

import (
	"sync"
	"time"
)

var state = struct {
	mu sync.RWMutex

	State
}{}

// Get the current state; thread-safe
func Get() State {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.State
}

// Set the current state; thread-safe
func Set(s *State) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.State = *s
}

// State represents the global state for pitemp
type State struct {
	Temperature, Humidity float32
	IP                    string
	LastSensorUpdate      time.Time
}
