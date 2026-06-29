package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type PasswordStore struct {
	path string
}

type passwordFile struct {
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
}

func NewPasswordStore(path string) *PasswordStore {
	return &PasswordStore{path: path}
}

func (s *PasswordStore) IsSet() bool {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return false
	}
	var pf passwordFile
	return json.Unmarshal(data, &pf) == nil && pf.Hash != ""
}

func (s *PasswordStore) Set(password string) error {
	if len(password) < 4 {
		return fmt.Errorf("password too short (min 4 characters)")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	pf := passwordFile{
		Hash:      string(hash),
		CreatedAt: time.Now(),
	}

	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("write password file: %w", err)
	}

	return nil
}

func (s *PasswordStore) Verify(password string) bool {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return false
	}

	var pf passwordFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return false
	}

	return bcrypt.CompareHashAndPassword([]byte(pf.Hash), []byte(password)) == nil
}
