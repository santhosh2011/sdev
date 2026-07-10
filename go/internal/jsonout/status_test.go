package jsonout

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStatusReportEmptyProjectsMarshalsAsArrayNotNull(t *testing.T) {
	report := StatusReport{Projects: []ProjectCount{}, Next: []string{}}

	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"projects":[]`) {
		t.Fatalf("projects must marshal as [], got: %s", b)
	}
}
