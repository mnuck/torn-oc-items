package tracking

import (
	"sync"

	"github.com/rs/zerolog/log"
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
		log.Debug().
			Int("crime_id", crimeID).
			Str("crime_name", crimeName).
			Str("from_state", previousState).
			Str("to_state", newState).
			Msg("Crime state transition detected")

		return &StateTransition{
			CrimeID:   crimeID,
			FromState: previousState,
			ToState:   newState,
			CrimeName: crimeName,
		}
	}

	if !exists {
		log.Debug().
			Int("crime_id", crimeID).
			Str("crime_name", crimeName).
			Str("state", newState).
			Msg("First time seeing crime, recording state")
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
