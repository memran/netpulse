package state

import "context"

// Manager wraps AppState with a snapshot event stream for the renderer.
type Manager struct {
	state  *AppState
	events chan AppStateSnapshot
}

func NewManager() *Manager {
	return &Manager{
		state:  New(),
		events: make(chan AppStateSnapshot, 1),
	}
}

func (m *Manager) State() *AppState {
	return m.state
}

func (m *Manager) Snapshot() AppStateSnapshot {
	return m.state.Read()
}

func (m *Manager) Events() <-chan AppStateSnapshot {
	return m.events
}

func (m *Manager) Start(ctx context.Context) {
	sub := m.state.Subscribe()
	m.emitLatest()

	for {
		select {
		case <-sub:
			m.emitLatest()
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) emitLatest() {
	snap := m.state.Read()
	select {
	case m.events <- snap:
	default:
		select {
		case <-m.events:
		default:
		}
		select {
		case m.events <- snap:
		default:
		}
	}
}
