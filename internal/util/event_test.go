package util

import (
	"testing"
	"time"
)

func TestMessageRetryDecision(t *testing.T) {
	for _, test := range []struct {
		attempt, max uint64
		delay        time.Duration
		terminate    bool
	}{
		{1, 5, 2 * time.Second, false},
		{5, 5, 0, true},
		{1, 0, 0, true},
	} {
		delay, terminate := messageRetryDecision(test.attempt, test.max)
		if delay != test.delay || terminate != test.terminate {
			t.Fatalf("attempt=%d max=%d: got (%s, %t)", test.attempt, test.max, delay, terminate)
		}
	}
}
