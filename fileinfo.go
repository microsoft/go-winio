package winio

import (
	"syscall"
	"unsafe"
)

//sys getFileInformationByHandleEx(h syscall.Handle, class uint32, buffer *byte, size uint32) (err error) = GetFileInformationByHandleEx
//sys setFileInformationByHandle(h syscall.Handle, class uint32, buffer *byte, size uint32) (err error) = SetFileInformationByHandle

type FileBasicInfo struct {
	CreationTime, LastAccessTime, LastWriteTime, ChangeTime syscall.Filetime
	FileAttributes                                          uintptr // includes padding
}

func GetFileBasicInfo(h syscall.Handle) (*FileBasicInfo, error) {
	bi := &FileBasicInfo{}
	if err := getFileInformationByHandleEx(h, 0, (*byte)(unsafe.Pointer(bi)), uint32(unsafe.Sizeof(bi))); err != nil {
		return nil, err
	}
	return bi, nil
}

func SetFileBasicInfo(h syscall.Handle, bi *FileBasicInfo) error {
	return setFileInformationByHandle(h, 0, (*byte)(unsafe.Pointer(bi)), uint32(unsafe.Sizeof(bi)))
}
