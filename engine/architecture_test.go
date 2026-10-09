package guard

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/archguard"
)

// rings declares the layer of every package of this module (docs/architecture/
// hexagonal-target.md, section 1). A package missing here fails the guard, so a
// new package starts with a decision about where it sits; a row whose package is
// gone fails it too.
//
// Where the document leaves a package open the choice is made here:
//
//   - runtime is an adapter: it holds the concrete runtime adapters (Claude, Codex,
//     OpenCode, Pi, longterm-mem) and the registration of each, and reaches the
//     machine's files. runtime/core is its pure core, a domain package (H26): the
//     Target vocabulary, the Adapter port, the Registry, the Config the composition
//     root fills, and the prompt rules.
//   - runtime/opencodeprompt is the pure half of the OpenCode adapter (H26): it derives
//     the prompt config from the text of the contracts a ContractSource hands it,
//     verifies a recorded one against the current one, and hashes it. The adapter reads
//     the files; this package parses text.
//   - pathguard, capability, projection, workflow and the like are domain
//     packages because their pure half is the part that stays; the file system
//     half is debt below.
//   - assets, gadu, synctrigger, filelock, gitprov, statestore and atomicfile are
//     adapters: infrastructure the domain reaches only through a port.
//   - capability/presence is the adapter of the workflow's DependencyProber port:
//     it stats files and the PATH, and capability itself stays pure.
//   - shaper/fsadapter is the adapter of the shaper's file-facing ports: the
//     ClearanceStore and the ContainedSource (H10).
//   - reviewreceipt/fsstore is the adapter of the four ports of the review receipt
//     capture (H12): the transaction stores found through gitprov, the receipts, the
//     persisted receipts and the changes, all of them files.
//   - hookwire is the adapter of the Claude Code hook protocol (H14): it decodes what a hook is
//     handed and encodes what it answers, imports nothing of the module, and is the one home of
//     the hook's JSON tags; the policies take and return plain values.
//   - skills/registryyaml is the adapter of the skills domain's RegistryRepository (H15): the
//     YAML file of the registry, its reader and its writer, with the policy for what the reader
//     does not understand (H16). It never calls the domain's Validate: the domain judges what
//     the adapter returns.
//   - skills/skillsfs is the adapter of the skills domain's file-facing ports (H17): the tree of
//     skills an overlay keeps, and the files of a project and the staged writes of an overlay.
//     It is the one place engine/skills reaches the operating system through.
//   - skills/app holds the use cases of `engine skills` (H20): typed inputs and results over the
//     ports of the skills domain, no argument vector, no printing, no exit. The command line and
//     the words a person reads are the CLI adapter in cmd.
//   - skills/projectidentity is the adapter of the skills domain's ProjectIdentity port (H18): the
//     sources that name the project a directory is (the id the person gave, the origin remote read
//     from .git/config without running git, the name of the directory) and the chain that asks them
//     in order. The rule that reduces a remote url to a name is the identity module's, shared with
//     longterm-mem (D2); the order of the chain is the composition root's.
//   - execrunner is the adapter of the CommandRunner port the Pi adapter owns (H24): it looks a
//     program up on the PATH and runs it with a fixed argument vector and a deadline. It is
//     the one place in the module that starts the CLI of a runtime.
//   - pipkg/gitsource is the adapter of the package builder's SourceRepo port (H25): it asks the
//     git of the machine through a Runner (execrunner), under the environment and the deadline
//     the composition root hands it. pipkg itself stays an adapter (it writes the package tree);
//     the port is the part of it that is not the file system.
//   - settings is a domain package (H27): the Document that merges and removes the hook entries of
//     Claude Code's settings.json over bytes, and the helpers that say which families a decoded
//     settings object holds. It imports guardmarkers and the pure standard library, no file.
//   - settings/settingsfile is the adapter of that model (H27): it reads and writes the file on
//     atomicfile, and answers the HookInstaller port the Claude runtime adapter owns.
//   - propagator/app holds the use case of `engine propagate` (H28): the pass that reads the
//     contract and the registry and writes the scoped row, and the bounded loop that reads every
//     write back and weighs what each pass found. It answers with a typed Outcome and typed errors,
//     over the RegistryStore and ContractSource ports it owns; it takes no lock, prints nothing and
//     exits nowhere.
//   - propagator/fsstore is the adapter of that RegistryStore (H28): it reads the registry file and
//     writes it through a temporary file and a rename. The words a person reads and the lock are the
//     command's, in cmd.
//   - projection/app holds the use cases of the session binding and of the projection hook (H29):
//     BindWorkflow (bind, unbind and describe a repository's binding) and HookService (what the
//     prompt hook projects and what the tool-call gate allows), over the RepoLocator and
//     BindingStore ports of projection and the WorkflowReader and Clock it owns. It answers with
//     typed values and typed refusals, decodes no hook input, prints nothing and takes no time from
//     the machine.
//   - gitfs is the adapter of the projection domain's RepoLocator port (H29): it finds the repository
//     a directory belongs to, and the worktree and HEAD a workflow records, by reading the files of
//     the repository with no subprocess. It shares with gitprov only what a pointer file names.
//   - status holds the use case of `engine status`, the doctor of an installation (H30): the checks
//     of the binary, the hooks and guards in Claude's settings.json, the contract and the registry
//     of the project, over the Files and SettingsSource ports it owns. It answers with a Report of
//     typed Checks; the home and the directory are in its Request, it prints nothing and exits
//     nowhere. The line a person reads and the exit code are the command's, in cmd.
//   - status/fsfiles is the adapter of status's Files port (H30): the file system of the machine.
//     The adapter of its SettingsSource port is settings/settingsfile.Reader.
//   - repotest is test support: the hand-made repositories the tests of gitfs and cmd share.
//   - piguard is test support: the `pi` that refuses to run, put first on the PATH of a test
//     run by a TestMain, so nothing started by accident reaches the real CLI.
//   - installer, shelltest, capabilitytest, shaper/shapertest (the documents the shaper's
//     tests share), reviewreceipt/receipttest (the review documents the receipt capture's
//     tests share) and this guard (the module root) are test-only.
var rings = map[string]archguard.Ring{
	".":                         archguard.Support,
	"assets":                    archguard.Adapter,
	"atomicfile":                archguard.Adapter,
	"capability":                archguard.Domain,
	"capability/presence":       archguard.Adapter,
	"capabilitytest":            archguard.Support,
	"cmd":                       archguard.Root,
	"contract":                  archguard.Domain,
	"execrunner":                archguard.Adapter,
	"filelock":                  archguard.Adapter,
	"gadu":                      archguard.Adapter,
	"gate":                      archguard.Domain,
	"gitfs":                     archguard.Adapter,
	"gitprov":                   archguard.Adapter,
	"goal":                      archguard.Domain,
	"hookwire":                  archguard.Adapter,
	"installer":                 archguard.Support,
	"guardmarkers":              archguard.Domain,
	"jsonstrict":                archguard.Domain,
	"memoryscope":               archguard.Domain,
	"pathguard":                 archguard.Domain,
	"pathguard/fsresolve":       archguard.Adapter,
	"piguard":                   archguard.Support,
	"pipkg":                     archguard.Adapter,
	"pipkg/gitsource":           archguard.Adapter,
	"prespec":                   archguard.Domain,
	"projection":                archguard.Domain,
	"projection/app":            archguard.Application,
	"projection/fsstore":        archguard.Adapter,
	"propagator":                archguard.Domain,
	"propagator/app":            archguard.Application,
	"propagator/fsstore":        archguard.Adapter,
	"repotest":                  archguard.Support,
	"reviewreceipt":             archguard.Domain,
	"reviewreceipt/fsstore":     archguard.Adapter,
	"reviewreceipt/receipttest": archguard.Support,
	"roles":                     archguard.Domain,
	"roles/filechain":           archguard.Adapter,
	"runtime":                   archguard.Adapter,
	"runtime/core":              archguard.Domain,
	"runtime/opencodeprompt":    archguard.Domain,
	"settings":                  archguard.Domain,
	"settings/settingsfile":     archguard.Adapter,
	"shaper":                    archguard.Domain,
	"shaper/fsadapter":          archguard.Adapter,
	"shaper/shapertest":         archguard.Support,
	"shelltest":                 archguard.Support,
	"skills":                    archguard.Domain,
	"skills/app":                archguard.Application,
	"skills/projectidentity":    archguard.Adapter,
	"skills/registryyaml":       archguard.Adapter,
	"skills/skillsfs":           archguard.Adapter,
	"statestore":                archguard.Adapter,
	"status":                    archguard.Application,
	"status/fsfiles":            archguard.Adapter,
	"synctrigger":               archguard.Adapter,
	"workflow":                  archguard.Domain,
	"workflow/filelog":          archguard.Adapter,
	"workflowprofile":           archguard.Domain,
}

// knownDebt is every violation of the rule that exists today, each owed to the
// work unit that removes it: package, then the import or member it should not
// use, then the unit. It only shrinks: a unit deletes its lines when it lands, and
// a line whose violation is gone fails the guard, so none can be left behind. A
// new violation is not added here to make the guard pass; it is fixed.
//
// The guard reads imports and the members selected from a few standard packages.
// It cannot see package-level mutable variables used as test seams, so these
// known ones are not listed below and are owed to their units all the same:
//
//   - cmd: skillsLockWait and the other global seams of main.go (H31)
var knownDebt = archguard.Debt{}

// TestArchitectureFollowsTheDependencyRule is the fitness function: it fails on
// a package without a ring, on a violation that is not known debt, and on debt
// that is no longer a violation.
func TestArchitectureFollowsTheDependencyRule(t *testing.T) {
	problems, err := archguard.Check(".", rings, knownDebt)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}
