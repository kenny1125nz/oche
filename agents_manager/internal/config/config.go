package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Auth      AuthConfig      `yaml:"auth"`
	SSH       SSHConfig       `yaml:"ssh"`
	HTTP      HTTPConfig      `yaml:"http"`
	AgentPool AgentPoolConfig `yaml:"agent_pool"`
	Store     StoreConfig     `yaml:"store"`
	Home      string          `yaml:"home"`
	WorkspacePaths []string   `yaml:"workspaces"`
}

type AuthConfig struct {
	PasswordFile          string `yaml:"password_file"`
	AccessTokenExpiry     int    `yaml:"access_token_expiry"`
	RefreshTokenExpiry    int    `yaml:"refresh_token_expiry"`
	MaxLoginAttempts      int    `yaml:"max_login_attempts"`
	LockoutDuration       int    `yaml:"lockout_duration"`
}

type SSHConfig struct {
	Port                   int    `yaml:"port"`
	Bind                   string `yaml:"bind"`
	HostKeyPath            string `yaml:"host_key_path"`
	AuthorizedKeysPath     string `yaml:"authorized_keys_path"`
	PasswordAuth           bool   `yaml:"password_auth"`
	PasswordMinLength      int    `yaml:"password_min_length"`
	PasswordRequireMixed   bool   `yaml:"password_require_mixed_case"`
	PasswordRequireDigit   bool   `yaml:"password_require_digit"`
	PasswordMaxAttempts    int    `yaml:"password_max_attempts"`
}

type HTTPConfig struct {
	Port      int    `yaml:"port"`
	Bind      string `yaml:"bind"`
	StaticDir string `yaml:"static_dir"`
}

type AgentPoolConfig struct {
	MaxInstances     int `yaml:"max_instances"`
	StdoutBufferSize int `yaml:"stdout_buffer_size"`
}

type StoreConfig struct {
	Path string `yaml:"path"`
}

func expandPath(p string, home string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

func ensureDir(p string) error {
	dir := filepath.Dir(p)
	return os.MkdirAll(dir, 0755)
}

func Default() *Config {
	return &Config{
		Auth: AuthConfig{
			PasswordFile:       "~/.agents_manager/passwd",
			AccessTokenExpiry:  3600,
			RefreshTokenExpiry: 2592000,
			MaxLoginAttempts:   5,
			LockoutDuration:    300,
		},
		SSH: SSHConfig{
			Port:                2222,
			Bind:                "0.0.0.0",
			HostKeyPath:         "~/.agents_manager/ssh_host_key",
			AuthorizedKeysPath:  "~/.ssh/authorized_keys",
			PasswordAuth:        true,
			PasswordMinLength:   12,
			PasswordRequireMixed: true,
			PasswordRequireDigit: true,
			PasswordMaxAttempts:  3,
		},
		HTTP: HTTPConfig{
			Port:      8080,
			Bind:      "0.0.0.0",
			StaticDir: "~/.agents_manager/web",
		},
		AgentPool: AgentPoolConfig{
			MaxInstances:     4,
			StdoutBufferSize: 1048576,
		},
		Store: StoreConfig{
			Path: "~/.agents_manager/sessions.db",
		},
		Home: "~/.agents_manager",
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.Getenv("HOME")
	}
	if homeDir == "" {
		homeDir = "."
	}

	// Load home config first (user defaults)
	homeConfig := filepath.Join(homeDir, ".agents_manager", "config.yaml")
	if data, err := os.ReadFile(homeConfig); err == nil {
		yaml.Unmarshal(data, cfg)
	}

	// Load CWD config on top (project-level overrides)
	if data, err := os.ReadFile(path); err == nil {
		yaml.Unmarshal(data, cfg)
	}

	cfg.Home = expandPath(cfg.Home, homeDir)

	cfg.Auth.PasswordFile = expandPath(cfg.Auth.PasswordFile, homeDir)
	cfg.SSH.HostKeyPath = expandPath(cfg.SSH.HostKeyPath, homeDir)
	cfg.SSH.AuthorizedKeysPath = expandPath(cfg.SSH.AuthorizedKeysPath, homeDir)
	cfg.HTTP.StaticDir = expandPath(cfg.HTTP.StaticDir, homeDir)
	cfg.Store.Path = expandPath(cfg.Store.Path, homeDir)

	for i, p := range cfg.WorkspacePaths {
		cfg.WorkspacePaths[i] = expandPath(p, homeDir)
	}

	_ = ensureDir(cfg.Home)
	_ = ensureDir(cfg.Auth.PasswordFile)
	_ = ensureDir(cfg.SSH.HostKeyPath)
	_ = ensureDir(cfg.Store.Path)
	_ = os.MkdirAll(cfg.HTTP.StaticDir, 0755)

	return cfg, nil
}
