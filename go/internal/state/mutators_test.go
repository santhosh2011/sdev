package state

import "testing"

func TestSetLeaseCreatesEntryAndSetsHolder(t *testing.T) {
	home := t.TempDir()
	writeEnvOffset(t, home, "widget/a", 20) // .env so the bare entry adopts its offset

	if err := SetLease(home, "widget/a", "nightowl"); err != nil {
		t.Fatal(err)
	}
	l, _ := Load(FilePath(home))
	tk := l.Tasks["widget/a"]
	if !tk.Lease || tk.LeaseHolder != "nightowl" || tk.Offset != 20 {
		t.Fatalf("task = %+v, want leased by nightowl at offset 20", tk)
	}
}

func TestSetLockRecordsPidAndToken(t *testing.T) {
	home := t.TempDir()
	seedLedger(t, home, "widget/a", Task{Offset: 10})

	if err := SetLock(home, "widget/a", 4242, "tok"); err != nil {
		t.Fatal(err)
	}
	tk := loadTask(t, home, "widget/a")
	if tk.Pid != 4242 || tk.ProcToken != "tok" {
		t.Fatalf("task = %+v, want pid 4242 token tok", tk)
	}
}

func TestClearReservationResetsLeaseAndLockKeepsOffset(t *testing.T) {
	home := t.TempDir()
	seedLedger(t, home, "widget/a", Task{Offset: 10, Lease: true, LeaseHolder: "x", Pid: 5, ProcToken: "t"})

	if err := ClearReservation(home, "widget/a"); err != nil {
		t.Fatal(err)
	}
	tk := loadTask(t, home, "widget/a")
	if tk.Lease || tk.LeaseHolder != "" || tk.Pid != 0 || tk.ProcToken != "" {
		t.Fatalf("reservation not cleared: %+v", tk)
	}
	if tk.Offset != 10 {
		t.Fatalf("offset must be preserved, got %+v", tk)
	}
}

func TestClearReservationNoopWhenMissing(t *testing.T) {
	if err := ClearReservation(t.TempDir(), "widget/nope"); err != nil {
		t.Fatalf("ClearReservation on missing key: %v", err)
	}
}

func loadTask(t *testing.T, home, key string) Task {
	t.Helper()
	l, err := Load(FilePath(home))
	if err != nil {
		t.Fatal(err)
	}
	return l.Tasks[key]
}
