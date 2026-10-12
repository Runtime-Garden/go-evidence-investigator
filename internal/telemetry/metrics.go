package telemetry

import (
	"sync"
	"time"
)

type Stage struct {
	Calls      int64 `json:"calls"`
	Errors     int64 `json:"errors"`
	DurationMS int64 `json:"total_duration_ms"`
}
type Metrics struct {
	mu sync.Mutex
	m  map[string]Stage
}

func New() *Metrics { return &Metrics{m: make(map[string]Stage)} }
func (m *Metrics) Observe(stage string, d time.Duration, err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.m[stage]
	v.Calls++
	v.DurationMS += d.Milliseconds()
	if err != nil {
		v.Errors++
	}
	m.m[stage] = v
}
func (m *Metrics) Snapshot() map[string]Stage {
	out := make(map[string]Stage)
	if m == nil {
		return out
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.m {
		out[k] = v
	}
	return out
}
