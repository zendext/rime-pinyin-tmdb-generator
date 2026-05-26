package lock

import (
	"errors"
	"os"
	"path/filepath"
)

type File struct {
	path string
}

func Acquire(path string) (*File, error) {
	if path == "" {
		return &File{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, os.ErrExist) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString("locked\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return &File{path: path}, nil
}

func (f *File) Release() error {
	if f == nil || f.path == "" {
		return nil
	}
	return os.Remove(f.path)
}
