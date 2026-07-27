// Package workspacerevision binds a verification tool execution to the exact
// mutation state produced by a preceding tool in the same model response.
package workspacerevision

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Epoch and Digest are contract-owned value types shared by the workspace
// revision barrier and runtime flight state. Contracts must not depend on the
// runtime implementation that consumes them.
type Epoch uint64
type Digest string

var (
	ErrInvalidReceipt  = errors.New("invalid workspace revision receipt")
	ErrRevisionChanged = errors.New("workspace revision changed")
)

// Receipt is an opaque, process-local proof of the mutation epoch and the
// post-commit contents of every path changed by that mutation. Its authority
// fields are deliberately unexported so model input and serialized state
// cannot manufacture a valid dependency edge.
type Receipt struct {
	ledger *Ledger
	scope  string
	epoch  Epoch
	digest [sha256.Size]byte
	mac    [sha256.Size]byte
	paths  []string
}

func (r Receipt) Valid() bool  { return r.ledger != nil && r.epoch != 0 && len(r.paths) != 0 }
func (r Receipt) Epoch() Epoch { return r.epoch }
func (r Receipt) Digest() Digest {
	if !r.Valid() {
		return ""
	}
	return Digest(hex.EncodeToString(r.digest[:]))
}

// Ledger owns independent epochs for each canonical workspace root. Sharing a
// ledger across parent and child runtimes is safe: distinct worktrees use
// distinct scopes, while concurrent mutations of the same worktree invalidate
// an outstanding verification receipt.
type Ledger struct {
	mu     sync.Mutex
	secret [sha256.Size]byte
	epochs map[string]Epoch
}

func NewLedger() *Ledger {
	ledger := &Ledger{epochs: make(map[string]Epoch)}
	if _, err := io.ReadFull(rand.Reader, ledger.secret[:]); err != nil {
		// Receipt authority also requires the exact unexported ledger pointer;
		// this fallback preserves corruption detection if platform entropy is
		// unavailable without turning a transient host issue into tool failure.
		seed := sha256.Sum256([]byte(filepath.Clean(os.TempDir()) + "\x00" + err.Error()))
		ledger.secret = seed
	}
	return ledger
}

// CurrentEpoch returns the cooperating mutation epoch for one workspace. Zero
// is a valid pristine revision. The boolean is false only for an invalid scope
// or a nil ledger.
func (l *Ledger) CurrentEpoch(scope string) (Epoch, bool) {
	if l == nil || strings.TrimSpace(scope) == "" {
		return 0, false
	}
	canonical, err := canonicalWorkspaceScope(scope)
	if err != nil {
		return 0, false
	}
	l.mu.Lock()
	epoch := l.epochs[canonical]
	l.mu.Unlock()
	return epoch, true
}

// MatchesEpoch fail-closes a deferred view when any cooperating mutation has
// advanced the workspace since that view was captured.
func (l *Ledger) MatchesEpoch(scope string, expected Epoch) bool {
	current, ok := l.CurrentEpoch(scope)
	return ok && current == expected
}

// Commit snapshots the changed paths only after their mutation transaction has
// committed. This deliberately excludes unrelated build artifacts that a test
// command may create, while the scope epoch catches cooperating mutations in
// the same workspace.
func (l *Ledger) Commit(scope string, paths []string) (Receipt, error) {
	if l == nil {
		return Receipt{}, ErrInvalidReceipt
	}
	canonicalScope, canonicalPaths, err := canonicalize(scope, paths)
	if err != nil {
		return Receipt{}, err
	}
	digest, err := digestPaths(canonicalScope, canonicalPaths)
	if err != nil {
		return Receipt{}, err
	}
	l.mu.Lock()
	if l.epochs == nil {
		l.epochs = make(map[string]Epoch)
	}
	l.epochs[canonicalScope]++
	epoch := l.epochs[canonicalScope]
	receipt := Receipt{ledger: l, scope: canonicalScope, epoch: epoch, digest: digest, paths: canonicalPaths}
	receipt.mac = l.authenticate(receipt)
	l.mu.Unlock()
	return receipt, nil
}

// Validate proves that the receipt belongs to this ledger, remains the latest
// cooperating mutation epoch, and that every changed path still has its exact
// post-commit state.
func (l *Ledger) Validate(receipt Receipt) error {
	if l == nil || receipt.ledger != l || !receipt.Valid() {
		return ErrInvalidReceipt
	}
	l.mu.Lock()
	wantMAC := l.authenticate(receipt)
	currentEpoch := l.epochs[receipt.scope]
	l.mu.Unlock()
	if !hmac.Equal(wantMAC[:], receipt.mac[:]) {
		return ErrInvalidReceipt
	}
	if currentEpoch != receipt.epoch {
		return ErrRevisionChanged
	}
	digest, err := digestPaths(receipt.scope, receipt.paths)
	if err != nil || !hmac.Equal(digest[:], receipt.digest[:]) {
		return ErrRevisionChanged
	}
	return nil
}

func (l *Ledger) authenticate(receipt Receipt) [sha256.Size]byte {
	h := hmac.New(sha256.New, l.secret[:])
	_, _ = h.Write([]byte(receipt.scope))
	var epoch [8]byte
	binary.BigEndian.PutUint64(epoch[:], uint64(receipt.epoch))
	_, _ = h.Write(epoch[:])
	_, _ = h.Write(receipt.digest[:])
	for _, path := range receipt.paths {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(path))
	}
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

func canonicalize(scope string, paths []string) (string, []string, error) {
	canonicalScope, err := canonicalWorkspaceScope(scope)
	if err != nil {
		return "", nil, ErrInvalidReceipt
	}
	rawScope, err := filepath.Abs(strings.TrimSpace(scope))
	if err != nil {
		return "", nil, ErrInvalidReceipt
	}
	rawScope = filepath.Clean(rawScope)
	unique := make(map[string]struct{}, len(paths))
	canonicalPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		absolute, absErr := filepath.Abs(path)
		if absErr != nil {
			return "", nil, absErr
		}
		absolute = filepath.Clean(absolute)
		// Preserve paths for post-delete receipts by translating a path under
		// the caller's lexical scope into the scope's resolved namespace. For
		// paths supplied through a different alias, resolve the existing path.
		if rawRelative, rawRelErr := filepath.Rel(rawScope, absolute); rawRelErr == nil &&
			rawRelative != ".." && !strings.HasPrefix(rawRelative, ".."+string(filepath.Separator)) {
			absolute = filepath.Clean(filepath.Join(canonicalScope, rawRelative))
		} else if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
			absolute = filepath.Clean(resolved)
		}
		relative, relErr := filepath.Rel(canonicalScope, absolute)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", nil, ErrInvalidReceipt
		}
		if _, exists := unique[absolute]; exists {
			continue
		}
		unique[absolute] = struct{}{}
		canonicalPaths = append(canonicalPaths, absolute)
	}
	if len(canonicalPaths) == 0 {
		return "", nil, ErrInvalidReceipt
	}
	sort.Strings(canonicalPaths)
	return canonicalScope, canonicalPaths, nil
}

func canonicalWorkspaceScope(scope string) (string, error) {
	if strings.TrimSpace(scope) == "" {
		return "", ErrInvalidReceipt
	}
	canonical, err := filepath.Abs(strings.TrimSpace(scope))
	if err != nil {
		return "", err
	}
	canonical = filepath.Clean(canonical)
	if resolved, resolveErr := filepath.EvalSymlinks(canonical); resolveErr == nil {
		canonical = filepath.Clean(resolved)
	}
	return canonical, nil
}

func digestPaths(scope string, paths []string) ([sha256.Size]byte, error) {
	h := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(scope, path)
		if err != nil {
			return [sha256.Size]byte{}, err
		}
		writeDigestField(h, []byte(filepath.ToSlash(relative)))
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			_, _ = h.Write([]byte{0})
			continue
		}
		if err != nil {
			return [sha256.Size]byte{}, err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return [sha256.Size]byte{}, ErrRevisionChanged
		}
		_, _ = h.Write([]byte{1})
		file, err := os.Open(path)
		if err != nil {
			return [sha256.Size]byte{}, err
		}
		openedInfo, statErr := file.Stat()
		if statErr != nil || !os.SameFile(info, openedInfo) {
			_ = file.Close()
			return [sha256.Size]byte{}, ErrRevisionChanged
		}
		contentHash := sha256.New()
		_, copyErr := io.Copy(contentHash, file)
		closedInfo, finalStatErr := file.Stat()
		closeErr := file.Close()
		if copyErr != nil {
			return [sha256.Size]byte{}, copyErr
		}
		if finalStatErr != nil || !os.SameFile(openedInfo, closedInfo) || openedInfo.Size() != closedInfo.Size() || !openedInfo.ModTime().Equal(closedInfo.ModTime()) {
			return [sha256.Size]byte{}, ErrRevisionChanged
		}
		if closeErr != nil {
			return [sha256.Size]byte{}, closeErr
		}
		var mode [4]byte
		binary.BigEndian.PutUint32(mode[:], uint32(closedInfo.Mode().Perm()))
		_, _ = h.Write(mode[:])
		writeDigestField(h, contentHash.Sum(nil))
	}
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

func writeDigestField(writer io.Writer, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write(value)
}

// MutationTool and VerificationTool explicitly opt tools into the adjacent
// mutation-to-dependent-Run edge recognized by the runtime scheduler. Despite
// its historical name, VerificationTool is also implemented by a Run plan that
// may mutate: RequiresPatchCommit is an execution dependency, not a read-only
// classification.
type MutationTool interface {
	ProvidesWorkspaceRevisionBarrier() bool
}

type VerificationTool interface {
	ConsumesWorkspaceRevisionBarrier() bool
}

// PatchCommitDependentTool exposes an explicit model-authored dependency. The
// scheduler uses it independently from read/write classification so a failed
// mutation suppresses both verification-only and mutating Run graphs.
type PatchCommitDependentTool interface {
	RequiresPatchCommit(input map[string]any) bool
}

// MutationResult exposes an opaque post-commit receipt through local typed
// tool data. The receipt is never serialized to a provider or SDK client.
type MutationResult interface {
	WorkspaceRevisionReceipt() (Receipt, bool)
}

type contextKey struct{}

func WithReceipt(ctx context.Context, receipt Receipt) context.Context {
	return context.WithValue(ctx, contextKey{}, receipt)
}

func FromContext(ctx context.Context) (Receipt, bool) {
	receipt, ok := ctx.Value(contextKey{}).(Receipt)
	return receipt, ok && receipt.Valid()
}
