package util

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"testing/fstest"
)

// TarGzMemory reads a tar.gz file and store all data into memory using MapFS
func TarGzMemory(source io.Reader) (fstest.MapFS, error) {
	gzRead, err := gzip.NewReader(source)
	if err != nil {
		return nil, err
	}

	tarRead := tar.NewReader(gzRead)
	files := make(fstest.MapFS)

	for {
		cur, err := tarRead.Next()

		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		if cur.Typeflag != tar.TypeReg {
			continue
		}

		data, err := io.ReadAll(tarRead)

		if err != nil {
			return nil, err
		}

		files[cur.Name] = &fstest.MapFile{
			Data:    data,
			Mode:    fs.FileMode(cur.Mode),
			ModTime: cur.ModTime,
		}
	}
	return files, nil
}

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
