package auth

import (
	"encoding/json"
	"net/http"
	"time"
)

type Manager struct {
	password   *PasswordStore
	jwt        *JWT
	limiter    *RateLimiter
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func NewManager(secret []byte, passwordPath string, accessTTL, refreshTTL time.Duration, maxAttempts int, lockoutDuration time.Duration) *Manager {
	return &Manager{
		password:   NewPasswordStore(passwordPath),
		jwt:        NewJWT(secret),
		limiter:    NewRateLimiter(maxAttempts, lockoutDuration),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

func (m *Manager) StatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"password_set": m.password.IsSet(),
	})
}

func (m *Manager) PasswordStore() *PasswordStore {
	return m.password
}

func (m *Manager) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ip := r.RemoteAddr

	if m.limiter.IsLocked(ip) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "account locked, try later"})
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if !m.password.Verify(body.Password) {
		m.limiter.RecordFailure(ip)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid password"})
		return
	}

	m.limiter.Reset(ip)

	accessToken, err := m.jwt.Sign(map[string]any{"sub": "user", "type": "access"}, m.accessTTL)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "token generation failed"})
		return
	}

	refreshToken, err := m.jwt.Sign(map[string]any{"sub": "user", "type": "refresh"}, m.refreshTTL)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "token generation failed"})
		return
	}

	json.NewEncoder(w).Encode(TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(m.accessTTL.Seconds()),
	})
}

func (m *Manager) Refresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	claims, err := m.jwt.Verify(body.RefreshToken)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired refresh token"})
		return
	}

	if claims["type"] != "refresh" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid token type"})
		return
	}

	accessToken, err := m.jwt.Sign(map[string]any{"sub": "user", "type": "access"}, m.accessTTL)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "token generation failed"})
		return
	}

	json.NewEncoder(w).Encode(TokenPair{
		AccessToken:  accessToken,
		ExpiresIn:    int(m.accessTTL.Seconds()),
	})
}

func (m *Manager) ValidateToken(tokenStr string) (map[string]any, error) {
	return m.jwt.Verify(tokenStr)
}

func (m *Manager) SetupHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if m.password.IsSet() {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "password already set"})
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if err := m.password.Set(body.Password); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "password set"})
}
