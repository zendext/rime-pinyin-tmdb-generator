package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	for {
		file, err := createLock(path)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		stale, pid := staleLock(path)
		if !stale {
			if pid > 0 {
				return nil, fmt.Errorf("lock held by pid %d", pid)
			}
			return nil, err
		}
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return nil, removeErr
		}
	}
}

func createLock(path string) (*File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(f, "pid=%d\n", os.Getpid()); err != nil {
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

func staleLock(path string) (bool, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, 0
	}
	pid := parsePID(string(data))
	if pid <= 0 {
		return true, 0
	}
	if processAlive(pid) {
		return false, pid
	}
	return true, pid
}

func parsePID(data string) int {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pid=") {
			pid, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "pid=")))
			return pid
		}
	}
	return 0
}

func (f *File) Release() error {
	if f == nil || f.path == "" {
		return nil
	}
	return os.Remove(f.path)
}
