package store

import (
	"sync"
	"time"
)

type Session struct {
	SessionID  string  `json:"session_id"`
	AbducoName string  `json:"abduco_name"`
	Title      string  `json:"title,omitempty"`
	Kind       string  `json:"kind"`
	State      string  `json:"state"`
	UpdatedAt  float64 `json:"updated_at"`
}

type Event struct {
	Type      string  `json:"type"`
	SessionID string  `json:"session_id"`
	UpdatedAt float64 `json:"updated_at"`

	// Present for session-upsert
	Session *Session `json:"session,omitempty"`
	// Present for session-state
	State string `json:"state,omitempty"`
}

type subscriber struct {
	ch chan Event
}

type Store struct {
	mu          sync.RWMutex
	sessions    map[string]Session
	subscribers map[*subscriber]struct{}
}

func New() *Store {
	return &Store{
		sessions:    make(map[string]Session),
		subscribers: make(map[*subscriber]struct{}),
	}
}

func NewWithSeeds() *Store {
	s := New()
	now := NowUnix()
	s.sessions["sess-1"] = Session{
		SessionID:  "sess-1",
		AbducoName: "pi:gmux:1",
		Title:      "gmux bootstrap",
		Kind:       "pi",
		State:      "running",
		UpdatedAt:  now,
	}
	s.sessions["sess-2"] = Session{
		SessionID:  "sess-2",
		AbducoName: "pi:gmux:2",
		Title:      "docs cleanup",
		Kind:       "pi",
		State:      "waiting",
		UpdatedAt:  now - 15,
	}
	return s
}

func (s *Store) List() []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Session, 0, len(s.sessions))
	for _, item := range s.sessions {
		items = append(items, item)
	}
	return items
}

func (s *Store) Get(id string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *Store) Upsert(sess Session) {
	s.mu.Lock()
	sess.UpdatedAt = NowUnix()
	s.sessions[sess.SessionID] = sess
	s.mu.Unlock()

	s.broadcast(Event{
		Type:      "session-upsert",
		SessionID: sess.SessionID,
		UpdatedAt: sess.UpdatedAt,
		Session:   &sess,
	})
}

func (s *Store) SetState(id, state string) (Session, bool) {
	s.mu.Lock()
	sess, ok := s.sessions[id]
	if !ok {
		s.mu.Unlock()
		return Session{}, false
	}
	sess.State = state
	sess.UpdatedAt = NowUnix()
	s.sessions[id] = sess
	s.mu.Unlock()

	s.broadcast(Event{
		Type:      "session-state",
		SessionID: id,
		UpdatedAt: sess.UpdatedAt,
		State:     state,
	})
	return sess, true
}

func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	_, ok := s.sessions[id]
	if ok {
		delete(s.sessions, id)
	}
	s.mu.Unlock()

	if ok {
		s.broadcast(Event{
			Type:      "session-remove",
			SessionID: id,
			UpdatedAt: NowUnix(),
		})
	}
	return ok
}

func (s *Store) Subscribe() (<-chan Event, func()) {
	sub := &subscriber{ch: make(chan Event, 64)}

	s.mu.Lock()
	s.subscribers[sub] = struct{}{}
	s.mu.Unlock()

	cancel := func() {
		s.mu.Lock()
		delete(s.subscribers, sub)
		s.mu.Unlock()
		close(sub.ch)
	}
	return sub.ch, cancel
}

func (s *Store) broadcast(ev Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for sub := range s.subscribers {
		select {
		case sub.ch <- ev:
		default:
			// slow subscriber, drop event
		}
	}
}

func NowUnix() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second)
}
