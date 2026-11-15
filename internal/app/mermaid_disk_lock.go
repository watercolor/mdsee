package app

import (
	"context"
	"os"
	"time"
)

// Lock files have stable inodes and are retained when cached artifacts are cleared.
func lockMermaidFile(ctx context.Context, path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			file.Close()
			return nil, err
		}
		ok, err := tryMermaidFileLock(file)
		if err != nil {
			file.Close()
			return nil, err
		}
		if ok {
			return file, nil
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
