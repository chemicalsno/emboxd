package history

import (
	"container/ring"
	"sync"
)

// DefaultMaxEvents is the default maximum number of events to store
const DefaultMaxEvents = 100

// Store is a thread-safe, fixed-size event history store
type Store struct {
	sync.RWMutex
	events *ring.Ring
	size   int
}

// NewStore creates a new event history store with the specified size
func NewStore(size int) *Store {
	if size <= 0 {
		size = DefaultMaxEvents
	}

	return &Store{
		events: ring.New(size),
		size:   size,
	}
}

// Add adds an event to the history
func (s *Store) Add(event *Event) {
	if event == nil {
		return
	}

	s.Lock()
	defer s.Unlock()

	// Store the event in the current position and advance
	s.events.Value = event
	s.events = s.events.Next()
}

// GetAll returns all events in the store
func (s *Store) GetAll() []*Event {
	s.RLock()
	defer s.RUnlock()

	if s.events.Value == nil {
		// Empty store
		return []*Event{}
	}

	// Count non-nil items
	var count int
	s.events.Do(func(x interface{}) {
		if x != nil {
			count++
		}
	})

	events := make([]*Event, 0, count)
	s.events.Do(func(x interface{}) {
		if x != nil {
			events = append(events, x.(*Event))
		}
	})

	// Sort events from newest to oldest
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	return events
}

// GetLatest returns the most recent n events
func (s *Store) GetLatest(n int) []*Event {
	events := s.GetAll()
	
	if n <= 0 || n >= len(events) {
		return events
	}
	
	return events[:n]
}

// Clear removes all events from the store
func (s *Store) Clear() {
	s.Lock()
	defer s.Unlock()
	
	// Reset all values to nil
	current := s.events
	for i := 0; i < s.size; i++ {
		current.Value = nil
		current = current.Next()
	}
}