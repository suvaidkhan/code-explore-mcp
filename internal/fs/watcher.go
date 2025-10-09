package fs

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	debounceDuration = 60 * time.Second
)

type FileHandler func(ctx context.Context, filepath []string)

type Watcher struct {
	workspace        string
	filter           *FileFilter
	handler          FileHandler
	fsWatcher        *fsnotify.Watcher
	debounceTimer    *time.Timer
	pendingFiles     map[string]bool
	mu               sync.RWMutex
	debounceDuration time.Duration
	ctx              context.Context
	cancel           context.CancelFunc
}

func NewWatcher(ctx context.Context, workspace string, supported []string, handler FileHandler) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	w := &Watcher{
		workspace:    workspace,
		filter:       NewFileFilter(workspace, supported),
		handler:      handler,
		fsWatcher:    fsWatcher,
		pendingFiles: make(map[string]bool),
		ctx:          ctx,
		cancel:       cancel,
	}

	err = w.addWatcher()
	if err != nil {
		fsWatcher.Close()
		cancel()
		return nil, err
	}

	go w.watch()

	return w, nil
}

func (w *Watcher) addWatcher() error {
	var supported []string
	for ext := range w.filter.supported {
		supported = append(supported, ext)
	}

	return WalkSourceFiles(w.workspace, supported, func(filePath string) error {
		dir := filepath.Dir(filepath.Join(w.workspace, filePath))
		return w.fsWatcher.Add(dir)
	})
}

func (w *Watcher) watch() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}

			w.handleEvent(event)
		case _, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.shouldIgnoreEvent(event) {
		return
	}
	relpath, err := filepath.Rel(w.workspace, event.Name)
	if err != nil {
		return
	}
	w.pendingFiles[relpath] = true
	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	w.debounceTimer = time.AfterFunc(w.debounceDuration, w.processPendingFiles)
}

func (w *Watcher) shouldIgnoreEvent(event fsnotify.Event) bool {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0 {
		return true
	}

	return w.filter.ShouldIgnore(event.Name)
}

func (w *Watcher) processPendingFiles() {
	w.mu.Lock()
	defer w.mu.Unlock()
	changes := make([]string, 0, len(w.pendingFiles))
	for filePath := range w.pendingFiles {
		changes = append(changes, filePath)
	}

	if len(changes) > 0 {
		w.handler(w.ctx, changes)
	}

	w.pendingFiles = map[string]bool{}
}

func (w *Watcher) Close() error {
	w.cancel()

	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	return w.fsWatcher.Close()
}
