// Package compose normalizes `docker compose ps` output into sdev's service
// shape.
package compose

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/santhosh2011/sdev/internal/jsonout"
)

// rawService is a compose ps record. Field names vary across compose versions
// (Service/Name, State/Status), so both spellings are accepted.
type rawService struct {
	Service    string
	Name       string
	State      string
	Status     string
	Publishers []rawPublisher
}

type rawPublisher struct {
	PublishedPort int
	TargetPort    int
}

// ParsePS normalizes `docker compose ps --format json` output, which is either
// JSON Lines (one object per line) or a single JSON array, into the service
// shape sdev exposes. Unparseable or empty input yields an empty (non-nil)
// slice, mirroring the bash `|| echo '[]'` fallback.
func ParsePS(raw []byte) []jsonout.PSService {
	services := []jsonout.PSService{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	for {
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			break
		}
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 {
			continue
		}
		if trimmed[0] == '[' {
			var arr []rawService
			if json.Unmarshal(trimmed, &arr) == nil {
				for _, s := range arr {
					services = append(services, normalize(s))
				}
			}
			continue
		}
		var s rawService
		if json.Unmarshal(trimmed, &s) == nil {
			services = append(services, normalize(s))
		}
	}
	return services
}

// normalize maps a compose record to the sdev service shape, preferring the
// primary field name and falling back to the alternate spelling.
func normalize(s rawService) jsonout.PSService {
	name := s.Service
	if name == "" {
		name = s.Name
	}
	state := s.State
	if state == "" {
		state = s.Status
	}
	ports := []string{}
	for _, p := range s.Publishers {
		if p.PublishedPort > 0 {
			ports = append(ports, strconv.Itoa(p.PublishedPort)+"->"+strconv.Itoa(p.TargetPort))
		}
	}
	return jsonout.PSService{Name: name, State: state, Ports: ports}
}
