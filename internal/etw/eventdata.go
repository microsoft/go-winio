package etw

import (
	"bytes"
)

// EventData maintains a buffer which builds up the data for an ETW event. It
// needs to be paired with EventMetadata which describes the event.
type EventData struct {
	buffer bytes.Buffer
}

// NewEventData returns a new EventData with an empty buffer.
func NewEventData() *EventData {
	return &EventData{}
}

// AddString appends the data for a string to the end of the buffer.
func (ed *EventData) AddString(data string) {
	ed.buffer.WriteString(data)
	ed.buffer.WriteByte(0)
}
