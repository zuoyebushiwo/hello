package labkit

import (
	"bytes"
	"sync"
	"testing"
)

func TestLockedWriterConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	w := NewLockedWriter(&buf)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = w.Write([]byte("x"))
		}()
	}
	wg.Wait()
	if buf.Len() != 32 {
		t.Fatalf("len = %d, want 32", buf.Len())
	}
}
