package types

import "github.com/agent-dance/luban/i18n"

// PolicyDisposition is the cross-layer result of shell policy analysis.
// Callers must enforce Block unconditionally. RequiredAsk is invocation-scoped
// in interactive modes, while an explicit automatic mode may consume it
// without prompting; explicit deny rules remain authoritative in every mode.
type PolicyDisposition string

const (
	PolicyAllow       PolicyDisposition = "allow"
	PolicyRequiredAsk PolicyDisposition = "required_ask"
	PolicyBlock       PolicyDisposition = "block"
)

// PolicyRisk is a stable machine-readable risk class. It is deliberately
// separate from user-visible copy so permission receipts and audit records do
// not depend on translated text.
type PolicyRisk string

const (
	PolicyRiskNone             PolicyRisk = ""
	PolicyRiskHigh             PolicyRisk = "high"
	PolicyRiskCritical         PolicyRisk = "critical"
	PolicyRiskUnrestrictedCode PolicyRisk = "unrestricted_code"
)

// PolicyCodeUnrestrictedCode identifies shell invocations whose executable or
// side-effect grammar is not completely modeled. Such invocations require a
// fresh human decision even when a broader permission rule says Allow.
const PolicyCodeUnrestrictedCode = "shell.policy.ask.unrestricted_code"

// PolicyContext contains only authority that is relevant to static shell
// analysis. Environment values are deliberately not copied wholesale: known
// variables must be supplied explicitly so an absent value remains unknown.
type PolicyContext struct {
	CWD              string
	HomeDir          string
	AllowedDirs      []string
	TrustedTempRoots []string
	KnownEnvironment map[string]string
	Sandboxed        bool
	Interactive      bool
}

// PolicyRemediation is a stable, structured recovery instruction. PublicKey
// and PublicArgs are rendered at the final user-facing boundary.
type PolicyRemediation struct {
	Action     string   `json:"action"`
	PublicKey  i18n.Key `json:"public_key"`
	PublicArgs []any    `json:"public_args,omitempty"`
}

// PolicyDecision is shared by hard safety, mandatory approval, tool-specific
// permission, and execution gates. PrivateCause is never user-visible.
type PolicyDecision struct {
	Disposition  PolicyDisposition  `json:"disposition"`
	Code         string             `json:"code"`
	PublicKey    i18n.Key           `json:"public_key,omitempty"`
	PublicArgs   []any              `json:"public_args,omitempty"`
	RuleSource   string             `json:"rule_source,omitempty"`
	Risk         PolicyRisk         `json:"risk,omitempty"`
	Remediation  *PolicyRemediation `json:"remediation,omitempty"`
	PrivateCause error              `json:"-"`
}

func (d PolicyDecision) IsBlock() bool       { return d.Disposition == PolicyBlock }
func (d PolicyDecision) IsRequiredAsk() bool { return d.Disposition == PolicyRequiredAsk }

// ExecutionBindingCode is the analyzer authority fingerprint carried by a
// one-time execution receipt. The disposition and typed risk are included so
// a receipt issued under a weaker classification cannot authorize a stronger
// one merely because a caller accidentally reused its policy code.
func (d PolicyDecision) ExecutionBindingCode() string {
	return string(d.Disposition) + "\x1e" + d.Code + "\x1e" + string(d.Risk)
}

func (d PolicyDecision) Clone() PolicyDecision {
	cloned := d
	cloned.PublicArgs = append([]any(nil), d.PublicArgs...)
	if d.Remediation != nil {
		remediation := *d.Remediation
		remediation.PublicArgs = append([]any(nil), d.Remediation.PublicArgs...)
		cloned.Remediation = &remediation
	}
	return cloned
}
