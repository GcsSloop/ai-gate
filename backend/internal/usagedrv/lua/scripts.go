package lua

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var managedScriptKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type ManagedScriptStore struct {
	root string
}

func NewManagedScriptStore(root string) (*ManagedScriptStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("managed script root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve managed script root: %w", err)
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir managed script root: %w", err)
	}
	return &ManagedScriptStore{root: absRoot}, nil
}

func (s *ManagedScriptStore) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *ManagedScriptStore) Save(key string, content string) error {
	if s == nil {
		return fmt.Errorf("managed script store is not configured")
	}
	path, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		return fmt.Errorf("save managed script %q: %w", key, err)
	}
	return nil
}

func (s *ManagedScriptStore) Load(key string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("managed script store is not configured")
	}
	path, err := s.resolvePath(key)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("load managed script %q: %w", key, err)
	}
	return string(raw), nil
}

func (s *ManagedScriptStore) PathForKey(key string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("managed script store is not configured")
	}
	return s.resolvePath(key)
}

func (s *ManagedScriptStore) List() ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("managed script store is not configured")
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("list managed scripts: %w", err)
	}
	items := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".lua") {
			continue
		}
		key := strings.TrimSuffix(name, ".lua")
		if managedScriptKeyPattern.MatchString(key) {
			items = append(items, key)
		}
	}
	sort.Strings(items)
	return items, nil
}

func ParseManagedScriptKey(script string) (string, bool) {
	value := strings.TrimSpace(script)
	if !strings.HasPrefix(value, "managed:") {
		return "", false
	}
	key := strings.TrimSpace(strings.TrimPrefix(value, "managed:"))
	if !managedScriptKeyPattern.MatchString(key) {
		return "", false
	}
	return key, true
}

func (s *ManagedScriptStore) resolvePath(key string) (string, error) {
	key = strings.TrimSpace(key)
	if !managedScriptKeyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid managed script key %q", key)
	}
	return filepath.Join(s.root, key+".lua"), nil
}
