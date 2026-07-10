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

func TestTakePoolPopsFirstMatchingSource(t *testing.T) {
	home := t.TempDir()
	seedPool(t, home,
		PoolEntry{Source: "/src/a", Path: "/pool/a1"},
		PoolEntry{Source: "/src/b", Path: "/pool/b1"},
		PoolEntry{Source: "/src/a", Path: "/pool/a2"},
	)

	got, err := TakePool(home, "/src/a")
	if err != nil {
		t.Fatalf("TakePool: %v", err)
	}
	if got != "/pool/a1" {
		t.Fatalf("TakePool = %q, want /pool/a1", got)
	}
	l, _ := Load(FilePath(home))
	if len(l.Pool) != 2 {
		t.Fatalf("pool len = %d, want 2 after take", len(l.Pool))
	}
}

func TestTakePoolEmptyWhenNoMatch(t *testing.T) {
	home := t.TempDir()
	seedPool(t, home, PoolEntry{Source: "/src/a", Path: "/pool/a1"})

	got, err := TakePool(home, "/src/zzz")
	if err != nil {
		t.Fatalf("TakePool: %v", err)
	}
	if got != "" {
		t.Fatalf("TakePool = %q, want empty", got)
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
