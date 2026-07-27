package pierbackend

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// FileConfig is the non-secret, portable configuration accepted by the
// benchmark CLI. Provider credentials remain in ProviderCredentialEnv and are
// never serialized here.
type FileConfig struct {
	PierBinary                 string `json:"pier_binary"`
	DatasetRepositoryRoot      string `json:"dataset_repository_root"`
	EvaluatorRepositoryRoot    string `json:"evaluator_repository_root"`
	EvaluatorManifestPath      string `json:"evaluator_manifest_path"`
	InventoryLockPath          string `json:"inventory_lock_path"`
	PythonModuleRoot           string `json:"python_module_root"`
	PrivateWorkRoot            string `json:"private_work_root"`
	RegistryGatePath           string `json:"registry_gate_path,omitempty"`
	CodexV8CanaryReceiptPath   string `json:"codex_v8_canary_receipt_path"`
	CodexV8CanaryReceiptSHA256 string `json:"codex_v8_canary_receipt_sha256"`
	LubanV8CanaryReceiptPath   string `json:"luban_v8_canary_receipt_path"`
	LubanV8CanaryReceiptSHA256 string `json:"luban_v8_canary_receipt_sha256"`
	EgressProxyImage           string `json:"egress_proxy_image"`
	ProxyListenAddress         string `json:"proxy_listen_address"`
	ProxyAdvertiseHost         string `json:"proxy_advertise_host"`
	ProviderUpstream           string `json:"provider_upstream"`
	ProviderCredentialEnv      string `json:"provider_credential_env"`
}

func LoadConfigFile(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var source FileConfig
	if err := decoder.Decode(&source); err != nil {
		return Config{}, fmt.Errorf("decode Pier backend config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("Pier backend config contains trailing JSON")
	}
	return Config{
		PierBinary: source.PierBinary, DatasetRepositoryRoot: source.DatasetRepositoryRoot,
		EvaluatorRepositoryRoot: source.EvaluatorRepositoryRoot, EvaluatorManifestPath: source.EvaluatorManifestPath,
		InventoryLockPath: source.InventoryLockPath, PythonModuleRoot: source.PythonModuleRoot,
		PrivateWorkRoot: source.PrivateWorkRoot, RegistryGatePath: source.RegistryGatePath,
		CodexV8CanaryReceiptPath: source.CodexV8CanaryReceiptPath, CodexV8CanaryReceiptSHA256: source.CodexV8CanaryReceiptSHA256,
		LubanV8CanaryReceiptPath: source.LubanV8CanaryReceiptPath, LubanV8CanaryReceiptSHA256: source.LubanV8CanaryReceiptSHA256,
		EgressProxyImage:   source.EgressProxyImage,
		ProxyListenAddress: source.ProxyListenAddress,
		ProxyAdvertiseHost: source.ProxyAdvertiseHost, ProviderUpstream: source.ProviderUpstream,
		ProviderCredentialEnv: source.ProviderCredentialEnv,
	}, nil
}
