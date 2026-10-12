package storage

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/s4na/fragment/internal/app"
)

type Store struct {
	Root        string
	LockTimeout time.Duration
	renameFn    func(string, string) error
	syncFn      func(string) error
}

func Open(root string) (*Store, error) {
	if !filepath.IsAbs(root) {
		return nil, app.Fail("config_error", "保存先には絶対パスを設定してください")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, app.Fail("config_error", "保存先が見つかりません")
	}
	s := &Store{Root: resolved, LockTimeout: 5 * time.Second, renameFn: os.Rename, syncFn: syncDir}
	if _, err = s.checked(".fragment/write.lock", false); err != nil {
		return nil, app.Fail("config_error", "保存先をconfig initで初期化してください")
	}
	return s, nil
}
func Init(root string) (*Store, error) {
	if !filepath.IsAbs(root) {
		return nil, app.Fail("config_error", "保存先には絶対パスを設定してください")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	s := &Store{Root: root, LockTimeout: 5 * time.Second, renameFn: os.Rename, syncFn: syncDir}
	if _, err = s.checked(".fragment/write.lock", true); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Join(root, ".fragment"), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(root, ".fragment/write.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	return Open(root)
}
func (s *Store) checked(rel string, missing bool) (string, error) {
	if !filepath.IsLocal(rel) {
		return "", app.Fail("invalid_input", "保存先の外は参照できません")
	}
	current := s.Root
	for _, part := range strings.Split(filepath.FromSlash(rel), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) && missing {
			continue
		}
		if err != nil {
			return "", mapError(err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", app.Fail("invalid_input", "シンボリックリンクは利用できません")
		}
	}
	return current, nil
}
func mapError(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return app.Fail("not_found", "対象が見つかりません")
	}
	return app.ErrorInfo(err)
}
func (s *Store) project(name string) error {
	for _, kind := range []string{"flow", "stock"} {
		p, err := s.checked(filepath.Join(name, kind), false)
		if err != nil {
			return err
		}
		info, err := os.Stat(p)
		if err != nil {
			return mapError(err)
		}
		if !info.IsDir() {
			return app.Fail("config_error", "flowとstockの構成が不正です")
		}
	}
	return nil
}
func (s *Store) Projects(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, mapError(err)
	}
	names := []string{}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if strings.HasPrefix(e.Name(), ".fragment") {
			continue
		}
		if e.Type()&os.ModeSymlink != 0 {
			return nil, app.Fail("invalid_input", "保存先内にシンボリックリンクがあります")
		}
		if !e.IsDir() {
			continue
		}
		if err = s.project(e.Name()); err != nil {
			return nil, app.Fail("config_error", "プロジェクトのflow/stockが不正です: "+e.Name())
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}
func (s *Store) CreateProject(ctx context.Context, name string) error {
	unlock, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	target, err := s.checked(name, true)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return mapError(err)
		}
		return app.Fail("already_exists", "同名のパスが存在します")
	}
	stage, err := os.MkdirTemp(filepath.Join(s.Root, ".fragment"), "project-")
	if err != nil {
		return mapError(err)
	}
	defer os.RemoveAll(stage)
	for _, kind := range []string{"flow", "stock"} {
		p := filepath.Join(stage, kind)
		if err = os.Mkdir(p, 0700); err != nil {
			return mapError(err)
		}
		if err = s.syncFn(p); err != nil {
			return mapError(err)
		}
	}
	if err = s.syncFn(stage); err != nil {
		return mapError(err)
	}
	if err = s.renameFn(stage, target); err != nil {
		return mapError(err)
	}
	if err = s.syncFn(s.Root); err != nil {
		return app.Fail("commit_unknown", "作成結果を再取得して確認してください")
	}
	return nil
}
func (s *Store) Documents(ctx context.Context, project, kind string) ([]string, error) {
	if err := s.project(project); err != nil {
		return nil, err
	}
	base, err := s.checked(filepath.Join(project, kind), false)
	if err != nil {
		return nil, err
	}
	paths := []string{}
	err = filepath.WalkDir(base, func(p string, e fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if p == base {
			return nil
		}
		if strings.HasPrefix(e.Name(), ".fragment") {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.Type()&os.ModeSymlink != 0 {
			return app.Fail("invalid_input", "文書フォルダにシンボリックリンクがあります")
		}
		if e.IsDir() {
			return nil
		}
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".md") {
			rel, err := filepath.Rel(base, p)
			if err != nil {
				return err
			}
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, mapError(err)
	}
	sort.Strings(paths)
	return paths, nil
}
func (s *Store) Read(ctx context.Context, k app.Key) (app.Document, error) {
	if err := ctx.Err(); err != nil {
		return app.Document{}, err
	}
	if err := s.project(k.Project); err != nil {
		return app.Document{}, err
	}
	p, err := s.checked(filepath.Join(k.Project, k.Kind, filepath.FromSlash(k.Path)), false)
	if err != nil {
		return app.Document{}, err
	}
	info, err := os.Stat(p)
	if err != nil {
		return app.Document{}, mapError(err)
	}
	if !info.Mode().IsRegular() {
		return app.Document{}, app.Fail("invalid_input", "通常のファイルだけ利用できます")
	}
	f, err := os.Open(p)
	if err != nil {
		return app.Document{}, mapError(err)
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, app.MaxDocumentBytes+1))
	if err != nil {
		return app.Document{}, mapError(err)
	}
	if len(body) > app.MaxDocumentBytes {
		return app.Document{}, app.Fail("too_large", "本文の上限は8MiBです")
	}
	if !utf8.Valid(body) {
		return app.Document{}, app.Fail("invalid_input", "本文はUTF-8にしてください")
	}
	return app.Document{Key: k, Content: string(body), Revision: app.Revision(body)}, nil
}
func (s *Store) Write(ctx context.Context, k app.Key, content string, revision *string) (app.Document, error) {
	unlock, err := s.lock(ctx)
	if err != nil {
		return app.Document{}, err
	}
	defer unlock()
	if err = s.project(k.Project); err != nil {
		return app.Document{}, err
	}
	target, err := s.checked(filepath.Join(k.Project, k.Kind, filepath.FromSlash(k.Path)), true)
	if err != nil {
		return app.Document{}, err
	}
	if revision == nil {
		if _, err = os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
			if err != nil {
				return app.Document{}, mapError(err)
			}
			return app.Document{}, app.Fail("already_exists", "文書が存在します")
		}
	} else {
		doc, err := s.Read(ctx, k)
		if err != nil {
			return app.Document{}, err
		}
		if doc.Revision != *revision {
			return app.Document{}, app.Fail("conflict", "文書が更新されています。最新データを再取得し、既存の内容を確認して変更案を作り直し、リトライしてください")
		}
	}
	parent := filepath.Dir(target)
	if err = os.MkdirAll(parent, 0700); err != nil {
		return app.Document{}, mapError(err)
	}
	f, err := os.CreateTemp(parent, ".fragment-tmp-")
	if err != nil {
		return app.Document{}, mapError(err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.WriteString(content); err != nil {
		f.Close()
		return app.Document{}, mapError(err)
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return app.Document{}, mapError(err)
	}
	if err = f.Close(); err != nil {
		return app.Document{}, mapError(err)
	}
	if err = s.renameFn(tmp, target); err != nil {
		return app.Document{}, mapError(err)
	}
	if err = s.syncFn(parent); err != nil {
		return app.Document{}, app.Fail("commit_unknown", "保存された可能性があります。本文を再取得してください")
	}
	return app.Document{Key: k, Content: content, Revision: app.Revision([]byte(content))}, nil
}
func syncDir(p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
