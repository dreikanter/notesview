package index

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var uidPattern = regexp.MustCompile(`^(\d{8}_\d+)`)

func IsUID(s string) bool {
	return regexp.MustCompile(`^\d{8}_\d+$`).MatchString(s)
}

type Index struct {
	root string
	mu   sync.RWMutex
	uids map[string]string
}

func New(root string) *Index {
	return &Index{
		root: root,
		uids: make(map[string]string),
	}
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
