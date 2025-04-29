package autotester

import (
	"time"

	"github.com/teambition/rrule-go"
)

// ParseSchedule parses an RRULE string (e.g. "FREQ=HOURLY;INTERVAL=4").
// Empty string → no schedule.
func ParseSchedule(ruleStr string) (*rrule.RRule, error) {
	if ruleStr == "" {
		return nil, nil
	}
	// Prepend DTSTART=now in UTC
	start := time.Now().UTC().Format("20060102T150405Z")
	full := "DTSTART=" + start + ";" + ruleStr
	return rrule.StrToRRule(full)
}

// StartSchedule spawns a goroutine that waits for each recurrence,
// then calls the provided callback. It stops when quit is closed.
func StartSchedule(ruleStr string, quit <-chan struct{}, callback func()) {
	if ruleStr == "" {
		return
	}
	rr, err := ParseSchedule(ruleStr)
	if err != nil {
		// Invalid schedule string: nothing to do
		return
	}
	go func() {
		for {
			// Compute next time after now
			next := rr.After(time.Now(), false)
			if next.IsZero() {
				// No more occurrences
				return
			}
			// Wait until then or until quit
			select {
			case <-time.After(time.Until(next)):
				callback()
			case <-quit:
				return
			}
		}
	}()
}
