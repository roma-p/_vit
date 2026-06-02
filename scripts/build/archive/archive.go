
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ExtractTarGz extracts a single binary from a tar.gz archive
func ExtractTarGz(tarGzPath, destDir, binaryName string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Name == binaryName || filepath.Base(header.Name) == binaryName {
			dest := filepath.Join(destDir, binaryName)
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0755)
			if err != nil {
				return err
			}
			defer out.Close()

			if _, err := io.Copy(out, tr); err != nil {
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("binary %s not found in archive", binaryName)
}

// ExtractZip extracts a single binary from a zip archive
func ExtractZip(zipPath, destDir, binaryName string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name == binaryName || filepath.Base(f.Name) == binaryName {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			dest := filepath.Join(destDir, binaryName)
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0755)
			if err != nil {
				return err
			}
			defer out.Close()

			_, err = io.Copy(out, rc)
			return err
		}
	}
	return fmt.Errorf("binary %s not found in archive", binaryName)
}

// CreateTarGzArchive creates a new tar.gz archive and returns the writer and cleanup function
func CreateTarGzArchive(archivePath string) (*tar.Writer, func() error, error) {
	out, err := os.Create(archivePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create archive: %w", err)
	}

	gzw := gzip.NewWriter(out)
	tw := tar.NewWriter(gzw)

	closer := func() error {
		if err := tw.Close(); err != nil {
			return err
		}
		if err := gzw.Close(); err != nil {
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return nil
	}
	return tw, closer, nil
}

// CreateZipArchive creates a new zip archive and returns the writer and cleanup function
func CreateZipArchive(archivePath string) (*zip.Writer, func() error, error) {
	out, err := os.Create(archivePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create archive: %w", err)
	}
	zw := zip.NewWriter(out)

	closer := func() error {
		if err := zw.Close(); err != nil {
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return nil
	}
	return zw, closer, nil
}

// AddFileToTar adds a single file to a tar archive
func AddFileToTar(tw *tar.Writer, srcPath, destName string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", srcPath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	header := &tar.Header{
		Name:    destName,
		Size:    stat.Size(),
		Mode:    int64(stat.Mode()),
		ModTime: stat.ModTime(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("failed to write file to tar: %w", err)
	}

	return nil
}

// AddDirToTar recursively adds a directory to a tar archive
func AddDirToTar(tw *tar.Writer, srcDir, destPrefix string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == srcDir {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(destPrefix, relPath)

		if info.IsDir() {
			header := &tar.Header{
				Name:     destPath + "/",
				Mode:     int64(info.Mode()),
				ModTime:  info.ModTime(),
				Typeflag: tar.TypeDir,
			}
			return tw.WriteHeader(header)
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		header := &tar.Header{
			Name:    destPath,
			Size:    info.Size(),
			Mode:    int64(info.Mode()),
			ModTime: info.ModTime(),
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		_, err = io.Copy(tw, file)
		return err
	})
}

// AddFileToZip adds a single file to a zip archive
func AddFileToZip(zw *zip.Writer, srcPath, destName string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", srcPath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	header := &zip.FileHeader{
		Name:     destName,
		Method:   zip.Deflate,
		Modified: stat.ModTime(),
	}
	header.SetMode(stat.Mode())

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("failed to create zip header: %w", err)
	}

	if _, err := io.Copy(writer, file); err != nil {
		return fmt.Errorf("failed to write file to zip: %w", err)
	}

	return nil
}

// AddDirToZip recursively adds a directory to a zip archive
func AddDirToZip(zw *zip.Writer, srcDir, destPrefix string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == srcDir {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(destPrefix, relPath)

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		header := &zip.FileHeader{
			Name:     destPath,
			Method:   zip.Deflate,
			Modified: info.ModTime(),
		}
		header.SetMode(info.Mode())

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		_, err = io.Copy(writer, file)
		return err
	})
}
