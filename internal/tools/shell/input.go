package shell

type bashInput struct {
	Command                   string  `json:"command"`
	Timeout                   float64 `json:"timeout,omitempty"`
	Description               string  `json:"description,omitempty"`
	RunInBackground           bool    `json:"run_in_background,omitempty"`
	DangerouslyDisableSandbox bool    `json:"dangerouslyDisableSandbox,omitempty"`
}

type powerShellInput struct {
	Command         string  `json:"command"`
	Timeout         float64 `json:"timeout,omitempty"`
	Description     string  `json:"description,omitempty"`
	RunInBackground bool    `json:"run_in_background,omitempty"`
}
