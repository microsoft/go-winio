package etw

// FieldOpt defines the option function type that can be passed to
// Provider.WriteEvent to add fields to the event.
type FieldOpt func(em *EventMetadata, ed *EventData)

// StringField adds a single string field to the event.
func StringField(name string, value string) FieldOpt {
	return func(em *EventMetadata, ed *EventData) {
		em.WriteField(name, InTypeANSIString, OutTypeUTF8, 0)
		ed.WriteString(value)
	}
}

// StringArray adds an array of strings to the event.
func StringArray(name string, values []string) FieldOpt {
	return func(em *EventMetadata, ed *EventData) {
		em.WriteArray(name, InTypeANSIString, OutTypeUTF8, 0)
		ed.WriteUint16(uint16(len(values)))
		for _, v := range values {
			ed.WriteString(v)
		}
	}
}

// Struct adds a nested struct to the event, the FieldOpts in the opts argument
// are used to specify the fields of the struct.
func Struct(name string, opts ...FieldOpt) FieldOpt {
	return func(em *EventMetadata, ed *EventData) {
		em.WriteStruct(name, uint8(len(opts)), 0)
		for _, opt := range opts {
			opt(em, ed)
		}
	}
}
