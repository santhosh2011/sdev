package jsonout

// LSReport is the payload of `sdev ls --json`. Project is null when the listing
// is unscoped.
type LSReport struct {
	Project       *string      `json:"project"`
	Alive         []LSAlive    `json:"alive"`
	Archived      []LSArchived `json:"archived"`
	OrphanVolumes []string     `json:"orphan_volumes"`
	Totals        LSTotals     `json:"totals"`
}

// LSAlive is one live task in an LSReport.
type LSAlive struct {
	Task      string `json:"task"`
	Offset    int    `json:"offset"`
	NginxPort int    `json:"nginx_port"`
	URL       string `json:"url"`
	Running   int    `json:"running"`
	Status    string `json:"status"`
}

// LSArchived is one archived task in an LSReport.
type LSArchived struct {
	Task     string `json:"task"`
	Archived string `json:"archived"`
}

// LSTotals aggregates an LSReport.
type LSTotals struct {
	Alive    int `json:"alive"`
	Archived int `json:"archived"`
	Orphans  int `json:"orphans"`
	Running  int `json:"running"`
}
