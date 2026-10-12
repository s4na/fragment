package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/s4na/fragment/internal/app"
	"github.com/s4na/fragment/internal/storage"
)

type Config struct {
	DataRoot string `json:"data_root"`
}

func Path() (string, error) {
	dir := os.Getenv("FRAGMENT_CONFIG_DIR")
	if dir != "" {
		if !filepath.IsAbs(dir) {
			return "", app.Fail("config_error", "FRAGMENT_CONFIG_DIRには絶対パスを指定してください")
		}
		return filepath.Join(dir, "config.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fragment/config.json"), nil
}
func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Config{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Config{}, app.Fail("config_error", "fragment config init <保存先の絶対パス> を実行してください")
	}
	var c Config
	if err = json.Unmarshal(b, &c); err != nil || !filepath.IsAbs(c.DataRoot) {
		return c, app.Fail("config_error", "保存先の設定が不正です")
	}
	return c, nil
}
func Init(root string) (Config, error) {
	s, err := storage.Init(root)
	if err != nil {
		return Config{}, err
	}
	c := Config{DataRoot: s.Root}
	p, err := Path()
	if err != nil {
		return c, err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return c, err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return c, err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".config-")
	if err != nil {
		return c, err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return c, err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return c, err
	}
	if err = f.Close(); err != nil {
		return c, err
	}
	if err = os.Rename(f.Name(), p); err != nil {
		return c, err
	}
	return c, nil
}
