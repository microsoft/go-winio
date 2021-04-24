// +build windows

package backuptar

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Microsoft/go-winio"
)

func ensurePresent(t *testing.T, m map[string]string, keys ...string) {
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Error(k, "not present in tar header")
		}
	}
}

func TestTarFileFromBackupStream(t *testing.T) {
	f, err := ioutil.TempFile("", "tst")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	if _, err = f.Write([]byte("testing 1 2 3\n")); err != nil {
		t.Fatal(err)
	}

	if _, err = f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

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
}

func TestBackupStreamFromTar(t *testing.T) {
	f, err := ioutil.TempFile("", "tst")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	expectedContent := "testing 1 2 3\n"
	if _, err = f.Write([]byte(expectedContent)); err != nil {
		t.Fatal(err)
	}

	if _, err = f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

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

	tarContentReader := tar.NewReader(&buf)
	hdr2, err := tarContentReader.Next()
	if err != nil {
		t.Fatal(err)
	}
	var backupStreamBuf bytes.Buffer
	_, err = WriteBackupStreamFromTarFile(&backupStreamBuf, tarContentReader, hdr2)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	bsr := winio.NewBackupStreamReader(&backupStreamBuf)

	// read the first header that has security descriptor
	_, err = bsr.Next()
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	// read header for contents
	bhdr, err := bsr.Next()
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	resultBuf := make([]byte, int(bhdr.Size))
	written, err := bsr.Read(resultBuf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if int64(written) != bhdr.Size {
		t.Fatal("unexpected size of read bytes for backup stream")
	}

	if expectedContent != string(resultBuf) {
		t.Fatalf("expected to read \"%v\" instead got \"%v\"", expectedContent, string(resultBuf))
	}
}
