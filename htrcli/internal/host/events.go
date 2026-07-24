package host

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const defaultEventCap = 500

var generationCounter int64

// Event is the daemon's internal representation of one captured event.
type Event struct {
	Seq       int
	Kind      string
	Timestamp int64
	Data      []byte
}

type eventBucket struct {
	entries []Event
	nextSeq int
}

// EventStore holds per-(tab, kind) capped ring buffers of captured events.
type EventStore struct {
	mu          sync.Mutex
	buckets     map[string]*eventBucket
	cap         int
	capOverride int
	generation  int64
}

// NewEventStore creates an empty store and stamps it with a unique generation.
func NewEventStore() *EventStore {
	return &EventStore{
		buckets:    make(map[string]*eventBucket),
		cap:        defaultEventCap,
		generation: time.Now().UnixNano() + atomic.AddInt64(&generationCounter, 1),
	}
}

func (s *EventStore) capFor() int {
	if s.capOverride > 0 {
		return s.capOverride
	}
	return s.cap
}

func bucketKey(tabID int, kind string) string {
	return kind + ":" + strconv.Itoa(tabID)
}

// Ingest appends entries for a (tab, kind), assigning the next sequence number
// and evicting the oldest entries past the cap.
func (s *EventStore) Ingest(tabID int, kind string, entries []Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := bucketKey(tabID, kind)
	b, ok := s.buckets[key]
	if !ok {
		b = &eventBucket{}
		s.buckets[key] = b
	}

	for _, e := range entries {
		b.nextSeq++
		e.Seq = b.nextSeq
		e.Kind = kind
		b.entries = append(b.entries, e)
	}

	if cap := s.capFor(); len(b.entries) > cap {
		overflow := len(b.entries) - cap
		b.entries = b.entries[overflow:]
	}
}

// Read returns entries with seq > since, plus how many entries this client
// missed and the oldest sequence still available.
func (s *EventStore) Read(tabID int, kind string, since int) (entries []Event, dropped int, oldestAvailableSeq int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.buckets[bucketKey(tabID, kind)]
	if !ok || len(b.entries) == 0 {
		return nil, 0, 0
	}

	oldestAvailableSeq = b.entries[0].Seq
	if since < oldestAvailableSeq-1 {
		dropped = oldestAvailableSeq - since - 1
	}

	for _, e := range b.entries {
		if e.Seq > since {
			entries = append(entries, e)
		}
	}
	return entries, dropped, oldestAvailableSeq
}

// Generation identifies this daemon process instance.
func (s *EventStore) Generation() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation
}
