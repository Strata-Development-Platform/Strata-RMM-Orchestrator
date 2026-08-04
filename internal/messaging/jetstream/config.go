package jetstream

import (
	"time"
)

// Config holds JetStream stream and consumer configuration.
type Config struct {
	MaxMemoryStore string
	MaxFileStore   string
	StoragePath    string
	NumReplicas    int
	AckWait        time.Duration
	MaxDeliver     int
	AckPolicy      string // "all", "explicit", "none"
	ReplayPolicy   string // "instant", "original"
}

// Default returns the default JetStream configuration.
func Default() Config {
	return Config{
		MaxMemoryStore: "2GB",
		MaxFileStore:   "50GB",
		StoragePath:    "/var/lib/strata/jetstream",
		NumReplicas:    1,
		AckWait:        30 * time.Second,
		MaxDeliver:     10,
		AckPolicy:      "explicit",
		ReplayPolicy:   "instant",
	}
}

// StreamConfigFor returns the stream configuration for a given stream name.
func (c Config) StreamConfigFor(name string, subjects []string) *StreamConfig {
	return &StreamConfig{
		Name:        name,
		Subjects:    subjects,
		MaxAge:      7 * 24 * time.Hour,
		MaxBytes:    parseSize(c.MaxMemoryStore),
		MaxMsgs:     10_000_000,
		Retention:   "limits",
		Storage:     "file",
		Replicas:    c.NumReplicas,
		Discard:     "old",
		AllowRollup: false,
	}
}

// ConsumerConfigFor returns the consumer configuration for a given stream.
func (c Config) ConsumerConfigFor(stream, name string) *ConsumerConfig {
	return &ConsumerConfig{
		Stream:     stream,
		Name:       name,
		Durable:    name,
		AckPolicy:  c.AckPolicy,
		AckWait:    c.AckWait,
		MaxDeliver: c.MaxDeliver,
		Replay:     c.ReplayPolicy,
		RateLimit:  0, // no rate limit by default
	}
}

// StreamConfig represents a JetStream stream definition.
type StreamConfig struct {
	Name        string   `json:"name"`
	Subjects    []string `json:"subjects"`
	MaxAge      time.Duration `json:"max_age,omitempty"`
	MaxBytes    int64    `json:"max_bytes,omitempty"`
	MaxMsgs     int64    `json:"max_msgs,omitempty"`
	Retention   string   `json:"retention"`
	Storage     string   `json:"storage"`
	Replicas    int      `json:"num_replicas"`
	Discard     string   `json:"discard"`
	AllowRollup bool     `json:"allow_rollup_hdrs"`
}

// ConsumerConfig represents a JetStream consumer definition.
type ConsumerConfig struct {
	Stream     string        `json:"stream_name"`
	Name       string        `json:"name"`
	Durable    string        `json:"durable_name"`
	AckPolicy  string        `json:"ack_policy"`
	AckWait    time.Duration `json:"ack_wait"`
	MaxDeliver int           `json:"max_deliver"`
	Replay     string        `json:"replay_policy"`
	RateLimit  uint64        `json:"rate_limit_bps"`
}

func parseSize(s string) int64 {
	s = s + "b"
	var result int64
	for i, c := range s {
		if c >= '0' && c <= '9' {
			if result == 0 {
				result = int64(c - '0')
			} else {
				result = result*10 + int64(c - '0')
			}
		} else if i > 0 {
			multiplier := int64(1)
			switch s[i] {
			case 'K', 'k':
				multiplier = 1024
			case 'M', 'm':
				multiplier = 1024 * 1024
			case 'G', 'g':
				multiplier = 1024 * 1024 * 1024
			case 'T', 't':
				multiplier = 1024 * 1024 * 1024 * 1024
			}
			result *= multiplier
			break
		}
	}
	return result
}
