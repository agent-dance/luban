// Package tools — file_edit_vscode_notifier.go: notifies the VSCode IDE
// extension after each FileEdit / FileWrite so the in-IDE diff view
// refreshes in real time. Mirrors the TS notifyVscodeFileUpdated hook.
//
// The Go runtime is host-agnostic, so we expose a small interface that the
// embedding host (CLI, server, plugin) implements. When unset the notifier
// is a no-op — preserving backwards compatibility with hosts that have no
// IDE.
package tools

import "context"

// VSCodeNotifier is the optional hook FileEditTool / FileWriteTool call
// after a successful write. Implementations typically issue a JSON-RPC
// notification over stdio / a Unix socket to a running VSCode extension.
//
// All callers MUST treat errors as non-fatal: a missing or wedged extension
// must not block the underlying file operation.
type VSCodeNotifier interface {
	// NotifyFileUpdated is invoked once per successful edit/write with the
	// file's absolute path and (optionally) the resulting content. The
	// notifier may use either to refresh its UI; passing the content
	// avoids an extra read on the host side.
	NotifyFileUpdated(ctx context.Context, absPath, content string) error
}

// noopVSCodeNotifier is the default when no notifier is supplied.
type noopVSCodeNotifier struct{}

// NewNoopVSCodeNotifier returns a notifier that swallows every call.
// Useful in tests and as the safe default in CLI bootstrap.
func NewNoopVSCodeNotifier() VSCodeNotifier { return noopVSCodeNotifier{} }

// NotifyFileUpdated implements VSCodeNotifier as a no-op.
func (noopVSCodeNotifier) NotifyFileUpdated(ctx context.Context, absPath, content string) error {
	return nil
}
