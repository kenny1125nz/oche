package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Workspace struct {
	ID         string `json:"id"`
	TargetDir  string `json:"target_dir"`
	ActiveFile string `json:"active_file,omitempty"`
}

type Entry struct {
	Name     string  `json:"name"`
	IsDir    bool    `json:"is_dir"`
	Children []Entry `json:"children,omitempty"`
}

type Registry struct {
	workspaces map[string]*Workspace
}

func NewRegistry() *Registry {
	return &Registry{
		workspaces: make(map[string]*Workspace),
	}
}

func (r *Registry) Discover(paths []string) error {
	for _, p := range paths {
		if strings.HasSuffix(p, "/*") || strings.HasSuffix(p, "\\*") {
			base := strings.TrimRight(p, "/*\\")
			entries, err := os.ReadDir(base)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return fmt.Errorf("read dir %s: %w", base, err)
			}
			for _, entry := range entries {
				if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
					subPath := filepath.Join(base, entry.Name())
					ws := &Workspace{
						ID:        entry.Name(),
						TargetDir: subPath,
					}
					r.workspaces[ws.ID] = ws
				}
			}
		} else {
			info, err := os.Stat(p)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return fmt.Errorf("stat %s: %w", p, err)
			}
			if !info.IsDir() {
				continue
			}
			id := filepath.Base(p)
			r.workspaces[id] = &Workspace{
				ID:        id,
				TargetDir: p,
			}
		}
	}
	return nil
}

func (r *Registry) Get(id string) (*Workspace, error) {
	ws, ok := r.workspaces[id]
	if !ok {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	return ws, nil
}

func (r *Registry) List() []*Workspace {
	result := make([]*Workspace, 0, len(r.workspaces))
	for _, ws := range r.workspaces {
		result = append(result, ws)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func (r *Registry) Len() int {
	return len(r.workspaces)
}

var defaultIgnores = []string{
	".git", "node_modules", "__pycache__", ".DS_Store",
	"*.log", ".venv", "vendor", "target", "dist", ".next", ".turbo",
}

func loadGitIgnore(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return nil
	}
	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func shouldIgnore(name string, isDir bool, ignores []string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if len(ignores) > 0 {
		for _, pattern := range ignores {
			if matched, _ := filepath.Match(pattern, name); matched {
				return true
			}
		}
		return false
	}
	for _, pattern := range defaultIgnores {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}
	return false
}

func (w *Workspace) Tree(depth int) []Entry {
	if depth <= 0 || w.TargetDir == "" {
		return nil
	}
	entries, err := os.ReadDir(w.TargetDir)
	if err != nil {
		return nil
	}

	ignores := loadGitIgnore(w.TargetDir)

	var result []Entry
	for _, e := range entries {
		name := e.Name()
		if shouldIgnore(name, e.IsDir(), ignores) {
			continue
		}
		entry := Entry{Name: name, IsDir: e.IsDir()}
		if e.IsDir() && depth > 1 {
			sub := &Workspace{TargetDir: filepath.Join(w.TargetDir, name)}
			entry.Children = sub.Tree(depth - 1)
		}
		result = append(result, entry)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}
