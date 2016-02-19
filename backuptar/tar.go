package backuptar

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio"
)

const (
	c_ISUID  = 04000   // Set uid
	c_ISGID  = 02000   // Set gid
	c_ISVTX  = 01000   // Save text (sticky bit)
	c_ISDIR  = 040000  // Directory
	c_ISFIFO = 010000  // FIFO
	c_ISREG  = 0100000 // Regular file
	c_ISLNK  = 0120000 // Symbolic link
	c_ISBLK  = 060000  // Block special file
	c_ISCHR  = 020000  // Character special file
	c_ISSOCK = 0140000 // Socket
)

const (
	xattrFileAttributes     = "MSWINDOWS.fileattr"
	xattrAccessTime         = "MSWINDOWS.accesstime"
	xattrChangeTime         = "MSWINDOWS.changetime"
	xattrCreateTime         = "MSWINDOWS.createtime"
	xattrWriteTime          = "MSWINDOWS.writetime"
	xattrSecurityDescriptor = "MSWINDOWS.sd"
	xattrMountPoint         = "MSWINDOWS.mountpoint"
)

func writeZeroes(w io.Writer, count int64) error {
	buf := make([]byte, 8192)
	c := len(buf)
	for i := int64(0); i < count; i += int64(c) {
		if int64(c) > count-i {
			c = int(count - i)
		}
		_, err := w.Write(buf[:c])
		if err != nil {
			return err
		}
	}
	return nil
}

func copySparse(t *tar.Writer, br *winio.BackupStreamReader) error {
	curOffset := int64(0)
	for {
		bhdr, err := br.Next()
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		if err != nil {
			return err
		}
		if bhdr.Id != winio.BackupSparseBlock {
			return fmt.Errorf("unexpected stream %d", bhdr.Id)
		}

		// archive/tar does not support writing sparse files
		// so just write zeroes to catch up to the current offset.
		err = writeZeroes(t, bhdr.Offset-curOffset)
		if bhdr.Size == 0 {
			break
		}
		n, err := io.Copy(t, br)
		if err != nil {
			return err
		}
		curOffset = bhdr.Offset + n
	}
	return nil
}

func win32TimeFromTar(key string, xattrs map[string]string, unixTime time.Time) syscall.Filetime {
	if s, ok := xattrs[key]; ok {
		n, err := strconv.ParseUint(s, 10, 64)
		if err == nil {
			return syscall.Filetime{uint32(n & 0xffffffff), uint32(n >> 32)}
		}
	}
	return syscall.NsecToFiletime(unixTime.UnixNano())
}

func win32TimeToTar(ft syscall.Filetime) (string, time.Time) {
	return fmt.Sprintf("%u", uint64(ft.LowDateTime)+(uint64(ft.HighDateTime)<<32)), time.Unix(0, ft.Nanoseconds())
}

// Writes a file to a tar writer using data from a Win32 backup stream.
//
// Currently this encodes Win32 metadata as tar xattrs (i.e. SCHILY.xattr.*), which is incorrect.
// This should be fixed archive/tar exposes the ability to write general pax headers.
//
// The additional Win32 metadata is:
//
// MSWINDOWS.fileattr: The Win32 file attributes, as a decimal value
//
// MSWINDOWS.accesstime: The last access time, as a Filetime expressed as a 64-bit decimal value.
//
// MSWINDOWS.createtime: The creation time, as a Filetime expressed as a 64-bit decimal value.
//
// MSWINDOWS.changetime: The creation time, as a Filetime expressed as a 64-bit decimal value.
//
// MSWINDOWS.writetime: The creation time, as a Filetime expressed as a 64-bit decimal value.
//
// MSWINDOWS.sd: The Win32 security descriptor, in SDDL (string) format
//
// MSWINDOWS.mountpoint: If present, this is a mount point and not a symlink, even though the type is '2' (symlink)
func WriteTarFileFromBackupStream(t *tar.Writer, r io.Reader, name string, size int64, fileInfo *winio.FileBasicInfo) error {
	name = filepath.ToSlash(name)
	hdr := &tar.Header{
		Name:     name,
		Size:     size,
		Typeflag: tar.TypeReg,
		Xattrs:   make(map[string]string),
	}
	hdr.Xattrs[xattrFileAttributes] = fmt.Sprintf("%d", fileInfo.FileAttributes)
	hdr.Xattrs[xattrAccessTime], hdr.AccessTime = win32TimeToTar(fileInfo.LastAccessTime)
	hdr.Xattrs[xattrChangeTime], hdr.ChangeTime = win32TimeToTar(fileInfo.ChangeTime)
	hdr.Xattrs[xattrCreateTime], _ = win32TimeToTar(fileInfo.CreationTime)
	hdr.Xattrs[xattrWriteTime], hdr.ModTime = win32TimeToTar(fileInfo.LastWriteTime)

	if (fileInfo.FileAttributes & syscall.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		hdr.Mode |= c_ISDIR
		hdr.Size = 0
		hdr.Typeflag = tar.TypeDir
	}

	br := winio.NewBackupStreamReader(r)
	var dataHdr *winio.BackupHeader
	for dataHdr == nil {
		bhdr, err := br.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch bhdr.Id {
		case winio.BackupData:
			hdr.Mode |= c_ISREG
			dataHdr = bhdr
		case winio.BackupSecurity:
			sd, err := ioutil.ReadAll(br)
			if err != nil {
				return err
			}
			sddl, err := winio.SecurityDescriptorToSddl(sd)
			if err != nil {
				return err
			}
			hdr.Xattrs[xattrSecurityDescriptor] = sddl

		case winio.BackupReparseData:
			hdr.Mode |= c_ISLNK
			hdr.Typeflag = tar.TypeSymlink
			reparseBuffer, err := ioutil.ReadAll(br)
			rp, err := winio.DecodeReparsePoint(reparseBuffer)
			if err != nil {
				return err
			}
			if rp.IsMountPoint {
				hdr.Xattrs[xattrMountPoint] = "1"
			}
			hdr.Linkname = rp.Target
		case winio.BackupEaData, winio.BackupLink, winio.BackupPropertyData, winio.BackupObjectId, winio.BackupTxfsData:
			// ignore these streams
		default:
			return fmt.Errorf("%s: unknown stream ID %d", name, bhdr.Id)
		}
	}

	err := t.WriteHeader(hdr)
	if err != nil {
		return err
	}

	if dataHdr != nil {
		// A data stream was found. Copy the data.
		if (dataHdr.Attributes & winio.StreamSparseAttributes) == 0 {
			if size != dataHdr.Size {
				return fmt.Errorf("%s: mismatch between file size %d and header size %d", name, size, dataHdr.Size)
			}
			_, err = io.Copy(t, br)
			if err != nil {
				return err
			}
		} else {
			err = copySparse(t, br)
			if err != nil {
				return err
			}
		}
	}

	// Look for streams after the data stream. The only ones we handle are alternate data streams.
	// Other streams may have metadata that could be serialized, but the tar header has already
	// been written. In practice, this means that we don't get EA or TXF metadata.
	for {
		bhdr, err := br.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch bhdr.Id {
		case winio.BackupAlternateData:
			altName := bhdr.Name
			if strings.HasSuffix(altName, ":$DATA") {
				altName = altName[:len(altName)-len(":$DATA")]
			}
			if (bhdr.Attributes & winio.StreamSparseAttributes) == 0 {
				hdr = &tar.Header{
					Name:       name + altName,
					Mode:       hdr.Mode,
					Typeflag:   tar.TypeReg,
					Size:       bhdr.Size,
					ModTime:    hdr.ModTime,
					AccessTime: hdr.AccessTime,
					ChangeTime: hdr.ChangeTime,
				}
				err = t.WriteHeader(hdr)
				if err != nil {
					return err
				}
				_, err = io.Copy(t, br)
				if err != nil {
					return err
				}

			} else {
				// Unsupported for now, since the size of the alternate stream is not present
				// in the backup stream until after the data has been read.
				return errors.New("tar of sparse alternate data streams is unsupported")
			}
		case winio.BackupEaData, winio.BackupLink, winio.BackupPropertyData, winio.BackupObjectId, winio.BackupTxfsData:
			// ignore these streams
		default:
			return fmt.Errorf("%s: unknown stream ID %d after data", name, bhdr.Id)
		}
	}
	return nil
}

// Retrieves basic Win32 file information from a tar header, using the additional metadata written by
// WriteTarFileFromBackupStream.
func FileInfoFromHeader(hdr *tar.Header) (name string, size int64, fileInfo *winio.FileBasicInfo, err error) {
	name = hdr.Name
	if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
		size = hdr.Size
	}
	fileInfo = &winio.FileBasicInfo{
		LastAccessTime: win32TimeFromTar(xattrAccessTime, hdr.Xattrs, hdr.AccessTime),
		LastWriteTime:  win32TimeFromTar(xattrWriteTime, hdr.Xattrs, hdr.ModTime),
		ChangeTime:     win32TimeFromTar(xattrChangeTime, hdr.Xattrs, hdr.ChangeTime),
		CreationTime:   win32TimeFromTar(xattrCreateTime, hdr.Xattrs, hdr.ModTime),
	}
	if attrStr, ok := hdr.Xattrs[xattrFileAttributes]; ok {
		attr, err := strconv.ParseUint(attrStr, 10, 32)
		if err != nil {
			return "", 0, nil, err
		}
		fileInfo.FileAttributes = uintptr(attr)
	} else {
		if hdr.Typeflag == tar.TypeDir {
			fileInfo.FileAttributes |= syscall.FILE_ATTRIBUTE_DIRECTORY
		}
	}
	return
}

// Writes a Win32 backup stream from the current tar file. Since this function may process multiple
// tar file entries in order to collect all the alternate data streams for the file, it returns the next
// tar file that was not processed, or io.EOF is there are no more.
func WriteBackupStreamFromTarFile(w io.Writer, t *tar.Reader, hdr *tar.Header) (*tar.Header, error) {
	bw := winio.NewBackupStreamWriter(w)
	if sddl, ok := hdr.Xattrs[xattrSecurityDescriptor]; ok {
		sd, err := winio.SddlToSecurityDescriptor(sddl)
		if err != nil {
			return nil, err
		}
		bhdr := winio.BackupHeader{
			Id:   winio.BackupSecurity,
			Size: int64(len(sd)),
		}
		err = bw.WriteHeader(&bhdr)
		if err != nil {
			return nil, err
		}
		_, err = bw.Write(sd)
		if err != nil {
			return nil, err
		}
	}
	if hdr.Typeflag == tar.TypeSymlink {
		_, isMountPoint := hdr.Xattrs[xattrMountPoint]
		rp := winio.ReparsePoint{
			Target:       hdr.Linkname,
			IsMountPoint: isMountPoint,
		}
		reparse := winio.EncodeReparsePoint(&rp)
		bhdr := winio.BackupHeader{
			Id:   winio.BackupReparseData,
			Size: int64(len(reparse)),
		}
		err := bw.WriteHeader(&bhdr)
		if err != nil {
			return nil, err
		}
		_, err = bw.Write(reparse)
		if err != nil {
			return nil, err
		}
	}
	if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
		bhdr := winio.BackupHeader{
			Id:   winio.BackupData,
			Size: hdr.Size,
		}
		err := bw.WriteHeader(&bhdr)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(bw, t)
		if err != nil {
			return nil, err
		}
	}
	// Copy all the alternate data streams and return the next non-ADS header.
	for {
		ahdr, err := t.Next()
		if err != nil {
			return nil, err
		}
		if ahdr.Typeflag != tar.TypeReg || !strings.HasPrefix(ahdr.Name, hdr.Name+":") {
			return ahdr, nil
		}
		bhdr := winio.BackupHeader{
			Id:   winio.BackupAlternateData,
			Size: ahdr.Size,
			Name: ahdr.Name[len(hdr.Name)+1:] + ":$DATA",
		}
		err = bw.WriteHeader(&bhdr)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(bw, t)
		if err != nil {
			return nil, err
		}
	}
}
