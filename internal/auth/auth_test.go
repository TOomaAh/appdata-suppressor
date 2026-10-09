package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestHandler() (*Auth, http.Handler) {
	a := New("admin", "secret", time.Hour)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", a.HandleLogin)
	mux.HandleFunc("POST /api/logout", a.HandleLogout)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return a, a.Middleware(mux)
}

func login(h http.Handler, user, pass string) *httptest.ResponseRecorder {
	body := `{"user":"` + user + `","password":"` + pass + `"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func get(h http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUnauthenticated(t *testing.T) {
	_, h := newTestHandler()
	if rec := get(h, "/api/scan"); rec.Code != http.StatusUnauthorized {
		t.Errorf("API without session: got %d, want 401", rec.Code)
	}
	if rec := get(h, "/api/session"); rec.Code != http.StatusUnauthorized {
		t.Errorf("session without login: got %d, want 401", rec.Code)
	}
	// The UI itself is public; it shows its login form on 401.
	if rec := get(h, "/"); rec.Code != http.StatusOK {
		t.Errorf("UI without session: got %d, want 200", rec.Code)
	}
}

func TestLoginLogout(t *testing.T) {
	_, h := newTestHandler()
	rec := login(h, "admin", "secret")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("login: got %d, want 204", rec.Code)
	}
	cookie := rec.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Error("session cookie must be HttpOnly and SameSite=Strict")
	}
	if rec := get(h, "/api/scan", cookie); rec.Code != http.StatusOK {
		t.Errorf("API with session: got %d, want 200", rec.Code)
	}

	req := httptest.NewRequest("POST", "/api/logout", nil)
	req.AddCookie(cookie)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if rec := get(h, "/api/scan", cookie); rec.Code != http.StatusUnauthorized {
		t.Errorf("API after logout: got %d, want 401", rec.Code)
	}
}

func TestLoginRejectsNonJSON(t *testing.T) {
	_, h := newTestHandler()
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"user":"admin","password":"secret"}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("got %d, want 415", rec.Code)
	}
}

func TestLockout(t *testing.T) {
	_, h := newTestHandler()
	for i := range maxFailures {
		if rec := login(h, "admin", "wrong"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d, want 401", i, rec.Code)
		}
	}
	// Even the right password is refused while locked out.
	if rec := login(h, "admin", "secret"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("after %d failures: got %d, want 429", maxFailures, rec.Code)
	}
}

func TestExpiredSession(t *testing.T) {
	a, h := newTestHandler()
	cookie := login(h, "admin", "secret").Result().Cookies()[0]
	a.mu.Lock()
	a.sessions[cookie.Value] = time.Now().Add(-time.Second)
	a.mu.Unlock()
	if rec := get(h, "/api/scan", cookie); rec.Code != http.StatusUnauthorized {
		t.Errorf("expired session: got %d, want 401", rec.Code)
	}
}
