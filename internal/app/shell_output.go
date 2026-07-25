package app

import (
	"github.com/agent-dance/luban/internal/runtime/compact"
	toolshell "github.com/agent-dance/luban/internal/tools/shell"
)

type shellOutputPersister struct{}

func (shellOutputPersister) PersistShellOutput(root string, content []byte, maxBytes int64, preview string) (toolshell.PersistedOutput, error) {
	store := compact.NewResultStore(root)
	path, originalSize, err := store.PersistRawOutput("bash", content, maxBytes)
	if err != nil {
		return toolshell.PersistedOutput{}, err
	}
	return toolshell.PersistedOutput{
		Path: path, OriginalSize: originalSize,
		ModelText: compact.BuildPersistedOutputMessage(path, originalSize, preview),
	}, nil
}
