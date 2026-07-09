package state

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func alwaysDead(int, string) bool  { return false }
func alwaysAlive(int, string) bool { return true }

func TestAllocateOffsetFreshReturnsStep(t *testing.T) {
	home := t.TempDir()
	mkTaskDir(t, home, "widget/a")

	off, err := AllocateOffset(home, "widget/a", false, "", false, 10, alwaysDead)
	if err != nil {
		t.Fatal(err)
	}
	if off != 10 {
		t.Fatalf("offset = %d, want 10", off)
	}
}

func TestAllocateOffsetSeedsFromExistingEnv(t *testing.T) {
	home := t.TempDir()
	writeEnvOffset(t, home, "widget/x", 10)
	writeEnvOffset(t, home, "widget/y", 20)
	mkTaskDir(t, home, "widget/z")

	off, err := AllocateOffset(home, "widget/z", false, "", false, 10, alwaysDead)
	if err != nil {
		t.Fatal(err)
	}
	if off != 30 {
		t.Fatalf("offset = %d, want 30 (10 and 20 seeded as used)", off)
	}
}

func TestAllocateOffsetLeaseKeepsReservation(t *testing.T) {
	home := t.TempDir()
	seedLedger(t, home, "widget/held", Task{Offset: 10, Lease: true, LeaseHolder: "bg"})
	mkTaskDir(t, home, "widget/new")

	off, err := AllocateOffset(home, "widget/new", false, "", false, 10, alwaysDead)
	if err != nil {
		t.Fatal(err)
	}
	if off != 20 {
		t.Fatalf("offset = %d, want 20 (lease keeps 10 reserved)", off)
	}
	l, _ := Load(FilePath(home))
	if _, ok := l.Tasks["widget/held"]; !ok {
		t.Fatal("leased task was reclaimed")
	}
}

func TestAllocateOffsetDeadLockReclaimed(t *testing.T) {
	home := t.TempDir()
	seedLedger(t, home, "widget/dead", Task{Offset: 10, Pid: 999999, ProcToken: "gone"})
	mkTaskDir(t, home, "widget/new")

	off, err := AllocateOffset(home, "widget/new", false, "", false, 10, alwaysDead)
	if err != nil {
		t.Fatal(err)
	}
	if off != 10 {
		t.Fatalf("offset = %d, want 10 (dead lock reclaimed)", off)
	}
}

func TestAllocateOffsetLiveLockKept(t *testing.T) {
	home := t.TempDir()
	seedLedger(t, home, "widget/live", Task{Offset: 10, Pid: 4242, ProcToken: "tok"})
	mkTaskDir(t, home, "widget/new")

	off, err := AllocateOffset(home, "widget/new", false, "", false, 10, alwaysAlive)
	if err != nil {
		t.Fatal(err)
	}
	if off != 20 {
		t.Fatalf("offset = %d, want 20 (live lock keeps 10)", off)
	}
}

func TestFreeTaskRemovesEntry(t *testing.T) {
	home := t.TempDir()
	seedLedger(t, home, "widget/x", Task{Offset: 10})

	if err := FreeTask(home, "widget/x"); err != nil {
		t.Fatal(err)
	}
	l, _ := Load(FilePath(home))
	if _, ok := l.Tasks["widget/x"]; ok {
		t.Fatal("task not freed")
	}
}

func mkTaskDir(t *testing.T, home, key string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "projects", key), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeEnvOffset(t *testing.T, home, key string, off int) {
	t.Helper()
	mkTaskDir(t, home, key)
	body := "PORT_OFFSET=" + strconv.Itoa(off) + "\n"
	if err := os.WriteFile(filepath.Join(home, "projects", key, ".env"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func seedLedger(t *testing.T, home, key string, task Task) {
	t.Helper()
	if err := Init(home); err != nil {
		t.Fatal(err)
	}
	l, err := Load(FilePath(home))
	if err != nil {
		t.Fatal(err)
	}
	l.Seeded = true
	l.Tasks[key] = task
	if err := Save(home, l); err != nil {
		t.Fatal(err)
	}
}
