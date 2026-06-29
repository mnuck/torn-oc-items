package tracking

import (
	"log/slog"
	"sync"
)

type StateTransition struct {
	CrimeID   int
	FromState string
	ToState   string
	CrimeName string
}

type StateTracker struct {
	crimeStates map[int]string
	mutex       sync.RWMutex
}

func NewStateTracker() *StateTracker {
	return &StateTracker{
		crimeStates: make(map[int]string),
	}
}

func (st *StateTracker) UpdateCrimeState(crimeID int, crimeName, newState string) *StateTransition {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	previousState, exists := st.crimeStates[crimeID]
	st.crimeStates[crimeID] = newState

	if exists && previousState != newState {
		slog.Debug("Crime state transition detected",
			"crime_id", crimeID,
			"crime_name", crimeName,
			"from_state", previousState,
			"to_state", newState,
		)

		return &StateTransition{
			CrimeID:   crimeID,
			FromState: previousState,
			ToState:   newState,
			CrimeName: crimeName,
		}
	}

	if !exists {
		slog.Debug("First time seeing crime, recording state",
			"crime_id", crimeID,
			"crime_name", crimeName,
			"state", newState,
		)
	}

	return nil
}

func (st *StateTracker) GetCrimeState(crimeID int) (string, bool) {
	st.mutex.RLock()
	defer st.mutex.RUnlock()

	state, exists := st.crimeStates[crimeID]
	return state, exists
}

func (st *StateTracker) GetTrackedCrimesCount() int {
	st.mutex.RLock()
	defer st.mutex.RUnlock()

	return len(st.crimeStates)
}

func IsTransitionOfInterest(transition *StateTransition) bool {
	return transition.FromState == "planning" && transition.ToState == "completed"
}
