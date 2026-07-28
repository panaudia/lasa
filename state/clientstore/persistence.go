package clientstore

import "sync"

// MemPersistence is an in-memory Persistence, for tests and ephemeral
// deployments that still want reconnect continuity within a process.
type MemPersistence struct {
	mu    sync.Mutex
	saved map[string]*Snapshot
}

func NewMemPersistence() *MemPersistence {
	return &MemPersistence{saved: map[string]*Snapshot{}}
}

func (m *MemPersistence) Load(spaceID string) (*Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.saved[spaceID]
	if !ok {
		return nil, nil
	}
	// Return a copy so the store cannot alias saved state.
	entries := make(map[string]KV, len(s.Entries))
	for k, v := range s.Entries {
		entries[k] = v
	}
	out := &Snapshot{Entries: entries}
	if s.Cursor != nil {
		c := *s.Cursor
		out.Cursor = &c
	}
	return out, nil
}

func (m *MemPersistence) Save(spaceID string, s *Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saved[spaceID] = s
	return nil
}
