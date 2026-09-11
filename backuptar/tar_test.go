//go:build windows

package backuptar

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func ensurePresent(t *testing.T, m map[string]string, keys ...string) {
	t.Helper()

	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Error(k, "not present in tar header")
		}
	}
}

func setSparse(t *testing.T, f *os.File) {
	t.Helper()

	if err := windows.DeviceIoControl(windows.Handle(f.Fd()), windows.FSCTL_SET_SPARSE, nil, 0, nil, 0, nil, nil); err != nil {
		t.Fatal(err)
	}
}

// compareReaders validates that two readers contain the exact same data.
func compareReaders(t *testing.T, rActual io.Reader, rExpected io.Reader) {
	t.Helper()

	const size = 8 * 1024
	var bufExpected, bufActual [size]byte
	var readCount int64
	// Loop, first reading from rExpected, then reading the same amount from rActual.
	// For each set of reads, compare the bytes to make sure they are identical.
	// When we run out of data in rExpected, exit the loop.
	for {
		// Do a read from rExpected and see how many bytes we get.
		nExpected, err := rExpected.Read(bufExpected[:])
		if err == io.EOF && nExpected == 0 {
			break
		} else if err != nil && err != io.EOF {
			t.Fatalf("Failed reading from rExpected at %d: %s", readCount, err)
		}
		// Do a ReadFull from rActual for the same number of bytes we got from rExpected.
		if nActual, err := io.ReadFull(rActual, bufActual[:nExpected]); err != nil {
			t.Fatalf("Only read %d bytes out of %d from rActual at %d: %s", nActual, nExpected, readCount, err)
		}
		readCount += int64(nExpected)
		for i, bExpected := range bufExpected[:nExpected] {
			if bExpected != bufActual[i] {
				t.Fatalf("Mismatched bytes at %d. got 0x%x, expected 0x%x", i, bufActual[i], bExpected)
			}
		}
	}
	// Now we just need to make sure there isn't any further data in rActual.
	var b [1]byte
	if n, err := rActual.Read(b[:]); n != 0 || err != io.EOF {
		t.Fatalf("rActual didn't return EOF at expected end. Read %d bytes with error %s", n, err)
	}
}

func readBackupStream(t *testing.T, f *os.File) []byte {
	t.Helper()
	br := winio.NewBackupFileReader(f, true)
	defer br.Close()
	buf, err := io.ReadAll(br)
	if err != nil {
		t.Fatalf("reading backup stream: %s", err)
	}
	return buf
}

func buildBackupStream(t *testing.T, entries []struct {
	hdr  winio.BackupHeader
	data []byte
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	bw := winio.NewBackupStreamWriter(&buf)
	for _, e := range entries {
		hdr := e.hdr
		if err := bw.WriteHeader(&hdr); err != nil {
			t.Fatalf("WriteHeader(id=%d): %v", hdr.Id, err)
		}
		if len(e.data) > 0 {
			if _, err := bw.Write(e.data); err != nil {
				t.Fatalf("Write(id=%d): %v", hdr.Id, err)
			}
		}
	}
	return buf.Bytes()
}

func TestRoundTrip(t *testing.T) {
	// Each test case is a name mapped to a function which must create a file and return its path.
	// The test then round-trips that file through backuptar, and validates the output matches the input.
	//
	//nolint:gosec // G306: Expect WriteFile permissions to be 0600 or less
	for name, setup := range map[string]func(*testing.T) string{
		"normalFile": func(t *testing.T) string {
			t.Helper()

			path := filepath.Join(t.TempDir(), "foo.txt")
			if err := os.WriteFile(path, []byte("testing 1 2 3\n"), 0644); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"normalFileEmpty": func(t *testing.T) string {
			t.Helper()

			path := filepath.Join(t.TempDir(), "foo.txt")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			return path
		},
		"sparseFileEmpty": func(t *testing.T) string {
			t.Helper()

			path := filepath.Join(t.TempDir(), "foo.txt")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			return path
		},
		"sparseFileWithNoAllocatedRanges": func(t *testing.T) string {
			t.Helper()

			path := filepath.Join(t.TempDir(), "foo.txt")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			// Set file size without writing data to produce a file with size > 0
			// but no allocated ranges.
			if err := f.Truncate(1000000); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"sparseFileWithOneAllocatedRange": func(t *testing.T) string {
			t.Helper()

			path := filepath.Join(t.TempDir(), "foo.txt")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			if _, err := f.WriteString("test sparse data"); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"sparseFileWithMultipleAllocatedRanges": func(t *testing.T) string {
			t.Helper()

			path := filepath.Join(t.TempDir(), "foo.txt")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			if _, err = f.Write([]byte("testing 1 2 3\n")); err != nil {
				t.Fatal(err)
			}
			// The documentation talks about FSCTL_SET_ZERO_DATA, but seeking also
			// seems to create a hole.
			if _, err = f.Seek(1000000, 0); err != nil {
				t.Fatal(err)
			}
			if _, err = f.Write([]byte("more data later\n")); err != nil {
				t.Fatal(err)
			}
			return path
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := setup(t)
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			fi, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			bi, err := winio.GetFileBasicInfo(f)
			if err != nil {
				t.Fatal(err)
			}

			br := winio.NewBackupFileReader(f, true)
			defer br.Close()
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			err = WriteTarFileFromBackupStream(tw, br, f.Name(), fi.Size(), bi)
			if err != nil {
				t.Fatal(err)
			}
			tr := tar.NewReader(&buf)
			hdr, err := tr.Next()
			if err != nil {
				t.Fatal(err)
			}

			name, size, bi2, err := FileInfoFromHeader(hdr)
			if err != nil {
				t.Fatal(err)
			}
			if name != filepath.ToSlash(f.Name()) {
				t.Errorf("got name %s, expected %s", name, filepath.ToSlash(f.Name()))
			}
			if size != fi.Size() {
				t.Errorf("got size %d, expected %d", size, fi.Size())
			}
			if !reflect.DeepEqual(*bi2, *bi) {
				t.Errorf("got %#v, expected %#v", *bi2, *bi)
			}
			ensurePresent(t, hdr.PAXRecords, "MSWINDOWS.fileattr", "MSWINDOWS.rawsd")
			// Reset file position so we can compare file contents.
			// The file contents of the actual file should match what we get from the tar.
			if _, err := f.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			compareReaders(t, tr, f)
		})
	}
}

// TestRoundTripSeekable covers the two-pass sparse export regression.
// TestRoundTrip uses a non-seekable BackupFileReader and only covers the single-pass path.
func TestRoundTripSeekable(t *testing.T) {
	//nolint:gosec // G306: test files do not need restrictive permissions
	for name, setup := range map[string]func(*testing.T) string{
		"normalFile": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo.txt")
			if err := os.WriteFile(path, []byte("testing 1 2 3\n"), 0644); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"normalFileEmpty": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo.txt")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			return path
		},
		"sparseFileEmpty": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo.txt")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			return path
		},
		"sparseFileAllHoles": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo.txt")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			// Grow the logical size without allocating data.
			if err := f.Truncate(1048576); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"sparseFileOneRange": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo.txt")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			if _, err := f.WriteString("test sparse data"); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"sparseFileMultipleRanges": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo.txt")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			if _, err = f.Write([]byte("leading data\n")); err != nil {
				t.Fatal(err)
			}
			if _, err = f.Seek(1048576, 0); err != nil {
				t.Fatal(err)
			}
			if _, err = f.Write([]byte("trailing data\n")); err != nil {
				t.Fatal(err)
			}
			return path
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := setup(t)
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			fi, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			bi, err := winio.GetFileBasicInfo(f)
			if err != nil {
				t.Fatal(err)
			}

			// bytes.Reader implements io.Seeker, selecting the two-pass path.
			streamBytes := readBackupStream(t, f)
			seekable := bytes.NewReader(streamBytes)
			if _, ok := io.Reader(seekable).(io.Seeker); !ok {
				t.Fatal("test reader must implement io.Seeker to exercise the two-pass path")
			}

			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			if err := WriteTarFileFromBackupStream(tw, seekable, f.Name(), fi.Size(), bi); err != nil {
				t.Fatalf("WriteTarFileFromBackupStream (seekable): %s", err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}

			tr := tar.NewReader(&buf)
			hdr, err := tr.Next()
			if err != nil {
				t.Fatal(err)
			}
			_, size, _, err := FileInfoFromHeader(hdr)
			if err != nil {
				t.Fatal(err)
			}
			if size != fi.Size() {
				t.Errorf("got size %d, expected %d", size, fi.Size())
			}
			if _, err := f.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			compareReaders(t, tr, f)
		})
	}
}

// Build an empty sparse stream with a trailing terminator independently of the Windows build.
func TestWriteTarZeroLengthSparseTerminator(t *testing.T) {
	stream := buildBackupStream(t, []struct {
		hdr  winio.BackupHeader
		data []byte
	}{
		{hdr: winio.BackupHeader{Id: winio.BackupData, Attributes: winio.StreamSparseAttributes, Size: 0}},
		{hdr: winio.BackupHeader{Id: winio.BackupSparseBlock, Attributes: winio.StreamSparseAttributes, Size: 0, Offset: 0}},
	})

	// Use a seekable reader to exercise the two-pass path.
	seekable := bytes.NewReader(stream)

	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	bi := &winio.FileBasicInfo{}
	if err := WriteTarFileFromBackupStream(tw, seekable, "Files/Windows/Temp/DOBDAB.tmp", 0, bi); err != nil {
		if strings.Contains(err.Error(), "unknown stream ID 9") {
			t.Fatalf("sparse stream regression reproduced: %v", err)
		}
		t.Fatalf("WriteTarFileFromBackupStream: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	tr := tar.NewReader(&out)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	_, size, _, err := FileInfoFromHeader(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Errorf("got size %d, want 0 for zero-length sparse file", size)
	}
	compareReaders(t, tr, bytes.NewReader(nil))
}

func TestZeroReader(t *testing.T) {
	const size = 512
	var b [size]byte
	var bExpected [size]byte
	var r zeroReader
	n, err := r.Read(b[:])
	if err != nil {
		t.Fatalf("Unexpected read error: %s", err)
	}
	if n != size {
		t.Errorf("Wrong read size. got %d, expected %d", n, size)
	}
	for i := range b {
		if b[i] != bExpected[i] {
			t.Errorf("Wrong content at index %d. got %d, expected %d", i, b[i], bExpected[i])
		}
	}
}
