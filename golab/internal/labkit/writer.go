package labkit

import (
	"io"
	"sync"
)

// LockedWriter serializes writes to a destination that is not safe for
// concurrent use, such as bytes.Buffer. slog handlers may call Write from many
// goroutines, so the writer must provide its own synchronization.
type LockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func NewLockedWriter(w io.Writer) *LockedWriter {
	return &LockedWriter{w: w}
}

func (w *LockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}
