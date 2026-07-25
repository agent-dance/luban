package approvalcommit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sync"
)

type receiptContextKey struct{}

type receipt struct {
	mu         sync.Mutex
	toolName   string
	digest     [32]byte
	digestOK   bool
	policyCode string
	consumed   bool
}

// PermissionCommitStatus distinguishes direct execution without a commit from
// an invalid or already-consumed receipt. Security-sensitive consumers may
// apply a conservative direct-execution policy only to Absent; Invalid must
// always fail closed.
type PermissionCommitStatus uint8

const (
	PermissionCommitAbsent PermissionCommitStatus = iota
	PermissionCommitValid
	PermissionCommitInvalid
)

// Bind creates the one-shot receipt installed by trusted registry dispatch
// after its permission grant has been validated. The private context key and
// receipt fields keep external modules from manufacturing this capability.
func Bind(ctx context.Context, toolName string, input map[string]any, policyCode string) context.Context {
	digest, ok := inputDigest(toolName, input)
	return context.WithValue(ctx, receiptContextKey{}, &receipt{
		toolName:   toolName,
		digest:     digest,
		digestOK:   ok,
		policyCode: policyCode,
	})
}

// Consume validates and atomically consumes the exact receipt installed by
// registry dispatch. A receipt cannot be replayed or crossed to a different
// tool, input, or policy decision.
func Consume(ctx context.Context, toolName string, input map[string]any, policyCode string) PermissionCommitStatus {
	bound, ok := ctx.Value(receiptContextKey{}).(*receipt)
	if !ok || bound == nil {
		return PermissionCommitAbsent
	}
	digest, digestOK := inputDigest(toolName, input)
	bound.mu.Lock()
	defer bound.mu.Unlock()
	if bound.consumed {
		return PermissionCommitInvalid
	}
	bound.consumed = true
	if bound.digestOK && digestOK && bound.toolName == toolName && bound.digest == digest && bound.policyCode == policyCode {
		return PermissionCommitValid
	}
	return PermissionCommitInvalid
}

func inputDigest(toolName string, input map[string]any) ([32]byte, bool) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return [32]byte{}, false
	}
	return sha256.Sum256(append(append([]byte(toolName), 0), encoded...)), true
}
