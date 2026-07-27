package sandbox

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var environmentFingerprintKey = func() [sha256.Size]byte {
	var key [sha256.Size]byte
	if _, err := rand.Read(key[:]); err != nil {
		panic(err)
	}
	return key
}()

// Config defines what the sandboxed process is allowed to do.
type Config struct {
	// ReadOnlyPaths are absolute paths the process can read but not write.
	ReadOnlyPaths []string

	// ReadWritePaths are absolute paths the process can read and write.
	ReadWritePaths []string

	// AllowedDomains are domain names the process can access via network.
	// Empty = no network. Use ["*"] to allow all.
	AllowedDomains []string

	// WorkDir is the working directory for the sandboxed process.
	WorkDir string

	// Environment is the already-captured environment for the child process.
	// Its zero value captures the default filtered environment. The opaque
	// snapshot prevents callers from accidentally bypassing filtering with a
	// raw []string.
	Environment EnvironmentSnapshot
}

// Backend wraps command execution with OS-level sandboxing.
type Backend interface {
	// Name returns the backend identifier (e.g., "bwrap", "sandbox-exec", "none").
	Name() string

	// Available reports whether this backend can run on the current system.
	Available() bool

	// Command returns a sandboxed exec.Cmd.
	// The returned command has NOT been started — the caller calls cmd.Start().
	Command(ctx context.Context, cfg Config, name string, args ...string) (*exec.Cmd, error)
}

// Capability is an immutable description of the executable authority behind
// a real sandbox backend. ExecutableIdentity is intentionally opaque outside
// this package; callers bind the stable ID into permission receipts instead of
// making policy decisions from individual stat fields.
type Capability struct {
	Backend            string
	ExecutablePath     string
	ExecutableIdentity string
	// protections is deliberately package-private. An exported third-party
	// Backend may publish executable identity for fail-closed execution, but it
	// cannot self-assert a protection property used for permission auto-approval.
	protections CapabilityProtection
}

// CapabilityProtection records isolation properties that are actually
// enforced by a backend. Auto-approval must require the relevant property;
// executable identity alone does not make a broad read-write bind safe.
type CapabilityProtection uint64

const (
	// ProtectionProtectedPaths proves that commands cannot mutate the runtime's
	// protected credential, configuration, and VCS paths through indirect code.
	ProtectionProtectedPaths CapabilityProtection = 1 << iota
)

func (c Capability) Enforces(protection CapabilityProtection) bool {
	return protection != 0 && c.protections&protection == protection
}

// ID returns the stable digest used to bind permission preflight, approval,
// and execution to the same sandbox executable authority.
func (c Capability) ID() string {
	if strings.TrimSpace(c.Backend) == "" || !filepath.IsAbs(c.ExecutablePath) || strings.TrimSpace(c.ExecutableIdentity) == "" {
		return ""
	}
	encoded := fmt.Sprintf("%s\x00%s\x00%s\x00%d", c.Backend, filepath.Clean(c.ExecutablePath), c.ExecutableIdentity, c.protections)
	digest := sha256.Sum256([]byte(encoded))
	return hex.EncodeToString(digest[:])
}

// CapabilityProvider is implemented by backends whose OS isolation boundary
// is rooted in a prepared, immutable executable authority. Merely satisfying
// Backend is not sufficient for permission auto-approval: third-party
// backends must explicitly publish and continuously validate a capability.
type CapabilityProvider interface {
	SandboxCapability() (Capability, bool)
}

// Snapshot returns a currently valid immutable capability for backend. It
// rejects name-only backends and mismatched backend identities.
func Snapshot(backend Backend) (Capability, bool) {
	if backend == nil || backend.Name() == "none" {
		return Capability{}, false
	}
	provider, ok := backend.(CapabilityProvider)
	if !ok {
		return Capability{}, false
	}
	capability, ok := provider.SandboxCapability()
	if !ok || capability.Backend != backend.Name() || capability.ID() == "" {
		return Capability{}, false
	}
	return capability, true
}

// IsRealBackend reports whether backend provides an actual OS isolation
// boundary. NoopBackend remains an unsandboxed execution backend, but it
// must never be treated as sandbox authority for permission auto-approval.
func IsRealBackend(backend Backend) bool {
	_, ok := Snapshot(backend)
	return ok
}

// platformBackends is set by platform-specific init() functions.
var platformBackends []Backend

// Detect returns the best available backend for the current platform.
// Returns a NoopBackend if no sandboxing is available.
func Detect() Backend {
	for _, b := range platformBackends {
		if b.Available() {
			return b
		}
	}
	return NoopBackend{}
}

// NoopBackend passes commands through without sandboxing.
type NoopBackend struct{}

func (NoopBackend) Name() string    { return "none" }
func (NoopBackend) Available() bool { return true }
func (NoopBackend) Command(ctx context.Context, cfg Config, name string, args ...string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	cfg.Environment.Apply(cmd)
	return cmd, nil
}

// validatePaths checks all paths are absolute and contain no control characters.
func validatePaths(paths []string) error {
	for _, p := range paths {
		if p == "" {
			return fmt.Errorf("sandbox: empty path")
		}
		if !filepath.IsAbs(p) {
			return fmt.Errorf("sandbox: relative path not allowed: %q", p)
		}
		for _, r := range p {
			if r < 0x20 || r == 0x7f {
				return fmt.Errorf("sandbox: path contains control character: %q", p)
			}
		}
	}
	return nil
}

// EnvironmentPolicy is trusted host configuration for child-process
// environment delegation. Allowlist names copy an exact variable from the
// host; Overrides supply a value without consulting the host. Overrides are
// themselves an explicit delegation, so their names do not also need to be in
// the allowlist. The fields remain private so policy values cannot be emitted
// by generic event or debug serializers.
type EnvironmentPolicy struct {
	resolve     func([]string) []string
	fingerprint string
}

// NewEnvironmentPolicy constructs an immutable environment delegation. Bad
// names and values containing NUL are ignored rather than being returned in an
// error that might accidentally disclose an override value. Callers must bind
// Fingerprint to their permission authority before executing a command.
func NewEnvironmentPolicy(allowlist []string, overrides map[string]string) EnvironmentPolicy {
	normalizedAllowlist := make([]string, 0, len(allowlist))
	seen := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		if !validEnvironmentName(name) {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		normalizedAllowlist = append(normalizedAllowlist, name)
	}
	sort.Strings(normalizedAllowlist)
	normalizedOverrides := make(map[string]string, len(overrides))
	for name, value := range overrides {
		if !validEnvironmentName(name) || strings.IndexByte(value, 0) >= 0 {
			continue
		}
		normalizedOverrides[name] = value
	}
	fingerprint := environmentPolicyFingerprint(normalizedAllowlist, normalizedOverrides)
	return EnvironmentPolicy{
		resolve: func(parent []string) []string {
			return resolveEnvironmentEntries(parent, normalizedAllowlist, normalizedOverrides)
		},
		fingerprint: fingerprint,
	}
}

// Clone returns an immutable policy copy.
func (p EnvironmentPolicy) Clone() EnvironmentPolicy {
	// The resolver closes over constructor-owned, immutable copies.
	return p
}

// Fingerprint returns a non-reversible binding over names and override values.
// It is safe to put in a permission receipt; raw values are never returned.
func (p EnvironmentPolicy) Fingerprint() string {
	if p.fingerprint != "" {
		return p.fingerprint
	}
	return environmentPolicyFingerprint(nil, nil)
}

func environmentPolicyFingerprint(allowlist []string, overrides map[string]string) string {
	h := hmac.New(sha256.New, environmentFingerprintKey[:])
	for _, name := range allowlist {
		h.Write([]byte{'a', 0})
		h.Write([]byte(name))
		h.Write([]byte{0})
	}
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h.Write([]byte{'o', 0})
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write([]byte(overrides[name]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// String deliberately exposes only the policy fingerprint so generic debug
// formatting cannot print delegated values.
func (p EnvironmentPolicy) String() string {
	return "env-policy:" + p.Fingerprint()
}

// GoString applies the same redaction to %#v debug formatting.
func (p EnvironmentPolicy) GoString() string {
	return p.String()
}

// EnvironmentSnapshot is an opaque, immutable child environment captured at
// a permission boundary. Apply is the only exported access to its values.
type EnvironmentSnapshot struct {
	apply       func(*exec.Cmd)
	fingerprint string
}

// CaptureEnvironment resolves policy against the current process environment.
func CaptureEnvironment(policy EnvironmentPolicy) EnvironmentSnapshot {
	return captureEnvironment(os.Environ(), policy)
}

func captureEnvironment(parent []string, policy EnvironmentPolicy) EnvironmentSnapshot {
	entries := resolveEnvironmentEntries(parent, nil, nil)
	if policy.resolve != nil {
		entries = policy.resolve(parent)
	}
	return newEnvironmentSnapshot(entries)
}

func resolveEnvironmentEntries(parent, allowlist []string, overrides map[string]string) []string {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		allowed[name] = struct{}{}
	}
	values := make(map[string]string)
	for _, entry := range parent {
		name, value, found := strings.Cut(entry, "=")
		if !found || !validEnvironmentName(name) || strings.IndexByte(value, 0) >= 0 {
			continue
		}
		_, explicitlyAllowed := allowed[name]
		if !explicitlyAllowed && !defaultEnvironmentNameAllowed(name) {
			continue
		}
		values[name] = value
	}
	for name, value := range overrides {
		values[name] = value
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]string, 0, len(names))
	for _, name := range names {
		entry := name + "=" + values[name]
		entries = append(entries, entry)
	}
	return entries
}

func newEnvironmentSnapshot(entries []string) EnvironmentSnapshot {
	frozen := append([]string{}, entries...)
	h := hmac.New(sha256.New, environmentFingerprintKey[:])
	for _, entry := range frozen {
		h.Write([]byte(entry))
		h.Write([]byte{0})
	}
	return EnvironmentSnapshot{
		apply: func(cmd *exec.Cmd) {
			cmd.Env = append([]string{}, frozen...)
		},
		fingerprint: hex.EncodeToString(h.Sum(nil)),
	}
}

// Apply assigns the snapshot to cmd. A zero snapshot captures the safe
// default instead of leaving cmd.Env nil, because nil means inherit all host
// credentials to os/exec.
func (s EnvironmentSnapshot) Apply(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if s.apply == nil {
		s = CaptureEnvironment(EnvironmentPolicy{})
	}
	s.apply(cmd)
}

// WithOverrides returns a detached snapshot with trusted executor-owned
// values replacing entries in the captured environment. This is intended for
// private cache and temporary directories selected by the runtime; model input
// must never be forwarded as overrides.
func (s EnvironmentSnapshot) WithOverrides(overrides map[string]string) EnvironmentSnapshot {
	command := &exec.Cmd{}
	s.Apply(command)
	values := make(map[string]string, len(command.Env)+len(overrides))
	for _, entry := range command.Env {
		name, value, ok := strings.Cut(entry, "=")
		if ok && validEnvironmentName(name) {
			values[name] = value
		}
	}
	for name, value := range overrides {
		if validEnvironmentName(name) && strings.IndexByte(value, 0) < 0 {
			values[name] = value
		}
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]string, 0, len(names))
	for _, name := range names {
		entries = append(entries, name+"="+values[name])
	}
	return newEnvironmentSnapshot(entries)
}

// Fingerprint returns a non-reversible binding over the resolved environment.
func (s EnvironmentSnapshot) Fingerprint() string {
	if s.apply == nil {
		s = CaptureEnvironment(EnvironmentPolicy{})
	}
	return s.fingerprint
}

// String deliberately exposes only the snapshot fingerprint so generic debug
// formatting cannot print child environment values.
func (s EnvironmentSnapshot) String() string {
	return "env-snapshot:" + s.Fingerprint()
}

// GoString applies the same redaction to %#v debug formatting.
func (s EnvironmentSnapshot) GoString() string {
	return s.String()
}

// SafeEnv returns the deterministic default tool environment. It is retained
// for callers that only need filtering; process construction should prefer an
// EnvironmentSnapshot so an empty result cannot accidentally mean inheritance.
func SafeEnv(env []string) []string {
	return resolveEnvironmentEntries(env, nil, nil)
}

func validEnvironmentName(name string) bool {
	if name == "" || strings.IndexByte(name, '=') >= 0 || strings.IndexByte(name, 0) >= 0 {
		return false
	}
	for _, r := range name {
		if r <= 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

var defaultEnvironmentNames = map[string]struct{}{
	"PATH": {}, "HOME": {}, "USER": {}, "LOGNAME": {}, "SHELL": {},
	"LANG": {}, "TERM": {}, "COLORTERM": {}, "NO_COLOR": {}, "FORCE_COLOR": {},
	"TMPDIR": {}, "TMP": {}, "TEMP": {}, "TZ": {},
	"SYSTEMROOT": {}, "WINDIR": {}, "COMSPEC": {}, "PATHEXT": {},
	"CC": {}, "CXX": {}, "CPP": {}, "AR": {}, "AS": {}, "LD": {},
	"NM": {}, "OBJCOPY": {}, "OBJDUMP": {}, "RANLIB": {}, "STRIP": {},
	"CFLAGS": {}, "CXXFLAGS": {}, "CPPFLAGS": {}, "LDFLAGS": {},
	"CPATH": {}, "LIBRARY_PATH": {}, "LD_LIBRARY_PATH": {}, "DYLD_LIBRARY_PATH": {},
	"PKG_CONFIG": {}, "PKG_CONFIG_PATH": {}, "PKG_CONFIG_LIBDIR": {},
	"SDKROOT": {}, "MACOSX_DEPLOYMENT_TARGET": {}, "DEVELOPER_DIR": {}, "ARCHFLAGS": {},
	"MAKEFLAGS": {}, "NINJA_STATUS": {}, "BAZELISK_HOME": {}, "USE_BAZEL_VERSION": {}, "TEST_TMPDIR": {},
	"CGO_ENABLED": {},
	"GO111MODULE": {}, "GOAMD64": {}, "GOARCH": {}, "GOARM": {}, "GOARM64": {},
	"GOBIN": {}, "GOCACHE": {}, "GOCACHEPROG": {}, "GOENV": {}, "GOEXE": {},
	"GOEXPERIMENT": {}, "GOFLAGS": {}, "GOHOSTARCH": {}, "GOHOSTOS": {},
	"GOINSECURE": {}, "GOMIPS": {}, "GOMIPS64": {}, "GOMODCACHE": {},
	"GONOPROXY": {}, "GONOSUMDB": {}, "GOOS": {}, "GOPATH": {}, "GOPRIVATE": {},
	"GOPROXY": {}, "GOROOT": {}, "GOSUMDB": {}, "GOTELEMETRY": {},
	"GOTELEMETRYDIR": {}, "GOTMPDIR": {}, "GOTOOLCHAIN": {}, "GOTOOLDIR": {},
	"GOVCS": {}, "GOWORK": {},
	"CARGO_HOME": {}, "CARGO_TARGET_DIR": {}, "CARGO_BUILD_TARGET": {},
	"CARGO_BUILD_JOBS": {}, "CARGO_BUILD_RUSTC_WRAPPER": {},
	"CARGO_ENCODED_RUSTFLAGS": {}, "CARGO_NET_OFFLINE": {},
	"RUSTC": {}, "RUSTDOC": {}, "RUSTFLAGS": {}, "RUSTDOCFLAGS": {},
	"RUSTUP_HOME": {}, "RUSTUP_TOOLCHAIN": {}, "RUST_BACKTRACE": {}, "RUST_LOG": {},
	"NODE_PATH": {}, "NODE_OPTIONS": {}, "NODE_ENV": {},
	"VIRTUAL_ENV": {}, "CONDA_PREFIX": {}, "PYENV_ROOT": {}, "PYTHONHOME": {},
	"PYTHONPATH": {}, "PYTHONUSERBASE": {}, "PYTHONNOUSERSITE": {},
	"PYTHONDONTWRITEBYTECODE": {}, "PYTHONHASHSEED": {},
	"JAVA_HOME": {}, "JDK_HOME": {}, "MAVEN_HOME": {}, "M2_HOME": {},
	"GRADLE_HOME": {}, "GRADLE_USER_HOME": {}, "JAVA_TOOL_OPTIONS": {},
	"JDK_JAVA_OPTIONS": {}, "MAVEN_OPTS": {}, "GRADLE_OPTS": {}, "KOTLIN_HOME": {},
}

var defaultEnvironmentPrefixes = []string{
	"LC_", "XDG_", "CMAKE_", "NPM_CONFIG_", "npm_config_",
}

var secretEnvironmentMarkers = [...]string{
	"KEY", "API_KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL",
	"PRIVATE_KEY", "ACCESS_KEY", "SESSION_KEY", "CLIENT_SECRET", "AUTH", "AUTHORIZATION",
}

func defaultEnvironmentNameAllowed(name string) bool {
	if secretEnvironmentName(name) {
		return false
	}
	if _, exists := defaultEnvironmentNames[strings.ToUpper(name)]; exists {
		return true
	}
	for _, prefix := range defaultEnvironmentPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func secretEnvironmentName(name string) bool {
	upper := strings.ToUpper(name)
	if upper == "AUTHORIZATION" || upper == "PROXY_AUTHORIZATION" ||
		upper == "SSH_AUTH_SOCK" || upper == "GPG_AGENT_INFO" ||
		upper == "GOOGLE_APPLICATION_CREDENTIALS" || upper == "AWS_ACCESS_KEY_ID" ||
		upper == "AWS_SECRET_ACCESS_KEY" || upper == "AWS_SESSION_TOKEN" ||
		upper == "GH_TOKEN" || upper == "GITHUB_TOKEN" || upper == "GITHUB_PAT" {
		return true
	}
	for _, marker := range secretEnvironmentMarkers {
		if upper == marker || strings.HasSuffix(upper, "_"+marker) || strings.Contains(upper, "_"+marker+"_") {
			return true
		}
	}
	return false
}
