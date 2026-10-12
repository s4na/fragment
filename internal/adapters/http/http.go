package httpadapter

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/s4na/fragment/internal/app"
	"github.com/s4na/fragment/internal/chat"
	"github.com/s4na/fragment/internal/render"
)

type Server struct {
	Service            *app.Service
	Root, Origin, Host string
	session, csrf      string
	cookieName         string
	Assets             fs.FS
	Chat               *chat.Manager
}

func secret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func New(s *app.Service, root, origin, host string, assets fs.FS) *Server {
	return &Server{Service: s, Root: root, Origin: origin, Host: host, session: secret(), csrf: secret(), cookieName: "fragment_session_" + app.Revision([]byte(origin))[:16], Assets: assets}
}

// SessionURL is stable and can be opened directly or bookmarked in any browser.
func (s *Server) SessionURL(project string) string {
	query := ""
	if project != "" {
		query = "?" + url.Values{"project": {project}}.Encode()
	}
	return s.Origin + "/" + query
}
func equal(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Cache-Control", "no-store")
	if r.Host != s.Host {
		writeError(w, app.Fail("forbidden", "Hostが一致しません"))
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != s.Origin {
		writeError(w, app.Fail("forbidden", "Originが一致しません"))
		return
	}
	isAPI := strings.HasPrefix(r.URL.Path, "/api/")
	if isAPI {
		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" || site == "same-site" {
			writeError(w, app.Fail("forbidden", "外部サイトからのアクセスは許可されません"))
			return
		}
	} else {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(405)
			return
		}
		http.FileServer(http.FS(s.Assets)).ServeHTTP(w, r)
		return
	}
	// The loopback URL itself is the entry point. Establish or renew the
	// browser session without requiring a launch token, including after restart.
	if r.URL.Path == "/api/session" {
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: s.cookieName, Value: s.session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		writeJSON(w, 200, map[string]string{"csrf": s.csrf})
		return
	}
	cookie, err := r.Cookie(s.cookieName)
	if err != nil || !equal(cookie.Value, s.session) {
		writeError(w, app.Fail("unauthorized", "ページを再読み込みして接続し直してください"))
		return
	}
	if r.Method != http.MethodGet {
		if r.Header.Get("Origin") != s.Origin || !equal(r.Header.Get("X-Fragment-CSRF"), s.csrf) {
			writeError(w, app.Fail("forbidden", "書き込みの認証に失敗しました"))
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	q := r.URL.Query()
	key := app.Key{Project: q.Get("project"), Kind: q.Get("kind"), Path: q.Get("path")}
	switch r.URL.Path {
	case "/api/chat":
		if s.Chat == nil {
			writeError(w, app.Fail("unavailable", "Codexチャットは利用できません"))
			return
		}
		switch r.Method {
		case http.MethodGet:
			value, e := s.Chat.State(q.Get("project"))
			if e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 200, value)
		case http.MethodPost:
			var in struct {
				Text string `json:"text"`
			}
			if e := decode(w, r, &in); e != nil {
				writeError(w, e)
				return
			}
			value, e := s.Chat.Send(ctx, q.Get("project"), in.Text)
			if e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 202, value)
		default:
			w.WriteHeader(405)
		}
	case "/api/state":
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		writeJSON(w, 200, map[string]string{"data_root": s.Root})
	case "/api/projects":
		switch r.Method {
		case http.MethodGet:
			projects, e := s.Service.Projects(ctx)
			if e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 200, map[string]any{"projects": projects})
		case http.MethodPost:
			var in struct {
				Project string `json:"project"`
			}
			if e := decode(w, r, &in); e != nil {
				writeError(w, e)
				return
			}
			if e := s.Service.CreateProject(ctx, in.Project); e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 201, in)
		default:
			w.WriteHeader(405)
		}
	case "/api/documents":
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		limit := 0
		if q.Has("limit") {
			limit, err = strconv.Atoi(q.Get("limit"))
			if err != nil {
				writeError(w, app.Fail("invalid_input", "limitが不正です"))
				return
			}
		}
		page, e := s.Service.Documents(ctx, key.Project, key.Kind, q.Get("after"), limit)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, page)
	case "/api/document":
		switch r.Method {
		case http.MethodGet:
			doc, e := s.Service.Get(ctx, key)
			if e != nil {
				writeError(w, e)
				return
			}
			html, e := render.HTML(key, doc.Content)
			if e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 200, struct {
				app.Document
				HTML string `json:"html"`
			}{doc, html})
		case http.MethodPost, http.MethodPut:
			var in struct {
				Content  string `json:"content"`
				Revision string `json:"revision"`
			}
			if e := decode(w, r, &in); e != nil {
				writeError(w, e)
				return
			}
			var doc app.Document
			var e error
			status := 200
			if r.Method == http.MethodPost {
				doc, e = s.Service.Create(ctx, key, in.Content)
				status = 201
			} else {
				doc, e = s.Service.Update(ctx, key, in.Content, in.Revision)
			}
			if e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, status, doc)
		default:
			w.WriteHeader(405)
		}
	case "/api/preview":
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		if e := key.Validate(); e != nil {
			writeError(w, e)
			return
		}
		var in struct {
			Content string `json:"content"`
		}
		if e := decode(w, r, &in); e != nil {
			writeError(w, e)
			return
		}
		if len(in.Content) > app.MaxDocumentBytes {
			writeError(w, app.Fail("too_large", "本文の上限は8MiBです"))
			return
		}
		html, e := render.HTML(key, in.Content)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, map[string]string{"html": html})
	default:
		http.NotFound(w, r)
	}
}
func decode(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 6*app.MaxDocumentBytes+1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return app.Fail("invalid_input", "JSON本文が不正または大きすぎます")
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return app.Fail("invalid_input", "JSON本文は一つだけ指定してください")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error) {
	e := app.ErrorInfo(err)
	status := 500
	switch e.Code {
	case "invalid_input":
		status = 400
	case "config_error", "conflict", "already_exists", "chat_busy", "reread_required":
		status = 409
	case "not_found":
		status = 404
	case "too_large":
		status = 413
	case "lock_timeout", "unavailable":
		status = 503
	case "unauthorized":
		status = 401
	case "forbidden":
		status = 403
	}
	response := map[string]any{"error": e}
	if e.Code == "conflict" || e.Code == "already_exists" || e.Code == "reread_required" {
		response["retry_required"] = true
		response["next_action"] = "get_document"
		response["message"] = "最新データを再取得し、既存の内容を確認して変更案を作り直し、リトライしてください。"
	}
	writeJSON(w, status, response)
}
