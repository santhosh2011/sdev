package state

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleLedger = `version: 1
seeded: true
pool_seq: 3
tasks:
  acme/b:
    offset: 20
    created_at: "2026-07-01T00:00:00Z"
    lease: true
    lease_holder: nightowl
    pid: 0
    proc_token: ""
    ephemeral: false
  acme/c:
    offset: 30
    created_at: "2026-07-02T00:00:00Z"
    lease: false
    lease_holder: ""
    pid: 4242
    proc_token: "Wed Jul 9 10:00:00 2026"
    ephemeral: true
pool:
  - project: acme
    repo: api
    repo_path: legacy_api
    source: /src/acme/api
    path: /pool/acme/api.1
    returned_at: "2026-07-03T00:00:00Z"
`

func TestLoadCountsTasksPoolAndSeq(t *testing.T) {
	l := mustLoad(t, sampleLedger)

	if l.PoolSeq != 3 || len(l.Tasks) != 2 || len(l.Pool) != 1 {
		t.Fatalf("PoolSeq=%d Tasks=%d Pool=%d, want 3/2/1", l.PoolSeq, len(l.Tasks), len(l.Pool))
	}
}

func TestLoadParsesLeasedTask(t *testing.T) {
	b := mustLoad(t, sampleLedger).Tasks["acme/b"]

	if b.Offset != 20 || !b.Lease || b.LeaseHolder != "nightowl" {
		t.Fatalf("acme/b = %+v, want offset 20 leased by nightowl", b)
	}
}

func TestLoadParsesEphemeralLockAndPool(t *testing.T) {
	l := mustLoad(t, sampleLedger)

	c := l.Tasks["acme/c"]
	if c.Pid != 4242 || !c.Ephemeral {
		t.Fatalf("acme/c = %+v, want pid 4242 ephemeral", c)
	}
	if l.Pool[0].Path != "/pool/acme/api.1" {
		t.Fatalf("Pool[0].Path = %q", l.Pool[0].Path)
	}
}

func mustLoad(t *testing.T, body string) *Ledger {
	t.Helper()
	l, err := Load(writeLedger(t, body))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestLoadMissingFileYieldsEmpty(t *testing.T) {
	l, err := Load(filepath.Join(t.TempDir(), "nope.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if l.Tasks == nil || len(l.Tasks) != 0 {
		t.Fatalf("Tasks = %v, want empty non-nil", l.Tasks)
	}
}

func TestLeasedKeysReturnsSortedLeasedOnly(t *testing.T) {
	l, _ := Load(writeLedger(t, sampleLedger))
	got := l.LeasedKeys()
	if len(got) != 1 || got[0] != "acme/b" {
		t.Fatalf("LeasedKeys = %v, want [acme/b]", got)
	}
}

func TestStatusLabelLeasedWithHolder(t *testing.T) {
	tk := Task{Lease: true, LeaseHolder: "nightowl"}
	if got := tk.StatusLabel(deadAlive); got != "leased:nightowl" {
		t.Fatalf("StatusLabel = %q, want leased:nightowl", got)
	}
}

func TestStatusLabelEphemeralLiveLock(t *testing.T) {
	tk := Task{Ephemeral: true, Pid: 4242, ProcToken: "tok"}
	if got := tk.StatusLabel(liveAlive); got != "ephemeral lock:4242" {
		t.Fatalf("StatusLabel = %q, want 'ephemeral lock:4242'", got)
	}
}

func TestStatusLabelStaleLock(t *testing.T) {
	tk := Task{Pid: 4242}
	if got := tk.StatusLabel(deadAlive); got != "lock:stale" {
		t.Fatalf("StatusLabel = %q, want lock:stale", got)
	}
}

func TestStatusLabelEmptyWhenNoReservation(t *testing.T) {
	if got := (Task{}).StatusLabel(deadAlive); got != "" {
		t.Fatalf("StatusLabel = %q, want empty", got)
	}
}

func liveAlive(int, string) bool { return true }
func deadAlive(int, string) bool { return false }

func writeLedger(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "state.yml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
