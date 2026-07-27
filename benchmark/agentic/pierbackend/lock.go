package pierbackend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

type imageReference struct {
	Registry   string
	Repository string
	Reference  string
}

var abbreviatedCommitPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// GenerateInventoryLock resolves every mutable task image reference to an OCI
// digest and writes a new immutable lock. An existing, different lock is never
// overwritten.
func GenerateInventoryLock(ctx context.Context, config Config, manifest harness.Manifest) (InventoryLock, string, error) {
	commit, err := gitCommit(ctx, config.DatasetRepositoryRoot)
	if err != nil {
		return InventoryLock{}, "", err
	}
	if commit != manifest.Dataset.Commit {
		return InventoryLock{}, "", errors.New("dataset checkout is not at the manifest commit")
	}
	taskRoot := filepath.Join(config.DatasetRepositoryRoot, manifest.Dataset.Root)
	var tasks []LockedTask
	repositories := map[string]string{}
	err = filepath.WalkDir(taskRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "task.toml" {
			return nil
		}
		values, parseErr := parseTaskTOML(path)
		if parseErr != nil {
			return parseErr
		}
		directory := filepath.Dir(path)
		relative, relErr := filepath.Rel(taskRoot, directory)
		if relErr != nil {
			return relErr
		}
		manifestSHA, hashErr := harness.HashFile(path)
		if hashErr != nil {
			return hashErr
		}
		instructionSHA, hashErr := harness.HashFile(filepath.Join(directory, "instruction.md"))
		if hashErr != nil {
			return hashErr
		}
		id := values["metadata.task_id"]
		baseCommit := values["metadata.base_commit_hash"]
		if id == "" || !abbreviatedCommitPattern.MatchString(baseCommit) {
			return fmt.Errorf("task %q has an invalid source identity", id)
		}
		if _, duplicate := repositories[id]; duplicate {
			return fmt.Errorf("task %s is duplicated", id)
		}
		repositories[id] = values["metadata.repository_url"]
		tasks = append(tasks, LockedTask{
			ID: id, RelativePath: filepath.ToSlash(relative),
			BaseCommit: baseCommit, ManifestSHA256: manifestSHA,
			InstructionSHA256: instructionSHA, Image: values["environment.docker_image"],
		})
		return nil
	})
	if err != nil {
		return InventoryLock{}, "", err
	}
	if len(tasks) != manifest.Selection.ExpectedTaskCount {
		return InventoryLock{}, "", fmt.Errorf("dataset contains %d tasks, expected %d", len(tasks), manifest.Selection.ExpectedTaskCount)
	}
	slices.SortFunc(tasks, func(left, right LockedTask) int { return strings.Compare(left.ID, right.ID) })
	coverage := "full"
	var taskIDs []string
	if manifest.Selection.Mode == "tasks" {
		coverage = "tasks"
		selected := make(map[string]struct{}, len(manifest.Selection.TaskIDs))
		for _, id := range manifest.Selection.TaskIDs {
			selected[id] = struct{}{}
		}
		filtered := make([]LockedTask, 0, len(selected))
		for _, task := range tasks {
			if _, keep := selected[task.ID]; keep {
				filtered = append(filtered, task)
				delete(selected, task.ID)
			}
		}
		if len(selected) != 0 || len(filtered) != len(manifest.Selection.TaskIDs) {
			return InventoryLock{}, "", errors.New("pilot selection is not fully present in the dataset universe")
		}
		tasks = filtered
		taskIDs = make([]string, 0, len(tasks))
		for _, task := range tasks {
			taskIDs = append(taskIDs, task.ID)
		}
	}
	client := &http.Client{Timeout: 60 * time.Second}
	if err := resolveTaskBaseCommits(ctx, tasks, repositories, client); err != nil {
		return InventoryLock{}, "", err
	}
	if err := resolveTaskImages(ctx, tasks, client, sharedRegistryGatePath(config)); err != nil {
		return InventoryLock{}, "", err
	}
	lock := InventoryLock{
		SchemaVersion: InventorySchemaVersion, DatasetCommit: commit, Coverage: coverage,
		UniverseTaskCount: manifest.Selection.ExpectedTaskCount, TaskIDs: taskIDs, Tasks: tasks,
	}
	inventory := make([]harness.Task, 0, len(tasks))
	for _, task := range tasks {
		inventory = append(inventory, task.HarnessTask())
	}
	digest, err := harness.HashTaskInventory(inventory)
	if err != nil {
		return InventoryLock{}, "", err
	}
	if existing, loadErr := loadInventoryLock(config.InventoryLockPath); loadErr == nil {
		oldHash, oldErr := harness.HashCanonical(existing)
		newHash, newErr := harness.HashCanonical(lock)
		if oldErr != nil || newErr != nil || oldHash != newHash {
			return InventoryLock{}, "", errors.New("refusing to overwrite a different immutable inventory lock")
		}
		return existing, digest, nil
	} else if !errors.Is(loadErr, os.ErrNotExist) {
		return InventoryLock{}, "", loadErr
	}
	if err := harness.WriteJSONAtomic(config.InventoryLockPath, lock, 0o644); err != nil {
		return InventoryLock{}, "", err
	}
	return lock, digest, nil
}

func resolveTaskBaseCommits(ctx context.Context, tasks []LockedTask, repositories map[string]string, client *http.Client) error {
	for index := range tasks {
		if len(tasks[index].BaseCommit) == 40 {
			continue
		}
		resolved, err := resolveGitHubCommit(ctx, client, repositories[tasks[index].ID], tasks[index].BaseCommit)
		if err != nil {
			return fmt.Errorf("resolve task %s base commit: %w", tasks[index].ID, err)
		}
		tasks[index].BaseCommit = resolved
	}
	return nil
}

func resolveGitHubCommit(ctx context.Context, client *http.Client, repositoryURL, abbreviated string) (string, error) {
	parsed, err := url.Parse(repositoryURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("abbreviated commits require a canonical HTTPS GitHub repository URL")
	}
	path := strings.Trim(strings.TrimSuffix(parsed.EscapedPath(), ".git"), "/")
	segments := strings.Split(path, "/")
	if len(segments) != 2 || segments[0] == "" || segments[1] == "" || !abbreviatedCommitPattern.MatchString(abbreviated) || len(abbreviated) >= 40 {
		return "", errors.New("abbreviated GitHub commit input is invalid")
	}
	commitURL := "https://github.com/" + path + "/commit/" + abbreviated
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, commitURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "text/html")
	request.Header.Set("User-Agent", "agentic-bench-lock-v2")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub commit page status %d", response.StatusCode)
	}
	const maximumPageBytes = 8 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumPageBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maximumPageBytes {
		return "", errors.New("GitHub commit page exceeds the resolution limit")
	}
	linkPattern := regexp.MustCompile(regexp.QuoteMeta("/"+path+"/commit/") + `([0-9a-f]{40})`)
	candidates := map[string]struct{}{}
	for _, match := range linkPattern.FindAllSubmatch(body, -1) {
		candidate := string(match[1])
		if strings.HasPrefix(candidate, abbreviated) {
			candidates[candidate] = struct{}{}
		}
	}
	if len(candidates) != 1 {
		return "", fmt.Errorf("abbreviated commit has %d canonical matches", len(candidates))
	}
	for candidate := range candidates {
		return candidate, nil
	}
	panic("unreachable")
}

func resolveTaskImages(ctx context.Context, tasks []LockedTask, client *http.Client, gatePath string) error {
	type resolution struct {
		digest string
		err    error
	}
	unique := map[string]struct{}{}
	for _, task := range tasks {
		if task.ID == "" || task.BaseCommit == "" || task.Image == "" {
			return errors.New("task inventory contains an incomplete identity")
		}
		unique[task.Image] = struct{}{}
	}
	results := make(map[string]resolution, len(unique))
	var resultMu sync.Mutex
	jobs := make(chan string)
	var workers sync.WaitGroup
	// Public ECR applies a fairly small anonymous request budget.  A wide fanout
	// makes lock generation nondeterministically fail with HTTP 429 even though
	// every tag is valid.  Two workers keep the command practical while staying
	// below the observed public-registry throttle.
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for image := range jobs {
				digest, err := resolveImageDigest(ctx, client, image, gatePath)
				resultMu.Lock()
				results[image] = resolution{digest: digest, err: err}
				resultMu.Unlock()
			}
		}()
	}
	for image := range unique {
		select {
		case jobs <- image:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return ctx.Err()
		}
	}
	close(jobs)
	workers.Wait()
	for index := range tasks {
		resolved := results[tasks[index].Image]
		if resolved.err != nil {
			return fmt.Errorf("resolve image %s: %w", tasks[index].Image, resolved.err)
		}
		if !harness.IsImageDigest(resolved.digest) {
			return fmt.Errorf("registry returned an invalid digest for %s", tasks[index].Image)
		}
		tasks[index].ImageDigest = resolved.digest
	}
	return nil
}

func resolveImageDigest(ctx context.Context, client *http.Client, image, gatePath string) (string, error) {
	reference, err := parseImageReference(image)
	if err != nil {
		return "", err
	}
	var lastErr error
	for attempt := 0; attempt < 7; attempt++ {
		lease, gateErr := acquireRegistryGate(ctx, gatePath)
		if gateErr != nil {
			return "", gateErr
		}
		digest, retryAfter, retry, throttled, requestErr := resolveImageDigestOnce(ctx, client, reference)
		finishErr := lease.finish(requestErr == nil, throttled, retryAfter)
		if finishErr != nil {
			return "", finishErr
		}
		if requestErr == nil {
			return digest, nil
		}
		lastErr = requestErr
		if !retry || attempt == 6 {
			return "", requestErr
		}
		delay := registryRetryDelay(attempt, retryAfter)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	return "", lastErr
}

func resolveImageDigestOnce(ctx context.Context, client *http.Client, reference imageReference) (string, string, bool, bool, error) {
	manifestURL := "https://" + reference.Registry + "/v2/" + reference.Repository + "/manifests/" + url.PathEscape(reference.Reference)
	request, err := registryManifestRequest(ctx, manifestURL, "")
	if err != nil {
		return "", "", false, false, err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", "", true, false, err
	}
	if response.StatusCode == http.StatusUnauthorized {
		challenge := response.Header.Get("WWW-Authenticate")
		response.Body.Close()
		token, tokenErr := registryBearerToken(ctx, client, challenge)
		if tokenErr != nil {
			return "", "", false, false, tokenErr
		}
		request, err = registryManifestRequest(ctx, manifestURL, token)
		if err != nil {
			return "", "", false, false, err
		}
		response, err = client.Do(request)
		if err != nil {
			return "", "", true, false, err
		}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", response.Header.Get("Retry-After"), retryableRegistryStatus(response.StatusCode), response.StatusCode == http.StatusTooManyRequests, fmt.Errorf("registry status %d", response.StatusCode)
	}
	return strings.ToLower(strings.TrimSpace(response.Header.Get("Docker-Content-Digest"))), "", false, false, nil
}

func registryManifestRequest(ctx context.Context, manifestURL, bearerToken string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, manifestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json")
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	return request, nil
}

func retryableRegistryStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func registryRetryDelay(attempt int, retryAfter string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds >= 0 {
		delay := time.Duration(seconds) * time.Second
		if delay > 60*time.Second {
			return 60 * time.Second
		}
		return delay
	}
	if instant, err := http.ParseTime(retryAfter); err == nil {
		delay := time.Until(instant)
		if delay < 0 {
			return 0
		}
		if delay > 60*time.Second {
			return 60 * time.Second
		}
		return delay
	}
	delay := time.Second << min(attempt, 5)
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

func parseImageReference(image string) (imageReference, error) {
	if strings.Contains(image, "@") {
		return imageReference{}, errors.New("inventory source image must be a tag, not a digest")
	}
	slash := strings.IndexByte(image, '/')
	colon := strings.LastIndexByte(image, ':')
	if slash < 1 || colon <= slash+1 || colon == len(image)-1 {
		return imageReference{}, fmt.Errorf("unsupported image reference %q", image)
	}
	return imageReference{Registry: image[:slash], Repository: image[slash+1 : colon], Reference: image[colon+1:]}, nil
}

func registryBearerToken(ctx context.Context, client *http.Client, challenge string) (string, error) {
	if !strings.HasPrefix(challenge, "Bearer ") {
		return "", errors.New("registry did not provide a bearer challenge")
	}
	fields := map[string]string{}
	for _, field := range strings.Split(strings.TrimPrefix(challenge, "Bearer "), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(field), "=")
		if ok {
			fields[key] = strings.Trim(value, "\"")
		}
	}
	realm, err := url.Parse(fields["realm"])
	if err != nil || realm.Scheme != "https" || realm.Host == "" {
		return "", errors.New("registry bearer realm is not HTTPS")
	}
	query := realm.Query()
	for _, key := range []string{"service", "scope"} {
		if fields[key] != "" {
			query.Set(key, fields[key])
		}
	}
	realm.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("registry token status %d", response.StatusCode)
	}
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Token == "" {
		payload.Token = payload.AccessToken
	}
	if payload.Token == "" {
		return "", errors.New("registry bearer response has no token")
	}
	return payload.Token, nil
}
