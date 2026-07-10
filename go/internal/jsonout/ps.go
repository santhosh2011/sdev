package jsonout

// PSReport is the payload of `sdev ps --json`.
type PSReport struct {
	Task     string      `json:"task"`
	URL      string      `json:"url"`
	Services []PSService `json:"services"`
}

// PSService is one normalized compose service within a PSReport.
type PSService struct {
	Name  string   `json:"name"`
	State string   `json:"state"`
	Ports []string `json:"ports"`
}
