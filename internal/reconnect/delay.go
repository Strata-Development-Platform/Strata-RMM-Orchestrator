package reconnect

import (
	"crypto/rand"
	"io"
	"math/big"
	"time"
)

const maximumExponent = 6

// Delay returns capped exponential backoff with full jitter. A fleet using the
// same base delay therefore does not reconnect in synchronized waves.
func Delay(base, maximum time.Duration, attempt int, random io.Reader) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if maximum < base {
		maximum = base
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt > maximumExponent {
		attempt = maximumExponent
	}
	capDelay := base * time.Duration(1<<attempt)
	if capDelay > maximum {
		capDelay = maximum
	}
	if random == nil {
		random = rand.Reader
	}
	value, err := rand.Int(random, big.NewInt(int64(capDelay)+1))
	if err != nil {
		return capDelay
	}
	return time.Duration(value.Int64())
}
