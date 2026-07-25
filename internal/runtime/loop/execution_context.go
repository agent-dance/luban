package loop

import (
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/skills"
)

func (q *QueryLoop) bindToolExecutionContext(
	exec executioncontract.ToolExecutionContext,
	runToken string,
	visibleMessagesResolver func(skills.SkillID) SkillLoadedLedgerState,
	generation skills.ProjectSourceGeneration,
) executioncontract.ToolExecutionContext {
	if q.executionOwner == nil {
		q.executionOwner = executioncontract.NewOwner()
	}
	return executioncontract.Bind(q.executionOwner, exec, executioncontract.BindSpec{
		RunToken: runToken,
		Identity: executioncontract.RuntimeOwnerIdentity{
			SessionID: exec.SessionID, SessionProjectDir: exec.SessionProjectDir,
			ProjectRoot: exec.ProjectRoot, CWD: exec.CWD,
		},
		SkillProjectGeneration: uint64(generation),
		ResolveSkillLedger: func(rawID string) executioncontract.SkillLoadedLedgerState {
			state := visibleMessagesResolver(skills.SkillID(rawID))
			return executioncontract.SkillLoadedLedgerState{
				ContextEpoch: state.ContextEpoch, LoadedContextEpoch: state.LoadedContextEpoch,
				ContentDigest: string(state.ContentDigest), PayloadDigest: string(state.PayloadDigest),
			}
		},
		ReadEvidenceOwnerID:  q.readEvidenceOwnerID,
		ReadEvidenceEpoch:    q.currentReadEvidenceEpoch(),
		ReadEvidenceActorID:  exec.ActorID,
		CurrentEvidenceEpoch: q.currentReadEvidenceEpoch,
	})
}

// OwnsToolExecution proves that exec belongs to this loop's currently active
// run. Engine operations use this exact authority rather than trusting public
// session or workspace strings.
func (q *QueryLoop) OwnsToolExecution(exec executioncontract.ToolExecutionContext) bool {
	return q != nil && exec.OwnedBy(q.executionOwner)
}
