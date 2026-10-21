package labkit

import "sync"

// Message is a deliberately small broker message. Attempt is incremented on
// every Receive, mirroring a delivery count.
type Message struct {
	ID      string
	Payload string
	Attempt int
}

// Queue models at-least-once delivery: Receive makes a message invisible but
// does not remove it; Ack removes it; Nack makes it visible again. Publishing the
// same ID twice simulates broker duplicate delivery.
type Queue struct {
	mu       sync.Mutex
	ready    []*Message
	inflight map[string][]*Message
	acked    map[string]int
}

func NewQueue() *Queue {
	return &Queue{inflight: make(map[string][]*Message), acked: make(map[string]int)}
}

func (q *Queue) Publish(id, payload string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ready = append(q.ready, &Message{ID: id, Payload: payload})
}

func (q *Queue) Receive() (*Message, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.ready) == 0 {
		return nil, false
	}
	msg := q.ready[0]
	q.ready = q.ready[1:]
	msg.Attempt++
	q.inflight[msg.ID] = append(q.inflight[msg.ID], msg)
	return msg, true
}

func (q *Queue) Ack(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.acked[id] += len(q.inflight[id]) + 1
	delete(q.inflight, id)
}

func (q *Queue) Nack(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	msgs := q.inflight[id]
	delete(q.inflight, id)
	if len(msgs) == 0 {
		return
	}
	q.ready = append(msgs, q.ready...)
}

func (q *Queue) Ready() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.ready)
}

func (q *Queue) Inflight() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, msgs := range q.inflight {
		n += len(msgs)
	}
	return n
}
