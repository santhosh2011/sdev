package state

import "testing"

func TestDropPoolRemovesEntryByPath(t *testing.T) {
	home := t.TempDir()
	seedPool(t, home,
		PoolEntry{Project: "widget", Repo: "api", Path: "/pool/a"},
		PoolEntry{Project: "widget", Repo: "api", Path: "/pool/b"},
	)

	if err := DropPool(home, "/pool/a"); err != nil {
		t.Fatalf("DropPool: %v", err)
	}
	l, _ := Load(FilePath(home))
	if len(l.Pool) != 1 || l.Pool[0].Path != "/pool/b" {
		t.Fatalf("pool = %+v, want only /pool/b", l.Pool)
	}
}

func TestDropPoolNoopWhenPathAbsent(t *testing.T) {
	home := t.TempDir()
	seedPool(t, home, PoolEntry{Project: "widget", Repo: "api", Path: "/pool/b"})

	if err := DropPool(home, "/pool/missing"); err != nil {
		t.Fatalf("DropPool: %v", err)
	}
	l, _ := Load(FilePath(home))
	if len(l.Pool) != 1 {
		t.Fatalf("pool = %+v, want unchanged", l.Pool)
	}
}

func seedPool(t *testing.T, home string, entries ...PoolEntry) {
	t.Helper()
	if err := Init(home); err != nil {
		t.Fatal(err)
	}
	l, err := Load(FilePath(home))
	if err != nil {
		t.Fatal(err)
	}
	l.Pool = entries
	if err := Save(home, l); err != nil {
		t.Fatal(err)
	}
}
