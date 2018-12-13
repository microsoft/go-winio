package etw

import (
	"bytes"
	"encoding/binary"
)

type InType byte

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

type EventMetadata struct {
	buffer bytes.Buffer
}

func NewEventMetadata(name string) *EventMetadata {
	em := EventMetadata{}
	binary.Write(&em.buffer, binary.LittleEndian, uint16(0))    // Length placeholder
	binary.Write(&em.buffer, binary.LittleEndian, byte(0))      // Tags
	binary.Write(&em.buffer, binary.LittleEndian, []byte(name)) // Event name
	binary.Write(&em.buffer, binary.LittleEndian, byte(0))      // Null terminator for name
	return &em
}

func (em *EventMetadata) AddField(name string, inType InType) {
	binary.Write(&em.buffer, binary.LittleEndian, []byte(name)) // Field name
	binary.Write(&em.buffer, binary.LittleEndian, byte(0))      // Null terminator for name
	binary.Write(&em.buffer, binary.LittleEndian, byte(inType)) // In type
}
