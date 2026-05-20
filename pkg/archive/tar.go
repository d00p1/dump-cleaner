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

	root, err := os.OpenRoot(src)
	if err != nil {
		return fmt.Errorf("unable to tar files - %v", err.Error())
	}
	defer root.Close()

	tw := tar.NewWriter(writer)

	defer func() { _ = tw.Close(); _ = writer.Close() }()

	return filepath.WalkDir(src, func(file string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// return on non-regular files
		if !d.Type().IsRegular() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.ReplaceAll(file, src, ""), string(filepath.Separator))

		// write the header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// open files for taring using root-scoped API to prevent symlink TOCTOU
		f, err := root.Open(strings.TrimPrefix(strings.ReplaceAll(file, src, ""), string(filepath.Separator)))
		if err != nil {
			return err
		}

		// copy file data into tar writer
		if _, err := io.Copy(tw, f); err != nil {
			_ = f.Close()
			return err
		}

		_ = f.Close()

		return nil
	})
}

func Unpack(r io.Reader, dst string) error {
	root, err := os.OpenRoot(dst)
	if err != nil {
		return fmt.Errorf("open root directory %q: %w", dst, err)
	}
	defer root.Close()

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

		// clean the path to prevent directory traversal
		cleanName := filepath.Clean(header.Name)
		if filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, "..") {
			return fmt.Errorf("illegal file path: %s", header.Name)
		}

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if err := root.MkdirAll(cleanName, 0o750); err != nil {
				return err
			}

		// if it's a file create it
		case tar.TypeReg:
			mode := header.Mode
			if mode > 0o755 {
				mode = 0o755
			}
			f, err := root.OpenFile(cleanName, os.O_CREATE|os.O_RDWR, os.FileMode(mode))
			if err != nil {
				return err
			}

			// copy over contents, limited to header.Size to prevent decompression bomb
			if _, err := io.Copy(f, io.LimitReader(tr, header.Size)); err != nil {
				_ = f.Close()
				return err
			}

			_ = f.Close()
		}
	}
}
