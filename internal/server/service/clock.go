package service

import "time"

// SystemClock returns the current system time.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time {
	return time.Now()
}
