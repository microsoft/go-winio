package etw

type Channel uint8

const (
	ChannelTracelogging Channel = 11
)

type Level uint8

const (
	LevelAlways Level = iota
	LevelCritical
	LevelError
	LevelWarning
	LevelInfo
	LevelVerbose
)

type Event struct {
	Descriptor *EventDescriptor
	Metadata   *EventMetadata
	Data       *EventData
}

type EventDescriptor struct {
	ID      uint16
	Version uint8
	Channel Channel
	Level   Level
	Opcode  uint8
	Task    uint16
	Keyword uint64
}

func NewEventDescriptor() *EventDescriptor {
	return &EventDescriptor{
		ID:      0,
		Version: 0,
		Channel: ChannelTracelogging,
		Level:   LevelVerbose,
		Opcode:  0,
		Task:    0,
		Keyword: 0,
	}
}

func NewEvent(name string, descriptor *EventDescriptor) *Event {
	return &Event{
		Descriptor: descriptor,
		Metadata:   NewEventMetadata(name),
		Data:       &EventData{},
	}
}
