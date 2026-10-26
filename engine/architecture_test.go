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
//   - assets, agentgen, synctrigger, filelock, gitprov, statestore and atomicfile are
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
//   - skills/projectidentity is the adapter of the skills domain's ProjectIdentity port (H18): it
//     asks the shared resolver of the identity module (identity/resolve, C23 M2), whose chain and
//     disk adapter (identity/gitfs) are shared with longterm-mem; the composition root hands it the
//     adapter.
//   - skills/engramexport adapts the read-only `engram export` command to the CandidateSource port
//     owned by skills and consumed by skills/app (C23 M10); it filters topics and never writes Engram.
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
//   - runtime/ownership is a domain package (B3): the rule for the record of the runtimes the
//     overlay has installed longterm-mem for (how the record is read as evidence, which names it
//     vouches for, when the shared binary and the engine's registration record may go). It reads and
//     writes no file.
//   - runtime/ownership/app holds the use case of `engine installed-targets` (B3): record a target,
//     release one, report the record, assess what an uninstall comes to and retire the shared binary
//     and the record, each read-modify-write one critical section, over the Store and Binaries ports
//     it owns. It takes no time from the machine, prints nothing and exits nowhere.
//   - runtime/ownership/fsstore is the adapter of those two ports (B3): the record as the file the
//     script always kept (appended to, and rewritten through a temporary file and a rename), the lock
//     around it as an advisory file lock on engine/filelock, and the shared binary as a file. The
//     words a person reads are the command's, in cmd.
//   - synctrigger/osproc is the adapter of the Runner port of synctrigger (H34): it starts the
//     programs of the machine in sessions and process groups of their own and kills a group at a
//     deadline. synctrigger itself reads neither the clock nor the process table: it is handed both.
//   - sddregistry is a domain package (B2): the rule over text for the SDD executor rows of a
//     project's skill registry (which skills are executors, how a row reads, how a registry that
//     has none starts, how the rows of an earlier repair are replaced). It reads and writes no file.
//   - sddregistry/app holds the use case of `engine registry repair-sdd` (B2): it reads the
//     registry, or starts one, reads the executor skills a target has installed and writes the
//     registry back, over the RegistryStore and SkillSource ports it owns. It takes no lock, prints
//     nothing and exits nowhere.
//   - sddregistry/fsstore is the adapter of those two ports (B2): the registry as a file, written
//     as `engine propagate` writes it (propagator/fsstore), and the skills of a target as files.
//     The lock and the words a person reads are the command's, in cmd.
//   - sha256hex is a domain package (H34): what a SHA-256 digest looks like on the wire (64
//     lowercase hex characters) and how one is made, which every record that carries a digest
//     (the goal, the workflow event, the role handoff, the clearance, the approval, the project
//     lock, the binding) shares. Pure standard library.
//   - gitobject is a domain package (H34): what a full git object name looks like (40 lowercase
//     hex characters, or 64), which the workflow event that records a commit, the observation of a
//     worktree by the git binary (gitprov) and the reader of a repository's files (gitfs) share.
//     Pure standard library.
//   - fieldrule is a domain package (H34): the rules for the shape of one field of a versioned
//     document that more than one record shares (today the string array of a Goal and of a Shaper
//     handoff). Pure standard library.
//   - reviewresult is a domain package (own-rdd T1): the contract of what the reviewer role returns, and
//     the rules that refuse a reply (the candidate it echoes, its inspection, the paths it read, the enums,
//     the ids and the proofs of its findings). It reads and writes no file and runs no git: the caller hands
//     it the candidate as a value, the paths of the diff included.
//   - review is a domain package (own-rdd T2): what `labdrian review bind` and `review record` bind and
//     store (the candidate of a review with the digest of its diff, the name of a feature, the names of the
//     stored results and the worktree of a review, the stored file and the role handoff skeleton). It reads
//     no file and runs no git: the application hands it the bytes of the diff and the paths.
//   - review/app holds the use cases of `engine review` (own-rdd T2): Bind and Record, over the Git and
//     Results ports it owns. It answers with typed values and typed refusals, prints nothing and exits
//     nowhere; the words a person reads and the exit codes are the command's, in cmd.
//   - repotest is test support: the hand-made repositories the tests of gitfs and cmd share.
//   - piguard is test support: the `pi` that refuses to run, put first on the PATH of a test
//     run by a TestMain, so nothing started by accident reaches the real CLI.
//   - testhome is test support: the home directory a test binary runs in, which the TestMain of a
//     package that can reach a path derived from HOME enters, so nothing a test forgets to name is
//     found in the machine of whoever runs it.
//   - enginetest is test support: the engine binary built once for a test binary and put in the
//     home where bin/labdrian-overlay looks for it, which the tests that run the script over an
//     overlay with no engine source need (the script asks the engine where each manifest row goes).
//   - overlaytest is test support (H35): the world the tests of the overlay script run it in (where the
//     script and the checkout are, the environment of a run, the fixture repository with the branches the
//     script merges and compares, programs of the test's own, and a run that keeps the two streams and the
//     exit code apart), which the tests of installer and shelltest share.
//   - installer, shelltest, capabilitytest, shaper/shapertest (the documents the shaper's
//     tests share), reviewreceipt/receipttest (the review documents the receipt capture's
//     tests share) and this guard (the module root) are test-only.
var rings = map[string]archguard.Ring{
	".":                         archguard.Support,
	"agentgen":                  archguard.Adapter,
	"assets":                    archguard.Adapter,
	"atomicfile":                archguard.Adapter,
	"capability":                archguard.Domain,
	"capability/presence":       archguard.Adapter,
	"capabilitytest":            archguard.Support,
	"cmd":                       archguard.Root,
	"contract":                  archguard.Domain,
	"enginetest":                archguard.Support,
	"execrunner":                archguard.Adapter,
	"fieldrule":                 archguard.Domain,
	"filelock":                  archguard.Adapter,
	"gate":                      archguard.Domain,
	"gitfs":                     archguard.Adapter,
	"gitobject":                 archguard.Domain,
	"gitprov":                   archguard.Adapter,
	"goal":                      archguard.Domain,
	"hookwire":                  archguard.Adapter,
	"installer":                 archguard.Support,
	"guardmarkers":              archguard.Domain,
	"jsonstrict":                archguard.Domain,
	"memoryscope":               archguard.Domain,
	"overlaytest":               archguard.Support,
	"pathguard":                 archguard.Domain,
	"pathguard/fsresolve":       archguard.Adapter,
	"piguard":                   archguard.Support,
	"pipkg":                     archguard.Adapter,
	"pipkg/gitsource":           archguard.Adapter,
	"projection":                archguard.Domain,
	"projection/app":            archguard.Application,
	"projection/fsstore":        archguard.Adapter,
	"projection/evidencelog":    archguard.Adapter,
	"propagator":                archguard.Domain,
	"propagator/app":            archguard.Application,
	"propagator/fsstore":        archguard.Adapter,
	"repotest":                  archguard.Support,
	"review":                    archguard.Domain,
	"review/app":                archguard.Application,
	"reviewreceipt":             archguard.Domain,
	"reviewreceipt/fsstore":     archguard.Adapter,
	"reviewreceipt/receipttest": archguard.Support,
	"reviewresult":              archguard.Domain,
	"roles":                     archguard.Domain,
	"roles/filechain":           archguard.Adapter,
	"runtime":                   archguard.Adapter,
	"runtime/core":              archguard.Domain,
	"runtime/opencodeprompt":    archguard.Domain,
	"runtime/ownership":         archguard.Domain,
	"runtime/ownership/app":     archguard.Application,
	"runtime/ownership/fsstore": archguard.Adapter,
	"sddregistry":               archguard.Domain,
	"sddregistry/app":           archguard.Application,
	"sddregistry/fsstore":       archguard.Adapter,
	"settings":                  archguard.Domain,
	"settings/settingsfile":     archguard.Adapter,
	"settings/codexhooks":       archguard.Domain,
	"sha256hex":                 archguard.Domain,
	"shaper":                    archguard.Domain,
	"shaper/fsadapter":          archguard.Adapter,
	"shaper/shapertest":         archguard.Support,
	"shelltest":                 archguard.Support,
	"skills":                    archguard.Domain,
	"skills/app":                archguard.Application,
	"skills/engramexport":       archguard.Adapter,
	"skills/projectidentity":    archguard.Adapter,
	"skills/registryyaml":       archguard.Adapter,
	"skills/skillsfs":           archguard.Adapter,
	"statestore":                archguard.Adapter,
	"status":                    archguard.Application,
	"status/fsfiles":            archguard.Adapter,
	"synctrigger":               archguard.Adapter,
	"synctrigger/osproc":        archguard.Adapter,
	"testhome":                  archguard.Support,
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
// It cannot see package-level mutable variables used as test seams; for cmd, the
// package that had them, cmd/process_boundary_test.go reads its source and fails
// on a reassigned package-level variable, and on a use of the process outside main.go.
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
