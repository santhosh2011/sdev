package proc

import (
	"os"
	"testing"
)

func TestAliveTrueForCurrentProcess(t *testing.T) {
	if !Alive(os.Getpid(), "") {
		t.Fatal("Alive(self) = false, want true")
	}
}

func TestAliveFalseForZeroPid(t *testing.T) {
	if Alive(0, "") {
		t.Fatal("Alive(0) = true, want false")
	}
}

func TestAliveFalseWhenTokenMismatches(t *testing.T) {
	if Alive(os.Getpid(), "definitely-not-the-real-start-time") {
		t.Fatal("Alive(self, wrong-token) = true, want false")
	}
}
