package compose

import "testing"

func TestParsePSNormalizesServicesAndPorts(t *testing.T) {
	raw := []byte(`{"Service":"nginx","State":"running","Publishers":[{"PublishedPort":8100,"TargetPort":80}]}
{"Service":"api","State":"running","Publishers":[]}`)

	got := ParsePS(raw)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Name != "nginx" || got[0].State != "running" {
		t.Fatalf("service[0] = %+v", got[0])
	}
	if len(got[0].Ports) != 1 || got[0].Ports[0] != "8100->80" {
		t.Fatalf("nginx ports = %v, want [8100->80]", got[0].Ports)
	}
	if got[1].Ports == nil || len(got[1].Ports) != 0 {
		t.Fatalf("api ports = %v, want []", got[1].Ports)
	}
}

func TestParsePSEmptyInputYieldsEmptyNonNil(t *testing.T) {
	got := ParsePS([]byte(""))
	if got == nil || len(got) != 0 {
		t.Fatalf("ParsePS(empty) = %v, want []", got)
	}
}

func TestParsePSFlattensArrayFormAndFallbackFields(t *testing.T) {
	raw := []byte(`[{"Name":"db","Status":"Up","Publishers":[{"PublishedPort":5432,"TargetPort":5432}]}]`)

	got := ParsePS(raw)

	if len(got) != 1 || got[0].Name != "db" || got[0].State != "Up" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Ports[0] != "5432->5432" {
		t.Fatalf("ports = %v", got[0].Ports)
	}
}
