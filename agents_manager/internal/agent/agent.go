package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

type Registration struct {
	ID     string `json:"id"`
	Binary string `json:"binary"`
	Path   string `json:"path"`
	System bool   `json:"system"`
}

// ProviderConfig describes how to invoke an agent CLI for one-shot execution.
type ProviderConfig struct {
	// ArgsTemplate is the CLI arguments. "{input}" is replaced with the
	// instruction text. The binary is prepended automatically.
	ArgsTemplate []string `yaml:"args"`
	// UseStdin when true passes the instruction on stdin instead of in args.
	UseStdin bool `yaml:"use_stdin"`
}

// knownSystemAgents maps well-known CLI agent commands to their display IDs.
var knownSystemAgents = map[string]string{
	"claude":             "claude",
	"opencode":           "opencode",
	"aider":              "aider",
	"github-copilot-cli": "github-copilot-cli",
	"copilot":            "copilot",
}

func defaultProviderConfigs() map[string]ProviderConfig {
	return map[string]ProviderConfig{
		"opencode": {
			ArgsTemplate: []string{"run", "{input}"},
		},
		"claude": {
			ArgsTemplate: []string{"-p", "{input}"},
		},
		"aider": {
			ArgsTemplate: []string{"--message", "{input}", "--no-auto-commits", "--yes"},
		},
		"github-copilot-cli": {
			ArgsTemplate: []string{"ask", "{input}"},
		},
		"copilot": {
			ArgsTemplate: []string{"ask", "{input}"},
		},
	}
}

type Registry interface {
	Discover(home string) error
	DiscoverSystem()
	Get(id string) (*Registration, error)
	List() []*Registration
	Len() int
	ProviderFor(id string) (ProviderConfig, bool)
}

type registry struct {
	agents          map[string]*Registration
	providerConfigs map[string]ProviderConfig
}

func NewRegistry() Registry {
	return &registry{
		agents:          make(map[string]*Registration),
		providerConfigs: defaultProviderConfigs(),
	}
}

func (r *registry) Discover(home string) error {
	agentsDir := filepath.Join(home, "agents")

	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read agents dir %s: %w", agentsDir, err)
	}

	for _, entry := range entries {
		name := entry.Name()
		agentPath := filepath.Join(agentsDir, name)

		// Subdirectory: look for an executable with the same name inside
		if entry.IsDir() {
			innerPath := filepath.Join(agentPath, name)
			if info, statErr := os.Stat(innerPath); statErr == nil && (info.Mode().Perm()&0111) != 0 {
				r.agents[name] = &Registration{
					ID:     name,
					Binary: innerPath,
					Path:   agentPath,
				}
				continue
			}
			// Check for a bin/ subdirectory as well
			binPath := filepath.Join(agentPath, "bin", name)
			if info, statErr := os.Stat(binPath); statErr == nil && (info.Mode().Perm()&0111) != 0 {
				r.agents[name] = &Registration{
					ID:     name,
					Binary: binPath,
					Path:   agentPath,
				}
				continue
			}
			continue
		}

		// Direct executable file
		if info, statErr := os.Stat(agentPath); statErr == nil && info.Mode().IsRegular() && (info.Mode().Perm()&0111) != 0 {
			r.agents[name] = &Registration{
				ID:     name,
				Binary: agentPath,
				Path:   filepath.Dir(agentPath),
			}
		}
	}

	return nil
}

// DiscoverSystem scans $PATH for well-known CLI coding agents and registers
// any that are found. Already-registered agents (from ~/.agents_manager/agents/)
// take precedence and are not overwritten.
func (r *registry) DiscoverSystem() {
	for bin, id := range knownSystemAgents {
		if _, exists := r.agents[id]; exists {
			continue
		}
		full, err := exec.LookPath(bin)
		if err != nil {
			continue
		}
		r.agents[id] = &Registration{
			ID:     id,
			Binary: full,
			Path:   filepath.Dir(full),
			System: true,
		}
	}
}

func (r *registry) ProviderFor(id string) (ProviderConfig, bool) {
	cfg, ok := r.providerConfigs[id]
	return cfg, ok
}

func (r *registry) Get(id string) (*Registration, error) {
	reg, ok := r.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", id)
	}
	return reg, nil
}

func (r *registry) List() []*Registration {
	result := make([]*Registration, 0, len(r.agents))
	for _, reg := range r.agents {
		result = append(result, reg)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func (r *registry) Len() int {
	return len(r.agents)
}
