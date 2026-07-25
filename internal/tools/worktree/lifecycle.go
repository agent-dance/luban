package worktree

import "context"

type LifecycleEventType string

const (
	LifecycleEnter LifecycleEventType = "worktree_enter"
	LifecycleExit  LifecycleEventType = "worktree_exit"
)

type LifecycleEvent struct {
	Type        LifecycleEventType
	EntityID    string
	ToolName    string
	Status      string
	RepoRoot    string
	Branch      string
	Path        string
	CreatedHere bool
}

type LifecyclePublisher interface {
	PublishWorktreeLifecycle(context.Context, LifecycleEvent) error
}

type LifecyclePublisherFunc func(context.Context, LifecycleEvent) error

func (fn LifecyclePublisherFunc) PublishWorktreeLifecycle(ctx context.Context, event LifecycleEvent) error {
	return fn(ctx, event)
}

type LifecycleFactory func(repoRoot string) LifecyclePublisher
