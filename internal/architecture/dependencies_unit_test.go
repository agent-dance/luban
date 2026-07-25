package architecture_test

import "testing"

func TestRemovedRootPackagePredicate(t *testing.T) {
	for _, removed := range removedRootPackages {
		t.Run("removed exact "+removed, func(t *testing.T) {
			if !isRemovedRootPackage(removed) {
				t.Fatalf("isRemovedRootPackage(%q) = false, want true", removed)
			}
		})
	}
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "tools exact", path: modulePath + "/tools", want: true},
		{name: "tools child", path: modulePath + "/tools/compat", want: true},
		{name: "coordinator exact", path: modulePath + "/coordinator", want: true},
		{name: "coordinator child", path: modulePath + "/coordinator/metrics", want: true},
		{name: "internal tools remain", path: modulePath + "/internal/tools/tasks", want: false},
		{name: "similar module remains", path: modulePath + "-extra/tools", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isRemovedRootPackage(test.path); got != test.want {
				t.Fatalf("isRemovedRootPackage(%q) = %v, want %v", test.path, got, test.want)
			}
		})
	}
}

func TestAgentAndCollaborationDirectionPredicates(t *testing.T) {
	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "agent cannot import collaboration", got: forbiddenAgentImport(modulePath + "/internal/tools/collaboration"), want: true},
		{name: "agent cannot import compact", got: forbiddenAgentImport(modulePath + "/internal/runtime/compact"), want: true},
		{name: "agent may import file tool", got: forbiddenAgentImport(modulePath + "/internal/tools/file"), want: false},
		{name: "collaboration cannot import agent", got: forbiddenCollaborationImport(modulePath + "/internal/agent"), want: true},
		{name: "collaboration cannot import loop", got: forbiddenCollaborationImport(modulePath + "/internal/runtime/loop"), want: true},
		{name: "collaboration cannot import app", got: forbiddenCollaborationImport(modulePath + "/internal/app"), want: true},
		{name: "collaboration cannot import UI", got: forbiddenCollaborationImport(modulePath + "/internal/ui/tui"), want: true},
		{name: "collaboration may import agent contract", got: forbiddenCollaborationImport(modulePath + "/internal/contracts/agent"), want: false},
		{name: "collaboration may import runtime store", got: forbiddenCollaborationImport(modulePath + "/internal/store/runtime"), want: false},
		{name: "collaboration may import skill authority", got: forbiddenCollaborationImport(modulePath + "/internal/runtime/skillauthority"), want: false},
		{name: "collaboration may import toolbase", got: forbiddenCollaborationImport(modulePath + "/internal/tools/toolbase"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("predicate = %v, want %v", test.got, test.want)
			}
		})
	}
}

func TestTaskStoreAndCoreLeafPredicates(t *testing.T) {
	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "tasks store cannot import tools", got: forbiddenTaskStoreImport(modulePath + "/internal/tools/tasks"), want: true},
		{name: "tasks store cannot import agent", got: forbiddenTaskStoreImport(modulePath + "/internal/agent"), want: true},
		{name: "tasks store cannot import loop", got: forbiddenTaskStoreImport(modulePath + "/internal/runtime/loop"), want: true},
		{name: "tasks store may import paths", got: forbiddenTaskStoreImport(modulePath + "/internal/store/paths"), want: false},
		{name: "tasks store may import secureio", got: forbiddenTaskStoreImport(modulePath + "/internal/store/secureio"), want: false},
		{name: "scope is core leaf", got: isCoreLeafPackage(modulePath + "/internal/runtime/scope"), want: true},
		{name: "skill authority is core leaf", got: isCoreLeafPackage(modulePath + "/internal/runtime/skillauthority"), want: true},
		{name: "loop is not core leaf", got: isCoreLeafPackage(modulePath + "/internal/runtime/loop"), want: false},
		{name: "core leaf cannot import tools", got: forbiddenCoreLeafImport(modulePath + "/internal/tools/tasks"), want: true},
		{name: "core leaf cannot import agent", got: forbiddenCoreLeafImport(modulePath + "/internal/agent"), want: true},
		{name: "core leaf may import contracts", got: forbiddenCoreLeafImport(modulePath + "/internal/contracts/execution"), want: false},
		{name: "core leaf may import skills", got: forbiddenCoreLeafImport(modulePath + "/skills"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("predicate = %v, want %v", test.got, test.want)
			}
		})
	}
}

func TestToolSiblingPredicate(t *testing.T) {
	tests := []struct {
		name     string
		importer string
		imported string
		want     bool
	}{
		{name: "collaboration cannot import tasks", importer: modulePath + "/internal/tools/collaboration", imported: modulePath + "/internal/tools/tasks", want: true},
		{name: "tasks cannot import collaboration", importer: modulePath + "/internal/tools/tasks", imported: modulePath + "/internal/tools/collaboration", want: true},
		{name: "domain may import itself", importer: modulePath + "/internal/tools/tasks/subpackage", imported: modulePath + "/internal/tools/tasks", want: false},
		{name: "domain may import toolbase", importer: modulePath + "/internal/tools/tasks", imported: modulePath + "/internal/tools/toolbase", want: false},
		{name: "toolbase cannot import domain", importer: modulePath + "/internal/tools/toolbase", imported: modulePath + "/internal/tools/tasks", want: true},
		{name: "non-tool import ignored", importer: modulePath + "/internal/app", imported: modulePath + "/internal/tools/tasks", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := forbiddenToolSiblingImport(test.importer, test.imported); got != test.want {
				t.Fatalf("forbiddenToolSiblingImport(%q, %q) = %v, want %v", test.importer, test.imported, got, test.want)
			}
		})
	}
}

func TestCompositionOwnedImportPredicate(t *testing.T) {
	tests := []struct {
		name     string
		importer string
		imported string
		want     bool
	}{
		{name: "app composes collaboration", importer: modulePath + "/internal/app", imported: modulePath + "/internal/tools/collaboration", want: false},
		{name: "app composes tasks", importer: modulePath + "/internal/app", imported: modulePath + "/internal/tools/tasks", want: false},
		{name: "collaboration owns itself", importer: modulePath + "/internal/tools/collaboration", imported: modulePath + "/internal/tools/collaboration/internal", want: false},
		{name: "tasks own themselves", importer: modulePath + "/internal/tools/tasks/testsupport", imported: modulePath + "/internal/tools/tasks", want: false},
		{name: "agent cannot compose collaboration", importer: modulePath + "/internal/agent", imported: modulePath + "/internal/tools/collaboration", want: true},
		{name: "loop cannot compose tasks", importer: modulePath + "/internal/runtime/loop", imported: modulePath + "/internal/tools/tasks", want: true},
		{name: "ordinary tools are not composition owned", importer: modulePath + "/internal/agent", imported: modulePath + "/internal/tools/file", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := forbiddenCompositionOwnedImport(test.importer, test.imported); got != test.want {
				t.Fatalf("forbiddenCompositionOwnedImport(%q, %q) = %v, want %v", test.importer, test.imported, got, test.want)
			}
		})
	}
}
