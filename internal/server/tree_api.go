package server

import (
	"encoding/json"
	"net/http"
	"os"
)

// treeNode is the JSON shape returned by /api/tree/list.
// Kept minimal and stable so the TreeView component's loader contract
// does not depend on IndexEntry internals.
type treeNode struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

func (s *Server) handleTreeList(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path")

	absPath, err := SafePath(s.root, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fi, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !fi.IsDir() {
		http.Error(w, "not a directory: "+relPath, http.StatusBadRequest)
		return
	}

	entries, err := readDirEntries(absPath, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nodes := make([]treeNode, 0, len(entries))
	for _, e := range entries {
		p := e.Name
		if relPath != "" {
			p = relPath + "/" + e.Name
		}
		nodes = append(nodes, treeNode{Name: e.Name, Path: p, IsDir: e.IsDir})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(nodes); err != nil {
		s.logger.Warn("tree list encode failed", "err", err)
	}
}
