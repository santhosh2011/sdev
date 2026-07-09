// Package jsonout defines the machine-readable --json output structs: the JSON
// contract expressed as types, replacing hand-assembled jq output.
package jsonout

// StatusReport is the payload of `sdev status --json`.
type StatusReport struct {
	SdevHome      string         `json:"sdev_home"`
	ActiveProject string         `json:"active_project"`
	Projects      []ProjectCount `json:"projects"`
	Totals        StatusTotals   `json:"totals"`
	Next          []string       `json:"next"`
}

// ProjectCount is the per-project task/running tally within a StatusReport.
type ProjectCount struct {
	Name    string `json:"name"`
	Tasks   int    `json:"tasks"`
	Running int    `json:"running"`
}

// StatusTotals aggregates a StatusReport across all projects.
type StatusTotals struct {
	Projects int `json:"projects"`
	Tasks    int `json:"tasks"`
	Running  int `json:"running"`
}
