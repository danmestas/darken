package substrate

import (
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestResolveSkills_FlatMode(t *testing.T) {
	fs := fstest.MapFS{
		"go-systems.yaml": &fstest.MapFile{Data: []byte(`description: Go systems work
skills:
  - tdd
  - idiomatic-go
`)},
	}
	got, err := ResolveSkillsFromFS(fs, "go-systems")
	if err != nil {
		t.Fatalf("ResolveSkillsFromFS: %v", err)
	}
	want := []string{"tdd", "idiomatic-go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestResolveSkills_ExtendsParent(t *testing.T) {
	fs := fstest.MapFS{
		"philosophy.yaml": &fstest.MapFile{Data: []byte(`description: shared philosophy
skills:
  - hipp
  - ousterhout
`)},
		"planner.yaml": &fstest.MapFile{Data: []byte(`description: planner
extends: philosophy
skills:
  - superpowers
`)},
	}
	got, err := ResolveSkillsFromFS(fs, "planner")
	if err != nil {
		t.Fatalf("ResolveSkillsFromFS: %v", err)
	}
	want := []string{"hipp", "ousterhout", "superpowers"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestResolveSkills_DedupesFirstWins(t *testing.T) {
	fs := fstest.MapFS{
		"base.yaml": &fstest.MapFile{Data: []byte(`description: base
skills:
  - hipp
  - ousterhout
`)},
		"override.yaml": &fstest.MapFile{Data: []byte(`description: override
extends: base
skills:
  - ousterhout
  - extra
`)},
	}
	got, err := ResolveSkillsFromFS(fs, "override")
	if err != nil {
		t.Fatalf("ResolveSkillsFromFS: %v", err)
	}
	want := []string{"hipp", "ousterhout", "extra"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestResolveSkills_DetectsCycle(t *testing.T) {
	fs := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`description: a
extends: b
skills: []
`)},
		"b.yaml": &fstest.MapFile{Data: []byte(`description: b
extends: a
skills: []
`)},
	}
	_, err := ResolveSkillsFromFS(fs, "a")
	if err == nil {
		t.Fatal("expected cycle error; got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle: %v", err)
	}
}

func TestResolveSkills_MissingMode(t *testing.T) {
	fs := fstest.MapFS{}
	_, err := ResolveSkillsFromFS(fs, "nope")
	if err == nil {
		t.Fatal("expected not-found error; got nil")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error should mention mode name: %v", err)
	}
}

func TestResolveSkills_MissingParent(t *testing.T) {
	fs := fstest.MapFS{
		"child.yaml": &fstest.MapFile{Data: []byte(`description: child
extends: ghost
skills: []
`)},
	}
	_, err := ResolveSkillsFromFS(fs, "child")
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should mention missing parent: %v", err)
	}
}

func TestResolveSkills_MultiLevelChain(t *testing.T) {
	fs := fstest.MapFS{
		"grandparent.yaml": &fstest.MapFile{Data: []byte(`description: gp
skills:
  - a
`)},
		"parent.yaml": &fstest.MapFile{Data: []byte(`description: p
extends: grandparent
skills:
  - b
`)},
		"child.yaml": &fstest.MapFile{Data: []byte(`description: c
extends: parent
skills:
  - c
`)},
	}
	got, err := ResolveSkillsFromFS(fs, "child")
	if err != nil {
		t.Fatalf("ResolveSkillsFromFS: %v", err)
	}
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v; want %v", got, want)
	}
	_ = filepath.Join // avoid unused-import error
}
