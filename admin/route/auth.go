package route

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/0990/gotun/pkg/syncx"
	"github.com/sirupsen/logrus"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	resetDuration         = time.Hour
	authChallengeTTL      = 120 * time.Second
	authSessionTTL        = 24 * time.Hour
	authRealm             = "gotun"
	authAlgorithm         = "HMAC-SHA256"
	authCookieName        = "gotun_session"
	authStateFileName     = ".gotun_web_auth_state.json"
	authCookieMaxAge      = int(authSessionTTL / time.Second)
	authContentType       = "application/json; charset=utf-8"
	authChallengeRandSize = 16
	authSessionRandSize   = 32
	authKeySize           = 32
)

type AuthManager struct {
	failedAttempts syncx.Map[string, int]
	resetTimers    syncx.Map[string, *time.Timer]

	username string
	password string
	realm    string

	authEnabled       bool
	maxFailedAttempts int
	statePath         string

	mu         sync.Mutex
	authKey    []byte
	challenges map[string]*authChallengeState
	sessions   map[string]*authSession
}

type authChallengeState struct {
	ID        string
	Username  string
	ExpiresAt int64
}

type authSession struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	IssuedAt  int64  `json:"issued_at"`
	ExpiresAt int64  `json:"expires_at"`
	LastSeen  int64  `json:"last_seen"`
}

type authState struct {
	AuthKey  string                  `json:"auth_key"`
	Sessions map[string]*authSession `json:"sessions"`
}

type authResponse struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

type authChallengeRequest struct {
	Username string `json:"username"`
}

type authLoginRequest struct {
	Username string `json:"username"`
	Nonce    string `json:"nonce"`
	Proof    string `json:"proof"`
}

type signedChallenge struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	IssuedAt  int64  `json:"issued_at"`
	ExpiresAt int64  `json:"expires_at"`
	Random    string `json:"random"`
}

func NewAuthManager(configPath string, username string, password string, maxFailedAttempts int) *AuthManager {
	manager := &AuthManager{
		username:          username,
		password:          password,
		realm:             authRealm,
		authEnabled:       username != "" && password != "",
		maxFailedAttempts: maxFailedAttempts,
		statePath:         resolveAuthStatePath(configPath),
		challenges:        make(map[string]*authChallengeState),
		sessions:          make(map[string]*authSession),
	}

	if manager.authEnabled {
		manager.initState()
	}

	return manager
}

func resolveAuthStatePath(configPath string) string {
	dir := filepath.Dir(configPath)
	if dir == "" || dir == "." {
		return authStateFileName
	}
	return filepath.Join(dir, authStateFileName)
}

func (a *AuthManager) initState() {
	if err := a.loadState(); err != nil {
		logrus.WithError(err).Warn("load auth state failed, regenerate state")
		a.mu.Lock()
		a.authKey = mustRandomBytes(authKeySize)
		a.challenges = make(map[string]*authChallengeState)
		a.sessions = make(map[string]*authSession)
		if saveErr := a.saveStateLocked(); saveErr != nil {
			logrus.WithError(saveErr).Warn("save regenerated auth state failed")
		}
		a.mu.Unlock()
	}
}

func (a *AuthManager) loadState() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	data, err := os.ReadFile(a.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			a.authKey = mustRandomBytes(authKeySize)
			a.challenges = make(map[string]*authChallengeState)
			a.sessions = make(map[string]*authSession)
			return a.saveStateLocked()
		}
		return err
	}

	var state authState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	authKey, err := base64.RawURLEncoding.DecodeString(state.AuthKey)
	if err != nil || len(authKey) == 0 {
		return fmt.Errorf("decode auth key: %w", err)
	}

	a.authKey = authKey
	a.challenges = make(map[string]*authChallengeState)
	a.sessions = state.Sessions
	if a.sessions == nil {
		a.sessions = make(map[string]*authSession)
	}
	a.pruneExpiredSessionsLocked(time.Now().Unix())
	return a.saveStateLocked()
}

func (a *AuthManager) saveStateLocked() error {
	state := authState{
		AuthKey:  base64.RawURLEncoding.EncodeToString(a.authKey),
		Sessions: a.sessions,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(a.statePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
	}

	tmpFile, err := os.CreateTemp(dir, "gotun-auth-*.tmp")
	if err != nil {
		return err
	}

	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, a.statePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func (a *AuthManager) IsEnabled() bool {
	return a.authEnabled
}

func (a *AuthManager) Realm() string {
	return a.realm
}

func (a *AuthManager) RequireAuth(wrapped http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authEnabled || a.IsAuthenticated(r) {
			wrapped(w, r)
			return
		}
		a.respond(w, authResponse{
			Code: http.StatusUnauthorized,
			Msg:  "unauthorized",
		}, http.StatusUnauthorized)
	}
}

func (a *AuthManager) RequirePageAuth(wrapped http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authEnabled || a.IsAuthenticated(r) {
			wrapped(w, r)
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

func (a *AuthManager) IsAuthenticated(r *http.Request) bool {
	if !a.authEnabled {
		return true
	}

	cookie, err := r.Cookie(authCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}

	now := time.Now().Unix()

	a.mu.Lock()
	defer a.mu.Unlock()

	session, ok := a.sessions[cookie.Value]
	if !ok {
		return false
	}
	if session.ExpiresAt <= now {
		delete(a.sessions, cookie.Value)
		if err := a.saveStateLocked(); err != nil {
			logrus.WithError(err).Warn("remove expired session failed")
		}
		return false
	}

	session.LastSeen = now
	return true
}

func (a *AuthManager) HandleChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respond(w, authResponse{Code: http.StatusMethodNotAllowed, Msg: "method not allowed"}, http.StatusMethodNotAllowed)
		return
	}
	if !a.authEnabled {
		a.respond(w, authResponse{Code: http.StatusBadRequest, Msg: "auth disabled"}, http.StatusBadRequest)
		return
	}

	ip := getIP(r)
	if a.isBlocked(ip) {
		a.respond(w, authResponse{Code: http.StatusTooManyRequests, Msg: "登录失败次数过多，请在1小时后重试"}, http.StatusTooManyRequests)
		return
	}

	var req authChallengeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respond(w, authResponse{Code: http.StatusBadRequest, Msg: "invalid request body"}, http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		a.respond(w, authResponse{Code: http.StatusBadRequest, Msg: "username required"}, http.StatusBadRequest)
		return
	}

	nonce, err := a.issueChallenge(req.Username)
	if err != nil {
		logrus.WithError(err).Warn("issue challenge failed")
		a.respond(w, authResponse{Code: http.StatusInternalServerError, Msg: "issue challenge failed"}, http.StatusInternalServerError)
		return
	}

	a.respond(w, authResponse{
		Code: http.StatusOK,
		Msg:  "ok",
		Data: map[string]string{
			"realm":     a.realm,
			"nonce":     nonce,
			"algorithm": authAlgorithm,
		},
	}, http.StatusOK)
}

func (a *AuthManager) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respond(w, authResponse{Code: http.StatusMethodNotAllowed, Msg: "method not allowed"}, http.StatusMethodNotAllowed)
		return
	}
	if !a.authEnabled {
		a.respond(w, authResponse{Code: http.StatusBadRequest, Msg: "auth disabled"}, http.StatusBadRequest)
		return
	}

	ip := getIP(r)
	if a.isBlocked(ip) {
		a.respond(w, authResponse{Code: http.StatusTooManyRequests, Msg: "登录失败次数过多，请在1小时后重试"}, http.StatusTooManyRequests)
		return
	}

	var req authLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respond(w, authResponse{Code: http.StatusBadRequest, Msg: "invalid request body"}, http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Nonce = strings.TrimSpace(req.Nonce)
	req.Proof = strings.TrimSpace(strings.ToLower(req.Proof))
	if req.Username == "" || req.Nonce == "" || req.Proof == "" {
		a.respond(w, authResponse{Code: http.StatusBadRequest, Msg: "username, nonce and proof are required"}, http.StatusBadRequest)
		return
	}

	if req.Username != a.username {
		a.recordAuthFailure(ip)
		a.respond(w, authResponse{Code: http.StatusUnauthorized, Msg: "invalid credentials"}, http.StatusUnauthorized)
		return
	}

	if _, err := a.consumeChallenge(req.Username, req.Nonce); err != nil {
		a.recordAuthFailure(ip)
		a.respond(w, authResponse{Code: http.StatusUnauthorized, Msg: "invalid or expired challenge"}, http.StatusUnauthorized)
		return
	}

	expectedProof := a.signProof(deriveUserKey(a.username, a.realm, a.password), req.Nonce)
	proofBytes, err := hex.DecodeString(req.Proof)
	if err != nil || !hmac.Equal(proofBytes, expectedProof) {
		a.recordAuthFailure(ip)
		logrus.WithFields(logrus.Fields{
			"ip":       ip,
			"url_path": r.URL.Path,
		}).Info("login failed")
		a.respond(w, authResponse{Code: http.StatusUnauthorized, Msg: "invalid credentials"}, http.StatusUnauthorized)
		return
	}

	session, err := a.createSession(req.Username)
	if err != nil {
		logrus.WithError(err).Warn("create auth session failed")
		a.respond(w, authResponse{Code: http.StatusInternalServerError, Msg: "create session failed"}, http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   authCookieMaxAge,
		Expires:  time.Unix(session.ExpiresAt, 0),
	})

	a.onAuthSuccess(ip, r.URL.Path)
	a.respond(w, authResponse{
		Code: http.StatusOK,
		Msg:  "ok",
		Data: map[string]string{
			"username": session.Username,
		},
	}, http.StatusOK)
}

func (a *AuthManager) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respond(w, authResponse{Code: http.StatusMethodNotAllowed, Msg: "method not allowed"}, http.StatusMethodNotAllowed)
		return
	}

	if cookie, err := r.Cookie(authCookieName); err == nil && cookie.Value != "" {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		if err := a.saveStateLocked(); err != nil {
			logrus.WithError(err).Warn("persist logout state failed")
		}
		a.mu.Unlock()
	}

	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})

	a.respond(w, authResponse{Code: http.StatusOK, Msg: "ok"}, http.StatusOK)
}

func (a *AuthManager) HandleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respond(w, authResponse{Code: http.StatusMethodNotAllowed, Msg: "method not allowed"}, http.StatusMethodNotAllowed)
		return
	}

	if !a.authEnabled {
		a.respond(w, authResponse{
			Code: http.StatusOK,
			Msg:  "ok",
			Data: map[string]interface{}{
				"authenticated": true,
				"username":      "",
			},
		}, http.StatusOK)
		return
	}

	username := ""
	authenticated := false
	if cookie, err := r.Cookie(authCookieName); err == nil && cookie.Value != "" {
		now := time.Now().Unix()
		a.mu.Lock()
		if session, ok := a.sessions[cookie.Value]; ok {
			if session.ExpiresAt > now {
				session.LastSeen = now
				authenticated = true
				username = session.Username
			} else {
				delete(a.sessions, cookie.Value)
				if err := a.saveStateLocked(); err != nil {
					logrus.WithError(err).Warn("persist expired session removal failed")
				}
			}
		}
		a.mu.Unlock()
	}

	a.respond(w, authResponse{
		Code: http.StatusOK,
		Msg:  "ok",
		Data: map[string]interface{}{
			"authenticated": authenticated,
			"username":      username,
		},
	}, http.StatusOK)
}

func (a *AuthManager) issueChallenge(username string) (string, error) {
	if !a.authEnabled {
		return "", fmt.Errorf("auth disabled")
	}

	now := time.Now()
	challengeID := randomToken(authChallengeRandSize)
	payload := signedChallenge{
		ID:        challengeID,
		Username:  username,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(authChallengeTTL).Unix(),
		Random:    randomToken(authChallengeRandSize),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	a.mu.Lock()
	a.pruneExpiredChallengesLocked(now.Unix())
	a.challenges[challengeID] = &authChallengeState{
		ID:        challengeID,
		Username:  username,
		ExpiresAt: payload.ExpiresAt,
	}
	signature := signWithKey(a.authKey, body)
	a.mu.Unlock()

	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (a *AuthManager) consumeChallenge(username string, nonce string) (*signedChallenge, error) {
	parts := strings.Split(nonce, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid nonce")
	}

	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now().Unix()
	a.pruneExpiredChallengesLocked(now)
	expected := signWithKey(a.authKey, body)
	if !hmac.Equal(signature, expected) {
		return nil, fmt.Errorf("invalid signature")
	}

	var payload signedChallenge
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Username != username {
		return nil, fmt.Errorf("username mismatch")
	}
	if payload.IssuedAt > now || payload.ExpiresAt <= now {
		return nil, fmt.Errorf("challenge expired")
	}
	if payload.ID == "" {
		return nil, fmt.Errorf("challenge id missing")
	}

	challenge, ok := a.challenges[payload.ID]
	if !ok {
		return nil, fmt.Errorf("challenge not found")
	}
	if challenge.Username != username || challenge.ExpiresAt <= now {
		delete(a.challenges, payload.ID)
		return nil, fmt.Errorf("challenge expired")
	}

	delete(a.challenges, payload.ID)

	return &payload, nil
}

func (a *AuthManager) createSession(username string) (*authSession, error) {
	now := time.Now().Unix()
	session := &authSession{
		ID:        randomToken(authSessionRandSize),
		Username:  username,
		IssuedAt:  now,
		ExpiresAt: now + int64(authSessionTTL/time.Second),
		LastSeen:  now,
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.pruneExpiredSessionsLocked(now)
	a.sessions[session.ID] = session
	if err := a.saveStateLocked(); err != nil {
		delete(a.sessions, session.ID)
		return nil, err
	}

	return session, nil
}

func (a *AuthManager) pruneExpiredSessionsLocked(now int64) {
	for id, session := range a.sessions {
		if session == nil || session.ExpiresAt <= now {
			delete(a.sessions, id)
		}
	}
}

func (a *AuthManager) pruneExpiredChallengesLocked(now int64) {
	for id, challenge := range a.challenges {
		if challenge == nil || challenge.ExpiresAt <= now {
			delete(a.challenges, id)
		}
	}
}

func (a *AuthManager) isBlocked(ip string) bool {
	if a.maxFailedAttempts <= 0 {
		return false
	}

	attempts, _ := a.failedAttempts.LoadOrStore(ip, 0)
	return attempts >= a.maxFailedAttempts
}

func (a *AuthManager) recordAuthFailure(ip string) {
	if a.maxFailedAttempts <= 0 {
		return
	}

	attempts, _ := a.failedAttempts.LoadOrStore(ip, 0)
	attempts++
	a.failedAttempts.Store(ip, attempts)

	if attempts >= a.maxFailedAttempts {
		a.cancelTimerIfExists(ip)
		timer := time.AfterFunc(resetDuration, func() {
			a.failedAttempts.Delete(ip)
			a.resetTimers.Delete(ip)
		})
		a.resetTimers.Store(ip, timer)
	}
}

func (a *AuthManager) onAuthSuccess(ip string, path string) {
	logrus.WithFields(logrus.Fields{
		"ip":       ip,
		"url_path": path,
	}).Info("login operate")

	a.failedAttempts.Delete(ip)
	a.cancelTimerIfExists(ip)
}

func (a *AuthManager) cancelTimerIfExists(ip string) {
	if timer, ok := a.resetTimers.Load(ip); ok {
		timer.Stop()
		a.resetTimers.Delete(ip)
	}
}

func (a *AuthManager) signProof(userKey []byte, nonce string) []byte {
	mac := hmac.New(sha256.New, userKey)
	_, _ = mac.Write([]byte(nonce))
	return mac.Sum(nil)
}

func (a *AuthManager) respond(w http.ResponseWriter, data authResponse, status int) {
	w.Header().Set("Content-Type", authContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func deriveUserKey(username string, realm string, password string) []byte {
	sum := sha256.Sum256([]byte(username + ":" + realm + ":" + password))
	return sum[:]
}

func signWithKey(key []byte, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func randomToken(size int) string {
	return base64.RawURLEncoding.EncodeToString(mustRandomBytes(size))
}

func mustRandomBytes(size int) []byte {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return buf
}

func getIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return ip
}
