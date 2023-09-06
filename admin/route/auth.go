package route

import (
	"encoding/json"
	"github.com/0990/gotun/pkg/syncx"
	"github.com/0990/gotun/pkg/util"
	auth "github.com/abbot/go-http-auth"
	"github.com/sirupsen/logrus"
	"net"
	"net/http"
	"time"
)

const resetDuration = time.Hour

type AuthManager struct {
	failedAttempts syncx.Map[string, int]
	resetTimers    syncx.Map[string, *time.Timer]

	digestAuth *auth.DigestAuth //nil表示不需要验证

	maxFailedAttempts int
}

func NewAuthManager(username string, password string, maxFailedAttempts int) *AuthManager {
	var digestAuth *auth.DigestAuth
	if username != "" && password != "" {
		realm := "example.com"
		secret := func(user, realm string) string {
			if user == username {
				return util.MD5(username + ":" + realm + ":" + password)
			}
			return ""
		}
		digestAuth = auth.NewDigestAuthenticator(realm, secret)
	}

	return &AuthManager{
		digestAuth:        digestAuth,
		maxFailedAttempts: maxFailedAttempts + 1,
	}
}

func (l *AuthManager) JustCheck(wrapped http.HandlerFunc) http.HandlerFunc {
	type Response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if l.digestAuth == nil {
			wrapped(w, r)
			return
		}

		ip := getIP(r)

		attempts, _ := l.failedAttempts.LoadOrStore(ip, 0)

		if attempts >= l.maxFailedAttempts {
			l.respond(w, r, &Response{
				Code: http.StatusTooManyRequests,
				Msg:  "登录失败次数过多，请在1小时后重试",
			}, http.StatusOK)
			logrus.WithField("ip", ip).Warn("login failed,too  many times")
			return
		}

		digestAuthCheck(l.digestAuth, l, wrapped)(w, r)
	}
}

func getIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return ip
}

func (s *AuthManager) respond(w http.ResponseWriter, r *http.Request, data interface{}, status int) {
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func (a *AuthManager) onDigestAuthSuccess(w http.ResponseWriter, r *http.Request) {

	ip := getIP(r)
	logrus.WithFields(logrus.Fields{
		"ip":       ip,
		"url_path": r.URL.Path,
	}).Info("login operate")

	a.failedAttempts.Delete(ip)
	a.cancelTimerIfExists(ip)
}

func (a *AuthManager) OnDigestAuthFail(w http.ResponseWriter, r *http.Request) {
	ip := getIP(r)

	auth := auth.DigestAuthParams(r.Header.Get("Authorization"))

	if auth == nil || a.digestAuth.Opaque != auth["opaque"] {
		return
	}

	attempts, _ := a.failedAttempts.LoadOrStore(ip, 0)

	logrus.WithFields(logrus.Fields{
		"ip":       ip,
		"url_path": r.URL.Path,
	}).Info("login failed")

	a.failedAttempts.Store(ip, attempts+1)

	if remaining := a.maxFailedAttempts - attempts - 1; remaining > 0 {
		//fmt.Fprintf(w, "登录失败，您还有%d次尝试机会", remaining)
	} else {
		//fmt.Fprintf(w, "登录失败，您已达到尝试次数上限，请在1小时后重试")
		a.cancelTimerIfExists(ip)
		timer := time.AfterFunc(resetDuration, func() {
			a.failedAttempts.Delete(ip)
			a.resetTimers.Delete(ip)
		})
		a.resetTimers.Store(ip, timer)
	}
}

func (a *AuthManager) cancelTimerIfExists(ip string) {
	if timer, ok := a.resetTimers.Load(ip); ok {
		timer.Stop()
		a.resetTimers.Delete(ip)
	}
}

func digestAuthCheck(a *auth.DigestAuth, authMgr *AuthManager, wrapped http.HandlerFunc) http.HandlerFunc {
	return digestAuthWrap(a, authMgr, func(w http.ResponseWriter, ar *auth.AuthenticatedRequest) {
		ar.Header.Set(auth.AuthUsernameHeader, ar.Username)
		wrapped(w, &ar.Request)
	})
}

func digestAuthWrap(a *auth.DigestAuth, authMgr *AuthManager, wrapped auth.AuthenticatedHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if username, authinfo := a.CheckAuth(r); username == "" {
			authMgr.OnDigestAuthFail(w, r)
			a.RequireAuth(w, r)
		} else {
			authMgr.onDigestAuthSuccess(w, r)
			ar := &auth.AuthenticatedRequest{Request: *r, Username: username}
			if authinfo != nil {
				w.Header().Set(a.Headers.V().AuthInfo, *authinfo)
			}
			wrapped(w, ar)
		}
	}
}
