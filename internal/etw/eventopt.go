package etw

// EventOpt defines the option function type that can be passed to
// Provider.WriteEvent to specify general event options, such as level and
// keyword.
type EventOpt func(*EventDescriptor, *uint32)

// WithEventOpts returns the variadic arguments as a single slice.
func WithEventOpts(opts ...EventOpt) []EventOpt {
	return opts
}

// WithLevel specifies the level of the event to be written.
func WithLevel(level Level) EventOpt {
	return func(descriptor *EventDescriptor, tags *uint32) {
		descriptor.Level = level
	}
}

// WithKeyword specifies the keywords of the event to be written. Multiple uses
// of this option are OR'd together.
func WithKeyword(keyword uint64) EventOpt {
	return func(descriptor *EventDescriptor, tags *uint32) {
		descriptor.Keyword |= keyword
	}
}

// WithTags specifies the tags of the event to be written. Tags is a 28-bit
// value (top 4 bits are ignored) which are interpreted by the event consumer.
func WithTags(newTags uint32) EventOpt {
	return func(descriptor *EventDescriptor, tags *uint32) {
		*tags |= newTags
	}
}
