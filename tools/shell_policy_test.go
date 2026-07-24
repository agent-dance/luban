package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type forgedPermissionContext struct{ context.Context }

func (forgedPermissionContext) Value(any) any { return true }

func deterministicShellPolicyContext() types.PolicyContext {
	return types.PolicyContext{
		CWD:         "/workspace/project",
		HomeDir:     "/Users/tester",
		AllowedDirs: []string{"/workspace/project"},
		KnownEnvironment: map[string]string{
			"HOME": "/Users/tester",
		},
		TrustedTempRoots: []string{"/tmp"},
	}
}

func TestAnalyzeShellCommandPolicyMatrix(t *testing.T) {
	context := deterministicShellPolicyContext()
	tests := []struct {
		name    string
		command string
		want    types.PolicyDisposition
	}{
		{"root short flags", `rm -rf /`, types.PolicyBlock},
		{"root end flags", `rm -rf -- /`, types.PolicyBlock},
		{"root long flags", `rm --recursive --force "/"`, types.PolicyBlock},
		{"root split flags", `rm -r -f /`, types.PolicyBlock},
		{"home expansion", `rm -rf "$HOME"`, types.PolicyBlock},
		{"home tilde", `rm -rf ~`, types.PolicyBlock},
		{"system etc", `rm -fr /etc`, types.PolicyBlock},
		{"system recursive without force", `rm -r /etc`, types.PolicyBlock},
		{"system single file", `rm -f /etc/passwd`, types.PolicyBlock},
		{"system var", `rm -rf /var/lib/app`, types.PolicyBlock},
		{"protected relative", `rm -rf .git`, types.PolicyBlock},
		{"fallback root", `rm -rf "${UNSET:-/}"`, types.PolicyBlock},
		{"fallback system", `rm -rf "${UNSET:-/etc}"`, types.PolicyBlock},
		{"root glob", `rm -rf /*`, types.PolicyBlock},
		{"raw dd", `dd if=/dev/zero of=/dev/nvme0n1`, types.PolicyBlock},
		{"raw redirect", `printf x >/dev/vda`, types.PolicyBlock},
		{"workspace cleanup", `rm -rf ./build/cache`, types.PolicyRequiredAsk},
		{"outside cleanup", `rm -rf /tmp/unproven`, types.PolicyRequiredAsk},
		{"dynamic target", `rm -rf "$TARGET"`, types.PolicyRequiredAsk},
		{"command substitution", `rm -rf "$(resolve_target)"`, types.PolicyRequiredAsk},
		{"dynamic eval", `eval "$PAYLOAD"`, types.PolicyRequiredAsk},
		{"known dynamic command root", `CMD='rm -rf /'; $CMD`, types.PolicyBlock},
		{"known eval root", `PAYLOAD='rm -rf /'; eval "$PAYLOAD"`, types.PolicyBlock},
		{"builtin eval root", `builtin eval 'rm -rf /'`, types.PolicyBlock},
		{"xargs root", `printf x | xargs rm -rf /`, types.PolicyBlock},
		{"find exec root", `find . -exec rm -rf / \;`, types.PolicyBlock},
		{"unknown shell script", `sh cleanup.sh`, types.PolicyRequiredAsk},
		{"dynamic flags", `rm "$FLAGS" ./cache`, types.PolicyRequiredAsk},
		{"dynamic flags cannot hide root", `rm "$FLAGS" /`, types.PolicyBlock},
		{"known unquoted flags cannot hide root", `FLAGS='-rf /'; rm $FLAGS`, types.PolicyBlock},
		{"parse failure", `rm -rf "`, types.PolicyRequiredAsk},
		{"single quoted variable is literal", `rm -rf '$TARGET'`, types.PolicyRequiredAsk},
		{"single quoted glob is literal", `rm -rf '/*'`, types.PolicyRequiredAsk},
		{"end flags makes dash operand literal", `rm -- -rf`, types.PolicyAllow},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := AnalyzeShellCommand(test.command, context)
			if got.Disposition != test.want {
				t.Fatalf("AnalyzeShellCommand(%q)=%s (%s), want %s", test.command, got.Disposition, got.Code, test.want)
			}
			if got.Disposition == types.PolicyRequiredAsk && got.Remediation == nil {
				t.Fatalf("required ask %q has no structured remediation", test.command)
			}
		})
	}
}

func TestAnalyzeShellCommandBlocksProtectedPathGlobs(t *testing.T) {
	context := deterministicShellPolicyContext()
	blocked := []struct {
		name    string
		command string
	}{
		{name: "output redirect", command: `printf x > .g[it]/config`},
		{name: "append redirect", command: `printf x >> .en[v]`},
		{name: "clobber redirect", command: `printf x >| .g[it]/config`},
		{name: "input redirect", command: `cat < .g[it]/config`},
		{name: "read operand", command: `cat .g[it]/config`},
		{name: "read basename operand", command: `head .en[v]`},
		{name: "write operand", command: `touch .g[it]/config`},
		{name: "copy destination", command: `cp source .g[it]/config`},
		{name: "dd output operand", command: `dd if=input of=.g[it]/config`},
		{name: "nested protected directory", command: `cat work/.s[sh]/id_rsa`},
		{name: "protected multi component suffix", command: `cat work/.k[ube]/config`},
		{name: "protected basename", command: `cat work/.ba[sh]rc`},
		{name: "canonical character class", command: `cat .g[i]t/config`},
		{name: "known unquoted variable", command: `target='.g[it]/config'; cat $target`},
		{name: "known unquoted redirect variable", command: `target='.g[it]/config'; printf x > $target`},
		{name: "fallback redirect variable", command: `printf x > ${target:-.g[it]/config}`},
		{name: "fallback dd output", command: `dd if=input of=${target:-.g[it]/config}`},
	}
	for _, test := range blocked {
		t.Run(test.name, func(t *testing.T) {
			decision := AnalyzeShellCommand(test.command, context)
			if decision.Disposition != types.PolicyBlock || decision.Code != "shell.policy.block.protected" {
				t.Fatalf("AnalyzeShellCommand(%q)=%s (%s), want protected Block", test.command, decision.Disposition, decision.Code)
			}
		})
	}
}

func TestAnalyzeShellCommandSafeGlobsAreNotHardBlocked(t *testing.T) {
	context := deterministicShellPolicyContext()
	commands := []string{
		`cat src/*.go`,
		`grep needle testdata/file-?.txt`,
		`touch build/*.tmp`,
		`cp src/*.go build/`,
		`printf x > build/out-[0-9].txt`,
		`printf x >| build/out-[0-9].txt`,
		`cat < input/file-?.txt`,
		`cat < .git/config`,
		`cat work/.g[ab]/config`,
		`cat .github/*.md`,
		`cat .gitignore*`,
		`printf x > build/.env-[0-9]`,
		`cat '.g[it]/config'`,
		`target='.g[it]/config'; cat "$target"`,
		`target='build/out-[0-9].txt'; printf x > "$target"`,
		`printf x > ${target:-build/out-[0-9].txt}`,
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			decision := AnalyzeShellCommand(command, context)
			if decision.Disposition == types.PolicyBlock {
				t.Fatalf("AnalyzeShellCommand(%q)=%s (%s), safe glob must not hard Block", command, decision.Disposition, decision.Code)
			}
		})
	}
}

func TestBashWritesToProtectedPathBlocksUnquotedGlob(t *testing.T) {
	hit, target := BashWritesToProtectedPath(`printf x > .g[it]/config`)
	if !hit || target != `.g[it]/config` {
		t.Fatalf("BashWritesToProtectedPath()=(%v, %q), want (true, %q)", hit, target, `.g[it]/config`)
	}

	if hit, target := BashWritesToProtectedPath(`printf x > 'build/out-[0-9].txt'`); hit {
		t.Fatalf("quoted safe glob unexpectedly matched protected path %q", target)
	}
}

func TestAnalyzeShellCommandHomeProjectIsNotHardBlocked(t *testing.T) {
	context := types.PolicyContext{
		CWD: "/Users/tester/project", HomeDir: "/Users/tester",
		AllowedDirs: []string{"/Users/tester/project"}, KnownEnvironment: map[string]string{"HOME": "/Users/tester"},
	}
	decision := AnalyzeShellCommand(`rm -rf ./build`, context)
	if decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("home project cleanup=%s (%s), want required ask", decision.Disposition, decision.Code)
	}
}

func TestAnalyzeShellCommandTrustedTempProvenanceIsNotBlocked(t *testing.T) {
	context := deterministicShellPolicyContext()
	tests := []string{
		`tmp=$(mktemp -d); rm -rf -- "$tmp"`,
		`tmp="$(mktemp -d)"; rm --recursive --force "${tmp}"`,
	}
	for _, command := range tests {
		decision := AnalyzeShellCommand(command, context)
		if decision.Disposition != types.PolicyAllow {
			t.Fatalf("trusted temporary cleanup = %s: %q (%s)", decision.Disposition, command, decision.Code)
		}
	}

	if decision := AnalyzeShellCommand(`tmp=$(mktemp -d); tmp=/; rm -rf "$tmp"`, context); decision.Disposition != types.PolicyBlock {
		t.Fatalf("provenance overwrite to root = %s, want block", decision.Disposition)
	}
	if decision := AnalyzeShellCommand(`tmp=$(mktemp -d); rm -rf "$tmp" "$TARGET"`, context); decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("unknown second operand = %s, want required ask", decision.Disposition)
	}
	if decision := AnalyzeShellCommand(`tmp=$(mktemp -d); rm -rf "$tmp" /`, context); decision.Disposition != types.PolicyBlock {
		t.Fatalf("root second operand = %s, want block", decision.Disposition)
	}
	if decision := AnalyzeShellCommand(`tmp=$(mktemp -d /etc/probe.XXXXXX); rm -rf "$tmp"`, context); decision.Disposition != types.PolicyBlock {
		t.Fatalf("system-rooted mktemp provenance = %s, want block", decision.Disposition)
	}
	if decision := AnalyzeShellCommand(`TMPDIR=/etc; tmp=$(mktemp -d); rm -rf "$tmp"`, context); decision.Disposition != types.PolicyBlock {
		t.Fatalf("system TMPDIR provenance = %s, want block", decision.Disposition)
	}
	if decision := AnalyzeShellCommand(`tmp=$(mktemp -d); rm -rf $tmp`, context); decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("unquoted trusted variable = %s, want required ask", decision.Disposition)
	}
	if decision := AnalyzeShellCommand(`if false; then tmp=$(mktemp -d); fi; rm -rf "$tmp"`, context); decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("conditional provenance = %s, want required ask", decision.Disposition)
	}
	if decision := AnalyzeShellCommand(`tmp=$(mktemp -d); victim="$tmp/../.."; rm -rf "$victim"`, context); decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("derived provenance escape = %s, want required ask", decision.Disposition)
	}
	if decision := AnalyzeShellCommand(`root=/etc; tmp=$(mktemp -d -p "$root"); rm -rf "$tmp"`, context); decision.Disposition != types.PolicyBlock {
		t.Fatalf("dynamic known mktemp root = %s, want block", decision.Disposition)
	}
	if decision := AnalyzeShellCommand(`tmp=$(mktemp -d -p "$TARGET"); rm -rf "$tmp"`, context); decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("dynamic mktemp root = %s, want required ask", decision.Disposition)
	}
	if decision := AnalyzeShellCommand(`tmp=$(/tmp/fake/mktemp -d); rm -rf "$tmp"`, context); decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("spoofed mktemp producer = %s, want required ask", decision.Disposition)
	}
	if decision := AnalyzeShellCommand(`tmp=/; false && tmp=$(mktemp -d); rm -rf "$tmp"`, context); decision.Disposition != types.PolicyBlock {
		t.Fatalf("control-flow worst join = %s, want block", decision.Disposition)
	}
	for _, command := range []string{
		`tmp=$(mktemp -d); export tmp=/; rm -rf "$tmp"`,
		`tmp=$(mktemp -d); readonly tmp=/; rm -rf "$tmp"`,
		`tmp=$(mktemp -d); declare tmp=/; rm -rf "$tmp"`,
		`tmp=$(mktemp -d); typeset tmp=/; rm -rf "$tmp"`,
		`tmp=$(mktemp -d); builtin export tmp=/; rm -rf "$tmp"`,
		`tmp=$(mktemp -d); command readonly tmp=/; rm -rf "$tmp"`,
		`tmp=$(mktemp -d); builtin eval 'tmp=/'; rm -rf "$tmp"`,
		`tmp=$(mktemp -d); printf -v tmp /; rm -rf "$tmp"`,
		`tmp=$(mktemp -d); printf -v tmp %s /; rm -rf "$tmp"`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyBlock {
			t.Errorf("assignment builtin lost root overwrite for %q: %#v", command, decision)
		}
	}
	for _, command := range []string{
		`tmp=$(mktemp -d); eval 'tmp=/'; rm -rf "$tmp"`,
		`tmp=$(mktemp -d); read tmp <<< /; rm -rf "$tmp"`,
		`tmp=$(mktemp -d); env tmp=/ sh -c 'rm -rf "$tmp"'`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition == types.PolicyAllow {
			t.Errorf("variable mutator retained trusted provenance for %q: %#v", command, decision)
		}
	}
	if decision := AnalyzeShellCommand(`hash -p /tmp/fake-mktemp mktemp; tmp=$(mktemp -d); rm -rf "$tmp"`, context); decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("hash producer mutation = %s, want required ask", decision.Disposition)
	}
}

func TestAnalyzeShellCommandSingleQuotedVariableIsNotDynamic(t *testing.T) {
	decision := AnalyzeShellCommand(`rm -rf '$TARGET'`, deterministicShellPolicyContext())
	if decision.Code != "shell.policy.ask.destructive" {
		t.Fatalf("single-quoted literal misclassified: %#v", decision)
	}
	dynamic := AnalyzeShellCommand(`rm -rf "$TARGET"`, deterministicShellPolicyContext())
	if dynamic.Code != "shell.policy.ask.dynamic_flags" && dynamic.Code != "shell.policy.ask.dynamic_target" {
		t.Fatalf("double-quoted variable did not require dynamic approval: %#v", dynamic)
	}
}

func TestAnalyzeShellCommandWrappersAndFlagsHaveOneVerdict(t *testing.T) {
	context := deterministicShellPolicyContext()
	commands := []string{
		`/bin/rm -rf /`,
		`command rm -rf /`,
		`builtin 'r''m' -rf /`,
		`builtin command 'r''m' -rf /`,
		`builtin exec 'r''m' -rf /`,
		`builtin eval 'rm -rf /'`,
		`env rm -rf /`,
		`env -i rm -rf /`,
		`env -u HOME rm -rf /`,
		`env --unset HOME rm -rf /`,
		`env -S 'rm -rf /'`,
		`env --split-string='rm -rf /'`,
		`sudo -n -- rm -rf /`,
		`sudo --user root rm -rf /`,
		`sudo TARGET=/ rm -rf /`,
		`nice -n 5 rm -rf /`,
		`nice -5 rm -rf /`,
		`timeout 5s rm -rf /`,
		`timeout -k 1s 5s rm -rf /`,
		`timeout --signal KILL 5s rm -rf /`,
		`nohup rm -rf /`,
		`stdbuf -oL -eL rm -rf /`,
		`doas -n -- rm -rf /`,
		`doas -u root rm -rf /`,
		`doas -a passwd rm -r /etc`,
		`ionice -c 3 rm -rf /`,
		`ionice --class=idle rm -rf /`,
		`unbuffer -p rm -rf /`,
		`taskset -c 0 rm -rf /`,
		`taskset 0x1 rm -rf /`,
		`chrt -f 99 rm -rf /`,
		`chrt --rr 99 rm -rf /`,
		`doas -n ionice -c3 taskset 0x1 chrt -r 99 rm -rf /`,
		`sh -c 'rm -rf /'`,
		`sh -xec 'rm -rf /'`,
		`ash -c 'rm -rf /'`,
		`busybox sh -c 'rm -rf /'`,
		`busybox rm -rf /`,
		`toybox rm -rf /`,
		`command tee .git/config`,
		`sudo cp source .git/config`,
		`doas tee /etc/passwd`,
		`taskset 0x1 dd if=/dev/zero of=/dev/disk0`,
		`printf x >/etc/passwd`,
	}
	for _, command := range commands {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyBlock {
			t.Errorf("wrapped command %q = %s (%s), want block", command, decision.Disposition, decision.Code)
		}
	}
}

func TestAnalyzeShellCommandAmbiguousWrapperFlagsKeepHardBlock(t *testing.T) {
	context := deterministicShellPolicyContext()
	for _, command := range []string{
		`sudo "$OPT" rm -rf /`,
		`doas "$OPT" rm -rf /`,
		`timeout "$OPT" 5s rm -rf /`,
		`nice "$OPT" rm -rf /`,
		`stdbuf "$OPT" rm -rf /`,
		`ionice "$OPT" rm -rf /`,
		`unbuffer "$OPT" rm -rf /`,
		`taskset "$OPT" 0x1 rm -rf /`,
		`chrt "$OPT" 99 rm -rf /`,
	} {
		decision := AnalyzeShellCommand(command, context)
		if decision.Disposition != types.PolicyBlock {
			t.Errorf("ambiguous wrapper downgraded hard block for %q: %#v", command, decision)
		}
	}
}

func TestAnalyzeShellCommandExactTerminatorAndAttachedFlagRegressions(t *testing.T) {
	context := deterministicShellPolicyContext()
	for _, command := range []string{
		`eval -- 'rm -rf /'`,
		`stdbuf -- rm -rf /`,
		`stdbuf --output L rm -rf /`,
		`stdbuf --input L --error L rm -rf /`,
		`xargs -n1 rm -rf /`,
		`xargs -I{} rm -rf /`,
		`xargs -P2 rm -rf /`,
		`xargs -L1 rm -rf /`,
		`xargs -- "$UNSET" rm -rf /`,
		`eval "$UNSET" 'rm -rf /'`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyBlock {
			t.Errorf("hard block regressed for %q: %#v", command, decision)
		}
	}
}

func TestAnalyzeFindExecBindsPlaceholderToStaticSearchRoots(t *testing.T) {
	context := deterministicShellPolicyContext()
	for _, command := range []string{
		`find /etc -maxdepth 0 -exec rm -rf {} \;`,
		`find / -maxdepth 0 -exec rm -rf {} \;`,
		`find .git -maxdepth 0 -exec rm -rf {} \;`,
		`find -- / -maxdepth 0 -exec rm -rf {} \;`,
		`find -H /etc -maxdepth 0 -exec rm -rf {} \;`,
		`find -L /etc -maxdepth 0 -exec rm -rf {} \;`,
		`find -P /etc -maxdepth 0 -exec rm -rf {} \;`,
		`find -O2 /etc -maxdepth 0 -exec rm -rf {} \;`,
		`find -D tree /etc -maxdepth 0 -exec rm -rf {} \;`,
		`find -Z /etc -maxdepth 0 -exec rm -rf {} \;`,
		`find "$UNSET" /etc -maxdepth 0 -exec rm -rf {} \;`,
		`find /etc -maxdepth 0 -delete`,
		`find . -fprint .git/config`,
		`find . -fprint0 .git/config`,
		`find . -fprintf .git/config '%p\n'`,
		`find . -fls .git/config`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyBlock {
			t.Errorf("find root provenance lost hard block for %q: %#v", command, decision)
		}
	}
}

func TestAnalyzeFindExecdirUsesMatchDirectoryCWD(t *testing.T) {
	context := deterministicShellPolicyContext()
	for _, command := range []string{
		`find /etc -maxdepth 0 -execdir rm -rf . \;`,
		`find /etc -maxdepth 0 -okdir rm -rf . \;`,
		`find /tmp /etc -maxdepth 0 -execdir touch relative-output \;`,
		`find /tmp /etc -maxdepth 0 -okdir dd of=relative-output \;`,
		`find .git -maxdepth 0 -execdir touch config \;`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyBlock {
			t.Errorf("find execdir cwd lost hard block for %q: %#v", command, decision)
		}
	}
	if decision := AnalyzeShellCommand(`find /etc -maxdepth 0 -exec rm -rf . \;`, context); decision.Disposition == types.PolicyBlock {
		t.Fatalf("ordinary -exec incorrectly inherited -execdir cwd: %#v", decision)
	}
	if decision := AnalyzeShellCommand(`find "$UNSET" -execdir touch relative-output \;`, context); decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("dynamic execdir cwd did not fail closed: %#v", decision)
	}
	if decision := AnalyzeShellCommand(`find . -path '*/.git/*' -execdir touch config \;`, context); decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("unbounded execdir match cwd did not retain approval floor: %#v", decision)
	}
}

func TestAnalyzeWrapperCWDTransitions(t *testing.T) {
	context := deterministicShellPolicyContext()
	for _, command := range []string{
		`env -C .git touch config`,
		`env -C.git touch config`,
		`env --chdir .git touch config`,
		`env --chdir=.git touch config`,
		`sudo -D .git touch config`,
		`sudo -D.git touch config`,
		`sudo --chdir .git touch config`,
		`sudo --chdir=.git touch config`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyBlock {
			t.Errorf("wrapper cwd lost protected target for %q: %#v", command, decision)
		}
	}
	for _, command := range []string{
		`env -C "$UNSET" touch config`,
		`sudo --chdir="$UNSET" touch config`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyRequiredAsk {
			t.Errorf("dynamic wrapper cwd did not fail closed for %q: %#v", command, decision)
		}
	}
	for _, command := range []string{
		`env --chdir=build touch output`,
		`sudo -Dbuild touch output`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition == types.PolicyBlock {
			t.Errorf("safe wrapper cwd was hard-blocked for %q: %#v", command, decision)
		}
	}
}

func TestAnalyzeSequentialCDCarriesCWD(t *testing.T) {
	context := deterministicShellPolicyContext()
	for _, command := range []string{
		`cd .git && touch config`,
		`cd .git; touch config`,
		`command cd .git && touch config`,
		`builtin cd .git && touch config`,
		`(cd .git; touch config)`,
		`{ cd .git; touch config; }`,
		`cd / && rm -rf etc`,
		`cd /; rm -rf etc`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyBlock {
			t.Errorf("sequential cd lost hard block for %q: %#v", command, decision)
		}
	}
	if decision := AnalyzeShellCommand(`cd "$UNSET" && touch config`, context); decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("dynamic cd mutation did not fail closed: %#v", decision)
	}
	if decision := AnalyzeShellCommand(`cd subdir && touch file`, context); decision.Disposition == types.PolicyBlock {
		t.Fatalf("safe child cwd was hard-blocked: %#v", decision)
	}
	for _, command := range []string{
		`if cd .git; then touch config; fi`,
		`CDPATH=.git; cd sub && touch config`,
		`pushd .git && touch config`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyRequiredAsk {
			t.Errorf("unmodeled cwd transition did not require approval for %q: %#v", command, decision)
		}
	}
}

func TestAnalyzeCWDTransitionsResolveSymlinks(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "system")
	if err := os.Symlink("/etc", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	context := deterministicShellPolicyContext()
	context.CWD = root
	context.AllowedDirs = []string{root}
	for _, command := range []string{
		`env -C system touch config`,
		`cd system && touch config`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyBlock {
			t.Errorf("symlink cwd escaped system block for %q: %#v", command, decision)
		}
	}
}

func TestAnalyzeTimeWrapperOutputTarget(t *testing.T) {
	context := deterministicShellPolicyContext()
	for _, command := range []string{
		`command time -o .git/config true`,
		`/usr/bin/time -o.git/config true`,
		`command time --output .git/config true`,
		`/usr/bin/time --output=.git/config true`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyBlock {
			t.Errorf("time output escaped protected block for %q: %#v", command, decision)
		}
	}
	if decision := AnalyzeShellCommand(`command time --output=build/timing.txt true`, context); decision.Disposition == types.PolicyBlock {
		t.Fatalf("safe time output was hard-blocked: %#v", decision)
	}
}

func TestAnalyzeTargetDirectoryWriteDestinations(t *testing.T) {
	context := deterministicShellPolicyContext()
	for _, command := range []string{
		`cp -t .git source`,
		`cp -t.git source`,
		`cp --target-directory .git source`,
		`cp --target-directory=.git source`,
		`install -t .git source`,
		`mv --target-directory=.git source`,
		`ln -t.git source`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyBlock {
			t.Errorf("target-directory escaped protected block for %q: %#v", command, decision)
		}
	}
	for _, command := range []string{
		`cp -t build source-a source-b`,
		`install --target-directory=build source`,
		`mv -tbuild source`,
		`ln --target-directory build source`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition == types.PolicyBlock {
			t.Errorf("safe target-directory was hard-blocked for %q: %#v", command, decision)
		}
	}
}

func TestAnalyzeFindOutputActionsRequireApprovalForDynamicTargets(t *testing.T) {
	context := deterministicShellPolicyContext()
	for _, command := range []string{
		`find . -fprint "$OUTPUT"`,
		`find . -fprint0 "$OUTPUT"`,
		`find . -fprintf "$OUTPUT" '%p\n'`,
		`find . -fls "$OUTPUT"`,
	} {
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyRequiredAsk {
			t.Errorf("dynamic find output target did not require approval for %q: %#v", command, decision)
		}
	}
}

func TestBashRuleMatcherUsesUnifiedTransparentWrappers(t *testing.T) {
	rules := []permissions.Rule{{Tool: "Bash", Pattern: "git status", Decision: permissions.DecisionDeny}}
	commands := []string{
		`command git status`,
		`builtin command git status`,
		`exec git status`,
		`env FOO=bar git status`,
		`sudo -n -- git status`,
		`doas -n -- git status`,
		`nohup git status`,
		`time -p git status`,
		`timeout 5s git status`,
		`nice -n 5 git status`,
		`ionice -c 3 git status`,
		`stdbuf -oL git status`,
		`stdbuf -- git status`,
		`stdbuf --output L git status`,
		`unbuffer git status`,
		`taskset 0x1 git status`,
		`chrt -f 99 git status`,
	}
	for _, command := range commands {
		decision, matched := MatchBashRule(command, rules)
		if decision != permissions.DecisionDeny || matched == nil {
			t.Errorf("transparent wrapper %q did not match deny rule: decision=%v matched=%#v", command, decision, matched)
		}
	}
}

func TestBashRuleDenySeesAnalyzerNestedExecutionForms(t *testing.T) {
	rules := []permissions.Rule{{Tool: "Bash", Pattern: "rm:*", Decision: permissions.DecisionDeny}}
	for _, command := range []string{
		`eval 'rm -rf /tmp/example'`,
		`eval -- 'rm -rf /tmp/example'`,
		`eval "$UNSET" 'rm -rf /tmp/example'`,
		`sh -c 'rm -rf /tmp/example'`,
		`busybox rm -rf /tmp/example`,
		`xargs rm -rf`,
		`xargs -- "$UNSET" rm -rf`,
		`find . -exec rm -rf {} \;`,
	} {
		decision, matched := MatchBashRule(command, rules)
		if decision != permissions.DecisionDeny || matched == nil {
			t.Errorf("nested execution %q hid deny rule: decision=%v matched=%#v", command, decision, matched)
		}
	}
}

func TestBashRuleAllowDoesNotExpandAcrossUnsafeExecutionAuthority(t *testing.T) {
	rules := []permissions.Rule{{Tool: "Bash", Pattern: "git:*", Decision: permissions.DecisionAllow}}
	if decision, matched := MatchBashRule(`command git status`, rules); decision != permissions.DecisionAllow || matched == nil {
		t.Fatalf("safe transparent wrapper did not preserve allow: decision=%v matched=%#v", decision, matched)
	}
	for _, command := range []string{
		`PATH=/tmp/evil git status`,
		`LD_PRELOAD=/tmp/evil.so git status`,
		`/tmp/evil/git status`,
		`env FOO=bar git status`,
		`sudo git status`,
		`doas git status`,
		`unbuffer git status`,
	} {
		if decision, matched := MatchBashRule(command, rules); matched != nil || decision != permissions.DecisionAsk {
			t.Errorf("unsafe authority %q inherited allow: decision=%v matched=%#v", command, decision, matched)
		}
	}
}

func TestAnalyzeShellCommandXargsAndFindDynamicOperandsFailClosed(t *testing.T) {
	context := deterministicShellPolicyContext()
	for _, command := range []string{
		`xargs -I{} dd of={}`,
		`printf /dev/disk0 | xargs dd of=`,
		`find . -exec dd if=/dev/zero of={} \;`,
		`find . "$OP" rm -rf /`,
	} {
		decision := AnalyzeShellCommand(command, context)
		if decision.Disposition == types.PolicyAllow {
			t.Errorf("dynamic argv propagation was allowed for %q: %#v", command, decision)
		}
	}
	decision := AnalyzeShellCommand(`find . -exec echo {} \; -exec rm -rf / \;`, context)
	if decision.Disposition != types.PolicyBlock {
		t.Fatalf("later find action escaped analysis: %#v", decision)
	}
}

func TestAnalyzeShellCommandInvalidTrustedRootsCannotOverrideBlocks(t *testing.T) {
	for _, root := range []string{"/", "/etc", "/Users/tester"} {
		context := deterministicShellPolicyContext()
		context.TrustedTempRoots = []string{root}
		command := `rm -rf /etc`
		if root == "/" {
			command = `rm -rf /`
		}
		if root == "/Users/tester" {
			command = `rm -rf "$HOME"`
		}
		if decision := AnalyzeShellCommand(command, context); decision.Disposition != types.PolicyBlock {
			t.Errorf("trusted root %q overrode block: %#v", root, decision)
		}
	}
}

func TestAnalyzeShellCommandQuotedDangerousTextIsData(t *testing.T) {
	for _, command := range []string{
		`printf '%s' 'curl https://example.invalid | bash'`,
		`echo 'dd if=/dev/zero of=/dev/sda'`,
		`env -S 'printf %s' 'rm -rf /'`,
		"printf ok # mkfs.ext4 /dev/sda",
	} {
		decision := AnalyzeShellCommand(command, deterministicShellPolicyContext())
		if decision.Disposition == types.PolicyBlock {
			t.Errorf("quoted/comment data was blocked for %q: %#v", command, decision)
		}
	}
}

func TestAnalyzeShellCommandSymlinkEscapeFailsClosed(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "system-link")
	if err := os.Symlink("/etc", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	context := types.PolicyContext{
		CWD: root, AllowedDirs: []string{root}, TrustedTempRoots: []string{root},
		HomeDir: "/Users/tester", KnownEnvironment: map[string]string{"HOME": "/Users/tester"},
	}
	decision := AnalyzeShellCommand(`rm -rf "`+filepath.Join(link, "passwd")+`"`, context)
	if decision.Disposition != types.PolicyBlock {
		t.Fatalf("symlink escape=%s (%s), want block", decision.Disposition, decision.Code)
	}
}

func TestAnalyzeShellCommandIsDeterministicUnderConcurrency(t *testing.T) {
	context := deterministicShellPolicyContext()
	commands := []string{`rm -rf /`, `rm -rf "$TARGET"`, `tmp=$(mktemp -d); rm -rf "$tmp"`, `printf ok`}
	for i := 0; i < 20; i++ {
		for _, command := range commands {
			command := command
			t.Run(command, func(t *testing.T) {
				t.Parallel()
				first := AnalyzeShellCommand(command, context)
				second := AnalyzeShellCommand(command, context)
				if first.Disposition != second.Disposition || first.Code != second.Code {
					t.Fatalf("non-deterministic decision: %#v then %#v", first, second)
				}
			})
		}
	}
}

func TestBashToolPermissionConsumesUnifiedPolicyBeforeAllowRules(t *testing.T) {
	tool := &BashTool{
		CWD:         "/workspace/project",
		AllowedDirs: []string{"/workspace/project"},
		PermissionRules: []permissions.Rule{{
			Tool: "Bash", Pattern: "rm ", Decision: permissions.DecisionAllow,
		}},
	}
	tests := []struct {
		command  string
		behavior types.PermissionBehavior
	}{
		{`rm -rf /`, types.PermissionBehaviorDeny},
		{`rm -rf "$TARGET"`, types.PermissionBehaviorAsk},
	}
	for _, test := range tests {
		decision, err := tool.CheckPermissions(context.Background(), map[string]any{"command": test.command}, types.ToolPermissionRequest{})
		if err != nil || decision.Behavior != test.behavior || !decision.Required {
			t.Fatalf("CheckPermissions(%q)=%#v err=%v", test.command, decision, err)
		}
		if len(decision.Suggestions) != 0 {
			t.Fatalf("mandatory policy proposed a broad persistent rule: %#v", decision.Suggestions)
		}
	}
}

func TestBashToolExplicitDenyOutranksRequiredAsk(t *testing.T) {
	tool := &BashTool{
		CWD: t.TempDir(),
		PermissionRules: []permissions.Rule{{
			Tool: "Bash", Pattern: "rm *", Decision: permissions.DecisionDeny,
		}},
	}
	decision, err := tool.CheckPermissions(context.Background(), map[string]any{
		"command": `rm -rf "$TARGET"`,
	}, types.ToolPermissionRequest{})
	if err != nil || decision.Behavior != types.PermissionBehaviorDeny || !decision.Required {
		t.Fatalf("explicit deny did not outrank RequiredAsk: decision=%#v err=%v", decision, err)
	}
	if decision.PolicyDecision != nil && decision.PolicyDecision.Disposition == types.PolicyRequiredAsk {
		t.Fatalf("RequiredAsk leaked through stronger deny: %#v", decision.PolicyDecision)
	}
}

func TestBashRuntimeContentRulesAndCompoundPartialMatchesFailClosed(t *testing.T) {
	tool := &BashTool{CWD: t.TempDir()}
	input := map[string]any{"command": `printf ok`}
	for _, test := range []struct {
		name     string
		runtime  types.ToolRuntimeContext
		behavior types.PermissionBehavior
	}{
		{"runtime deny", types.ToolRuntimeContext{DeniedRules: []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "printf *"}}}, types.PermissionBehaviorDeny},
		{"runtime ask", types.ToolRuntimeContext{AskRules: []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "printf *"}}}, types.PermissionBehaviorAsk},
		{"deny beats allow", types.ToolRuntimeContext{
			AllowedRules: []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "printf *"}},
			DeniedRules:  []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "printf *"}},
		}, types.PermissionBehaviorDeny},
	} {
		result, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: test.runtime})
		if err != nil || result.Behavior != test.behavior || !result.Required && test.behavior != types.PermissionBehaviorAllow {
			t.Errorf("%s: result=%#v err=%v", test.name, result, err)
		}
	}

	tool.PermissionRules = []permissions.Rule{{Tool: "Bash", Pattern: "printf *", Decision: permissions.DecisionAllow}}
	partial, err := tool.CheckPermissions(context.Background(), map[string]any{"command": `printf ok; echo uncovered`}, types.ToolPermissionRequest{})
	if err != nil || partial.Behavior != types.PermissionBehaviorAsk || !partial.Required {
		t.Fatalf("compound partial rule match failed open: result=%#v err=%v", partial, err)
	}

	scope := NewRuntimeScope(tool.CWD, false)
	scope.SetDeniedTools([]string{"Bash(printf *)"})
	reg := registry.New()
	reg.SetRuntimeContextProvider(scope)
	reg.Register(tool)
	result, err := reg.CheckToolPermissions(context.Background(), "Bash", input, types.ToolPermissionRequest{})
	if err != nil || result.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("RuntimeScope content deny was not injected: result=%#v err=%v", result, err)
	}
}

func TestRegistryBlanketAskCannotHideBashContentDenyOrHardBlock(t *testing.T) {
	root := t.TempDir()
	scope := NewRuntimeScope(root, true)
	scope.SetAskTools([]string{"Bash"})
	scope.SetDeniedTools([]string{"Bash(printf *)"})
	tool := &BashTool{CWD: root, AllowedDirs: []string{root}}
	reg := registry.New()
	reg.SetRuntimeContextProvider(scope)
	reg.Register(tool)

	denied, err := reg.CheckToolPermissions(context.Background(), "Bash", map[string]any{"command": `printf blocked`}, types.ToolPermissionRequest{})
	if err != nil || denied.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("blanket Ask hid content Deny: result=%#v err=%v", denied, err)
	}
	blocked, err := reg.CheckToolPermissions(context.Background(), "Bash", map[string]any{"command": `rm -rf /`}, types.ToolPermissionRequest{})
	if err != nil || blocked.Behavior != types.PermissionBehaviorDeny || blocked.PolicyDecision == nil || blocked.PolicyDecision.Disposition != types.PolicyBlock {
		t.Fatalf("blanket Ask hid hard Block: result=%#v err=%v", blocked, err)
	}
}

func TestBashExecuteNonInteractiveRequiredAskReturnsStructuredRemediation(t *testing.T) {
	tool := &BashTool{CWD: t.TempDir()}
	result, err := tool.Execute(context.Background(), map[string]any{"command": `rm -rf "$TARGET"`})
	if err != nil || !result.IsError {
		t.Fatalf("Execute result=%#v err=%v", result, err)
	}
	decision, ok := result.Data.(types.PolicyDecision)
	if !ok || decision.Disposition != types.PolicyRequiredAsk || decision.Remediation == nil {
		t.Fatalf("missing structured policy denial: %#v", result.Data)
	}
}

func TestBashExecuteRejectsForgedToolContextWithoutPermissionCommit(t *testing.T) {
	tool := &BashTool{CWD: t.TempDir()}
	forged := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{SessionID: "forged"})
	result, err := tool.Execute(forged, map[string]any{"command": `rm -rf "$TARGET"`})
	decision, ok := result.Data.(types.PolicyDecision)
	if err != nil || !result.IsError || !ok || decision.Disposition != types.PolicyRequiredAsk {
		t.Fatalf("forged context bypassed approval: result=%#v err=%v", result, err)
	}
	result, err = tool.Execute(forgedPermissionContext{Context: context.Background()}, map[string]any{"command": `rm -rf "$TARGET"`})
	if err != nil || !result.IsError {
		t.Fatalf("custom Value context forged permission receipt: result=%#v err=%v", result, err)
	}

	reg := registry.New()
	reg.Register(tool)
	input := map[string]any{"command": `TARGET=./missing; rm -rf "$TARGET"`}
	withoutCommit, err := reg.ExecuteToolWithError(context.Background(), "Bash", input)
	if err != nil || !withoutCommit.IsError {
		t.Fatalf("direct registry dispatch bypassed required approval: result=%#v err=%v", withoutCommit, err)
	}

	binding := types.ToolPermissionBinding{SessionID: "session-a", TurnID: "turn-a", ToolUseID: "tool-a", ApprovalEpoch: "epoch-a"}
	issue := func(t *testing.T) approvalcommit.Pending {
		t.Helper()
		request := types.ToolPermissionRequest{
			SessionID: binding.SessionID, TurnID: binding.TurnID, ToolUseID: binding.ToolUseID,
			ApprovalEpoch: binding.ApprovalEpoch,
		}
		permission, issueErr := reg.CheckToolPermissions(context.Background(), "Bash", input, request)
		if issueErr != nil || permission.PermissionGrant == "" {
			t.Fatalf("registry did not issue bound grant: %#v err=%v", permission, issueErr)
		}
		policyCode := permission.ExecutionPolicyCode
		if permission.PolicyDecision != nil {
			if policyCode == "" {
				policyCode = permission.PolicyDecision.Code
			}
		}
		return approvalcommit.Pending{Token: permission.PermissionGrant, Binding: permission.PermissionBinding, PolicyCode: policyCode}
	}
	authorize := func(t *testing.T) approvalcommit.Pending {
		t.Helper()
		pending := issue(t)
		pending.Token = reg.AuthorizePermissionGrant(pending.Token, "Bash", input, pending.Binding, pending.PolicyCode)
		if pending.Token == "" {
			t.Fatal("registry did not promote preflight to execution grant")
		}
		return pending
	}

	pending := issue(t)
	unapproved, err := reg.ExecuteToolWithError(approvalcommit.WithPending(context.Background(), pending), "Bash", input)
	if err != nil || !unapproved.IsError {
		t.Fatalf("preflight token executed without approval: result=%#v err=%v", unapproved, err)
	}
	committing := approvalcommit.WithPending(context.Background(), authorize(t))
	mapped, err := reg.ExecuteToolWithError(committing, "Bash", input)
	if err != nil || mapped.IsError {
		t.Fatalf("bound permission commit did not authorize execution: result=%#v err=%v", mapped, err)
	}
	replay, err := reg.ExecuteToolWithError(committing, "Bash", input)
	if err != nil || !replay.IsError {
		t.Fatalf("one-time permission grant replayed: result=%#v err=%v", replay, err)
	}

	mutated := approvalcommit.WithPending(context.Background(), authorize(t))
	changed, err := reg.ExecuteToolWithError(mutated, "Bash", map[string]any{"command": `printf changed`})
	if err != nil || !changed.IsError {
		t.Fatalf("input-bound grant accepted changed input: result=%#v err=%v", changed, err)
	}

	bindingMutations := []struct {
		name   string
		mutate func(*approvalcommit.Pending)
	}{
		{"session", func(p *approvalcommit.Pending) { p.Binding.SessionID = "session-b" }},
		{"policy owner session", func(p *approvalcommit.Pending) { p.Binding.PolicyOwnerSessionID = "session-b" }},
		{"turn", func(p *approvalcommit.Pending) { p.Binding.TurnID = "turn-b" }},
		{"tool use", func(p *approvalcommit.Pending) { p.Binding.ToolUseID = "tool-b" }},
		{"epoch", func(p *approvalcommit.Pending) { p.Binding.ApprovalEpoch = "epoch-b" }},
		{"policy code", func(p *approvalcommit.Pending) { p.PolicyCode = "shell.policy.other" }},
	}
	for _, test := range bindingMutations {
		pending := authorize(t)
		test.mutate(&pending)
		crossBinding, bindingErr := reg.ExecuteToolWithError(approvalcommit.WithPending(context.Background(), pending), "Bash", input)
		if bindingErr != nil || !crossBinding.IsError {
			t.Fatalf("grant crossed %s binding: result=%#v err=%v", test.name, crossBinding, bindingErr)
		}
	}

	reg.Register(NewEnterPlanModeTool(NewPlanState(t.TempDir())))
	crossTool, err := reg.ExecuteToolWithError(approvalcommit.WithPending(context.Background(), authorize(t)), "EnterPlanMode", map[string]any{})
	if err != nil || !crossTool.IsError {
		t.Fatalf("tool-bound grant crossed tool identity: result=%#v err=%v", crossTool, err)
	}
}

func TestPermissionGrantBindsRuntimeSnapshotAndBashLocalScope(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	scope := NewRuntimeScope(root, false)
	scope.SetAllowedDirs([]string{root})
	tool := &BashTool{CWD: root, AllowedDirs: []string{root}}
	reg := registry.New()
	reg.SetRuntimeContextProvider(scope)
	reg.Register(tool)
	input := map[string]any{"command": `rm -rf "$TARGET"`}
	request := types.ToolPermissionRequest{SessionID: "session", TurnID: "turn", ToolUseID: "bash", ApprovalEpoch: "epoch"}

	preflight, err := reg.CheckToolPermissions(context.Background(), "Bash", input, request)
	if err != nil || preflight.PermissionGrant == "" {
		t.Fatalf("runtime preflight=%#v err=%v", preflight, err)
	}
	scope.SetAllowedDirs([]string{other})
	if token := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, "Bash", input, preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	); token != "" {
		t.Fatalf("stale runtime snapshot authorized token %q", token)
	}

	scope.SetAllowedDirs([]string{root})
	preflight, err = reg.CheckToolPermissions(context.Background(), "Bash", input, request)
	if err != nil {
		t.Fatal(err)
	}
	executionGrant := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, "Bash", input, preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	)
	tool.CWD = other
	tool.AllowedDirs = []string{other}
	result, err := reg.ExecuteToolWithError(approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: executionGrant, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	}), "Bash", input)
	if err != nil || !result.IsError {
		t.Fatalf("changed Bash cwd reused old receipt: result=%#v err=%v", result, err)
	}
}

func TestPermissionGrantBindsLocalBashPermissionRules(t *testing.T) {
	root := t.TempDir()
	tool := &BashTool{
		CWD: root,
		PermissionRules: []permissions.Rule{{
			Tool: "Bash", Pattern: "printf *", Decision: permissions.DecisionAllow,
		}},
	}
	reg := registry.New()
	reg.Register(tool)
	input := map[string]any{"command": `printf ok`}
	request := types.ToolPermissionRequest{SessionID: "session", TurnID: "turn", ToolUseID: "bash", ApprovalEpoch: "epoch"}
	preflight, err := reg.CheckToolPermissions(context.Background(), "Bash", input, request)
	if err != nil || preflight.PermissionGrant == "" {
		t.Fatalf("preflight=%#v err=%v", preflight, err)
	}
	executionGrant := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, "Bash", input, preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	)
	tool.PermissionRules = []permissions.Rule{{Tool: "Bash", Pattern: "printf *", Decision: permissions.DecisionDeny}}
	result, err := reg.ExecuteToolWithError(approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: executionGrant, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	}), "Bash", input)
	if err != nil || !result.IsError {
		t.Fatalf("changed local PermissionRules reused old receipt: result=%#v err=%v", result, err)
	}
}

func TestPermissionGrantRejectsAllowCommandAfterCWDMutation(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	tool := &BashTool{CWD: root, AllowedDirs: []string{root}}
	reg := registry.New()
	reg.Register(tool)
	input := map[string]any{"command": `touch receipt-must-not-retarget`}
	request := types.ToolPermissionRequest{SessionID: "session", TurnID: "turn", ToolUseID: "bash", ApprovalEpoch: "epoch"}
	preflight, err := reg.CheckToolPermissions(context.Background(), "Bash", input, request)
	if err != nil || preflight.PermissionGrant == "" {
		t.Fatalf("preflight=%#v err=%v", preflight, err)
	}
	token := reg.AuthorizePermissionGrant(preflight.PermissionGrant, "Bash", input, preflight.PermissionBinding, preflight.ExecutionPolicyCode)
	tool.SetExecutionScope(other, []string{other})
	result, err := reg.ExecuteToolWithError(approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: token, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	}), "Bash", input)
	if err != nil || !result.IsError {
		t.Fatalf("changed CWD reused allow receipt: result=%#v err=%v", result, err)
	}
	for _, marker := range []string{filepath.Join(root, "receipt-must-not-retarget"), filepath.Join(other, "receipt-must-not-retarget")} {
		if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
			t.Fatalf("invalid receipt created marker %q: %v", marker, statErr)
		}
	}
}

func TestBashExecuteHardBlockUsesUnifiedPolicy(t *testing.T) {
	tool := &BashTool{CWD: t.TempDir()}
	result, err := tool.Execute(context.Background(), map[string]any{"command": `rm -rf /`})
	decision, ok := result.Data.(types.PolicyDecision)
	if err != nil || !result.IsError || !ok || decision.Disposition != types.PolicyBlock || decision.Code != "shell.policy.block.root" {
		t.Fatalf("hard block result=%#v err=%v", result, err)
	}
}

func TestDirectBashExecuteCannotBypassLocalPermissionRules(t *testing.T) {
	for _, decision := range []permissions.Decision{permissions.DecisionDeny, permissions.DecisionAsk} {
		tool := &BashTool{
			CWD: t.TempDir(),
			PermissionRules: []permissions.Rule{{
				Tool: "Bash", Pattern: "printf", Decision: decision,
			}},
		}
		result, err := tool.Execute(context.Background(), map[string]any{"command": `printf ok`})
		if err != nil || !result.IsError {
			t.Errorf("direct Execute bypassed local rule %v: result=%#v err=%v", decision, result, err)
		}
	}
}

func TestDirectRegistryDoesNotTreatBashPassthroughAsApproval(t *testing.T) {
	root := t.TempDir()
	tool := &BashTool{CWD: root, AllowedDirs: []string{root}}
	reg := registry.New()
	reg.Register(tool)
	marker := filepath.Join(root, "must-not-exist")
	result, err := reg.ExecuteToolWithError(context.Background(), "Bash", map[string]any{
		"command": `touch "` + marker + `"`,
	})
	if err != nil || !result.IsError {
		t.Fatalf("direct Bash passthrough executed: result=%#v err=%v", result, err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("direct Bash passthrough created marker: %v", statErr)
	}
}

func TestBashPermissionRejectionPreservesStructuredRemediation(t *testing.T) {
	tool := &BashTool{CWD: "/workspace/project", AllowedDirs: []string{"/workspace/project"}}
	result := tool.MapToolPermissionRejection(
		map[string]any{"command": `rm -rf "$TARGET"`}, "toolu_policy", "",
	)
	decision, ok := result.Data.(types.PolicyDecision)
	if !result.IsError || result.Outcome != types.ToolOutcomeDenied || !ok || decision.Remediation == nil {
		t.Fatalf("permission rejection lost policy structure: %#v", result)
	}
	if result.Content == "" {
		t.Fatal("permission rejection lost localized public remediation")
	}
}

func TestBashExecuteTrustedMktempCleanupIsNotSecondarilyBlocked(t *testing.T) {
	root := t.TempDir()
	tool := &BashTool{CWD: root, AllowedDirs: []string{root}}
	template := filepath.Join(root, "policy-cleanup.XXXXXX")
	command := `tmp=$(mktemp -d "` + template + `"); touch "$tmp/probe"; rm -rf -- "$tmp"; test ! -e "$tmp"`
	result, err := tool.Execute(context.Background(), map[string]any{"command": command})
	if err != nil || result.IsError {
		t.Fatalf("trusted cleanup failed: result=%#v err=%v", result, err)
	}
	matches, globErr := filepath.Glob(filepath.Join(root, "policy-cleanup.*"))
	if globErr != nil || len(matches) != 0 {
		t.Fatalf("cleanup left paths=%v err=%v", matches, globErr)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("test root was removed: %v", statErr)
	}
}
