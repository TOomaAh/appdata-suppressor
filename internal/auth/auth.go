// Package auth provides a single-user password login with in-memory sessions.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	CookieName  = "appdata_session"
	maxFailures = 5
	lockout     = time.Minute
)

// publicPaths are reachable without a session.
var publicPaths = map[string]bool{
	"/login.html": true,
	"/login.js":   true,
	"/style.css":  true,
	"/api/login":  true,
}

type Auth struct {
	user, pass [32]byte // SHA-256 digests, compared in constant time
	ttl        time.Duration

	mu       sync.Mutex
	sessions map[string]time.Time // token -> expiry
	failures map[string]*attempts // client IP -> failed logins
}

type attempts struct {
	count       int
	last        time.Time
	lockedUntil time.Time
}

func New(user, password string, ttl time.Duration) *Auth {
	return &Auth{
		user:     sha256.Sum256([]byte(user)),
		pass:     sha256.Sum256([]byte(password)),
		ttl:      ttl,
		sessions: make(map[string]time.Time),
		failures: make(map[string]*attempts),
	}
}

// Middleware rejects requests without a valid session: API calls get a 401,
// pages are redirected to the login form.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPaths[r.URL.Path] || a.validSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		http.Redirect(w, r, "/login.html", http.StatusSeeOther)
	})
}

// HandleLogin checks the JSON credentials {"user", "password"} and opens a session.
func (a *Auth) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if wait := a.lockedFor(ip); wait > 0 {
		secs := int(wait.Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeError(w, http.StatusTooManyRequests, fmt.Sprintf("too many failed attempts, retry in %ds", secs))
		return
	}
	if !IsJSON(r) {
		writeError(w, http.StatusUnsupportedMediaType, "expected application/json")
		return
	}

	var creds struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if !a.checkCredentials(creds.User, creds.Password) {
		a.recordFailure(ip)
		log.Printf("failed login for %q from %s", creds.User, ip)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	a.clearFailures(ip)
	token, err := a.newSession()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("login from %s", ip)
	http.SetCookie(w, a.cookie(r, token, int(a.ttl.Seconds())))
	w.WriteHeader(http.StatusNoContent)
}

// HandleLogout closes the current session.
func (a *Auth) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, a.cookie(r, "", -1))
	w.WriteHeader(http.StatusNoContent)
}

// IsJSON reports whether the request body is declared as JSON. Requiring it on
// POST endpoints blocks cross-site form posts, which cannot set this type.
func IsJSON(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

func (a *Auth) cookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	}
}

func (a *Auth) checkCredentials(user, pass string) bool {
	u := sha256.Sum256([]byte(user))
	p := sha256.Sum256([]byte(pass))
	// Both comparisons always run so timing does not reveal which one failed.
	userOK := subtle.ConstantTimeCompare(u[:], a.user[:])
	passOK := subtle.ConstantTimeCompare(p[:], a.pass[:])
	return userOK&passOK == 1
}

func (a *Auth) validSession(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	expiry, ok := a.sessions[c.Value]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(a.sessions, c.Value)
		return false
	}
	return true
}

func (a *Auth) newSession() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := hex.EncodeToString(b)

	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for t, expiry := range a.sessions {
		if now.After(expiry) {
			delete(a.sessions, t)
		}
	}
	a.sessions[token] = now.Add(a.ttl)
	return token, nil
}

func (a *Auth) lockedFor(ip string) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if f := a.failures[ip]; f != nil {
		return max(time.Until(f.lockedUntil), 0)
	}
	return 0
}

func (a *Auth) recordFailure(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, f := range a.failures {
		if now.Sub(f.last) > lockout && now.After(f.lockedUntil) {
			delete(a.failures, k)
		}
	}
	f := a.failures[ip]
	if f == nil {
		f = &attempts{}
		a.failures[ip] = f
	}
	f.count++
	f.last = now
	if f.count >= maxFailures {
		f.count = 0
		f.lockedUntil = now.Add(lockout)
	}
}

func (a *Auth) clearFailures(ip string) {
	a.mu.Lock()
	delete(a.failures, ip)
	a.mu.Unlock()
}

// clientIP uses the TCP peer address; X-Forwarded-For is ignored since any
// client could forge it to dodge the lockout.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
