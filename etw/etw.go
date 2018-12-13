package etw

//go:generate go run $GOROOT/src/syscall/mksyscall_windows.go -output zsyscall_windows.go etw.go

//sys eventRegister(providerId *windows.GUID, callback uintptr, callbackContext uintptr, providerHandle *providerHandle) (win32err error) = advapi32.EventRegister
//sys eventUnregister(providerHandle providerHandle) (win32err error) = advapi32.EventUnregister
//sys eventWriteTransfer(providerHandle providerHandle, descriptor *EventDescriptor, activityID *windows.GUID, relatedActivityID *windows.GUID, dataDescriptorCount uint32, dataDescriptors *eventDataDescriptor) (win32err error) = advapi32.EventWriteTransfer
