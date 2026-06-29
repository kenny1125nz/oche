package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Skill struct {
	Name        string `json:"name"`
	Binary      string `json:"binary"`
	Description string `json:"description"`
}

type Registry struct {
	globalDir string
}

func NewRegistry(home string) *Registry {
	return &Registry{
		globalDir: filepath.Join(home, "skills"),
	}
}

func (r *Registry) discoverFromDir(dir string) []Skill {
	var skills []Skill

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".desc") {
			continue
		}
		path := filepath.Join(dir, name)

		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || (info.Mode().Perm()&0111) == 0 {
			continue
		}

		skill := Skill{
			Name:   name,
			Binary: path,
		}

		descFile := path + ".desc"
		if descData, err := os.ReadFile(descFile); err == nil {
			skill.Description = strings.TrimSpace(string(descData))
		} else {
			// Try --help
			if out, err := exec.Command(path, "--help").Output(); err == nil {
				firstLine := strings.SplitN(string(out), "\n", 2)[0]
				skill.Description = strings.TrimSpace(firstLine)
			}
		}

		skills = append(skills, skill)
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills
}

func (r *Registry) GetGlobal() []Skill {
	return r.discoverFromDir(r.globalDir)
}

func (r *Registry) GetWorkspaceSkills(workspacePath string) []Skill {
	if workspacePath == "" {
		return nil
	}
	return r.discoverFromDir(filepath.Join(workspacePath, ".agents", "skills"))
}

func (r *Registry) GetAgentSkills(agentPath string) []Skill {
	if agentPath == "" {
		return nil
	}
	return r.discoverFromDir(filepath.Join(agentPath, "skills"))
}

func (r *Registry) GetAll(workspacePath, agentPath string) []Skill {
	seen := make(map[string]bool)
	var result []Skill

	for _, skill := range r.GetAgentSkills(agentPath) {
		if !seen[skill.Name] {
			seen[skill.Name] = true
			result = append(result, skill)
		}
	}
	for _, skill := range r.GetWorkspaceSkills(workspacePath) {
		if !seen[skill.Name] {
			seen[skill.Name] = true
			result = append(result, skill)
		}
	}
	for _, skill := range r.GetGlobal() {
		if !seen[skill.Name] {
			seen[skill.Name] = true
			result = append(result, skill)
		}
	}

	return result
}
