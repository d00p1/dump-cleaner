package archive

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxExtractSize = 15 * 1024 * 1024 * 1024 // 15 GB

func Pack(src string, writer io.WriteCloser) error {

	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("unable to tar files - %v", err.Error())
	}

	tw := tar.NewWriter(writer)

	defer func() { _ = tw.Close(); _ = writer.Close() }()

	return filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		// return on non-regular files (thanks to [kumo](https://medium.com/@komuw/just-like-you-did-fbdd7df829d3) for this suggested update)
		if !fi.Mode().IsRegular() {
			return nil
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.ReplaceAll(file, src, ""), string(filepath.Separator))

		// write the header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// open files for taring
		// #nosec G304 G122 -- source directory is user-specified; walk ensures only files under src are processed
		f, err := os.Open(file)
		if err != nil {
			return err
		}

		// copy file data into tar writer
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

		// manually close here after each file operation; defering would cause each file close
		// to wait until all operations have completed.
		_ = f.Close()

		return nil
	})
}

func Unpack(r io.Reader, dst string) error {
	tr := tar.NewReader(r)
	var totalRead int64

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		totalRead += header.Size
		if totalRead > maxExtractSize {
			return fmt.Errorf("extraction size limit exceeded (%d bytes)", maxExtractSize)
		}

		// the target location where the dir/file should be created
		// #nosec G305 -- path traversal validated with HasPrefix check below
		target := filepath.Clean(filepath.Join(dst, header.Name))
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) && target != filepath.Clean(dst) {
			return fmt.Errorf("illegal file path: %s", header.Name)
		}

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0o750); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			mode := header.Mode
			if mode > 0o755 {
				mode = 0o755
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(mode))
			if err != nil {
				return err
			}

			// copy over contents
			// #nosec G110 -- decompression bomb mitigated by maxExtractSize check
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			_ = f.Close()
		}
	}
}
