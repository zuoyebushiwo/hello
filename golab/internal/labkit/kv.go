package labkit

import (
	"context"
	"sync"
	"time"
)

type kvItem struct {
	value     string
	expiresAt time.Time
}

// KV is an in-memory stand-in for Redis. It deliberately models the operations
// used in real distributed systems: SetNX, TTL, compare-and-swap and a
// token-checked delete. It is not a Redis replacement; it makes the semantics
// observable in a single process.
type KV struct {
	mu   sync.Mutex
	data map[string]kvItem
	now  func() time.Time
}

func NewKV() *KV {
	return &KV{data: make(map[string]kvItem), now: time.Now}
}

func (kv *KV) getLocked(key string) (kvItem, bool) {
	item, ok := kv.data[key]
	if !ok {
		return kvItem{}, false
	}
	if !item.expiresAt.IsZero() && kv.now().After(item.expiresAt) {
		delete(kv.data, key)
		return kvItem{}, false
	}
	return item, true
}

func (kv *KV) Get(key string) (string, bool) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	item, ok := kv.getLocked(key)
	if !ok {
		return "", false
	}
	return item.value, true
}

func (kv *KV) Set(key, value string, ttl time.Duration) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	item := kvItem{value: value}
	if ttl > 0 {
		item.expiresAt = kv.now().Add(ttl)
	}
	kv.data[key] = item
}

// SetNX implements the primitive behind idempotency keys and distributed locks.
func (kv *KV) SetNX(key, value string, ttl time.Duration) bool {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if _, ok := kv.getLocked(key); ok {
		return false
	}
	item := kvItem{value: value}
	if ttl > 0 {
		item.expiresAt = kv.now().Add(ttl)
	}
	kv.data[key] = item
	return true
}

func (kv *KV) Delete(key string) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	delete(kv.data, key)
}

// DeleteIf only deletes when the stored value still equals token. This is the
// release-if-token pattern used by distributed locks.
func (kv *KV) DeleteIf(key, token string) bool {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	item, ok := kv.getLocked(key)
	if !ok || item.value != token {
		return false
	}
	delete(kv.data, key)
	return true
}

// CompareAndSwap updates value only when the current value equals old. Passing
// old == new refreshes the TTL, which is how a lock renewal works.
func (kv *KV) CompareAndSwap(key, old, new string, ttl time.Duration) bool {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	item, ok := kv.getLocked(key)
	if !ok || item.value != old {
		return false
	}
	item.value = new
	if ttl > 0 {
		item.expiresAt = kv.now().Add(ttl)
	} else {
		item.expiresAt = time.Time{}
	}
	kv.data[key] = item
	return true
}

func (kv *KV) Incr(key string) int64 {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	item, ok := kv.getLocked(key)
	var n int64
	if ok {
		for _, c := range item.value {
			if c < '0' || c > '9' {
				n = 0
				break
			}
			n = n*10 + int64(c-'0')
		}
	}
	n++
	kv.data[key] = kvItem{value: itoa(n)}
	return n
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Lock is a lease with optional renewal. Release is token-checked so an expired
// holder can never delete a newer holder's lock.
type Lock struct {
	kv      *KV
	key     string
	token   string
	ttl     time.Duration
	stop    chan struct{}
	once    sync.Once
	renewed bool
}

func (kv *KV) AcquireLock(key, token string, ttl time.Duration, renew bool) (*Lock, bool) {
	if !kv.SetNX(key, token, ttl) {
		return nil, false
	}
	l := &Lock{kv: kv, key: key, token: token, ttl: ttl, stop: make(chan struct{}), renewed: renew}
	if renew && ttl > 0 {
		go l.renewLoop()
	}
	return l, true
}

func (l *Lock) renewLoop() {
	interval := l.ttl / 3
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
			if !l.kv.CompareAndSwap(l.key, l.token, l.token, l.ttl) {
				return
			}
		}
	}
}

func (l *Lock) Release() bool {
	l.once.Do(func() { close(l.stop) })
	return l.kv.DeleteIf(l.key, l.token)
}

func (l *Lock) Token() string { return l.token }

// AcquireWithWait retries until the lock is acquired or ctx ends.
func (kv *KV) AcquireWithWait(ctx context.Context, key, token string, ttl time.Duration, renew bool, retry time.Duration) (*Lock, error) {
	for {
		if l, ok := kv.AcquireLock(key, token, ttl, renew); ok {
			return l, nil
		}
		if !Sleep(ctx, retry) {
			return nil, ctx.Err()
		}
	}
}
