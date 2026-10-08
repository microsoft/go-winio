//go:build windows

package winio

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// buildMountPointBuffer builds a raw REPARSE_DATA_BUFFER for a mount point.
func buildMountPointBuffer(substituteName, printName string) []byte {
	subUTF16 := utf16.Encode([]rune(substituteName + "\x00"))
	printUTF16 := utf16.Encode([]rune(printName + "\x00"))

	subBytes := len(subUTF16) * 2
	printBytes := len(printUTF16) * 2

	// Length fields exclude the trailing NUL.
	subNameLen := uint16((len(subUTF16) - 1) * 2)
	printNameLen := uint16((len(printUTF16) - 1) * 2)

	if printName == "" {
		printUTF16 = nil
		printBytes = 0
		printNameLen = 0
	}

	pathBufSize := subBytes + printBytes
	dataLen := 8 + pathBufSize

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint32(reparseTagMountPoint))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(dataLen))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0))
	_ = binary.Write(&buf, binary.LittleEndian, subNameLen)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(subBytes))
	_ = binary.Write(&buf, binary.LittleEndian, printNameLen)
	_ = binary.Write(&buf, binary.LittleEndian, subUTF16)
	if printUTF16 != nil {
		_ = binary.Write(&buf, binary.LittleEndian, printUTF16)
	}

	return buf.Bytes()
}

func TestDecodeJunctionEmptyPrintName(t *testing.T) {
	raw := buildMountPointBuffer(`\??\C:\test\tgt`, "")

	rp, err := DecodeReparsePoint(raw)
	if err != nil {
		t.Fatalf("decode reparse point failed: %v", err)
	}
	if rp.Target != `C:\test\tgt` {
		t.Errorf("target = %q, want %q", rp.Target, `C:\test\tgt`)
	}
	if !rp.IsMountPoint {
		t.Error("ismountpoint should be true")
	}
}

func TestDecodeJunctionBothNames(t *testing.T) {
	raw := buildMountPointBuffer(`\??\C:\test\tgt`, `C:\test\tgt`)

	rp, err := DecodeReparsePoint(raw)
	if err != nil {
		t.Fatalf("decode reparse point failed: %v", err)
	}
	if rp.Target != `C:\test\tgt` {
		t.Errorf("target = %q, want %q", rp.Target, `C:\test\tgt`)
	}
	if !rp.IsMountPoint {
		t.Error("ismountpoint should be true")
	}
}

func TestDecodeJunctionUNCPath(t *testing.T) {
	raw := buildMountPointBuffer(`\??\UNC\server\share`, `\\server\share`)

	rp, err := DecodeReparsePoint(raw)
	if err != nil {
		t.Fatalf("decode reparse point failed: %v", err)
	}
	if rp.Target != `\\server\share` {
		t.Errorf("target = %q, want %q", rp.Target, `\\server\share`)
	}
}

func TestDecodeJunctionVolumeGUIDPath(t *testing.T) {
	raw := buildMountPointBuffer(`\??\Volume{00000000-0000-0000-0000-000000000000}\test`, `\\?\Volume{00000000-0000-0000-0000-000000000000}\test`)

	rp, err := DecodeReparsePoint(raw)
	if err != nil {
		t.Fatalf("decode reparse point failed: %v", err)
	}
	if rp.Target != `\\?\Volume{00000000-0000-0000-0000-000000000000}\test` {
		t.Errorf("target = %q, want %q", rp.Target, `\\?\Volume{00000000-0000-0000-0000-000000000000}\test`)
	}
}

func TestRoundTripMountPointUNC(t *testing.T) {
	original := &ReparsePoint{
		Target:       `\\server\share`,
		IsMountPoint: true,
	}

	encoded := EncodeReparsePoint(original)
	decoded, err := DecodeReparsePoint(encoded)
	if err != nil {
		t.Fatalf("decode reparse point failed: %v", err)
	}
	if decoded.Target != original.Target {
		t.Errorf("target = %q, want %q", decoded.Target, original.Target)
	}
	if !decoded.IsMountPoint {
		t.Error("ismountpoint should be true")
	}
}

func TestRoundTripMountPoint(t *testing.T) {
	original := &ReparsePoint{
		Target:       `C:\test\tgt`,
		IsMountPoint: true,
	}

	encoded := EncodeReparsePoint(original)
	decoded, err := DecodeReparsePoint(encoded)
	if err != nil {
		t.Fatalf("decode reparse point failed: %v", err)
	}
	if decoded.Target != original.Target {
		t.Errorf("target = %q, want %q", decoded.Target, original.Target)
	}
	if !decoded.IsMountPoint {
		t.Error("ismountpoint should be true")
	}
}

func TestRoundTripSymlink(t *testing.T) {
	original := &ReparsePoint{
		Target:       `C:\test\tgt`,
		IsMountPoint: false,
	}

	encoded := EncodeReparsePoint(original)
	decoded, err := DecodeReparsePoint(encoded)
	if err != nil {
		t.Fatalf("decode reparse point failed: %v", err)
	}
	if decoded.Target != original.Target {
		t.Errorf("target = %q, want %q", decoded.Target, original.Target)
	}
	if decoded.IsMountPoint {
		t.Error("ismountpoint should be false")
	}
}

func TestRoundTripSymlinkVolumeGUIDPath(t *testing.T) {
	original := &ReparsePoint{
		Target:       `\\?\Volume{00000000-0000-0000-0000-000000000000}\test`,
		IsMountPoint: false,
	}

	encoded := EncodeReparsePoint(original)
	decoded, err := DecodeReparsePoint(encoded)
	if err != nil {
		t.Fatalf("decode reparse point failed: %v", err)
	}
	if decoded.Target != original.Target {
		t.Errorf("target = %q, want %q", decoded.Target, original.Target)
	}
	if decoded.IsMountPoint {
		t.Error("ismountpoint should be false")
	}
}
