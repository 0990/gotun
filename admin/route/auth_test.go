package route

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAuthLoginFlowAndPersistence(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config", "app.yaml")
	manager := NewAuthManager(configPath, "admin", "secret", 3)

	nonce := issueTestChallenge(t, manager, "admin", "127.0.0.1:12345")
	cookie := loginWithProof(t, manager, "admin", "secret", nonce, "127.0.0.1:12345")
	loginMustFail(t, manager, "admin", "secret", nonce, "127.0.0.1:12345", http.StatusUnauthorized)

	checkSessionAuthenticated(t, manager, cookie, true, "admin")

	reloaded := NewAuthManager(configPath, "admin", "secret", 3)
	checkSessionAuthenticated(t, reloaded, cookie, true, "admin")
	loginMustFail(t, reloaded, "admin", "secret", nonce, "127.0.0.1:12345", http.StatusUnauthorized)

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(cookie)
	logoutReq.RemoteAddr = "127.0.0.1:12345"
	logoutResp := httptest.NewRecorder()
	reloaded.HandleLogout(logoutResp, logoutReq)
	if logoutResp.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want %d", logoutResp.Code, http.StatusOK)
	}

	checkSessionAuthenticated(t, reloaded, cookie, false, "")
}

func TestAuthLoginLockout(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "app.yaml")
	manager := NewAuthManager(configPath, "admin", "secret", 1)

	nonce := issueTestChallenge(t, manager, "admin", "127.0.0.1:20001")
	body, _ := json.Marshal(authLoginRequest{
		Username: "admin",
		Nonce:    nonce,
		Proof:    stringsOf('0', 64),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:20001"
	resp := httptest.NewRecorder()
	manager.HandleLogin(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("first login status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}

	challengeReqBody, _ := json.Marshal(authChallengeRequest{Username: "admin"})
	challengeReq := httptest.NewRequest(http.MethodPost, "/api/auth/challenge", bytes.NewReader(challengeReqBody))
	challengeReq.RemoteAddr = "127.0.0.1:20001"
	challengeResp := httptest.NewRecorder()
	manager.HandleChallenge(challengeResp, challengeReq)
	if challengeResp.Code != http.StatusTooManyRequests {
		t.Fatalf("challenge after lockout status = %d, want %d", challengeResp.Code, http.StatusTooManyRequests)
	}
}

func TestAuthChallengeConsumedOnFailedLogin(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "app.yaml")
	manager := NewAuthManager(configPath, "admin", "secret", 3)

	nonce := issueTestChallenge(t, manager, "admin", "127.0.0.1:22001")
	loginMustFail(t, manager, "admin", "wrong-secret", nonce, "127.0.0.1:22001", http.StatusUnauthorized)
	loginMustFail(t, manager, "admin", "secret", nonce, "127.0.0.1:22001", http.StatusUnauthorized)
}

func TestAuthChallengeSingleUseUnderConcurrency(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "app.yaml")
	manager := NewAuthManager(configPath, "admin", "secret", 3)

	nonce := issueTestChallenge(t, manager, "admin", "127.0.0.1:24001")
	proof := hex.EncodeToString(manager.signProof(deriveUserKey("admin", manager.Realm(), "secret"), nonce))

	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, _ := json.Marshal(authLoginRequest{
				Username: "admin",
				Nonce:    nonce,
				Proof:    proof,
			})
			req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
			req.RemoteAddr = "127.0.0.1:24001"
			resp := httptest.NewRecorder()
			manager.HandleLogin(resp, req)
			statuses <- resp.Code
		}()
	}
	wg.Wait()
	close(statuses)

	successCount := 0
	failCount := 0
	for status := range statuses {
		switch status {
		case http.StatusOK:
			successCount++
		case http.StatusUnauthorized:
			failCount++
		default:
			t.Fatalf("unexpected status: %d", status)
		}
	}
	if successCount != 1 || failCount != 1 {
		t.Fatalf("success=%d fail=%d, want 1/1", successCount, failCount)
	}
}

func TestAuthExpiredChallengeRejected(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "app.yaml")
	manager := NewAuthManager(configPath, "admin", "secret", 3)

	now := time.Now()
	nonce := issueCustomChallenge(t, manager, "admin", now.Add(-3*time.Minute), now.Add(-2*time.Minute))
	loginMustFail(t, manager, "admin", "secret", nonce, "127.0.0.1:26001", http.StatusUnauthorized)
}

func TestAuthStateRecoveryFromBrokenFile(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "app.yaml")
	statePath := filepath.Join(configDir, authStateFileName)
	if err := os.WriteFile(statePath, []byte("{broken json"), 0o666); err != nil {
		t.Fatalf("write broken state: %v", err)
	}

	manager := NewAuthManager(configPath, "admin", "secret", 3)
	nonce := issueTestChallenge(t, manager, "admin", "127.0.0.1:33001")
	if nonce == "" {
		t.Fatal("nonce should not be empty after state recovery")
	}
}

func issueTestChallenge(t *testing.T, manager *AuthManager, username string, remoteAddr string) string {
	t.Helper()

	body, _ := json.Marshal(authChallengeRequest{Username: username})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/challenge", bytes.NewReader(body))
	req.RemoteAddr = remoteAddr
	resp := httptest.NewRecorder()
	manager.HandleChallenge(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("challenge status = %d, want %d", resp.Code, http.StatusOK)
	}

	payload := decodeAuthResponse(t, resp)
	data := payload.Data.(map[string]interface{})
	return data["nonce"].(string)
}

func loginWithProof(t *testing.T, manager *AuthManager, username string, password string, nonce string, remoteAddr string) *http.Cookie {
	t.Helper()

	proof := hex.EncodeToString(manager.signProof(deriveUserKey(username, manager.Realm(), password), nonce))
	body, _ := json.Marshal(authLoginRequest{
		Username: username,
		Nonce:    nonce,
		Proof:    proof,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.RemoteAddr = remoteAddr
	resp := httptest.NewRecorder()
	manager.HandleLogin(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", resp.Code, http.StatusOK)
	}

	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == authCookieName {
			return cookie
		}
	}
	t.Fatal("auth cookie not set")
	return nil
}

func loginMustFail(t *testing.T, manager *AuthManager, username string, password string, nonce string, remoteAddr string, wantStatus int) {
	t.Helper()

	proof := hex.EncodeToString(manager.signProof(deriveUserKey(username, manager.Realm(), password), nonce))
	body, _ := json.Marshal(authLoginRequest{
		Username: username,
		Nonce:    nonce,
		Proof:    proof,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.RemoteAddr = remoteAddr
	resp := httptest.NewRecorder()
	manager.HandleLogin(resp, req)
	if resp.Code != wantStatus {
		t.Fatalf("login status = %d, want %d", resp.Code, wantStatus)
	}
}

func checkSessionAuthenticated(t *testing.T, manager *AuthManager, cookie *http.Cookie, wantAuthenticated bool, wantUsername string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp := httptest.NewRecorder()
	manager.HandleSession(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("session status = %d, want %d", resp.Code, http.StatusOK)
	}

	payload := decodeAuthResponse(t, resp)
	data := payload.Data.(map[string]interface{})
	authenticated, _ := data["authenticated"].(bool)
	username, _ := data["username"].(string)
	if authenticated != wantAuthenticated {
		t.Fatalf("authenticated = %v, want %v", authenticated, wantAuthenticated)
	}
	if username != wantUsername {
		t.Fatalf("username = %q, want %q", username, wantUsername)
	}
}

func decodeAuthResponse(t *testing.T, resp *httptest.ResponseRecorder) authResponse {
	t.Helper()

	var payload authResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return payload
}

func issueCustomChallenge(t *testing.T, manager *AuthManager, username string, issuedAt time.Time, expiresAt time.Time) string {
	t.Helper()

	payload := signedChallenge{
		ID:        randomToken(authChallengeRandSize),
		Username:  username,
		IssuedAt:  issuedAt.Unix(),
		ExpiresAt: expiresAt.Unix(),
		Random:    randomToken(authChallengeRandSize),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal challenge: %v", err)
	}

	manager.mu.Lock()
	manager.challenges[payload.ID] = &authChallengeState{
		ID:        payload.ID,
		Username:  username,
		ExpiresAt: payload.ExpiresAt,
	}
	signature := signWithKey(manager.authKey, body)
	manager.mu.Unlock()

	return encodeChallenge(body, signature)
}

func encodeChallenge(body []byte, signature []byte) string {
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func stringsOf(ch rune, count int) string {
	buf := make([]rune, count)
	for i := 0; i < count; i++ {
		buf[i] = ch
	}
	return string(buf)
}
