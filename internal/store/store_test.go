package store

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewAt(t.TempDir(), false)
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}
	return s
}

func TestRoundTrip(t *testing.T) {
	s := newTestStore(t)
	doc := `{"version":1,"theme":"dark","portableMode":false}`

	if err := s.Set("settings", doc); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get("settings")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != doc {
		t.Fatalf("Get = %q, want %q", got, doc)
	}
}

func TestGetMissingNamespaceIsEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Get("nothing-here")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "" {
		t.Fatalf("Get = %q, want empty string", got)
	}
}

func TestSetOverwritesAtomicallyAndLeavesNoTempFiles(t *testing.T) {
	s := newTestStore(t)

	if err := s.Set("devices", `{"version":1,"items":[1]}`); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set("devices", `{"version":1,"items":[1,2]}`); err != nil {
		t.Fatalf("Set (overwrite): %v", err)
	}

	got, err := s.Get("devices")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != `{"version":1,"items":[1,2]}` {
		t.Fatalf("Get = %q", got)
	}

	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "devices.json" {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("data folder holds %v, want only devices.json", names)
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)
	for _, ns := range []string{"settings", "snippets"} {
		if err := s.Set(ns, `{"version":1}`); err != nil {
			t.Fatalf("Set %s: %v", ns, err)
		}
	}
	if err := os.WriteFile(filepath.Join(s.Dir(), "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List = %v, want the two JSON namespaces only", got)
	}
}

func TestNamespaceRejectsPathTraversal(t *testing.T) {
	s := newTestStore(t)
	outside := filepath.Join(filepath.Dir(s.Dir()), "escaped.json")

	bad := []string{"../escaped", "..", "sub/dir", `sub\dir`, "with space", "dots.in.name", "", "/etc/passwd"}
	for _, ns := range bad {
		if err := s.Set(ns, `{"version":1}`); err == nil {
			t.Fatalf("Set(%q) was accepted, want rejection", ns)
		}
		if _, err := s.Get(ns); err == nil {
			t.Fatalf("Get(%q) was accepted, want rejection", ns)
		}
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatalf("a file was written outside the data folder: %s", outside)
	}
}

func TestSetRejectsDocumentsWithoutVersion(t *testing.T) {
	s := newTestStore(t)

	cases := map[string]string{
		"not json":         `{oops`,
		"array":            `[1,2,3]`,
		"missing version":  `{"theme":"dark"}`,
		"version not a nr": `{"version":"one"}`,
	}
	for name, doc := range cases {
		if err := s.Set("settings", doc); err == nil {
			t.Fatalf("%s: Set was accepted, want rejection", name)
		}
	}
	if got, _ := s.Get("settings"); got != "" {
		t.Fatalf("a rejected document was written: %q", got)
	}
}

func TestResolveDirPortableMode(t *testing.T) {
	exeDir := t.TempDir()

	dir, portable, err := resolveDir(exeDir)
	if err != nil {
		t.Fatalf("resolveDir: %v", err)
	}
	if portable {
		t.Fatalf("portable mode detected without a %s file", PortableMarker)
	}
	cfg, _ := os.UserConfigDir()
	if dir != filepath.Join(cfg, appDirName) {
		t.Fatalf("dir = %q, want the OS config folder", dir)
	}

	if err := os.WriteFile(filepath.Join(exeDir, PortableMarker), nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dir, portable, err = resolveDir(exeDir)
	if err != nil {
		t.Fatalf("resolveDir (portable): %v", err)
	}
	if !portable {
		t.Fatal("portable mode not detected next to the executable")
	}
	if dir != filepath.Join(exeDir, "data") {
		t.Fatalf("dir = %q, want %q", dir, filepath.Join(exeDir, "data"))
	}
}
