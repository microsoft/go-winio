package etw

import (
	"bytes"
	"encoding/binary"
	"unsafe"

	"golang.org/x/sys/windows"
)

type eventDataDescriptorType uint8

const (
	eventDataDescriptorTypeUserData eventDataDescriptorType = iota
	eventDataDescriptorTypeEventMetadata
	eventDataDescriptorTypeProviderMetadata
)

// Provider represents an ETW event provider. It is identified by a provider
// name and ID (GUID), which should always have a 1:1 mapping to each other
// (e.g. don't use multiple provider names with the same ID, or vice versa).
type Provider struct {
	handle   providerHandle
	metadata *bytes.Buffer
}

type providerHandle windows.Handle

// ProviderState informs the provider EnableCallback what action is being
// performed.
type ProviderState uint32

const (
	// ProviderStateDisable indicates the provider is being disabled.
	ProviderStateDisable ProviderState = iota
	// ProviderStateEnable indicates the provider is being enabled.
	ProviderStateEnable
	// ProviderStateCaptureState indicates the provider is having its current
	// state snap-shotted.
	ProviderStateCaptureState
)

// EnableCallback is the form of the callback function that receives provider
// enable/disable notifications from ETW.
type EnableCallback func(*windows.GUID, ProviderState, Level, uint64, uint64, uintptr)

type eventDataDescriptor struct {
	ptr       uint64
	size      uint32
	dataType  eventDataDescriptorType
	reserved1 uint8
	reserved2 uint16
}

func (descriptor *eventDataDescriptor) set(dataType eventDataDescriptorType, buffer *bytes.Buffer) {
	// Passing a pointer to Go-managed memory as part of a block of memory is
	// risky since the GC doesn't know about it. If we find a better way to do
	// this we should use it instead.
	descriptor.ptr = uint64(uintptr(unsafe.Pointer(&buffer.Bytes()[0])))
	descriptor.size = uint32(buffer.Len())
	descriptor.dataType = dataType
}

// NewProvider creates and registers a new provider.
func NewProvider(name string, id *windows.GUID, callback EnableCallback) (*Provider, error) {
	innerCallback := func(sourceID *windows.GUID, state ProviderState, level Level, matchAnyKeyword uint64, matchAllKeyword uint64, filterData uintptr, _ uintptr) uintptr {
		if callback != nil {
			callback(sourceID, state, level, matchAnyKeyword, matchAllKeyword, filterData)
		}
		return 0
	}

	var providerHandle providerHandle
	if err := eventRegister(id, windows.NewCallback(innerCallback), 0, &providerHandle); err != nil {
		return nil, err
	}

	var metadataBuffer bytes.Buffer
	binary.Write(&metadataBuffer, binary.LittleEndian, uint16(0))                       // Write empty size for buffer (to update later)
	binary.Write(&metadataBuffer, binary.LittleEndian, []byte(name))                    // Provider name
	binary.Write(&metadataBuffer, binary.LittleEndian, byte(0))                         // Null terminator for name
	binary.LittleEndian.PutUint16(metadataBuffer.Bytes(), uint16(metadataBuffer.Len())) // Update the size at the beginning of the buffer

	return &Provider{
		handle:   providerHandle,
		metadata: &metadataBuffer,
	}, nil
}

// Close unregisters the provider.
func (provider *Provider) Close() error {
	return eventUnregister(provider.handle)
}

// WriteEvent writes a single event to ETW, from this provider.
func (provider *Provider) WriteEvent(event *Event) error {
	// Finalize the event metadata buffer by filling in the buffer length at the
	// beginning.
	binary.LittleEndian.PutUint16(event.Metadata.buffer.Bytes(), uint16(event.Metadata.buffer.Len()))

	var dataDescriptors [3]eventDataDescriptor
	dataDescriptors[0].set(eventDataDescriptorTypeProviderMetadata, provider.metadata)
	dataDescriptors[1].set(eventDataDescriptorTypeEventMetadata, &event.Metadata.buffer)
	dataDescriptors[2].set(eventDataDescriptorTypeUserData, &event.Data.buffer)

	return eventWriteTransfer(provider.handle, event.Descriptor, nil, nil, 3, &dataDescriptors[0])
}
