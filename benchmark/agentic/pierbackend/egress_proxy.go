package pierbackend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

var (
	digestImageReferencePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$`)
	dockerImageIDPattern        = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type egressProxyImageSnapshot struct {
	Reference string
	ImageID   string
}

type dockerImageInspect struct {
	ID           string   `json:"Id"`
	RepoDigests  []string `json:"RepoDigests"`
	Architecture string   `json:"Architecture"`
	OS           string   `json:"Os"`
}

func validateEgressProxyImageReference(reference string) error {
	if digestImageReferencePattern.MatchString(reference) {
		return nil
	}
	return errors.New("egress proxy image must be an immutable repository@sha256 reference")
}

func ensureEgressProxyImage(ctx context.Context, config Config) (egressProxyImageSnapshot, error) {
	if err := validateEgressProxyImageReference(config.EgressProxyImage); err != nil {
		return egressProxyImageSnapshot{}, err
	}
	if snapshot, err := inspectEgressProxyImage(ctx, config); err == nil {
		return snapshot, nil
	}
	lease, err := acquireRegistryGate(ctx, sharedRegistryGatePath(config))
	if err != nil {
		return egressProxyImageSnapshot{}, fmt.Errorf("acquire proxy image registry coordination: %w", err)
	}
	if snapshot, inspectErr := inspectEgressProxyImage(ctx, config); inspectErr == nil {
		if finishErr := lease.finish(true, false, ""); finishErr != nil {
			return egressProxyImageSnapshot{}, finishErr
		}
		return snapshot, nil
	}
	command := exec.CommandContext(ctx, "docker", "pull", config.EgressProxyImage)
	command.Env = sanitizedProcessEnvironment(nil, config.ProviderCredentialEnv)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	pullErr := command.Run()
	throttled := registryThrottleEvidence(stdout.Bytes(), stderr.Bytes(), []byte(fmt.Sprint(pullErr)))
	finishErr := lease.finish(pullErr == nil, throttled, "")
	if pullErr != nil || finishErr != nil {
		var pullFailure error
		if pullErr != nil {
			pullFailure = fmt.Errorf(
				"pull frozen egress proxy image: %w: %s",
				pullErr,
				strings.TrimSpace(stderr.String()),
			)
		}
		return egressProxyImageSnapshot{}, errors.Join(
			pullFailure,
			finishErr,
		)
	}
	return inspectEgressProxyImage(ctx, config)
}

func inspectEgressProxyImage(ctx context.Context, config Config) (egressProxyImageSnapshot, error) {
	command := exec.CommandContext(ctx, "docker", "image", "inspect", config.EgressProxyImage)
	command.Env = sanitizedProcessEnvironment(nil, config.ProviderCredentialEnv)
	raw, err := command.Output()
	if err != nil {
		return egressProxyImageSnapshot{}, fmt.Errorf("inspect frozen egress proxy image: %w", err)
	}
	var records []dockerImageInspect
	// Docker adds fields across releases, so decode the complete records into
	// raw maps first and project only the immutable identity fields we consume.
	var source []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &source); err != nil {
		return egressProxyImageSnapshot{}, fmt.Errorf("decode frozen egress proxy image: %w", err)
	}
	for _, item := range source {
		var record dockerImageInspect
		projected, _ := json.Marshal(map[string]json.RawMessage{
			"Id":           item["Id"],
			"RepoDigests":  item["RepoDigests"],
			"Architecture": item["Architecture"],
			"Os":           item["Os"],
		})
		if err := json.Unmarshal(projected, &record); err != nil {
			return egressProxyImageSnapshot{}, fmt.Errorf("project frozen egress proxy image: %w", err)
		}
		records = append(records, record)
	}
	if len(records) != 1 {
		return egressProxyImageSnapshot{}, errors.New("docker inspect returned an unexpected proxy image count")
	}
	record := records[0]
	if !dockerImageIDPattern.MatchString(record.ID) || record.OS != "linux" || record.Architecture != "amd64" {
		return egressProxyImageSnapshot{}, errors.New("egress proxy local image is not a content-addressed linux/amd64 image")
	}
	if !slices.Contains(record.RepoDigests, config.EgressProxyImage) {
		return egressProxyImageSnapshot{}, errors.New("egress proxy local image is not bound to the configured repository digest")
	}
	return egressProxyImageSnapshot{Reference: config.EgressProxyImage, ImageID: record.ID}, nil
}
