package index

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var uidPattern = regexp.MustCompile(`^(\d{8}_\d+)`)

var fullUIDPattern = regexp.MustCompile(`^\d{8}_\d+$`)

func IsUID(s string) bool {
	return fullUIDPattern.MatchString(s)
}

type Index struct {
	root     string
	mu       sync.RWMutex
	uids     map[string]string
	building sync.Mutex
}

func New(root string) *Index {
	return &Index{
		root: root,
		uids: make(map[string]string),
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
		idx.Build()
	}()
}

func (idx *Index) Build() error {
	uids := make(map[string]string)
	err := filepath.WalkDir(idx.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
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
