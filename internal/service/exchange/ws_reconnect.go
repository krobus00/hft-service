package exchange

import (
	"math"
	"math/rand"
	"time"
)

func wsReconnectDelay(attempt int, rng *rand.Rand) time.Duration {
	backoff := float64(wsReconnectMinDelay) * math.Pow(wsReconnectFactor, float64(attempt))
	if backoff > float64(wsReconnectMaxDelay) {
		backoff = float64(wsReconnectMaxDelay)
	}

	base := time.Duration(backoff)
	if wsReconnectMaxDelay <= wsReconnectMinDelay {
		return base
	}

	jitterWindow := wsReconnectMaxDelay - wsReconnectMinDelay
	jitter := time.Duration(rng.Int63n(int64(jitterWindow) + 1))
	result := base + jitter
	if result > wsReconnectMaxDelay {
		return wsReconnectMaxDelay
	}

	return result
}
