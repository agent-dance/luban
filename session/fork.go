package session

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/securestore"
	"github.com/agent-dance/luban/types"
)

func localizedForkError(key i18n.Key, args ...any) error {
	return errors.New(i18n.Format(i18n.DetectOrLoadLanguage(), key, args...))
}

func localizedForkWrap(key i18n.Key, cause error, args ...any) error {
	return fmt.Errorf("%s: %w", i18n.Format(i18n.DetectOrLoadLanguage(), key, args...), cause)
}

// Fork creates a new session in the source namespace using the supplied
// model-visible snapshot. It copies only metadata that remains valid for a new
// branch and gives the fork its own artifact directory.
func (r *Repository) Fork(source Ref, messages []types.Message) (Ref, error) {
	if source.IsZero() {
		return Ref{}, localizedForkError(i18n.KeyRootSessionForkSourceIncomplete)
	}
	if len(messages) == 0 {
		return Ref{}, localizedForkError(i18n.KeyRootSessionForkSnapshotEmpty)
	}

	store := r.StoreForProjectDir(source.ProjectDir)
	sourceFile, err := store.openPrivateRegularFile(store.sessionPath(source.ID))
	if errors.Is(err, fs.ErrNotExist) {
		return Ref{}, localizedForkWrap(i18n.KeyRootSessionForkSourceMissing, fs.ErrNotExist, source.ID)
	}
	if err != nil {
		return Ref{}, err
	}
	defer sourceFile.Close()
	sourceIdentity, err := sourceFile.Stat()
	if err != nil {
		return Ref{}, err
	}
	if deleted, deleteErr := store.IsDeleted(source.ID); deleteErr != nil {
		return Ref{}, deleteErr
	} else if deleted {
		return Ref{}, localizedForkWrap(i18n.KeyRootSessionForkSourceMissing, fs.ErrNotExist, source.ID)
	}
	sourceMeta, err := store.GetMeta(source.ID)
	if err != nil {
		return Ref{}, localizedForkWrap(i18n.KeyRootSessionForkMetadataLoad, err)
	}
	sourceManifest, err := store.GetCompactionManifest(source.ID)
	if err != nil {
		return Ref{}, localizedForkWrap(i18n.KeyRootSessionForkMetadataLoad, err)
	}
	projectScope, err := store.internalControlProjectScope()
	if err != nil {
		return Ref{}, localizedForkWrap(i18n.KeyRootSessionForkMetadataLoad, err)
	}
	if err := validateInternalControlScopesForCommit(messages, source.ID, projectScope, sourceManifest.ContextGeneration); err != nil {
		return Ref{}, localizedForkWrap(i18n.KeyRootSessionForkMetadataLoad, err)
	}

	target := Ref{ID: uuid.NewString(), ProjectDir: source.ProjectDir}
	sourceArtifacts := store.ArtifactsDir(source.ID)
	targetArtifacts := store.ArtifactsDir(target.ID)
	if err := copyForkArtifacts(store, source.ID, target.ID, sourceArtifacts, targetArtifacts, messages); err != nil {
		return Ref{}, localizedForkWrap(i18n.KeyRootSessionForkArtifactsCopy, err)
	}
	cleanupArtifacts := true
	defer func() {
		if cleanupArtifacts {
			_ = store.removePrivateTree(targetArtifacts)
		}
	}()

	if err := validateForkSource(store, source.ID, sourceFile, sourceIdentity); err != nil {
		return Ref{}, localizedForkWrap(i18n.KeyRootSessionForkSourceMissing, err, source.ID)
	}
	targetControlScope := messagecontrol.NewScope(target.ID, projectScope, 0)
	forkMessages := rewriteForkArtifactPaths(messages, sourceArtifacts, targetArtifacts, targetControlScope)
	if err := store.Save(target.ID, forkMessages); err != nil {
		// Save publishes transcript before derived metadata. A metadata failure
		// must not leave the random target UUID discoverable as a half-fork.
		_ = store.Delete(target.ID)
		return Ref{}, localizedForkWrap(i18n.KeyRootSessionForkTranscriptSave, err)
	}
	forkMeta := SessionMeta{
		CacheLineageID:  normalizeCacheLineageID(source.ID, sourceMeta.CacheLineageID),
		CWD:             sourceMeta.CWD,
		GitBranch:       sourceMeta.GitBranch,
		Provider:        sourceMeta.Provider,
		Model:           sourceMeta.Model,
		SeenToolUseIDs:  forkToolUseIDs(forkMessages),
		LoadedToolNames: forkLoadedToolNames(forkMessages),
	}
	if err := store.SaveMeta(target.ID, forkMeta); err != nil {
		_ = store.Delete(target.ID)
		return Ref{}, localizedForkWrap(i18n.KeyRootSessionForkMetadataSave, err)
	}
	if err := validateForkSource(store, source.ID, sourceFile, sourceIdentity); err != nil {
		_ = store.Delete(target.ID)
		return Ref{}, localizedForkWrap(i18n.KeyRootSessionForkSourceMissing, err, source.ID)
	}
	cleanupArtifacts = false
	return target, nil
}

func validateForkSource(store *FileStore, sourceID string, held *os.File, identity fs.FileInfo) error {
	heldInfo, err := validateAndTightenPrivateRegularFile(held, store.sessionPath(sourceID))
	if err != nil {
		return err
	}
	if !os.SameFile(identity, heldInfo) {
		return fs.ErrInvalid
	}
	current, err := store.openPrivateRegularFile(store.sessionPath(sourceID))
	if err != nil {
		return err
	}
	currentInfo, statErr := current.Stat()
	closeErr := current.Close()
	if statErr != nil {
		return statErr
	}
	if closeErr != nil {
		return closeErr
	}
	if !os.SameFile(identity, currentInfo) {
		return fs.ErrInvalid
	}
	deleted, err := store.IsDeleted(sourceID)
	if err != nil {
		return err
	}
	if deleted {
		return fs.ErrNotExist
	}
	return nil
}

func forkLoadedToolNames(messages []types.Message) []string {
	seen := make(map[string]struct{})
	add := func(blocks []types.ContentBlock) {
		for _, block := range blocks {
			if reference, ok := block.(types.ToolReferenceBlock); ok {
				if name := strings.TrimSpace(reference.ToolName); name != "" {
					seen[name] = struct{}{}
				}
			}
		}
	}
	for _, message := range messages {
		for _, block := range message.Content {
			switch typed := block.(type) {
			case types.ToolReferenceBlock:
				add([]types.ContentBlock{typed})
			case types.ToolResultBlock:
				add(typed.ContentBlocks)
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func forkToolUseIDs(messages []types.Message) []string {
	seen := make(map[string]struct{})
	for _, message := range messages {
		for _, use := range message.GetToolUses() {
			if id := strings.TrimSpace(use.ID); id != "" {
				seen[id] = struct{}{}
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func copyForkArtifacts(store *FileStore, sourceID, targetID, source, target string, messages []types.Message) error {
	root, err := store.storageRoot()
	if err != nil {
		return err
	}
	sourceDir, err := root.OpenRoot(sourceID, false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer sourceDir.Close()
	references := forkArtifactReferenceText(messages)
	if !strings.Contains(references, source+string(os.PathSeparator)) && !containsForkArtifactReference(references, source) {
		return nil
	}
	targetDir, err := root.OpenRootExclusive(targetID)
	if err != nil {
		return err
	}
	defer targetDir.Close()
	err = walkForkArtifacts(sourceDir, targetDir, source, target, "", references)
	if err == nil {
		err = sourceDir.Validate()
	}
	if err == nil {
		err = targetDir.Validate()
	}
	if err == nil {
		err = root.Validate()
	}
	if err != nil {
		_ = store.removePrivateTree(target)
	}
	return err
}

func walkForkArtifacts(sourceRoot, targetRoot *securestore.Root, source, target, prefix, references string) error {
	entries, err := sourceRoot.ReadDir(".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		entryRel := filepath.Join(prefix, entry.Name())
		if prefix == "" {
			entryRel = entry.Name()
		}
		sourcePath := filepath.Join(source, entryRel)
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			if containsForkArtifactReference(references, sourcePath) {
				return localizedForkError(i18n.KeyRootSessionForkArtifactSymlink, sourcePath)
			}
			continue
		}
		if entryInfo.IsDir() {
			child, openErr := sourceRoot.OpenRoot(entry.Name(), false)
			if openErr != nil {
				return openErr
			}
			childInfo, infoErr := child.Info()
			if infoErr != nil || !os.SameFile(entryInfo, childInfo) {
				_ = child.Close()
				if infoErr != nil {
					return infoErr
				}
				return fs.ErrInvalid
			}
			if validateErr := child.Validate(); validateErr != nil {
				_ = child.Close()
				return validateErr
			}
			if err := targetRoot.MkdirAll(entry.Name()); err != nil {
				_ = child.Close()
				return err
			}
			targetChild, openErr := targetRoot.OpenRoot(entry.Name(), false)
			if openErr != nil {
				_ = child.Close()
				return openErr
			}
			walkErr := walkForkArtifacts(child, targetChild, source, target, entryRel, references)
			closeTargetErr := targetChild.Close()
			closeSourceErr := child.Close()
			if walkErr != nil {
				return walkErr
			}
			if closeTargetErr != nil {
				return closeTargetErr
			}
			if closeSourceErr != nil {
				return closeSourceErr
			}
			if err := sourceRoot.Validate(); err != nil {
				return err
			}
			if err := targetRoot.Validate(); err != nil {
				return err
			}
			continue
		}
		if !containsForkArtifactReference(references, sourcePath) {
			continue
		}
		if !entryInfo.Mode().IsRegular() {
			return localizedForkError(i18n.KeyRootSessionForkArtifactUnsupported, sourcePath)
		}
		if err := copyForkArtifactFile(sourceRoot, targetRoot, entry.Name(), sourcePath, filepath.Join(target, entryRel), entryInfo); err != nil {
			return err
		}
	}
	return nil
}

func containsForkArtifactReference(references, path string) bool {
	for remaining := references; ; {
		index := strings.Index(remaining, path)
		if index < 0 {
			return false
		}
		end := index + len(path)
		beforeBoundary := index == 0 || !isForkPathRune(rune(remaining[index-1]))
		afterBoundary := end == len(remaining) || !isForkPathRune(rune(remaining[end]))
		if beforeBoundary && afterBoundary {
			return true
		}
		remaining = remaining[index+1:]
	}
}

func isForkPathRune(value rune) bool {
	return value == '/' || value == '\\' || value == '_' || value == '-' || value == '.' ||
		value >= '0' && value <= '9' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func forkArtifactReferenceText(messages []types.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		collectForkContentStrings(&builder, message.Content)
	}
	return builder.String()
}

func collectForkContentStrings(builder *strings.Builder, content []types.ContentBlock) {
	for _, block := range content {
		switch value := block.(type) {
		case types.TextBlock:
			appendForkReference(builder, value.Text)
		case types.ThinkingBlock:
			appendForkReference(builder, value.Thinking)
		case types.ToolUseBlock:
			collectForkValueStrings(builder, value.Input)
		case types.ToolResultBlock:
			appendForkReference(builder, value.Content)
			collectForkContentStrings(builder, value.ContentBlocks)
			collectForkValueStrings(builder, value.Data)
			for _, metadata := range value.Metadata {
				appendForkReference(builder, metadata)
			}
			for _, message := range value.NewMessages {
				collectForkContentStrings(builder, message.Content)
			}
		case types.ContentReplacementBlock:
			appendForkReference(builder, value.Replacement)
		case types.UnknownBlock:
			appendForkReference(builder, string(value.Raw))
		}
	}
}

func collectForkValueStrings(builder *strings.Builder, value any) {
	switch typed := value.(type) {
	case string:
		appendForkReference(builder, typed)
	case []any:
		for _, item := range typed {
			collectForkValueStrings(builder, item)
		}
	case map[string]any:
		for _, item := range typed {
			collectForkValueStrings(builder, item)
		}
	case map[string]string:
		for _, item := range typed {
			appendForkReference(builder, item)
		}
	}
}

func appendForkReference(builder *strings.Builder, value string) {
	if value == "" {
		return
	}
	builder.WriteString(value)
	builder.WriteByte('\x00')
}

func copyForkArtifactFile(sourceRoot, targetRoot *securestore.Root, relative, source, target string, expected fs.FileInfo) error {
	in, err := sourceRoot.OpenFile(relative, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer in.Close()
	if _, err := validateAndTightenPrivateRegularFile(in, source); err != nil {
		return err
	}
	opened, err := in.Stat()
	if err != nil {
		return err
	}
	if expected == nil || !os.SameFile(expected, opened) {
		return &os.PathError{Op: "open", Path: source, Err: fs.ErrInvalid}
	}
	out, err := targetRoot.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = targetRoot.Remove(relative)
		return err
	}
	if _, err := validateAndTightenPrivateRegularFile(in, source); err != nil {
		_ = out.Close()
		_ = targetRoot.Remove(relative)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = targetRoot.Remove(relative)
		return err
	}
	if err := out.Close(); err != nil {
		_ = targetRoot.Remove(relative)
		return err
	}
	if err := sourceRoot.Validate(); err != nil {
		_ = targetRoot.Remove(relative)
		return err
	}
	if err := targetRoot.Validate(); err != nil {
		_ = targetRoot.Remove(relative)
		return err
	}
	return nil
}

func rewriteForkArtifactPaths(messages []types.Message, source, target string, controlScope messagecontrol.Scope) []types.Message {
	out := make([]types.Message, len(messages))
	for i, message := range messages {
		wasTrusted := message.HasInternalControlProvenance()
		message.Content = rewriteForkContent(message.Content, source, target, controlScope)
		if wasTrusted {
			message = message.WithInternalControlProvenance(messagecontrol.Runtime(), controlScope)
		}
		out[i] = message
	}
	return out
}

func rewriteForkContent(content []types.ContentBlock, source, target string, controlScope messagecontrol.Scope) []types.ContentBlock {
	out := make([]types.ContentBlock, len(content))
	for i, block := range content {
		switch value := block.(type) {
		case types.TextBlock:
			value.Text = strings.ReplaceAll(value.Text, source, target)
			out[i] = value
		case types.ThinkingBlock:
			value.Thinking = strings.ReplaceAll(value.Thinking, source, target)
			out[i] = value
		case types.ToolUseBlock:
			value.Input = rewriteForkStringMap(value.Input, source, target)
			out[i] = value
		case types.ToolResultBlock:
			value.Content = strings.ReplaceAll(value.Content, source, target)
			value.ContentBlocks = rewriteForkContent(value.ContentBlocks, source, target, controlScope)
			value.Data = rewriteForkValue(value.Data, source, target)
			value.Metadata = rewriteForkMetadata(value.Metadata, source, target)
			value.NewMessages = rewriteForkArtifactPaths(value.NewMessages, source, target, controlScope)
			out[i] = value
		case types.ContentReplacementBlock:
			wasTrusted := value.HasInternalReplacementProvenance()
			value.Replacement = strings.ReplaceAll(value.Replacement, source, target)
			if wasTrusted {
				value = value.WithInternalReplacementProvenance(messagecontrol.Runtime(), controlScope)
			}
			out[i] = value
		case types.ImageBlock:
			if value.Source != nil {
				clone := *value.Source
				value.Source = &clone
			}
			out[i] = value
		case types.DocumentBlock:
			if value.Source != nil {
				clone := *value.Source
				value.Source = &clone
			}
			out[i] = value
		case types.UnknownBlock:
			value.Raw = bytes.ReplaceAll(append([]byte(nil), value.Raw...), []byte(source), []byte(target))
			out[i] = value
		default:
			out[i] = block
		}
	}
	return out
}

func rewriteForkStringMap(input map[string]any, source, target string) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = rewriteForkValue(value, source, target)
	}
	return out
}

func rewriteForkMetadata(input map[string]string, source, target string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = strings.ReplaceAll(value, source, target)
	}
	return out
}

func rewriteForkValue(value any, source, target string) any {
	switch typed := value.(type) {
	case string:
		return strings.ReplaceAll(typed, source, target)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = rewriteForkValue(item, source, target)
		}
		return out
	case map[string]any:
		return rewriteForkStringMap(typed, source, target)
	case map[string]string:
		return rewriteForkMetadata(typed, source, target)
	default:
		return value
	}
}
