package schedule

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-dance/luban/internal/store/secureio"
)

const scheduleStoreSchemaVersion = 1

type persistedFile struct {
	SchemaVersion int            `json:"schema_version"`
	Jobs          []persistedJob `json:"jobs"`
}

type persistedJob struct {
	ID          string             `json:"id"`
	Expression  string             `json:"expression"`
	Prompt      string             `json:"prompt"`
	Recurring   bool               `json:"recurring"`
	CreatedAt   string             `json:"created_at"`
	LastFiredAt string             `json:"last_fired_at,omitempty"`
	LastWallKey string             `json:"last_wall_key,omitempty"`
	Pending     *persistedDelivery `json:"pending_delivery,omitempty"`
}

type persistedDelivery struct {
	ID            string `json:"id"`
	ScheduledAt   string `json:"scheduled_at"`
	WallKey       string `json:"wall_key"`
	NextAttemptAt string `json:"next_attempt_at"`
	Attempt       int    `json:"attempt"`
}

type repository struct {
	root     string
	dir      string
	path     string
	lockPath string
}

func newRepository(root string) (*repository, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return nil, newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	cleaned, err := filepath.Abs(filepath.Clean(trimmed))
	if err != nil {
		return nil, newDomainError(errorKindStoreInvalid, err)
	}
	dir := filepath.Join(cleaned, ".luban-code", "schedule")
	return &repository{
		root:     cleaned,
		dir:      dir,
		path:     filepath.Join(dir, "jobs.json"),
		lockPath: filepath.Join(dir, "jobs.lock"),
	}, nil
}

func (r *repository) read() ([]*storedJob, error) {
	if r == nil {
		return nil, newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	if err := secureio.PreparePrivateRuntimeLock(r.lockPath); err != nil {
		return nil, newDomainError(errorKindStoreRead, err)
	}
	value, err := secureio.WithRuntimeFileLockResult(r.lockPath, func() (any, error) {
		return r.readLocked()
	})
	if err != nil {
		return nil, newDomainError(errorKindStoreRead, err)
	}
	jobs, ok := value.([]*storedJob)
	if !ok {
		return nil, newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	return jobs, nil
}

func (r *repository) update(fn func([]*storedJob) ([]*storedJob, bool, error)) ([]*storedJob, error) {
	if r == nil || fn == nil {
		return nil, newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	if err := secureio.PreparePrivateRuntimeLock(r.lockPath); err != nil {
		return nil, newDomainError(errorKindStoreWrite, err)
	}
	value, err := secureio.WithRuntimeFileLockResult(r.lockPath, func() (any, error) {
		jobs, err := r.readLocked()
		if err != nil {
			return nil, err
		}
		updated, changed, err := fn(jobs)
		if err != nil {
			return nil, err
		}
		if changed {
			if err := r.writeLocked(updated); err != nil {
				return nil, err
			}
		}
		return updated, nil
	})
	if err != nil {
		if domainErrorKind(err) != 0 {
			return nil, err
		}
		return nil, newDomainError(errorKindStoreWrite, err)
	}
	jobs, ok := value.([]*storedJob)
	if !ok {
		return nil, newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	return jobs, nil
}

func (r *repository) readLocked() ([]*storedJob, error) {
	file, err := secureio.OpenPrivateRuntimeRegularFile(r.path)
	if errors.Is(err, fs.ErrNotExist) {
		return []*storedJob{}, nil
	}
	if err != nil {
		return nil, newDomainError(errorKindStoreRead, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 8<<20))
	if err != nil {
		return nil, newDomainError(errorKindStoreRead, err)
	}
	if err := rejectDuplicateJSONFields(data); err != nil {
		return nil, newDomainError(errorKindStoreInvalid, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var body persistedFile
	if err := decoder.Decode(&body); err != nil {
		return nil, newDomainError(errorKindStoreInvalid, err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return nil, newDomainError(errorKindStoreInvalid, err)
	}
	if body.SchemaVersion != scheduleStoreSchemaVersion {
		return nil, newDomainError(errorKindStoreVersion, schemaVersionError(body.SchemaVersion))
	}
	jobs := make([]*storedJob, 0, len(body.Jobs))
	if len(body.Jobs) > maxJobs {
		return nil, newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	seen := make(map[string]struct{}, len(body.Jobs))
	for _, item := range body.Jobs {
		job, err := decodePersistedJob(item, r.root)
		if err != nil {
			return nil, newDomainError(errorKindStoreInvalid, err)
		}
		if _, exists := seen[job.ID]; exists {
			return nil, newDomainError(errorKindStoreInvalid, fmt.Errorf("duplicate job id %q", job.ID))
		}
		seen[job.ID] = struct{}{}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func rejectDuplicateJSONFields(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanUniqueJSONValue(decoder); err != nil {
		return err
	}
	return rejectTrailingJSON(decoder)
}

func scanUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fs.ErrInvalid
			}
			if _, duplicate := seen[key]; duplicate {
				return fs.ErrInvalid
			}
			seen[key] = struct{}{}
			if err := scanUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return fs.ErrInvalid
		}
	case '[':
		for decoder.More() {
			if err := scanUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return fs.ErrInvalid
		}
	default:
		return fs.ErrInvalid
	}
	return nil
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fs.ErrInvalid
	}
	return err
}

func decodePersistedJob(item persistedJob, root string) (*storedJob, error) {
	if !validJobID(item.ID) || strings.TrimSpace(item.Expression) != item.Expression || item.Expression == "" || strings.TrimSpace(item.Prompt) == "" {
		return nil, fs.ErrInvalid
	}
	if _, err := parseExpression(item.Expression); err != nil {
		return nil, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, item.CreatedAt)
	if err != nil || createdAt.IsZero() {
		return nil, fs.ErrInvalid
	}
	job := &storedJob{Job: Job{
		ID:          item.ID,
		Expression:  item.Expression,
		Prompt:      item.Prompt,
		Recurring:   item.Recurring,
		Durable:     true,
		CreatedAt:   createdAt.UTC(),
		ProjectRoot: root,
	}, LastWallKey: item.LastWallKey}
	if item.LastFiredAt != "" {
		last, err := time.Parse(time.RFC3339Nano, item.LastFiredAt)
		if err != nil {
			return nil, err
		}
		last = last.UTC()
		if last.Before(job.CreatedAt) || !job.Recurring || strings.TrimSpace(item.LastWallKey) == "" {
			return nil, fs.ErrInvalid
		}
		job.LastFiredAt = &last
	} else if item.LastWallKey != "" {
		return nil, fs.ErrInvalid
	}
	if item.Pending != nil {
		pending, err := decodePersistedDelivery(*item.Pending)
		if err != nil {
			return nil, err
		}
		job.Pending = pending
		if pending.ScheduledAt.Before(job.CreatedAt) || pending.NextAttemptAt.Before(pending.ScheduledAt) || pending.ID != deliveryID(job.ID, pending.ScheduledAt) {
			return nil, fs.ErrInvalid
		}
	}
	return job, nil
}

func validJobID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if !((char >= '0' && char <= '9') ||
			(char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

func decodePersistedDelivery(item persistedDelivery) (*pendingDelivery, error) {
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.WallKey) == "" || item.Attempt < 1 {
		return nil, fs.ErrInvalid
	}
	scheduledAt, err := time.Parse(time.RFC3339Nano, item.ScheduledAt)
	if err != nil {
		return nil, err
	}
	nextAttemptAt, err := time.Parse(time.RFC3339Nano, item.NextAttemptAt)
	if err != nil {
		return nil, err
	}
	return &pendingDelivery{
		ID:            item.ID,
		ScheduledAt:   scheduledAt.UTC(),
		WallKey:       item.WallKey,
		NextAttemptAt: nextAttemptAt.UTC(),
		Attempt:       item.Attempt,
	}, nil
}

func (r *repository) writeLocked(jobs []*storedJob) error {
	body := persistedFile{SchemaVersion: scheduleStoreSchemaVersion, Jobs: make([]persistedJob, 0, len(jobs))}
	for _, job := range jobs {
		item, err := encodePersistedJob(job)
		if err != nil {
			return newDomainError(errorKindStoreInvalid, err)
		}
		body.Jobs = append(body.Jobs, item)
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return newDomainError(errorKindStoreWrite, err)
	}
	data = append(data, '\n')
	if err := secureio.AtomicWritePrivateRuntimeFile(r.path, data); err != nil {
		return newDomainError(errorKindStoreWrite, err)
	}
	return nil
}

func encodePersistedJob(job *storedJob) (persistedJob, error) {
	if job == nil || !job.Durable || !validJobID(job.ID) || strings.TrimSpace(job.Expression) != job.Expression || job.Expression == "" || strings.TrimSpace(job.Prompt) == "" || job.CreatedAt.IsZero() {
		return persistedJob{}, fs.ErrInvalid
	}
	item := persistedJob{
		ID:          job.ID,
		Expression:  job.Expression,
		Prompt:      job.Prompt,
		Recurring:   job.Recurring,
		CreatedAt:   job.CreatedAt.UTC().Format(time.RFC3339Nano),
		LastWallKey: job.LastWallKey,
	}
	if job.LastFiredAt != nil {
		item.LastFiredAt = job.LastFiredAt.UTC().Format(time.RFC3339Nano)
	}
	if job.Pending != nil {
		if strings.TrimSpace(job.Pending.ID) == "" || strings.TrimSpace(job.Pending.WallKey) == "" || job.Pending.Attempt < 1 || job.Pending.ScheduledAt.IsZero() || job.Pending.NextAttemptAt.IsZero() {
			return persistedJob{}, fs.ErrInvalid
		}
		item.Pending = &persistedDelivery{
			ID:            job.Pending.ID,
			ScheduledAt:   job.Pending.ScheduledAt.UTC().Format(time.RFC3339Nano),
			WallKey:       job.Pending.WallKey,
			NextAttemptAt: job.Pending.NextAttemptAt.UTC().Format(time.RFC3339Nano),
			Attempt:       job.Pending.Attempt,
		}
	}
	return item, nil
}
