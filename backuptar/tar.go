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
	// It's too late to change the header, so ignore everything after the data except for sparse data and alternate data streams.
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

func WriteTarFromBackupStream(t *tar.Writer, r io.Reader, name string, size int64, fileInfo *winio.FileBasicInfo) error {
	name = filepath.ToSlash(name)
	hdr := &tar.Header{
		Name:       name,
		Size:       size,
		Typeflag:   tar.TypeReg,
		ModTime:    time.Unix(0, fileInfo.LastWriteTime.Nanoseconds()),
		AccessTime: time.Unix(0, fileInfo.LastAccessTime.Nanoseconds()),
		ChangeTime: time.Unix(0, fileInfo.ChangeTime.Nanoseconds()),
		Xattrs:     make(map[string]string),
	}
	hdr.Xattrs["MSWINDOWS.fileattr"] = fmt.Sprintf("%d", fileInfo.FileAttributes)
	hdr.Xattrs["MSWINDOWS.createtime"] = fmt.Sprintf("%d", time.Unix(0, fileInfo.CreationTime.Nanoseconds()).Unix())

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
			hdr.Xattrs["MSWINDOWS.sd"] = sddl

		case winio.BackupReparseData:
			hdr.Mode |= c_ISLNK
			hdr.Typeflag = tar.TypeSymlink
			reparseBuffer, err := ioutil.ReadAll(br)
			target, isMountPoint, err := winio.DecodeReparsePoint(reparseBuffer)
			if err != nil {
				return err
			}
			if isMountPoint {
				hdr.Xattrs["MSWINDOWS.mountpoint"] = "1"
			}
			hdr.Linkname = target
		case winio.BackupEaData, winio.BackupLink, winio.BackupPropertyData, winio.BackupObjectId, winio.BackupTxfsData:
			// ignore these streams
		default:
			return fmt.Errorf("unknown stream ID %d", bhdr.Id)
		}
	}

	err := t.WriteHeader(hdr)
	if err != nil {
		return err
	}
	if dataHdr != nil {
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
			return fmt.Errorf("unknown stream ID %d after data", bhdr.Id)
		}
	}
	return nil
}

func FileInfoFromHeader(hdr *tar.Header) (name string, size int64, fileInfo *winio.FileBasicInfo, err error) {
	name = hdr.Name
	if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
		size = hdr.Size
	}
	fileInfo = &winio.FileBasicInfo{
		LastAccessTime: syscall.NsecToFiletime(hdr.AccessTime.UnixNano()),
		LastWriteTime:  syscall.NsecToFiletime(hdr.ModTime.UnixNano()),
		ChangeTime:     syscall.NsecToFiletime(hdr.ChangeTime.UnixNano()),
	}
	if attrStr, ok := hdr.Xattrs["MSWINDOWS.fileattr"]; ok {
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

	if ctimeStr, ok := hdr.Xattrs["MSWINDOWS.createtime"]; ok {
		ctime, err := strconv.ParseUint(ctimeStr, 10, 64)
		if err != nil {
			return "", 0, nil, err
		}
		fileInfo.CreationTime = syscall.NsecToFiletime(time.Unix(int64(ctime), 0).UnixNano())
	} else {
		fileInfo.CreationTime = fileInfo.LastWriteTime
	}
	return
}

func WriteBackupStreamFromTar(w io.Writer, t *tar.Reader, hdr *tar.Header) (*tar.Header, error) {
	bw := winio.NewBackupStreamWriter(w)
	if sddl, ok := hdr.Xattrs["MSWINDOWS.sd"]; ok {
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
		_, isMountPoint := hdr.Xattrs["MSWINDOWS.mountpoint"]
		reparse := winio.EncodeReparsePoint(hdr.Linkname, isMountPoint)
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
