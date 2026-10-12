package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"strings"
	"unicode/utf8"
)

const MaxDocumentBytes = 8 << 20

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string        { return e.Message }
func Fail(code, message string) error { return &Error{code, message} }
func ErrorInfo(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{"io_error", "ファイル操作に失敗しました"}
}

type Key struct {
	Project string `json:"project"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
}
type Document struct {
	Key
	Content  string `json:"content"`
	Revision string `json:"revision"`
}
type Page struct {
	Paths []string `json:"paths"`
	Next  string   `json:"next,omitempty"`
}

func Revision(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
func ValidateProject(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") || strings.HasPrefix(name, ".fragment") || !utf8.ValidString(name) {
		return Fail("invalid_input", "プロジェクト名は予約名を除く一つのフォルダ名にしてください")
	}
	return nil
}
func (k Key) Validate() error {
	if err := ValidateProject(k.Project); err != nil {
		return err
	}
	if k.Kind != "flow" && k.Kind != "stock" {
		return Fail("invalid_input", "kindはflowまたはstockにしてください")
	}
	if k.Path == "" || path.IsAbs(k.Path) || path.Clean(k.Path) != k.Path || strings.ContainsAny(k.Path, "\\\x00") || !utf8.ValidString(k.Path) || !strings.HasSuffix(k.Path, ".md") {
		return Fail("invalid_input", "文書は正規化された相対パスの.mdファイルにしてください")
	}
	for _, part := range strings.Split(k.Path, "/") {
		if part == ".." || part == "." || strings.HasPrefix(part, ".fragment") {
			return Fail("invalid_input", "予約領域や親フォルダは指定できません")
		}
	}
	return nil
}

type Repository interface {
	Projects(context.Context) ([]string, error)
	CreateProject(context.Context, string) error
	Documents(context.Context, string, string) ([]string, error)
	Read(context.Context, Key) (Document, error)
	Write(context.Context, Key, string, *string) (Document, error)
}
type Service struct{ Repo Repository }

func (s *Service) Projects(ctx context.Context) ([]string, error) { return s.Repo.Projects(ctx) }
func (s *Service) CreateProject(ctx context.Context, name string) error {
	if err := ValidateProject(name); err != nil {
		return err
	}
	return s.Repo.CreateProject(ctx, name)
}
func (s *Service) Documents(ctx context.Context, project, kind, after string, limit int) (Page, error) {
	if err := (Key{project, kind, "check.md"}).Validate(); err != nil {
		return Page{}, err
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 1000 {
		return Page{}, Fail("invalid_input", "limitは1から1000です")
	}
	paths, err := s.Repo.Documents(ctx, project, kind)
	if err != nil {
		return Page{}, err
	}
	result := Page{Paths: []string{}}
	for _, p := range paths {
		if p <= after {
			continue
		}
		if len(result.Paths) == limit {
			result.Next = result.Paths[len(result.Paths)-1]
			break
		}
		result.Paths = append(result.Paths, p)
	}
	return result, nil
}
func (s *Service) Get(ctx context.Context, k Key) (Document, error) {
	if err := k.Validate(); err != nil {
		return Document{}, err
	}
	return s.Repo.Read(ctx, k)
}
func (s *Service) Create(ctx context.Context, k Key, content string) (Document, error) {
	return s.write(ctx, k, content, nil)
}
func (s *Service) Update(ctx context.Context, k Key, content, revision string) (Document, error) {
	if len(revision) != 64 {
		return Document{}, Fail("invalid_input", "取得時のリビジョンが必要です")
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return Document{}, Fail("invalid_input", "リビジョンが不正です")
	}
	return s.write(ctx, k, content, &revision)
}
func (s *Service) write(ctx context.Context, k Key, content string, revision *string) (Document, error) {
	if err := k.Validate(); err != nil {
		return Document{}, err
	}
	if !utf8.ValidString(content) {
		return Document{}, Fail("invalid_input", "本文はUTF-8にしてください")
	}
	if len(content) > MaxDocumentBytes {
		return Document{}, Fail("too_large", "本文の上限は8MiBです")
	}
	return s.Repo.Write(ctx, k, content, revision)
}
