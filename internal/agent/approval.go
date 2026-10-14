package agent

import "context"

// ApprovalDecision is the user's response to an approval request, modeled on
// the reference agent's review decisions.
type ApprovalDecision uint8

const (
	// ApprovalDenied refuses the operation.
	ApprovalDenied ApprovalDecision = iota
	// ApprovalOnce allows this single operation.
	ApprovalOnce
	// ApprovalSession allows matching operations until the session ends.
	ApprovalSession
	// ApprovalPermanent allows the operation and records it for future
	// sessions (via Options.OnPermanentGrant).
	ApprovalPermanent
)

// ApprovalMode is the policy for a tool category.
type ApprovalMode string

const (
	// ApprovalModeAsk prompts the configured approver.
	ApprovalModeAsk ApprovalMode = "ask"
	// ApprovalModeAllow runs without prompting.
	ApprovalModeAllow ApprovalMode = "allow"
	// ApprovalModeDeny rejects outright.
	ApprovalModeDeny ApprovalMode = "deny"
)

// ApprovalRequest describes an operation awaiting approval.
type ApprovalRequest struct {
	// Tool is the tool requesting the operation (e.g. "shell").
	Tool string
	// Summary is a one-line, human-readable description (e.g. the command).
	Summary string
}

// Approver decides whether an operation may proceed. An error is treated as a
// denial.
type Approver func(ctx context.Context, request ApprovalRequest) (ApprovalDecision, error)

// approvalKind classifies a tool for policy lookup.
type approvalKind string

const (
	approvalKindNone    approvalKind = ""
	approvalKindShell   approvalKind = "shell"
	approvalKindWrite   approvalKind = "write"
	approvalKindNetwork approvalKind = "network"
)

// approvalRequestor is implemented by tools that may need user approval. The
// bool reports whether approval is required for the given arguments.
type approvalRequestor interface {
	ApprovalKind() approvalKind
	ApprovalSummary(arguments []byte) (string, bool)
}
