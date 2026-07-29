// Package store persists small namespaced JSON documents (settings, saved
// devices, snippets) as one file per namespace.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// PortableMarker sits next to the executable to force data to live beside it
// (USB stick workflow) instead of in the OS config directory.
const PortableMarker = "portable.txt"

const appDirName = "chit"

type Store struct {
	dir      string
	portable bool
}

// New picks the data directory (portable next to the exe, otherwise the OS
// config dir) and creates it.
func New() (*Store, error) {
	dir, portable, err := dataDir()
	if err != nil {
		return nil, err
	}
	return NewAt(dir, portable)
}

// NewAt uses an explicit directory. Used by tests and by settings changes.
func NewAt(dir string, portable bool) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("Could not create the data folder %s. Check that you have permission to write there.", dir)
	}
	return &Store{dir: dir, portable: portable}, nil
}

// Dir is the folder holding the JSON files. Shown in Settings so a tech can
// find, back up or copy the data.
func (s *Store) Dir() string { return s.dir }

// Portable reports whether the portable marker file was found.
func (s *Store) Portable() bool { return s.portable }

// Get returns the raw JSON document for a namespace, or "" if it does not exist.
func (s *Store) Get(namespace string) (string, error) {
	if err := validNamespace(namespace); err != nil {
		return "", err
	}
	data, err := os.ReadFile(s.path(namespace))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("Could not read the saved %s data. The file may be open in another program.", namespace)
	}
	return string(data), nil
}

// Set replaces the document for a namespace. The write is atomic: a temp file
// in the same folder is renamed over the target, so a crash mid-write cannot
// leave a half-written file behind.
func (s *Store) Set(namespace, jsonData string) error {
	if err := validNamespace(namespace); err != nil {
		return err
	}
	if err := validDocument(namespace, jsonData); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(s.dir, namespace+".*.tmp")
	if err != nil {
		return fmt.Errorf("Could not save %s. Check that you have permission to write to %s.", namespace, s.dir)
	}
	tmpName := tmp.Name()

	writeErr := func() error {
		if _, err := tmp.WriteString(jsonData); err != nil {
			return err
		}
		if err := tmp.Sync(); err != nil {
			return err
		}
		return tmp.Close()
	}()
	if writeErr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("Could not save %s. The disk may be full or write protected.", namespace)
	}

	if err := os.Rename(tmpName, s.path(namespace)); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("Could not save %s. Another program may be holding the file open.", namespace)
	}
	return nil
}

// List returns the namespaces that have a stored document.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Could not read the data folder %s.", s.dir)
	}

	names := []string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	return names, nil
}

func (s *Store) path(namespace string) string {
	return filepath.Join(s.dir, namespace+".json")
}

// validNamespace keeps a namespace usable as a file name and blocks path
// traversal, since namespaces arrive from the frontend.
func validNamespace(namespace string) error {
	if namespace == "" || len(namespace) > 64 {
		return fmt.Errorf("%q is not a valid data name. Use 1 to 64 letters, numbers, dashes or underscores.", namespace)
	}
	for _, r := range namespace {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			return fmt.Errorf("%q is not a valid data name. Use only letters, numbers, dashes or underscores.", namespace)
		}
	}
	return nil
}

// validDocument enforces the shared schema rule: every stored document is a
// JSON object with a top-level integer "version".
func validDocument(namespace, jsonData string) error {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonData), &doc); err != nil {
		return fmt.Errorf("Could not save %s: the data is not valid JSON.", namespace)
	}
	raw, ok := doc["version"]
	if !ok {
		return fmt.Errorf("Could not save %s: the data is missing its \"version\" number.", namespace)
	}
	var version int
	if err := json.Unmarshal(raw, &version); err != nil {
		return fmt.Errorf("Could not save %s: \"version\" must be a whole number.", namespace)
	}
	return nil
}

func dataDir() (dir string, portable bool, err error) {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		exeDir = filepath.Dir(exe)
	}
	return resolveDir(exeDir)
}

func resolveDir(exeDir string) (dir string, portable bool, err error) {
	if exeDir != "" {
		if _, err := os.Stat(filepath.Join(exeDir, PortableMarker)); err == nil {
			return filepath.Join(exeDir, "data"), true, nil
		}
	}

	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", false, fmt.Errorf("Could not work out where to store settings on this machine. Create a %s file next to the app to run in portable mode.", PortableMarker)
	}
	return filepath.Join(cfg, appDirName), false, nil
}
