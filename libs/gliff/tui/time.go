package tui

import "time"

// After returns a command that waits for the specified duration
// then calls fn with the current time to produce a message.
// The timer starts when After is called, not when the command executes.
func After(d time.Duration, fn func(time.Time) Msg) Cmd {
	ch := time.After(d)
	return func() Msg {
		return fn(<-ch)
	}
}

// Every returns a command that waits until the next wall-clock aligned
// tick, then calls fn with the current time. For example, Every(time.Second, fn)
// fires at the start of each second, and Every(time.Minute, fn) fires at :00.
func Every(d time.Duration, fn func(time.Time) Msg) Cmd {
	now := time.Now()
	next := now.Truncate(d).Add(d)
	return After(next.Sub(now), fn)
}
