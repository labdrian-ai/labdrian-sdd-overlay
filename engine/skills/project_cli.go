package skills

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// RenderProjectRegisterCore is the testable CLI core for
//
//	labdrian skills project-register --project-root <abs> --candidate <key> [--dry-run] <draft-file>
//
// It is the one CLI entry point into the project-tier registry. It reads
// nothing but the three inputs the planner needs (the draft, the overlay
// registry, and the project's lock file when one exists), hands them to the
// pure planner PlanProjectRegister, and then either prints the plan
// (--dry-run) or executes it through ExecuteProjectPlan.
//
// Output (design.md, "Output lines"), all on stdout, all paths repo-relative
// with forward slashes so each one is usable directly as a git pathspec:
//
//   - --dry-run: one `plan: <rel>` line per planned path, in commit order
//     (the SKILL.md targets, then the lock LAST), and nothing is written.
//   - a real run: one `wrote: <rel>` line per path (printed by
//     ExecuteProjectPlan once the whole set is committed), then
//     `sha256: <hex>`, `revision: <n>`, and the Pi trust `note:` line.
//
// The `note:` line is printed on a SUCCESSFUL write only — never on a
// refusal and never under --dry-run — because it discloses a consequence
// (Pi prompting once for project trust) that only an actual write to
// `.agents/skills` creates. The agent procedure relays it to the user and
// never uses it as a pathspec.
//
// Every refusal exits 1 with the reason on stderr and NOTHING on stdout: the
// agent feeds stdout straight to `git add --`, so a path printed beside a
// refusal would be a pathspec for a file that does not exist.
//
// --project-root has no default and no cwd fallback (the R-002 precedent
// from RenderValidateCore), and a relative root is refused here, before any
// path is joined against it, so no read and no write can be aimed at a
// cwd-derived location.
//
// The `labdrian` wrapper always appends `--registry <path> --manifest <path>
// --source-root <path>` after the verb's own arguments. --manifest and
// --source-root are consumed and ignored; --registry's VALUE is read,
// because MatchCandidate runs against it (design.md, "Identity"). An
// unreadable or unparseable registry is a fail-closed refusal: registering
// while the identity check cannot run is exactly the shadowing that check
// exists to prevent.
//
// Argument discipline matches RenderLintCore's: an unrecognized dash-argument
// and a second positional are both usage errors naming the offending token,
// never silently dropped (review-b75e4a27b9494ff8 R4-001). A mistyped
// `--dryrun` that was dropped instead would register for real while the
// operator believed they had asked for a plan. `--` ends the options, so a
// draft path that legitimately begins with a dash can still be named.
//
// fsys is an unexported interface on purpose: production callers reach this
// through SkillsCore, and tests inject osProjectFS{} over t.TempDir().
func RenderProjectRegisterCore(
	args []string,
	readFile readFileFn,
	statFile func(string) (fs.FileInfo, error),
	resolvePath func(string) (string, error),
	fsys projectFS,
	stdout, stderr io.Writer,
	exit func(int),
) {
	projectRoot := ""
	candidate := ""
	registryPath := "skills.registry.yaml"
	draftPath := ""
	dryRun := false
	endOfOptions := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !endOfOptions {
			switch arg {
			case "--":
				endOfOptions = true
				continue
			case "--dry-run":
				dryRun = true
				continue
			case "--project-root":
				if i+1 < len(args) {
					projectRoot = args[i+1]
					i++
				}
				continue
			case "--candidate":
				if i+1 < len(args) {
					candidate = args[i+1]
					i++
				}
				continue
			case "--registry":
				if i+1 < len(args) {
					registryPath = args[i+1]
					i++
				}
				continue
			case "--manifest", "--source-root":
				// Wrapper-injected and unused here: consume the value so it
				// is never misread as the draft positional.
				if i+1 < len(args) {
					i++
				}
				continue
			}
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "error: skills project-register: unknown flag %q\n", arg)
				exit(1)
				return
			}
		}
		if draftPath == "" {
			draftPath = arg
			continue
		}
		fmt.Fprintf(stderr, "error: skills project-register: unexpected extra argument %q (project-register accepts exactly one <draft-file>)\n", arg)
		exit(1)
		return
	}

	if projectRoot == "" {
		fmt.Fprintln(stderr, "error: skills project-register requires --project-root <abs> (there is no working-directory fallback)")
		exit(1)
		return
	}
	if !filepath.IsAbs(projectRoot) {
		fmt.Fprintf(stderr, "error: skills project-register: --project-root %q must be an absolute path\n", projectRoot)
		exit(1)
		return
	}
	if candidate == "" {
		fmt.Fprintln(stderr, "error: skills project-register requires --candidate <key>")
		exit(1)
		return
	}
	if draftPath == "" {
		fmt.Fprintln(stderr, "error: skills project-register requires a <draft-file> argument")
		exit(1)
		return
	}

	registryData, err := readFile(registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading registry %q: %v\n", registryPath, err)
		exit(1)
		return
	}
	reg, err := ParseRegistry(bytes.NewReader(registryData))
	if err != nil {
		fmt.Fprintf(stderr, "error: parsing registry %q: %v\n", registryPath, err)
		exit(1)
		return
	}

	draftData, err := readFile(draftPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading draft %q: %v\n", draftPath, err)
		exit(1)
		return
	}

	// The lock file is optional: the first registration in a project creates
	// it. Any read error OTHER than "not there" is a refusal — a lock that
	// exists but cannot be read must never be silently replaced by an empty
	// one, which would drop every skill already registered.
	lockPath := filepath.Join(filepath.Clean(projectRoot), filepath.FromSlash(ProjectLockRelPath))
	lockData, err := readFile(lockPath)
	lockExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(stderr, "error: reading project lock %q: %v\n", ProjectLockRelPath, err)
		exit(1)
		return
	}

	plan, err := PlanProjectRegister(RegisterInput{
		ProjectRoot:  projectRoot,
		DraftPath:    draftPath,
		DraftData:    draftData,
		CandidateKey: candidate,
		Registry:     reg,
		LockData:     lockData,
		LockExists:   lockExists,
		Stat:         statFile,
		ResolvePath:  resolvePath,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	if dryRun {
		for _, w := range projectCommitOrder(plan) {
			fmt.Fprintf(stdout, "plan: %s\n", w.Rel)
		}
		exit(0)
		return
	}

	// ExecuteProjectPlan prints the `wrote:` lines itself, and only once the
	// whole set is committed, so a line a rollback would erase is never
	// emitted.
	if err := ExecuteProjectPlan(plan, fsys, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	fmt.Fprintf(stdout, "sha256: %s\n", plan.SHA256)
	fmt.Fprintf(stdout, "revision: %d\n", plan.Revision)
	fmt.Fprintln(stdout, PiTrustNote)
	exit(0)
}
