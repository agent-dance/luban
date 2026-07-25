package architecture_test

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const modulePath = "github.com/agent-dance/luban"

type packageInfo struct {
	ImportPath   string
	Imports      []string
	TestImports  []string
	XTestImports []string
}

var removedRootPackages = []string{
	modulePath + "/compact",
	modulePath + "/config",
	modulePath + "/coordinator",
	modulePath + "/engine",
	modulePath + "/goal",
	modulePath + "/input",
	modulePath + "/loop",
	modulePath + "/mcp",
	modulePath + "/render",
	modulePath + "/services/mcp",
	modulePath + "/session",
	modulePath + "/terminaltheme",
	modulePath + "/tmux",
	modulePath + "/tools",
	modulePath + "/tui",
	modulePath + "/ui",
}

func TestDependencyDirection(t *testing.T) {
	packages := listPackages(t)
	var violations []string
	for _, pkg := range packages {
		if isRemovedRootPackage(pkg.ImportPath) {
			violations = append(violations, pkg.ImportPath+" restores a removed root package")
		}
		imports := append([]string(nil), pkg.Imports...)
		imports = append(imports, pkg.TestImports...)
		imports = append(imports, pkg.XTestImports...)
		for _, imported := range imports {
			if !strings.HasPrefix(imported, modulePath) {
				continue
			}
			switch {
			case isRemovedRootPackage(imported):
				violations = append(violations, pkg.ImportPath+" imports removed root package "+imported)
			case pkg.ImportPath == modulePath+"/cmd/luban-code" && imported != modulePath+"/internal/app":
				violations = append(violations, pkg.ImportPath+" must be a thin entry point; imports "+imported)
			case pkg.ImportPath != modulePath+"/cmd/luban-code" && isPackageOrChild(imported, modulePath+"/internal/app"):
				violations = append(violations, pkg.ImportPath+" imports application composition "+imported)
			case isPackageOrChild(imported, modulePath+"/internal/runtime/engine") &&
				!isPackageOrChild(pkg.ImportPath, modulePath+"/internal/runtime/engine") &&
				!isPackageOrChild(pkg.ImportPath, modulePath+"/internal/app"):
				violations = append(violations, pkg.ImportPath+" bypasses application composition through "+imported)
			case isPackageOrChild(pkg.ImportPath, modulePath+"/internal/agent") && forbiddenAgentImport(imported):
				violations = append(violations, pkg.ImportPath+" reverses the agent dependency through "+imported)
			case isPackageOrChild(pkg.ImportPath, modulePath+"/internal/tools/collaboration") && forbiddenCollaborationImport(imported):
				violations = append(violations, pkg.ImportPath+" makes collaboration depend on a concrete runtime/composition layer "+imported)
			case isCoreLeafPackage(pkg.ImportPath) && forbiddenCoreLeafImport(imported):
				violations = append(violations, pkg.ImportPath+" makes a runtime core leaf depend on a higher layer "+imported)
			case isPackageOrChild(pkg.ImportPath, modulePath+"/internal/store/tasks") && forbiddenTaskStoreImport(imported):
				violations = append(violations, pkg.ImportPath+" makes task persistence depend on a runtime/tool/UI layer "+imported)
			case forbiddenToolSiblingImport(pkg.ImportPath, imported):
				violations = append(violations, pkg.ImportPath+" imports sibling tool implementation "+imported)
			case forbiddenCompositionOwnedImport(pkg.ImportPath, imported):
				violations = append(violations, pkg.ImportPath+" imports app-composed tool implementation "+imported)
			case isPackageOrChild(pkg.ImportPath, modulePath+"/sdk") && forbiddenSDKImport(imported):
				violations = append(violations, pkg.ImportPath+" imports an internal runtime contract "+imported)
			case strings.HasPrefix(pkg.ImportPath, modulePath+"/internal/presentation") && forbiddenPresentationImport(imported):
				violations = append(violations, pkg.ImportPath+" reverses presentation dependency through "+imported)
			case isPackageOrChild(pkg.ImportPath, modulePath+"/internal/tools/shell") && forbiddenShellImport(imported):
				violations = append(violations, pkg.ImportPath+" makes the shell domain depend on composition/runtime "+imported)
			case strings.HasPrefix(pkg.ImportPath, modulePath+"/internal/tools/") && forbiddenToolImport(imported):
				violations = append(violations, pkg.ImportPath+" reverses tool dependency through "+imported)
			case isPackageOrChild(pkg.ImportPath, modulePath+"/internal/ui") && forbiddenUIImport(imported):
				violations = append(violations, pkg.ImportPath+" reverses UI dependency through "+imported)
			case isPackageOrChild(pkg.ImportPath, modulePath+"/internal/contracts") && forbiddenContractImport(imported):
				violations = append(violations, pkg.ImportPath+" makes contracts depend on implementation "+imported)
			case isPackageOrChild(pkg.ImportPath, modulePath+"/internal/store") && forbiddenStoreImport(imported):
				violations = append(violations, pkg.ImportPath+" makes persistence depend on runtime/UI "+imported)
			}
		}
	}
	if len(violations) != 0 {
		sort.Strings(violations)
		t.Fatalf("invalid package dependency direction:\n%s", strings.Join(violations, "\n"))
	}
}

func forbiddenShellImport(imported string) bool {
	return forbiddenToolImport(imported) ||
		isPackageOrChild(imported, modulePath+"/internal/runtime")
}

func forbiddenAgentImport(imported string) bool {
	return isPackageOrChild(imported, modulePath+"/internal/runtime/compact") ||
		isPackageOrChild(imported, modulePath+"/internal/tools/collaboration")
}

func forbiddenCollaborationImport(imported string) bool {
	for _, target := range []string{
		modulePath + "/internal/agent",
		modulePath + "/internal/app",
		modulePath + "/internal/runtime/loop",
		modulePath + "/internal/ui",
		modulePath + "/tui",
		modulePath + "/ui",
	} {
		if isPackageOrChild(imported, target) {
			return true
		}
	}
	return false
}

func isCoreLeafPackage(path string) bool {
	return isPackageOrChild(path, modulePath+"/internal/runtime/scope") ||
		isPackageOrChild(path, modulePath+"/internal/runtime/skillauthority")
}

func forbiddenCoreLeafImport(imported string) bool {
	for _, target := range []string{
		modulePath + "/internal/agent",
		modulePath + "/internal/app",
		modulePath + "/internal/runtime/engine",
		modulePath + "/internal/runtime/loop",
		modulePath + "/internal/tools",
		modulePath + "/internal/ui",
		modulePath + "/tui",
		modulePath + "/ui",
	} {
		if isPackageOrChild(imported, target) {
			return true
		}
	}
	return false
}

func forbiddenTaskStoreImport(imported string) bool {
	for _, target := range []string{
		modulePath + "/internal/agent",
		modulePath + "/internal/app",
		modulePath + "/internal/runtime/loop",
		modulePath + "/internal/tools",
		modulePath + "/internal/ui",
		modulePath + "/tui",
		modulePath + "/ui",
	} {
		if isPackageOrChild(imported, target) {
			return true
		}
	}
	return false
}

func forbiddenToolSiblingImport(importer, imported string) bool {
	importerDomain, importerIsTool := internalToolDomain(importer)
	importedDomain, importedIsTool := internalToolDomain(imported)
	if !importerIsTool || !importedIsTool || importerDomain == importedDomain {
		return false
	}
	return importedDomain != "toolbase"
}

func internalToolDomain(path string) (string, bool) {
	prefix := modulePath + "/internal/tools/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	domain, _, _ := strings.Cut(strings.TrimPrefix(path, prefix), "/")
	if domain == "" {
		return "", false
	}
	return domain, true
}

func forbiddenCompositionOwnedImport(importer, imported string) bool {
	for _, owner := range []string{
		modulePath + "/internal/tools/collaboration",
		modulePath + "/internal/tools/tasks",
	} {
		if !isPackageOrChild(imported, owner) {
			continue
		}
		return !isPackageOrChild(importer, owner) &&
			!isPackageOrChild(importer, modulePath+"/internal/app")
	}
	return false
}

func isRemovedRootPackage(path string) bool {
	for _, removed := range removedRootPackages {
		if isPackageOrChild(path, removed) {
			return true
		}
	}
	return false
}

func forbiddenSDKImport(imported string) bool {
	return isPackageOrChild(imported, modulePath+"/internal/contracts") ||
		isPackageOrChild(imported, modulePath+"/internal/runtime/engine") ||
		isPackageOrChild(imported, modulePath+"/internal/runtime/loop")
}

func forbiddenUIImport(imported string) bool {
	return isPackageOrChild(imported, modulePath+"/internal/runtime/loop")
}

func forbiddenContractImport(imported string) bool {
	if !strings.HasPrefix(imported, modulePath+"/") {
		return false
	}
	return !isPackageOrChild(imported, modulePath+"/types")
}

func forbiddenStoreImport(imported string) bool {
	for _, target := range []string{
		modulePath + "/internal/agent",
		modulePath + "/internal/app",
		modulePath + "/internal/ui",
		modulePath + "/internal/tools",
		modulePath + "/internal/runtime/engine",
		modulePath + "/internal/runtime/loop",
		modulePath + "/sdk",
		modulePath + "/tui",
		modulePath + "/ui",
	} {
		if isPackageOrChild(imported, target) {
			return true
		}
	}
	return false
}

func forbiddenPresentationImport(imported string) bool {
	for _, prefix := range []string{
		modulePath + "/internal/app",
		modulePath + "/internal/ui/",
		modulePath + "/internal/tools/",
		modulePath + "/commands",
		modulePath + "/internal/runtime/engine",
		modulePath + "/internal/runtime/loop",
		modulePath + "/tui",
		modulePath + "/ui",
	} {
		if imported == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(imported, prefix) {
			return true
		}
	}
	return false
}

func isPackageOrChild(imported, target string) bool {
	return imported == target || strings.HasPrefix(imported, target+"/")
}

func forbiddenToolImport(imported string) bool {
	for _, target := range []string{
		modulePath + "/internal/app",
		modulePath + "/internal/ui",
		modulePath + "/sdk",
		modulePath + "/tui",
		modulePath + "/ui",
	} {
		if isPackageOrChild(imported, target) {
			return true
		}
	}
	return false
}

func listPackages(t *testing.T) []packageInfo {
	t.Helper()
	goMod, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(strings.TrimSpace(string(goMod)))
	command := exec.Command("go", "list", "-json", "./...")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []packageInfo
	for decoder.More() {
		var pkg packageInfo
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatal(err)
		}
		packages = append(packages, pkg)
	}
	return packages
}
