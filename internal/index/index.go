package index

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/dreikanter/notes-view/internal/logging"
)

var uidPattern = regexp.MustCompile(`^(\d{8}_\d+)`)

var fullUIDPattern = regexp.MustCompile(`^\d{8}_\d+$`)

func IsUID(s string) bool {
	return fullUIDPattern.MatchString(s)
}

type Index struct {
	root     string
	logger   *slog.Logger
	mu       sync.RWMutex
	uids     map[string]string
	building sync.Mutex
}

func New(root string, logger *slog.Logger) *Index {
	if logger == nil {
		logger = logging.Discard()
	}
	return &Index{
		root:   root,
		logger: logger,
		uids:   make(map[string]string),
	}
}

// Rebuild triggers a background index build, coalescing concurrent calls.
// If a build is already in progress, the call returns immediately.
func (idx *Index) Rebuild() {
	if !idx.building.TryLock() {
		return
	}
	go func() {
		defer idx.building.Unlock()
		if err := idx.Build(); err != nil {
			idx.logger.Error("index rebuild failed", "err", err)
		}
	}()
}

func (idx *Index) Build() error {
	uids := make(map[string]string)
	err := filepath.WalkDir(idx.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				idx.logger.Warn("skipping path: permission denied", "path", path)
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		matches := uidPattern.FindStringSubmatch(d.Name())
		if matches == nil {
			return nil
		}
		rel, err := filepath.Rel(idx.root, path)
		if err != nil {
			return nil
		}
		uids[matches[1]] = rel
		return nil
	})
	if err != nil {
		return err
	}
	idx.mu.Lock()
	idx.uids = uids
	idx.mu.Unlock()
	return nil
}

func (idx *Index) Lookup(uid string) (string, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	p, ok := idx.uids[uid]
	return p, ok
}
