package packages

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ImportArchive requires separate pins for the archive bytes and the package
// identity. It executes nothing and returns the same owned snapshot as staging.
func ImportArchive(ctx context.Context, filename, parent, archiveDigest, packageDigest string) (snapshot *Snapshot, err error) {
	if !validDigest(archiveDigest) || !validDigest(packageDigest) {
		return nil, errors.New("archive and package SHA-256 pins required")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return nil, errors.New("archive staging parent must be private and nonsymlink (0700)")
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp(parent, "package-import-")
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, os.RemoveAll(work))
		if err != nil && snapshot != nil {
			err = errors.Join(err, snapshot.Close())
			snapshot = nil
		}
	}()
	filename, err = filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	input, err := os.OpenRoot(filepath.Dir(filename))
	if err != nil {
		return nil, err
	}
	defer input.Close()
	file, err := openRegular(input, filepath.Base(filename))
	if err != nil {
		return nil, err
	}
	info, err = file.Stat()
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return nil, errors.Join(err, closeErr)
	}
	if info.Size() > MaxPackageBytes {
		return nil, errors.New("archive exceeds 512 MiB")
	}
	owned, err := os.OpenRoot(work)
	if err != nil {
		return nil, err
	}
	defer owned.Close()
	// Pin a private copy before decompression; the input may change afterward.
	expected := File{Path: filepath.Base(filename), Size: info.Size(), SHA256: archiveDigest, Executable: info.Mode().Perm()&0111 != 0}
	if err = copyPackageFile(ctx, input, owned, expected); err != nil {
		return nil, err
	}
	if err = owned.Mkdir("contents", 0700); err != nil {
		return nil, err
	}
	output, err := owned.OpenRoot("contents")
	if err != nil {
		return nil, err
	}
	defer output.Close()
	archive, err := owned.Open(expected.Path)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	extractor := archiveExtractor{ctx: ctx, root: output, seen: map[string]bool{}}
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		if e := preflightZIP(archive, expected.Size); e != nil {
			return nil, e
		}
		reader, e := zip.NewReader(archive, expected.Size)
		if e != nil {
			return nil, e
		}
		if len(reader.File) > 4096 {
			return nil, errors.New("archive entry quota exceeded")
		}
		for _, file := range reader.File {
			mode := file.Mode()
			if mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !mode.IsDir()) {
				return nil, errors.New("archive links and special files forbidden")
			}
			if file.UncompressedSize64 > uint64(MaxFileBytes) {
				return nil, errors.New("archive file exceeds 128 MiB")
			}
			if mode.IsDir() {
				if err = extractor.entry(file.Name, 0, true, false, nil); err != nil {
					return nil, err
				}
				continue
			}
			reader, e := file.Open()
			if e != nil {
				return nil, e
			}
			e = extractor.entry(file.Name, int64(file.UncompressedSize64), false, mode.Perm()&0111 != 0, reader)
			ce := reader.Close()
			if e != nil || ce != nil {
				return nil, errors.Join(e, ce)
			}
		}
	case strings.HasSuffix(lower, ".tar"), strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		var stream io.Reader = archive
		if !strings.HasSuffix(lower, ".tar") {
			reader, e := gzip.NewReader(archive)
			if e != nil {
				return nil, e
			}
			defer reader.Close()
			stream = reader
		}
		reader := tar.NewReader(stream)
		gitCommentSeen := false
		for {
			header, e := reader.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, e
			}
			switch header.Typeflag {
			case tar.TypeXGlobalHeader:
				comment := header.PAXRecords["comment"]
				bytes, e := hex.DecodeString(comment)
				if gitCommentSeen || len(header.PAXRecords) != 1 || e != nil || len(bytes) != 20 || strings.ToLower(comment) != comment {
					return nil, errors.New("unsupported global tar metadata")
				}
				gitCommentSeen = true
			case tar.TypeDir:
				err = extractor.entry(header.Name, 0, true, false, nil)
			case tar.TypeReg:
				err = extractor.entry(header.Name, header.Size, false, header.Mode&0111 != 0, reader)
			default:
				return nil, errors.New("archive links and special records forbidden")
			}
			if err != nil {
				return nil, err
			}
		}
		if err = validateArchiveTail(ctx, stream); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("supported package archives: .zip, .tar, .tar.gz, .tgz")
	}
	snapshot, err = StageDirectory(ctx, filepath.Join(work, "contents"), parent, packageDigest)
	if err == nil {
		snapshot.origin = Origin{Kind: "archive", Location: filename, Reference: archiveDigest}
	}
	return snapshot, err
}

type archiveExtractor struct {
	ctx     context.Context
	root    *os.Root
	seen    map[string]bool
	entries int
	total   int64
}

func (e *archiveExtractor) entry(name string, size int64, directory, executable bool, reader io.Reader) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	relativeRoot := name == "." || strings.HasPrefix(name, "./")
	for strings.HasPrefix(name, "./") {
		name = strings.TrimPrefix(name, "./")
	}
	if directory {
		name = strings.TrimSuffix(name, "/")
		if name == "" || name == "." {
			if !relativeRoot {
				return errors.New("unsafe archive root path")
			}
			e.entries++
			if e.entries > 4096 {
				return errors.New("archive entry quota exceeded")
			}
			return nil
		}
	}
	if (name != ManifestName && !validPath(name)) || strings.Contains(name, "\\") {
		return fmt.Errorf("unsafe archive path %q", name)
	}
	key := strings.ToLower(name)
	if e.seen[key] {
		return errors.New("duplicate archive entry")
	}
	e.seen[key] = true
	e.entries++
	if e.entries > 4096 {
		return errors.New("archive entry quota exceeded")
	}
	if directory {
		return e.root.MkdirAll(name, 0700)
	}
	if size < 0 || size > MaxFileBytes {
		return errors.New("archive file exceeds 128 MiB")
	}
	if name == ManifestName && size > 256<<10 {
		return errors.New("archive manifest exceeds 256 KiB")
	}
	e.total += size
	if e.total > MaxPackageBytes+(256<<10) {
		return errors.New("archive expanded size quota exceeded")
	}
	if err := e.root.MkdirAll(path.Dir(name), 0700); err != nil {
		return err
	}
	f, err := e.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = copyArchiveEntry(e.ctx, f, reader, size)
	if err == nil && executable {
		err = f.Chmod(0700)
	}
	return errors.Join(err, f.Close())
}
func copyArchiveEntry(ctx context.Context, dst io.Writer, src io.Reader, size int64) error {
	buffer := make([]byte, 32<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := src.Read(buffer)
		total += int64(n)
		if total > size {
			return errors.New("archive entry grew beyond declared size")
		}
		if n > 0 {
			written, err := dst.Write(buffer[:n])
			if err != nil {
				return err
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if total != size {
		return errors.New("archive entry shorter than declared size")
	}
	return nil
}

// Bound central-directory allocation before archive/zip builds its index. ZIP64
// and multipart archives are unnecessary for these package quotas and refused.
func preflightZIP(file io.ReaderAt, size int64) error {
	length := min(size, int64(65557))
	if length < 22 {
		return errors.New("truncated ZIP directory")
	}
	tail := make([]byte, int(length))
	if _, err := file.ReadAt(tail, size-length); err != nil {
		return err
	}
	for i := len(tail) - 22; i >= 0; i-- {
		if binary.LittleEndian.Uint32(tail[i:i+4]) != 0x06054b50 {
			continue
		}
		if i >= 20 && binary.LittleEndian.Uint32(tail[i-20:i-16]) == 0x07064b50 {
			return errors.New("ZIP64 packages are unsupported")
		}
		record := tail[i:]
		if i+22+int(binary.LittleEndian.Uint16(record[20:22])) != len(tail) {
			continue
		}
		disk := binary.LittleEndian.Uint16(record[4:6])
		directoryDisk := binary.LittleEndian.Uint16(record[6:8])
		localCount := binary.LittleEndian.Uint16(record[8:10])
		count := binary.LittleEndian.Uint16(record[10:12])
		directoryBytes := binary.LittleEndian.Uint32(record[12:16])
		offset := binary.LittleEndian.Uint32(record[16:20])
		if disk != 0 || directoryDisk != 0 || localCount != count || count > 4096 || directoryBytes > 8<<20 || int64(offset)+int64(directoryBytes) > size-length+int64(i) {
			return errors.New("ZIP directory exceeds package limits or uses unsupported multipart/ZIP64 layout")
		}
		return nil
	}
	return errors.New("ZIP end directory missing")
}
func validateArchiveTail(ctx context.Context, reader io.Reader) error {
	buffer := make([]byte, 32<<10)
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := reader.Read(buffer)
		total += n
		if total > 1<<20 {
			return errors.New("excessive tar padding")
		}
		for _, b := range buffer[:n] {
			if b != 0 {
				return errors.New("unexpected data after tar archive")
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
