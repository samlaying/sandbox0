package webhook

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// WatchAnnotations monitors the annotation file and triggers updates on change.
func WatchAnnotations(ctx context.Context, path string, logger *zap.Logger, onUpdate func(map[string]string)) error {
	if path == "" {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return err
	}

	readAndNotify := func() {
		annotations, readErr := readAnnotationsFile(path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				onUpdate(map[string]string{})
				return
			}
			if logger != nil {
				logger.Warn("Failed to read annotation file",
					zap.String("path", path),
					zap.Error(readErr),
				)
			}
			return
		}
		onUpdate(annotations)
	}

	readAndNotify()

	go func() {
		defer watcher.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Chmod) == 0 {
					continue
				}
				time.Sleep(50 * time.Millisecond)
				readAndNotify()
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				if logger != nil {
					logger.Warn("Annotation watcher error", zap.Error(err))
				}
			}
		}
	}()

	return nil
}

func readAnnotationsFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := string(data)
	return parseAnnotations(raw), nil
}

func parseAnnotations(raw string) map[string]string {
	result := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"`)
		if key != "" {
			result[key] = value
		}
	}
	return result
}
