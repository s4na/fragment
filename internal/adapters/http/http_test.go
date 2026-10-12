package httpadapter

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/s4na/fragment/internal/app"
	"github.com/s4na/fragment/internal/storage"
	"github.com/s4na/fragment/web"
)

func TestSessionWriteAndRead(t *testing.T) {
	st, err := storage.Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	assets, err := fs.Sub(web.Assets, "dist")
	if err != nil {
		t.Fatal(err)
	}
	s := New(&app.Service{Repo: st}, st.Root, "http://127.0.0.1:8787", "127.0.0.1:8787", assets)
	var cookie *http.Cookie
	csrf := ""
	request := func(method, path string, body any, origin, token string) *httptest.ResponseRecorder {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, s.Origin+path, bytes.NewReader(b))
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		r.Header.Set("X-Fragment-CSRF", token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	w := request("GET", "/api/projects", nil, "", "")
	if w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("POST", "/api/session", map[string]string{"token": s.bootstrap}, s.Origin, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookie = w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal(cookie)
	}
	var session map[string]string
	if err = json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	csrf = session["csrf"]
	w = request("POST", "/api/projects", map[string]string{"project": "p"}, s.Origin, "")
	if w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	w = request("POST", "/api/projects", map[string]string{"project": "p"}, "http://evil.test", csrf)
	if w.Code != 403 {
		t.Fatal("external origin accepted")
	}
	w = request("POST", "/api/projects", map[string]string{"project": "p"}, s.Origin, csrf)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("POST", "/api/document?project=p&kind=stock&path=a.md", map[string]string{"content": "# Hello"}, s.Origin, csrf)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var doc app.Document
	if err = json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	w = request("GET", "/api/document?project=p&kind=stock&path=a.md", nil, "", "")
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("Hello")) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("PUT", "/api/document?project=p&kind=stock&path=a.md", map[string]string{"content": "new", "revision": doc.Revision}, s.Origin, csrf)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("PUT", "/api/document?project=p&kind=stock&path=a.md", map[string]string{"content": "stale", "revision": doc.Revision}, s.Origin, csrf)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("GET", "/api/session", nil, "", "")
	if w.Code != 200 {
		t.Fatal("session cannot be restored")
	}
	w = request("GET", "/", nil, "", "")
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("type=\"module\"")) {
		t.Fatal("compiled UI not served", w.Body.String())
	}
	r := httptest.NewRequest("GET", "http://evil.test/api/projects", nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("Host accepted")
	}
}

func TestSessionsAcrossPorts(t *testing.T) {
	st, err := storage.Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	assets, err := fs.Sub(web.Assets, "dist")
	if err != nil {
		t.Fatal(err)
	}
	one := New(&app.Service{Repo: st}, st.Root, "http://127.0.0.1:8787", "127.0.0.1:8787", assets)
	two := New(&app.Service{Repo: st}, st.Root, "http://127.0.0.1:8788", "127.0.0.1:8788", assets)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, server := range []*Server{one, two} {
		u, _ := url.Parse(server.Origin)
		body, _ := json.Marshal(map[string]string{"token": server.bootstrap})
		r := httptest.NewRequest("POST", server.Origin+"/api/session", bytes.NewReader(body))
		r.Header.Set("Origin", server.Origin)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		jar.SetCookies(u, w.Result().Cookies())
	}
	// A browser sends both cookies to either port; each server must find its own.
	for _, server := range []*Server{one, two} {
		u, _ := url.Parse(server.Origin)
		r := httptest.NewRequest("GET", server.Origin+"/api/state", nil)
		for _, cookie := range jar.Cookies(u) {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal("another port invalidated the session", w.Code, w.Body.String())
		}
	}
}
