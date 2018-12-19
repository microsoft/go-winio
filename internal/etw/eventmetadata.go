package etw

import (
	"bytes"
	"encoding/binary"
)

// InType indicates the type of data contained in the ETW event.
type InType byte

// Various InType definitions for TraceLogging. These must match the definitions
// found in TraceLoggingProvider.h in the Windows SDK.
const (
	InTypeNull InType = iota
	InTypeUnicodeString
	InTypeAnsiString
	InTypeInt8
	InTypeUint8
	InTypeInt16
	InTypeUint16
	InTypeInt32
	InTypeUint32
	InTypeInt64
	InTypeUint64
	InTypeFloat
	InTypeDouble
	InTypeBool32
)

// EventMetadata maintains a buffer which builds up the metadatadata for an ETW
// event. It needs to be paired with EventData which describes the event.
type EventMetadata struct {
	buffer bytes.Buffer
}

// NewEventMetadata returns a new EventMetadata with event name and initial
// metadata written to the buffer.
func NewEventMetadata(name string) *EventMetadata {
	em := EventMetadata{}
	binary.Write(&em.buffer, binary.LittleEndian, uint16(0))    // Length placeholder
	binary.Write(&em.buffer, binary.LittleEndian, byte(0))      // Tags
	binary.Write(&em.buffer, binary.LittleEndian, []byte(name)) // Event name
	binary.Write(&em.buffer, binary.LittleEndian, byte(0))      // Null terminator for name
	return &em
}

// AddField appends a single field to the end of the event metadata buffer.
func (em *EventMetadata) AddField(name string, inType InType) {
	binary.Write(&em.buffer, binary.LittleEndian, []byte(name)) // Field name
	binary.Write(&em.buffer, binary.LittleEndian, byte(0))      // Null terminator for name
	binary.Write(&em.buffer, binary.LittleEndian, byte(inType)) // In type
}
