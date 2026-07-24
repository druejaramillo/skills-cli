package config

import (
	"path/filepath"
	"testing"
)

func TestLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	initial, err := Load(path)
	if err != nil {
		t.Fatalf("Load missing config: %v", err)
	}
	if len(initial.Sources) != 0 {
		t.Fatalf("missing config sources = %#v, want empty", initial.Sources)
	}

	want := Config{
		DefaultSource: "mine",
		Sources: map[string]Source{
			"mine": {Location: "/skills"},
		},
		Creator: Creator{Model: "openai/gpt-5.6-terra"},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load saved config: %v", err)
	}
	if got.DefaultSource != want.DefaultSource || got.Creator.Model != want.Creator.Model || got.Sources["mine"] != want.Sources["mine"] {
		t.Fatalf("Load = %#v, want %#v", got, want)
	}
}
