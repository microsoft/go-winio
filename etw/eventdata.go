package etw

import (
	"bytes"
	"encoding/binary"
)

type EventData struct {
	buffer bytes.Buffer
}

func (ed *EventData) AddString(data string) {
	binary.Write(&ed.buffer, binary.LittleEndian, []byte(data))
	binary.Write(&ed.buffer, binary.LittleEndian, byte(0))
}
