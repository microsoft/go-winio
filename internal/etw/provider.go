package etw

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"strings"
	"sync"
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
	ID         *windows.GUID
	handle     providerHandle
	metadata   []byte
	callback   EnableCallback
	index      uint
	enabled    bool
	level      Level
	keywordAny uint64
	keywordAll uint64
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

func (descriptor *eventDataDescriptor) set(dataType eventDataDescriptorType, buffer []byte) {
	// Passing a pointer to Go-managed memory as part of a block of memory is
	// risky since the GC doesn't know about it. If we find a better way to do
	// this we should use it instead.
	descriptor.ptr = uint64(uintptr(unsafe.Pointer(&buffer[0])))
	descriptor.size = uint32(len(buffer))
	descriptor.dataType = dataType
}

// Because the provider callback function needs to be able to access the
// provider data when it is invoked by ETW, we need to keep provider data stored
// in a global map based on an index. The index is passed as the callback
// context to ETW.
type providerMap struct {
	m    map[uint]*Provider
	i    uint
	lock sync.Mutex
}

var providers = providerMap{
	m: make(map[uint]*Provider),
}

func (p *providerMap) newProvider() *Provider {
	p.lock.Lock()
	defer p.lock.Unlock()

	i := p.i
	p.i++

	provider := &Provider{
		index: i,
	}

	p.m[i] = provider
	return provider
}

func (p *providerMap) removeProvider(provider *Provider) {
	p.lock.Lock()
	defer p.lock.Unlock()

	delete(p.m, provider.index)
}

func (p *providerMap) getProvider(index uint) *Provider {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.m[index]
}

func providerCallback(sourceID *windows.GUID, state ProviderState, level Level, matchAnyKeyword uint64, matchAllKeyword uint64, filterData uintptr, i uintptr) {
	provider := providers.getProvider(uint(i))

	switch state {
	case ProviderStateDisable:
		provider.enabled = false
	case ProviderStateEnable:
		provider.enabled = true
		provider.level = level
		provider.keywordAny = matchAnyKeyword
		provider.keywordAll = matchAllKeyword
	}

	if provider.callback != nil {
		provider.callback(sourceID, state, level, matchAnyKeyword, matchAllKeyword, filterData)
	}
}

// providerCallbackAdapter acts as the first-level callback from the C/ETW side
// for provider notifications. Because Go has trouble with callback arguments of
// different size, it has only pointer-sized arguments, which are then cast to
// the appropriate types when calling providerCallback.
func providerCallbackAdapter(sourceID *windows.GUID, state uintptr, level uintptr, matchAnyKeyword uintptr, matchAllKeyword uintptr, filterData uintptr, i uintptr) uintptr {
	providerCallback(sourceID, ProviderState(state), Level(level), uint64(matchAnyKeyword), uint64(matchAllKeyword), filterData, i)
	return 0
}

// providerIDFromName generates a provider ID based on the provider name. It
// uses the same algorithm as used by .NET's EventSource class, which is based
// on RFC 4122. More information on the algorithm can be found here:
// https://blogs.msdn.microsoft.com/dcook/2015/09/08/etw-provider-names-and-guids/
// The algorithm is roughly:
// Hash = Sha1(namespace + arg.ToUpper().ToUtf16be())
// Guid = Hash[0..15], with Hash[7] tweaked according to RFC 4122
func providerIDFromName(name string) (*windows.GUID, error) {
	namespace := []byte{0x48, 0x2C, 0x2D, 0xB2, 0xC3, 0x90, 0x47, 0xC8, 0x87, 0xF8, 0x1A, 0x15, 0xBF, 0xC1, 0x30, 0xFB}
	buffer := &bytes.Buffer{}
	buffer.Write(namespace)

	nameUTF16, err := windows.UTF16FromString(strings.ToUpper(name))
	if err != nil {
		return nil, err
	}
	// nameUTF16 includes a null terminator, which we don't want included in the
	// hash.
	binary.Write(buffer, binary.BigEndian, nameUTF16[:len(nameUTF16)-1])

	sum := sha1.Sum(buffer.Bytes())
	sum[7] = (sum[7] & 0xf) | 0x50

	return &windows.GUID{
		Data1: (uint32(sum[3]) << 24) | (uint32(sum[2]) << 16) | (uint32(sum[1]) << 8) | uint32(sum[0]),
		Data2: (uint16(sum[5]) << 8) | uint16(sum[4]),
		Data3: (uint16(sum[7]) << 8) | uint16(sum[6]),
		Data4: [8]byte{sum[8], sum[9], sum[10], sum[11], sum[12], sum[13], sum[14], sum[15]},
	}, nil
}

func NewProvider(name string, callback EnableCallback) (provider *Provider, err error) {
	id, err := providerIDFromName(name)
	if err != nil {
		return nil, err
	}
	return NewProviderWithID(name, id, callback)
}

// NewProvider creates and registers a new provider.
func NewProviderWithID(name string, id *windows.GUID, callback EnableCallback) (provider *Provider, err error) {
	provider = providers.newProvider()
	defer func() {
		if err != nil {
			providers.removeProvider(provider)
		}
	}()
	provider.ID = id
	provider.callback = callback

	if err := eventRegister(provider.ID, windows.NewCallback(providerCallbackAdapter), uintptr(provider.index), &provider.handle); err != nil {
		return nil, err
	}

	metadata := &bytes.Buffer{}
	binary.Write(metadata, binary.LittleEndian, uint16(0)) // Write empty size for buffer (to update later)
	metadata.WriteString(name)
	metadata.WriteByte(0)                                                   // Null terminator for name
	binary.LittleEndian.PutUint16(metadata.Bytes(), uint16(metadata.Len())) // Update the size at the beginning of the buffer
	provider.metadata = metadata.Bytes()

	return provider, nil
}

// Close unregisters the provider.
func (provider *Provider) Close() error {
	providers.removeProvider(provider)
	return eventUnregister(provider.handle)
}

// IsEnabled calls IsEnabledForLevelAndKeywords with LevelAlways and all
// keywords set.
func (provider *Provider) IsEnabled() bool {
	return provider.IsEnabledForLevelAndKeywords(LevelAlways, ^uint64(0))
}

// IsEnabledForLevel calls IsEnabledForLevelAndKeywords with the specified level
// and all keywords set.
func (provider *Provider) IsEnabledForLevel(level Level) bool {
	return provider.IsEnabledForLevelAndKeywords(level, ^uint64(0))
}

// IsEnabledForLevelAndKeywords allows event producer code to check if there are
// any event sessions that are interested in an event, based on the event level
// and keywords. Although this check happens automatically in the ETW
// infrastructure, it can be useful to check if an event will actually be
// consumed before doing expensive work to build the event data.
func (provider *Provider) IsEnabledForLevelAndKeywords(level Level, keywords uint64) bool {
	if !provider.enabled {
		return false
	}

	// ETW automatically sets the level to 255 if it is specified as 0, so we
	// don't need to worry about the level=0 (all events) case.
	if level > provider.level {
		return false
	}

	if keywords != 0 && (keywords&provider.keywordAny == 0 || keywords&provider.keywordAll != provider.keywordAll) {
		return false
	}

	return true
}

// WriteEvent writes a single event to ETW, from this provider.
func (provider *Provider) WriteEvent(event *Event) error {
	// Finalize the event metadata buffer by filling in the buffer length at the
	// beginning.
	binary.LittleEndian.PutUint16(event.Metadata.buffer.Bytes(), uint16(event.Metadata.buffer.Len()))

	var dataDescriptors [3]eventDataDescriptor
	dataDescriptors[0].set(eventDataDescriptorTypeProviderMetadata, provider.metadata)
	dataDescriptors[1].set(eventDataDescriptorTypeEventMetadata, event.Metadata.buffer.Bytes())
	dataDescriptors[2].set(eventDataDescriptorTypeUserData, event.Data.buffer.Bytes())

	return eventWriteTransfer(provider.handle, event.Descriptor, nil, nil, 3, &dataDescriptors[0])
}
