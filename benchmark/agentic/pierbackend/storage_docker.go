package pierbackend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

const dockerStorageInfoTemplate = `{"docker_root_dir":{{json .DockerRootDir}},"operating_system":{{json .OSType}},"storage_driver":{{json .Driver}}}`

type dockerStorageAuthority struct {
	rootDir       string
	storageDriver string
}

type dockerStorageCommand func(ctx context.Context, argv, environment []string) ([]byte, error)

type dockerStorageCommandError struct{ cause error }

func (failure dockerStorageCommandError) Error() string {
	return "Docker storage authority could not be inspected"
}

func (failure dockerStorageCommandError) Unwrap() error { return failure.cause }

func systemDockerStorageCommand(ctx context.Context, argv, environment []string) ([]byte, error) {
	if len(argv) == 0 || argv[0] != "docker" {
		return nil, errors.New("invalid Docker storage inspection argv")
	}
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Env = environment
	output, err := command.Output()
	if err != nil {
		return nil, dockerStorageCommandError{cause: err}
	}
	return output, nil
}

func resolveDockerStorageAuthority(ctx context.Context, environment []string, run dockerStorageCommand) (dockerStorageAuthority, error) {
	if run == nil {
		return dockerStorageAuthority{}, errors.New("Docker storage inspection command is required")
	}
	endpointRaw, err := run(ctx, []string{"docker", "context", "inspect", "--format", `{{json .Endpoints.docker.Host}}`}, environment)
	if err != nil {
		return dockerStorageAuthority{}, dockerStorageCommandError{cause: err}
	}
	endpoint, err := decodeSingleJSONString(endpointRaw)
	if err != nil || !validLocalDockerEndpoint(endpoint) {
		return dockerStorageAuthority{}, errors.New("Docker storage observation requires a local Unix daemon endpoint")
	}
	infoRaw, err := run(ctx, []string{"docker", "info", "--format", dockerStorageInfoTemplate}, environment)
	if err != nil {
		return dockerStorageAuthority{}, dockerStorageCommandError{cause: err}
	}
	var wire struct {
		RootDir         string `json:"docker_root_dir"`
		OperatingSystem string `json:"operating_system"`
		StorageDriver   string `json:"storage_driver"`
	}
	decoder := json.NewDecoder(bytes.NewReader(infoRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return dockerStorageAuthority{}, errors.New("Docker storage inspection returned an invalid receipt")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return dockerStorageAuthority{}, errors.New("Docker storage inspection returned trailing data")
	}
	if wire.OperatingSystem != "linux" || wire.StorageDriver == "" ||
		wire.RootDir == "" || strings.IndexByte(wire.RootDir, 0) >= 0 ||
		!filepath.IsAbs(wire.RootDir) || filepath.Clean(wire.RootDir) != wire.RootDir {
		return dockerStorageAuthority{}, errors.New("Docker storage inspection returned an unsupported authority")
	}
	return dockerStorageAuthority{rootDir: wire.RootDir, storageDriver: wire.StorageDriver}, nil
}

func decodeSingleJSONString(raw []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value string
	if err := decoder.Decode(&value); err != nil {
		return "", errors.New("Docker endpoint inspection returned invalid JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", errors.New("Docker endpoint inspection returned trailing data")
	}
	return value, nil
}

func validLocalDockerEndpoint(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "unix" || parsed.User != nil || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path == "" || !filepath.IsAbs(parsed.Path) {
		return false
	}
	return filepath.Clean(parsed.Path) == parsed.Path
}
