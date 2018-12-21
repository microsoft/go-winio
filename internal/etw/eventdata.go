package etw

import (
	"bytes"
	"encoding/binary"
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

// This is mostly added for testing purposes, and will be removed later, as we
// shouldn't take a dependency on binary.Write knowing how to marshal values
// correctly for TraceLogging.
func (ed *EventData) AddSimple(data interface{}) {
	binary.Write(&ed.buffer, binary.LittleEndian, data)
}
