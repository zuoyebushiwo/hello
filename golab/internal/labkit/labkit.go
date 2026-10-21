// Package labkit contains the small, dependency-free building blocks shared by
// the 15-day Go lab. Everything here is intentionally explicit: no framework,
// no hidden lifecycle, no magic.
package labkit

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Lab prints a consistent chapter banner and tracks failed assertions.
type Lab struct {
	day    string
	title  string
	start  time.Time
	failed int
}

func New(day, title string) *Lab {
	fmt.Printf("\n%s\n", strings.Repeat("=", 76))
	fmt.Printf("%s  %s\n", day, title)
	fmt.Printf("%s\n", strings.Repeat("=", 76))
	return &Lab{day: day, title: title, start: time.Now()}
}

func (l *Lab) Section(name string) {
	fmt.Printf("\n--- %s ---\n", name)
}

func (l *Lab) Infof(format string, args ...any) {
	fmt.Printf("    "+format+"\n", args...)
}

func (l *Lab) OKf(format string, args ...any) {
	fmt.Printf("  [OK]   "+format+"\n", args...)
}

func (l *Lab) Failf(format string, args ...any) {
	l.failed++
	fmt.Printf("  [FAIL] "+format+"\n", args...)
}

// Check is the lightweight assertion used by the runnable scenarios. A failed
// check makes Done exit with status 1 so the chapter can be used in CI.
func (l *Lab) Check(cond bool, format string, args ...any) bool {
	if cond {
		l.OKf(format, args...)
		return true
	}
	l.Failf(format, args...)
	return false
}

func (l *Lab) Failed() int { return l.failed }

func (l *Lab) Done() {
	elapsed := time.Since(l.start).Round(time.Millisecond)
	fmt.Printf("\n%s finished in %s (failed checks: %d)\n", l.day, elapsed, l.failed)
	if l.failed > 0 {
		os.Exit(1)
	}
}

// Sleep is context-aware. It returns false when ctx is cancelled first. This is
// the single most important habit when replacing Java's Thread.sleep.
func Sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Background returns a context that is cancelled when the returned stop func is
// called. It exists only to keep chapter mains short.
func Background() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
