//go:build windows
// +build windows

package winio

import (
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	testEas = []ExtendedAttribute{
		{Name: "foo", Value: []byte("bar")},
		{Name: "fizz", Value: []byte("buzz")},
	}

	testEasEncoded   = []byte{16, 0, 0, 0, 0, 3, 3, 0, 102, 111, 111, 0, 98, 97, 114, 0, 0, 0, 0, 0, 0, 4, 4, 0, 102, 105, 122, 122, 0, 98, 117, 122, 122, 0, 0, 0}
	testEasNotPadded = testEasEncoded[0 : len(testEasEncoded)-3]
	testEasTruncated = testEasEncoded[0:20]
)

func init() {
	rand.Seed(time.Now().Unix())
}

func Test_RoundTripEas(t *testing.T) {
	b, err := EncodeExtendedAttributes(testEas)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(testEasEncoded, b) {
		t.Fatalf("encoded mismatch %v %v", testEasEncoded, b)
	}
	eas, err := DecodeExtendedAttributes(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(testEas, eas) {
		t.Fatalf("mismatch %+v %+v", testEas, eas)
	}
}

func Test_EasDontNeedPaddingAtEnd(t *testing.T) {
	eas, err := DecodeExtendedAttributes(testEasNotPadded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(testEas, eas) {
		t.Fatalf("mismatch %+v %+v", testEas, eas)
	}
}

func Test_TruncatedEasFailCorrectly(t *testing.T) {
	_, err := DecodeExtendedAttributes(testEasTruncated)
	if err == nil {
		t.Fatal("expected error")
	}
}

func Test_NilEasEncodeAndDecodeAsNil(t *testing.T) {
	b, err := EncodeExtendedAttributes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 0 {
		t.Fatal("expected empty")
	}
	eas, err := DecodeExtendedAttributes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(eas) != 0 {
		t.Fatal("expected empty")
	}
}

// Test_SetFileEa makes sure that the test buffer is actually parsable by NtSetEaFile.
func Test_SetFileEa(t *testing.T) {
	f, err := ioutil.TempFile("", "winio")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	ntdll := syscall.MustLoadDLL("ntdll.dll")
	ntSetEaFile := ntdll.MustFindProc("NtSetEaFile")
	var iosb [2]uintptr
	r, _, _ := ntSetEaFile.Call(f.Fd(), uintptr(unsafe.Pointer(&iosb[0])), uintptr(unsafe.Pointer(&testEasEncoded[0])), uintptr(len(testEasEncoded)))
	if r != 0 {
		t.Fatalf("NtSetEaFile failed with %08x", r)
	}
}

func Test_SetGetFileEA(t *testing.T) {
	tempDir := t.TempDir()
	testfilePath := filepath.Join(tempDir, "testfile.txt")
	// create temp file
	testfile, err := os.Create(testfilePath)
	if err != nil {
		t.Fatalf("failed to create temporary file: %s", err)
	}
	testfile.Close()

	nAttrs := 3
	testEAs := make([]ExtendedAttribute, 3)
	// generate random extended attributes for test
	for i := 0; i < nAttrs; i++ {
		// EA name is automatically converted to upper case before storing, so
		// when reading it back it returns the upper case name. To avoid test
		// failures because of that keep the name upper cased.
		testEAs[i].Name = fmt.Sprintf("TESTEA%d", i+1)
		testEAs[i].Value = make([]byte, rand.Int31n(math.MaxUint8))
		rand.Read(testEAs[i].Value)
	}

	utf16Path := windows.StringToUTF16Ptr(testfilePath)
	fileAccessRightReadWriteEA := (0x8 | 0x10)
	fileHandle, err := windows.CreateFile(utf16Path, uint32(fileAccessRightReadWriteEA), 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("open file failed with: %s", err)
	}
	defer windows.Close(fileHandle)

	if err := SetFileEA(fileHandle, testEAs); err != nil {
		t.Fatalf("set EA for file failed: %s", err)
	}

	var readEAs []ExtendedAttribute
	if readEAs, err = GetFileEA(fileHandle); err != nil {
		t.Fatalf("get EA for file failed: %s", err)
	}

	if !reflect.DeepEqual(readEAs, testEAs) {
		t.Logf("expected: %+v, found: %+v\n", testEAs, readEAs)
		t.Fatalf("EAs read from testfile don't match")
	}
}
