package telemetry

import (
	"testing"
	"time"
)

// TestFormatLatency pins the scale selection. time.Duration's own formatting
// renders a one second request as "1.007531416s", where the trailing precision
// is noise when scanning a log.
func TestFormatLatency(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{1007531416 * time.Nanosecond, "1.01s"},
		{2 * time.Second, "2.00s"},
		{1500 * time.Millisecond, "1.50s"},
		{250 * time.Millisecond, "250ms"},
		{1 * time.Millisecond, "1ms"},
		{500 * time.Microsecond, "500µs"},
		{800 * time.Nanosecond, "800ns"},
	}

	for _, tc := range cases {
		if got := formatLatency(tc.in); got != tc.want {
			t.Errorf("formatLatency(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
