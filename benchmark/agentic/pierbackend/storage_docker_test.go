package pierbackend

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fixtureDockerStorageCommand struct {
	responses [][]byte
	errors    []error
	argv      [][]string
	env       [][]string
}

func (fixture *fixtureDockerStorageCommand) run(_ context.Context, argv, environment []string) ([]byte, error) {
	fixture.argv = append(fixture.argv, append([]string(nil), argv...))
	fixture.env = append(fixture.env, append([]string(nil), environment...))
	index := len(fixture.argv) - 1
	if index < len(fixture.errors) && fixture.errors[index] != nil {
		return nil, fixture.errors[index]
	}
	if index >= len(fixture.responses) {
		return nil, errors.New("unexpected Docker command")
	}
	return fixture.responses[index], nil
}

func TestResolveDockerStorageAuthorityUsesLocalDaemonRoot(t *testing.T) {
	fixture := &fixtureDockerStorageCommand{responses: [][]byte{
		[]byte(`"unix:///var/run/docker.sock"`),
		[]byte(`{"docker_root_dir":"/var/lib/docker","operating_system":"linux","storage_driver":"overlay2"}`),
	}}
	environment := []string{"HOME=/controller-home", "PATH=/controller-bin"}
	authority, err := resolveDockerStorageAuthority(context.Background(), environment, fixture.run)
	if err != nil {
		t.Fatal(err)
	}
	if authority.rootDir != "/var/lib/docker" || authority.storageDriver != "overlay2" {
		t.Fatalf("authority = %#v", authority)
	}
	wantArgv := [][]string{
		{"docker", "context", "inspect", "--format", `{{json .Endpoints.docker.Host}}`},
		{"docker", "info", "--format", dockerStorageInfoTemplate},
	}
	if !reflect.DeepEqual(fixture.argv, wantArgv) || !reflect.DeepEqual(fixture.env, [][]string{environment, environment}) {
		t.Fatalf("commands = %#v env=%#v", fixture.argv, fixture.env)
	}
}

func TestResolveDockerStorageAuthorityFailsClosedWithoutLeakingRawAuthority(t *testing.T) {
	const sentinel = "/private/credential-sentinel"
	tests := []struct {
		name      string
		responses [][]byte
		errors    []error
	}{
		{name: "remote endpoint", responses: [][]byte{[]byte(`"ssh://operator@` + sentinel + `"`)}},
		{name: "invalid endpoint JSON", responses: [][]byte{[]byte(`{"path":"` + sentinel + `"}`)}},
		{name: "endpoint trailing data", responses: [][]byte{[]byte(`"unix:///var/run/docker.sock" {"secret":"` + sentinel + `"}`)}},
		{name: "relative root", responses: [][]byte{[]byte(`"unix:///var/run/docker.sock"`), []byte(`{"docker_root_dir":"` + sentinel[1:] + `","operating_system":"linux","storage_driver":"overlay2"}`)}},
		{name: "non Linux daemon", responses: [][]byte{[]byte(`"unix:///var/run/docker.sock"`), []byte(`{"docker_root_dir":"` + sentinel + `","operating_system":"windows","storage_driver":"windowsfilter"}`)}},
		{name: "unknown field", responses: [][]byte{[]byte(`"unix:///var/run/docker.sock"`), []byte(`{"docker_root_dir":"` + sentinel + `","operating_system":"linux","storage_driver":"overlay2","secret":"` + sentinel + `"}`)}},
		{name: "info trailing data", responses: [][]byte{[]byte(`"unix:///var/run/docker.sock"`), []byte(`{"docker_root_dir":"` + sentinel + `","operating_system":"linux","storage_driver":"overlay2"}{}`)}},
		{name: "command failure", errors: []error{errors.New("raw failure " + sentinel)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := &fixtureDockerStorageCommand{responses: test.responses, errors: test.errors}
			_, err := resolveDockerStorageAuthority(context.Background(), nil, fixture.run)
			if err == nil {
				t.Fatal("resolveDockerStorageAuthority unexpectedly succeeded")
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Fatalf("public error leaked raw authority: %v", err)
			}
		})
	}
}

func TestValidLocalDockerEndpoint(t *testing.T) {
	for _, value := range []string{"unix:///var/run/docker.sock", "unix:///run/user/1000/docker.sock"} {
		if !validLocalDockerEndpoint(value) {
			t.Fatalf("local endpoint %q rejected", value)
		}
	}
	for _, value := range []string{"", "tcp://127.0.0.1:2375", "ssh://host/run/docker.sock", "unix://relative", "unix:///var/run/../run/docker.sock"} {
		if validLocalDockerEndpoint(value) {
			t.Fatalf("noncanonical endpoint %q accepted", value)
		}
	}
}
