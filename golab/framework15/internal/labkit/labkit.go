// Package labkit provides the small console helper shared by the framework
// course. It deliberately has no dependencies beyond the standard library.
package labkit

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Lab struct {
	day    string
	title  string
	start  time.Time
	failed int
}

func New(day, title string) *Lab {
	fmt.Printf("\n%s\n%s  %s\n%s\n", strings.Repeat("=", 76), day, title, strings.Repeat("=", 76))
	return &Lab{day: day, title: title, start: time.Now()}
}

func (l *Lab) Section(name string) {
	fmt.Printf("\n--- %s ---\n", name)
}

func (l *Lab) Infof(format string, args ...any) {
	fmt.Printf("    "+format+"\n", args...)
}

func (l *Lab) Check(condition bool, format string, args ...any) bool {
	if condition {
		fmt.Printf("  [OK]   "+format+"\n", args...)
		return true
	}
	l.failed++
	fmt.Printf("  [FAIL] "+format+"\n", args...)
	return false
}

func (l *Lab) Done() {
	fmt.Printf("\n%s finished in %s (failed checks: %d)\n", l.day, time.Since(l.start).Round(time.Millisecond), l.failed)
	if l.failed > 0 {
		os.Exit(1)
	}
}
