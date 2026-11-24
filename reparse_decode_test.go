//go:build windows

package winio

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestDecodeReparsePointMalformed(t *testing.T) {
	symlink := EncodeReparsePoint(&ReparsePoint{Target: `C:\foo`})
	mountPoint := EncodeReparsePoint(&ReparsePoint{Target: `C:\foo`, IsMountPoint: true})

	// A print name offset near the uint16 limit must not wrap around to the
	// start of the buffer.
	wrapped := append([]byte(nil), mountPoint...)
	binary.LittleEndian.PutUint16(wrapped[12:14], 0xffff)

	tests := []struct {
		name string
		b    []byte
	}{
		{"empty", nil},
		{"tag only", symlink[:4]},
		{"header only", symlink[:8]},
		{"symlink missing flags", symlink[:16]},
		{"symlink truncated name", symlink[:len(symlink)-6]},
		{"mount point truncated name", mountPoint[:len(mountPoint)-6]},
		{"wrapped offset", wrapped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp, err := DecodeReparsePoint(tt.b)
			if !errors.Is(err, errInvalidReparseBuffer) {
				t.Fatalf("DecodeReparsePoint() = %+v, %v; want error %v", rp, err, errInvalidReparseBuffer)
			}
		})
	}
}
