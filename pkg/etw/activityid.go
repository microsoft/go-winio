//go:build windows

package etw

import "github.com/Microsoft/go-winio/pkg/guid"

type eventActivityIDControlCode uint32

//nolint:unused // all values listed here for completeness.
const (
	// Sets the ActivityId parameter to the value of the current thread's activity ID.
	getEventActivityID eventActivityIDControlCode = iota + 1
	// Sets the current thread's activity ID to the value of the ActivityId parameter.
	setEventActivityID
	// Sets the ActivityId parameter to the value of a newly-generated locally-unique activity ID.
	createEventActivityID
	// Swaps the values of the ActivityId parameter and the current thread's activity ID.
	// (Saves the value of the current thread's activity ID, then sets the current thread's activity ID to
	// the value of the ActivityId parameter, then sets the ActivityId parameter to the saved value.)
	getSetEventActivityID
	// Sets the ActivityId parameter to the value of the current thread's activity ID,
	// then sets the current thread's activity ID to the value of a newly-generated locally-unique activity ID
	createSetEventActivityID
)

// Activity ID is thread local, but since go doesn't expose a way to initialize threads,
// we have no way of calling this for all threads, or even knowing if the current thread
// was initialized without a syscall to [eventActivityIdControl]

// InitializeThreadActivityID checks if the current thread's activity ID is empty, and, if so,
// creates a new activity ID for the thread.
//
// Subsequent ETW calls from this thread will use that Activity ID, if no ID is specified.
//
// See [EventActivityIdControl] for more information.
//
// [EventActivityIdControl]: https://learn.microsoft.com/en-us/windows/win32/api/evntprov/nf-evntprov-eventactivityidcontrol
func InitializeThreadActivityID() (guid.GUID, error) {
	// check if the current thread is intialized
	var g guid.GUID
	if err := eventActivityIdControl(getEventActivityID, &g); err != nil {
		return guid.GUID{}, err
	}
	if !g.IsEmpty() {
		return g, nil
	}

	// create a new activity ID
	if err := eventActivityIdControl(createEventActivityID, &g); err != nil {
		return guid.GUID{}, err
	}

	// set the ID
	if err := eventActivityIdControl(setEventActivityID, &g); err != nil {
		return guid.GUID{}, err
	}
	return g, nil
}

// GetThreadActivityID returns the current thread's activity ID.
//
// See [InitializeThreadActivityID] for more details.
func GetThreadActivityID() (guid.GUID, error) {
	var g guid.GUID
	err := eventActivityIdControl(getEventActivityID, &g)
	return g, err
}

// SetThreadActivityID returns the current thread's activity ID.
//
// See [InitializeThreadActivityID] for more details.
func SetThreadActivityID(g guid.GUID) error {
	return eventActivityIdControl(setEventActivityID, &g)
}
