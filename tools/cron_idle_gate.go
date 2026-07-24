package tools

// cron_idle_gate.go — idle-gating policy hook for the cron executor.
//
// Mirrors the TS scheduler's idle policy: when the system is idle (e.g.
// laptop closed, machine on battery), recurring fires can be skipped or
// deferred so users aren't woken by background work.
//
// The hook is exposed via the global IdleGate variable. Implementations
// can be swapped out by tests or runtimes that want to drive the gating
// from outside the package.

import "context"

// IdlePolicy decides whether the cron executor should fire a job at the
// given moment. Implementations may consult OS idle counters, battery
// state, or user-defined policies. Returning false means "skip this fire".
type IdlePolicy interface {
	// ShouldFire returns true if the executor may proceed with the job.
	ShouldFire(ctx context.Context, jobID string) bool
}

// IdlePolicyFunc adapts a closure to IdlePolicy.
type IdlePolicyFunc func(ctx context.Context, jobID string) bool

// ShouldFire implements IdlePolicy.
func (f IdlePolicyFunc) ShouldFire(ctx context.Context, jobID string) bool {
	return f(ctx, jobID)
}

// alwaysFirePolicy is the default policy: never gate.
type alwaysFirePolicy struct{}

// ShouldFire implements IdlePolicy.
func (alwaysFirePolicy) ShouldFire(context.Context, string) bool { return true }

// IdleGate is the global idle-policy hook consulted by the executor before
// firing a job. Defaults to "always fire". Tests / runtimes may swap this
// out to enforce a custom policy.
var IdleGate IdlePolicy = alwaysFirePolicy{}

// IdleGateConsult is a small wrapper that consults the current IdleGate
// hook. Returning false means the caller should skip the fire.
func IdleGateConsult(ctx context.Context, jobID string) bool {
	gate := IdleGate
	if gate == nil {
		return true
	}
	return gate.ShouldFire(ctx, jobID)
}
