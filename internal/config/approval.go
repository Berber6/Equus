package config

// ApprovalPolicy is the global approval policy, modeled on the reference
// agent's approval_policy setting.
type ApprovalPolicy string

const (
	// ApprovalPolicyUntrusted requires approval for every sensitive operation.
	ApprovalPolicyUntrusted ApprovalPolicy = "untrusted"
	// ApprovalPolicyOnRequest defers to the configured approver (a prompt in
	// interactive mode, denial when non-interactive). This is the default.
	ApprovalPolicyOnRequest ApprovalPolicy = "on-request"
	// ApprovalPolicyNever auto-approves every operation.
	ApprovalPolicyNever ApprovalPolicy = "never"
)

// ApprovalMode is a per-category decision.
type ApprovalMode string

const (
	// ApprovalModeAsk prompts the user.
	ApprovalModeAsk ApprovalMode = "ask"
	// ApprovalModeAllow runs without prompting.
	ApprovalModeAllow ApprovalMode = "allow"
	// ApprovalModeDeny rejects outright.
	ApprovalModeDeny ApprovalMode = "deny"
)

// ApprovalConfig controls which operations require confirmation.
type ApprovalConfig struct {
	Policy ApprovalPolicy `yaml:"policy"`
	Shell  ApprovalMode   `yaml:"shell"`
	Write  ApprovalMode   `yaml:"write"`
}

// EffectiveMode resolves the decision for a category, honoring the global
// policy first and the per-category override second.
func (approval ApprovalConfig) EffectiveMode(category string) ApprovalMode {
	// A per-category override wins when set.
	switch category {
	case "shell":
		if approval.Shell != "" {
			return approval.Shell
		}
	case "write":
		if approval.Write != "" {
			return approval.Write
		}
	}
	switch approval.Policy {
	case ApprovalPolicyNever:
		return ApprovalModeAllow
	case ApprovalPolicyUntrusted:
		return ApprovalModeAsk
	default:
		return ApprovalModeAsk
	}
}

// defaultApproval returns the built-in approval defaults: prompt for shell,
// allow file writes.
func defaultApproval() ApprovalConfig {
	return ApprovalConfig{
		Policy: ApprovalPolicyOnRequest,
		Shell:  ApprovalModeAsk,
		Write:  ApprovalModeAllow,
	}
}
