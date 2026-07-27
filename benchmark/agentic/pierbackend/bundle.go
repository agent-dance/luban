package pierbackend

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

const (
	CodexBundleSchemaVersion  = "agentic-bench/codex-vendor-bundle-v2"
	CodexBundleRelativePath   = "benchmark/agentic/pier/codex-0.145.0-linux-x64.bundle.json"
	CodexBundleManifestSHA256 = "58816453c58d3aae2575506bb39ea33f4303796a6bd217578173a364a7e4b9bc"
	CodexBundleTreeSHA256     = "8cf6573a8606622b108da33f980ba43f1f19762e242bdef59a54dfa396a509e5"
	LubanRuntimeTreeSHA256    = "7e297b3907ba3ab57a4ba598c1fe1e566631342800d187de4ada5272620cebef"
	CodexBinaryRelativePath   = "x86_64-unknown-linux-musl/bin/codex"
	CodexBinarySHA256         = "a2a05dafaa1acb002a45eaec0a462de5b13694fcfcd7bc43305f14781ce7be14"
)

type codexBundleManifest struct {
	SchemaVersion    string                `json:"schema_version"`
	Package          codexBundlePackage    `json:"package"`
	RegistrySnapshot codexRegistrySnapshot `json:"registry_snapshot"`
	BinaryPath       string                `json:"binary_path"`
	TreeSHA256       string                `json:"tree_sha256"`
	Files            []codexBundleFile     `json:"files"`
}

type codexBundlePackage struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Runtime       string `json:"runtime_version"`
	Target        string `json:"target"`
	SourceURL     string `json:"source_url"`
	DistIntegrity string `json:"dist_integrity"`
	TarballSHA256 string `json:"tarball_sha256"`
}

type codexRegistrySnapshot struct {
	FetchedAt                    string `json:"fetched_at"`
	PackageMetadataURL           string `json:"package_metadata_url"`
	VersionMetadataURL           string `json:"version_metadata_url"`
	DistTagsURL                  string `json:"dist_tags_url"`
	LatestVersion                string `json:"latest_version"`
	LinuxX64Version              string `json:"linux_x64_version"`
	PublishedAt                  string `json:"published_at"`
	RegistryModifiedAt           string `json:"registry_modified_at"`
	DistShasum                   string `json:"dist_shasum"`
	TarballSize                  int64  `json:"tarball_size"`
	DistFileCount                int    `json:"dist_file_count"`
	DistUnpackedSize             int64  `json:"dist_unpacked_size"`
	DistSignatureKeyID           string `json:"dist_signature_keyid"`
	DistSignature                string `json:"dist_signature"`
	DistAttestationURL           string `json:"dist_attestation_url"`
	DistAttestationPredicateType string `json:"dist_attestation_predicate_type"`
	NPMAuditSignaturesVersion    string `json:"npm_audit_signatures_version"`
	NPMAuditSignaturesVerified   bool   `json:"npm_audit_signatures_verified"`
	PackageFileCount             int    `json:"package_file_count"`
	PackageTreeSHA256            string `json:"package_tree_sha256"`
}

type codexBundleFile struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type codexBundleBinding struct {
	Root           string
	ManifestPath   string
	ManifestSHA256 string
	TreeSHA256     string
}

var frozenCodexPackage = codexBundlePackage{
	Name:          "@openai/codex",
	Version:       "0.145.0-linux-x64",
	Runtime:       "0.145.0",
	Target:        "x86_64-unknown-linux-musl",
	SourceURL:     "https://registry.npmjs.org/@openai/codex/-/codex-0.145.0-linux-x64.tgz",
	DistIntegrity: "sha512-u8w8LLv3DvsfrDCoswLIemZ0SoNEXyi511WsfFsSiYUazk9qMsB/NtU8N9vhAfN7mZAxLFoMex4v66JjHuZWwA==",
	TarballSHA256: "11239480f8e3efd1430f23bbe91c1a397856b8bbe6185ccbaee2382d25e03df2",
}

var frozenCodexRegistrySnapshot = codexRegistrySnapshot{
	FetchedAt:                    "2026-07-26T09:49:26Z",
	PackageMetadataURL:           "https://registry.npmjs.org/@openai%2fcodex",
	VersionMetadataURL:           "https://registry.npmjs.org/@openai%2fcodex/0.145.0-linux-x64",
	DistTagsURL:                  "https://registry.npmjs.org/-/package/@openai%2fcodex/dist-tags",
	LatestVersion:                "0.145.0",
	LinuxX64Version:              "0.145.0-linux-x64",
	PublishedAt:                  "2026-07-21T18:21:50.929Z",
	RegistryModifiedAt:           "2026-07-25T20:33:52.658Z",
	DistShasum:                   "ff7b16287345f0dc9d087002dfd0aafe280b01a7",
	TarballSize:                  135637111,
	DistFileCount:                8,
	DistUnpackedSize:             363710778,
	DistSignatureKeyID:           "SHA256:DhQ8wR5APBvFHLF/+Tc+AYvPOdTpcIDqOhxsBHRwC7U",
	DistSignature:                "MEYCIQC0FjMiAzCjgGQdi6PX3Cr/H+hs5baEiRdFeqdqNBLhZAIhAKbzR4enAHr2kA0gb8bnEXotrW5oCluk9WfF3v4wQz1U",
	DistAttestationURL:           "https://registry.npmjs.org/-/npm/v1/attestations/@openai%2fcodex@0.145.0-linux-x64",
	DistAttestationPredicateType: "https://slsa.dev/provenance/v1",
	NPMAuditSignaturesVersion:    "11.12.1",
	NPMAuditSignaturesVerified:   true,
	PackageFileCount:             8,
	PackageTreeSHA256:            "68e64b834dee9d80f5df3de9dd5f1217e8cd3c0173323d7153ed882f0b6b3429",
}

func resolveCodexBundleBinding(manifest harness.Manifest, config Config) (codexBundleBinding, error) {
	var codex *harness.AgentSpec
	for index := range manifest.Agents {
		if manifest.Agents[index].ID == "codex" {
			codex = &manifest.Agents[index]
			break
		}
	}
	if codex == nil {
		return codexBundleBinding{}, errors.New("formal comparison lacks the Codex agent")
	}
	manifestPath := filepath.Join(config.PythonModuleRoot, filepath.FromSlash(CodexBundleRelativePath))
	bundle, manifestSHA, err := loadCodexBundleManifest(manifestPath)
	if err != nil {
		return codexBundleBinding{}, err
	}
	if manifestSHA != CodexBundleManifestSHA256 || bundle.TreeSHA256 != CodexBundleTreeSHA256 || bundle.BinaryPath != CodexBinaryRelativePath || bundle.Package != frozenCodexPackage || bundle.RegistrySnapshot != frozenCodexRegistrySnapshot || len(bundle.Files) != 6 {
		return codexBundleBinding{}, errors.New("Codex bundle manifest differs from the frozen 0.145.0 vendor bundle")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Clean(codex.Binary))))
	expectedBinary := filepath.Join(root, filepath.FromSlash(CodexBinaryRelativePath))
	if filepath.Clean(codex.Binary) != expectedBinary || codex.BinarySHA256 != CodexBinarySHA256 {
		return codexBundleBinding{}, errors.New("Codex binary is not pinned at its original vendor path")
	}
	if err := validateCodexBundleTree(root, bundle); err != nil {
		return codexBundleBinding{}, err
	}
	return codexBundleBinding{
		Root: root, ManifestPath: manifestPath, ManifestSHA256: manifestSHA,
		TreeSHA256: bundle.TreeSHA256,
	}, nil
}

func (backend *Backend) codexBundleSnapshot() (codexBundleBinding, error) {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	if !backend.ready || backend.bundle.Root == "" || backend.bundle.TreeSHA256 != CodexBundleTreeSHA256 {
		return codexBundleBinding{}, errors.New("Codex runtime bundle requested before verified preflight")
	}
	return backend.bundle, nil
}

func loadCodexBundleManifest(filename string) (codexBundleManifest, string, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return codexBundleManifest{}, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest codexBundleManifest
	if err := decoder.Decode(&manifest); err != nil {
		return codexBundleManifest{}, "", fmt.Errorf("decode Codex bundle manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return codexBundleManifest{}, "", errors.New("Codex bundle manifest contains trailing JSON")
	}
	if manifest.SchemaVersion != CodexBundleSchemaVersion || manifest.Package != frozenCodexPackage || manifest.RegistrySnapshot != frozenCodexRegistrySnapshot || manifest.BinaryPath != CodexBinaryRelativePath || len(manifest.Files) == 0 {
		return codexBundleManifest{}, "", errors.New("Codex bundle manifest has unsupported identity or schema")
	}
	if !lowerHexSHA256(manifest.TreeSHA256) {
		return codexBundleManifest{}, "", errors.New("Codex bundle manifest has an invalid tree hash")
	}
	previous := ""
	seen := make(map[string]struct{}, len(manifest.Files))
	for _, entry := range manifest.Files {
		if !validBundleRelativePath(entry.Path) || entry.Path <= previous {
			return codexBundleManifest{}, "", errors.New("Codex bundle files are not uniquely sorted by canonical path")
		}
		if _, duplicate := seen[entry.Path]; duplicate {
			return codexBundleManifest{}, "", errors.New("Codex bundle manifest contains a duplicate file")
		}
		mode, err := parseBundleMode(entry.Mode)
		if err != nil || mode&0o111 != 0 && mode != 0o755 || mode&0o111 == 0 && mode != 0o644 || entry.Size < 0 || !lowerHexSHA256(entry.SHA256) {
			return codexBundleManifest{}, "", errors.New("Codex bundle file metadata is invalid")
		}
		seen[entry.Path] = struct{}{}
		previous = entry.Path
	}
	if _, present := seen[manifest.BinaryPath]; !present {
		return codexBundleManifest{}, "", errors.New("Codex bundle binary is absent from its file manifest")
	}
	sum := sha256.Sum256(raw)
	return manifest, hex.EncodeToString(sum[:]), nil
}

func validateCodexBundleTree(root string, manifest codexBundleManifest) error {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("Codex bundle root must be a real directory")
	}
	expectedFiles := make(map[string]codexBundleFile, len(manifest.Files))
	expectedDirs := map[string]struct{}{}
	for _, entry := range manifest.Files {
		expectedFiles[entry.Path] = entry
		for directory := path.Dir(entry.Path); directory != "."; directory = path.Dir(directory) {
			expectedDirs[directory] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(expectedFiles))
	err = filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Codex bundle contains a symlink: %s", relative)
		}
		if entry.IsDir() {
			if _, expected := expectedDirs[relative]; !expected {
				return fmt.Errorf("Codex bundle contains an unexpected directory: %s", relative)
			}
			return nil
		}
		expected, present := expectedFiles[relative]
		if !present {
			return fmt.Errorf("Codex bundle contains an unexpected file: %s", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || fmt.Sprintf("%04o", info.Mode().Perm()) != expected.Mode || info.Size() != expected.Size {
			return fmt.Errorf("Codex bundle file metadata differs from its manifest: %s", relative)
		}
		digest, err := harness.HashFile(filename)
		if err != nil {
			return err
		}
		if digest != expected.SHA256 {
			return fmt.Errorf("Codex bundle file hash differs from its manifest: %s", relative)
		}
		seen[relative] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(expectedFiles) {
		return errors.New("Codex bundle is incomplete")
	}
	actual := canonicalBundleTreeSHA256(manifest.Files)
	if actual != manifest.TreeSHA256 {
		return errors.New("Codex bundle canonical tree hash differs from its manifest")
	}
	return nil
}

func canonicalBundleTreeSHA256(files []codexBundleFile) string {
	ordered := append([]codexBundleFile(nil), files...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].Path < ordered[right].Path })
	digest := sha256.New()
	for _, entry := range ordered {
		_, _ = io.WriteString(digest, entry.Path+"\x00"+entry.Mode+"\x00"+strconv.FormatInt(entry.Size, 10)+"\x00"+entry.SHA256+"\n")
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func validBundleRelativePath(value string) bool {
	return value != "" && !strings.Contains(value, "\\") && !strings.HasPrefix(value, "/") && path.Clean(value) == value && fs.ValidPath(value)
}

func parseBundleMode(value string) (fs.FileMode, error) {
	if len(value) != 4 || value[0] != '0' {
		return 0, errors.New("bundle mode must be four-digit octal")
	}
	parsed, err := strconv.ParseUint(value, 8, 9)
	return fs.FileMode(parsed), err
}

func lowerHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}
