// Package events fans out live updates (scan progress, torrent stats, TV
// status) to connected UI clients over Server-Sent Events.
package events

import (
	"encoding/json"
	"sync"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Bus struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
	last map[string][]byte // latest event per type, replayed to new subscribers
}

func New() *Bus {
	return &Bus{subs: map[chan []byte]struct{}{}, last: map[string][]byte{}}
}

func (b *Bus) Publish(typ string, data any) {
	msg, err := json.Marshal(Event{Type: typ, Data: data})
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.last[typ] = msg
	for ch := range b.subs {
		select {
		case ch <- msg:
		default: // slow client: drop rather than block the publisher
		}
	}
}

func (b *Bus) Subscribe() (ch chan []byte, cancel func()) {
	ch = make(chan []byte, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	for _, m := range b.last {
		ch <- m
	}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}
