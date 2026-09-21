package skills

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
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
// Every value-taking flag requires a following non-flag token. A missing value
// or a value that starts with `-` is a usage refusal rather than an
// opportunity to reinterpret the next flag as data; this is what keeps a
// safety flag such as --dry-run from being swallowed. The wrapper's valid
// trailing --manifest and --source-root pairs remain accepted.
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
	i := 0
	consumeValue := func(flag string) (string, bool) {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "error: skills project-register: flag %q requires a value\n", flag)
			exit(1)
			return "", false
		}
		value := args[i+1]
		if strings.HasPrefix(value, "-") {
			fmt.Fprintf(stderr, "error: skills project-register: flag %q requires a value; got flag token %q\n", flag, value)
			exit(1)
			return "", false
		}
		i++
		return value, true
	}

	for ; i < len(args); i++ {
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
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				projectRoot = value
				continue
			case "--candidate":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				candidate = value
				continue
			case "--registry":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				registryPath = value
				continue
			case "--manifest", "--source-root":
				// Wrapper-injected and unused here: consume the value so it
				// is never misread as the draft positional.
				if _, ok := consumeValue(arg); !ok {
					return
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

// RenderProjectReviseCore is the testable CLI core for
// `labdrian skills project-revise --project-root <abs> --candidate <key>
// [--dry-run] <draft-file>`. It reads the existing project lock, derives the
// skill id from the draft frontmatter, proves ownership, and routes the plan
// through the same staged executor as project-register.
func RenderProjectReviseCore(
	args []string,
	readFile readFileFn,
	readDir func(string) ([]fs.DirEntry, error),
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
	i := 0
	consumeValue := func(flag string) (string, bool) {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "error: skills project-revise: flag %q requires a value\n", flag)
			exit(1)
			return "", false
		}
		value := args[i+1]
		if strings.HasPrefix(value, "-") {
			fmt.Fprintf(stderr, "error: skills project-revise: flag %q requires a value; got flag token %q\n", flag, value)
			exit(1)
			return "", false
		}
		i++
		return value, true
	}

	for ; i < len(args); i++ {
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
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				projectRoot = value
				continue
			case "--candidate":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				candidate = value
				continue
			case "--registry":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				registryPath = value
				_ = registryPath
				continue
			case "--manifest", "--source-root":
				if _, ok := consumeValue(arg); !ok {
					return
				}
				continue
			}
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "error: skills project-revise: unknown flag %q\n", arg)
				exit(1)
				return
			}
		}
		if draftPath == "" {
			draftPath = arg
			continue
		}
		fmt.Fprintf(stderr, "error: skills project-revise: unexpected extra argument %q (project-revise accepts exactly one <draft-file>)\n", arg)
		exit(1)
		return
	}

	if projectRoot == "" {
		fmt.Fprintln(stderr, "error: skills project-revise requires --project-root <abs> (there is no working-directory fallback)")
		exit(1)
		return
	}
	if !filepath.IsAbs(projectRoot) {
		fmt.Fprintf(stderr, "error: skills project-revise: --project-root %q must be an absolute path\n", projectRoot)
		exit(1)
		return
	}
	if candidate == "" {
		fmt.Fprintln(stderr, "error: skills project-revise requires --candidate <key>")
		exit(1)
		return
	}
	if draftPath == "" {
		fmt.Fprintln(stderr, "error: skills project-revise requires a <draft-file> argument")
		exit(1)
		return
	}

	draftData, err := readFile(draftPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading draft %q: %v\n", draftPath, err)
		exit(1)
		return
	}
	lockPath := filepath.Join(filepath.Clean(projectRoot), filepath.FromSlash(ProjectLockRelPath))
	lockData, err := readFile(lockPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading project lock %q: %v\n", ProjectLockRelPath, err)
		exit(1)
		return
	}

	plan, err := PlanProjectRevise(ReviseInput{
		ProjectRoot:  projectRoot,
		DraftPath:    draftPath,
		DraftData:    draftData,
		CandidateKey: candidate,
		LockData:     lockData,
		LockExists:   true,
		ReadFile:     readFile,
		ReadDir:      readDir,
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
	if err := ExecuteProjectRevisePlan(plan, fsys, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}
	fmt.Fprintf(stdout, "sha256: %s\n", plan.SHA256)
	fmt.Fprintf(stdout, "revision: %d\n", plan.Revision)
	exit(0)
}

// RenderProjectRetireCore is the testable CLI core for
// `labdrian skills project-retire --project-root <abs> [--dry-run] <id>`.
// It reads the project lock and overlay registry, delegates ownership-gated
// planning/execution to the retirement engine, and never infers retirement
// from a detector result. The command is an explicit operator/agent action.
//
// --reason and --absorbed-into are optional metadata carried into the
// retirement plan. The candidate record procedure validates and persists the
// reason; the engine only needs the values needed to gate AbsorbedInto's
// existence check before it mutates the project tree.
func RenderProjectRetireCore(
	args []string,
	readFile readFileFn,
	readDir func(string) ([]fs.DirEntry, error),
	statFile func(string) (fs.FileInfo, error),
	resolvePath func(string) (string, error),
	fsys projectFS,
	stdout, stderr io.Writer,
	exit func(int),
) {
	projectRoot := ""
	registryPath := "skills.registry.yaml"
	reason := ""
	absorbedInto := ""
	id := ""
	dryRun := false
	endOfOptions := false
	i := 0
	consumeValue := func(flag string) (string, bool) {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "error: skills project-retire: flag %q requires a value\n", flag)
			exit(1)
			return "", false
		}
		value := args[i+1]
		if strings.HasPrefix(value, "-") {
			fmt.Fprintf(stderr, "error: skills project-retire: flag %q requires a value; got flag token %q\n", flag, value)
			exit(1)
			return "", false
		}
		i++
		return value, true
	}

	for ; i < len(args); i++ {
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
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				projectRoot = value
				continue
			case "--registry":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				registryPath = value
				continue
			case "--reason":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				reason = value
				continue
			case "--absorbed-into":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				absorbedInto = value
				continue
			case "--manifest", "--source-root":
				// Wrapper-injected and unused here. Consume their values so a
				// value cannot be mistaken for the skill id.
				if _, ok := consumeValue(arg); !ok {
					return
				}
				continue
			}
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "error: skills project-retire: unknown flag %q\n", arg)
				exit(1)
				return
			}
		}
		if id == "" {
			id = arg
			continue
		}
		fmt.Fprintf(stderr, "error: skills project-retire: unexpected extra argument %q (project-retire accepts exactly one <id>)\n", arg)
		exit(1)
		return
	}

	if projectRoot == "" {
		fmt.Fprintln(stderr, "error: skills project-retire requires --project-root <abs> (there is no working-directory fallback)")
		exit(1)
		return
	}
	if !filepath.IsAbs(projectRoot) {
		fmt.Fprintf(stderr, "error: skills project-retire: --project-root %q must be an absolute path\n", projectRoot)
		exit(1)
		return
	}
	if id == "" {
		fmt.Fprintln(stderr, "error: skills project-retire requires a <id> argument")
		exit(1)
		return
	}

	registryData, err := readFile(registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading registry %q: %v\n", registryPath, err)
		exit(1)
		return
	}
	registry, err := ParseRegistry(bytes.NewReader(registryData))
	if err != nil {
		fmt.Fprintf(stderr, "error: parsing registry %q: %v\n", registryPath, err)
		exit(1)
		return
	}

	root := filepath.Clean(projectRoot)
	lockPath := filepath.Join(root, filepath.FromSlash(ProjectLockRelPath))
	lockData, err := readFile(lockPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading project lock %q: %v\n", ProjectLockRelPath, err)
		exit(1)
		return
	}
	plan, err := PlanProjectRetire(RetireInput{
		ProjectRoot:  root,
		ID:           id,
		Reason:       reason,
		AbsorbedInto: absorbedInto,
		Registry:     registry,
		LockData:     lockData,
		LockExists:   true,
		ReadFile:     readFile,
		ReadDir:      readDir,
		Stat:         statFile,
		ResolvePath:  resolvePath,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	if dryRun {
		for _, write := range projectRetireCommitOrder(plan) {
			fmt.Fprintf(stdout, "plan: %s\n", write.Rel)
		}
		exit(0)
		return
	}
	if err := ExecuteProjectRetirePlan(plan, fsys, stdout, stderr); err != nil {
		// This includes ErrRollbackIncomplete. The executor has already
		// printed repo-relative recovery pointers for that sentinel; every
		// non-nil execution result is an exit-1 refusal/failure.
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}
	exit(0)
}

func projectRetireCommitOrder(plan ProjectPlan) []ProjectWrite {
	order := append([]ProjectWrite(nil), plan.DeleteWrites...)
	return append(order, plan.Lock)
}

// RenderProjectStatusCore is the testable CLI core for
// `labdrian skills project-status --project-root <abs> [<id>]`. It reports
// ownership for one requested lock entry or every entry when no id is given,
// and reports global supersession using both the project id and the final
// candidate-key slug.
func RenderProjectStatusCore(
	args []string,
	readFile readFileFn,
	readDir func(string) ([]fs.DirEntry, error),
	resolvePath func(string) (string, error),
	stdout, stderr io.Writer,
	exit func(int),
) {
	projectRoot := ""
	registryPath := "skills.registry.yaml"
	id := ""
	endOfOptions := false
	i := 0
	consumeValue := func(flag string) (string, bool) {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "error: skills project-status: flag %q requires a value\n", flag)
			exit(1)
			return "", false
		}
		value := args[i+1]
		if strings.HasPrefix(value, "-") {
			fmt.Fprintf(stderr, "error: skills project-status: flag %q requires a value; got flag token %q\n", flag, value)
			exit(1)
			return "", false
		}
		i++
		return value, true
	}
	for ; i < len(args); i++ {
		arg := args[i]
		if !endOfOptions {
			switch arg {
			case "--":
				endOfOptions = true
				continue
			case "--project-root":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				projectRoot = value
				continue
			case "--registry":
				value, ok := consumeValue(arg)
				if !ok {
					return
				}
				registryPath = value
				continue
			case "--manifest", "--source-root":
				if _, ok := consumeValue(arg); !ok {
					return
				}
				continue
			}
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "error: skills project-status: unknown flag %q\n", arg)
				exit(1)
				return
			}
		}
		if id == "" {
			id = arg
			continue
		}
		fmt.Fprintf(stderr, "error: skills project-status: unexpected extra argument %q\n", arg)
		exit(1)
		return
	}
	if projectRoot == "" {
		fmt.Fprintln(stderr, "error: skills project-status requires --project-root <abs> (there is no working-directory fallback)")
		exit(1)
		return
	}
	if !filepath.IsAbs(projectRoot) {
		fmt.Fprintf(stderr, "error: skills project-status: --project-root %q must be an absolute path\n", projectRoot)
		exit(1)
		return
	}

	root := filepath.Clean(projectRoot)
	registryData, err := readFile(registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading registry %q: %v\n", registryPath, err)
		exit(1)
		return
	}
	registry, err := ParseRegistry(bytes.NewReader(registryData))
	if err != nil {
		fmt.Fprintf(stderr, "error: parsing registry %q: %v\n", registryPath, err)
		exit(1)
		return
	}

	lockData, err := readFile(filepath.Join(root, filepath.FromSlash(ProjectLockRelPath)))
	if err != nil {
		fmt.Fprintf(stderr, "error: reading project lock %q: %v\n", ProjectLockRelPath, err)
		exit(1)
		return
	}
	lock, err := ParseProjectLock(lockData)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	entries := lock.Skills
	if id != "" {
		entries = nil
		for _, entry := range lock.Skills {
			if entry.ID == id {
				entries = []ProjectLockEntry{entry}
				break
			}
		}
		if len(entries) == 0 {
			fmt.Fprintf(stderr, "error: project-status: skill %q is not in the project lock\n", id)
			exit(1)
			return
		}
	}

	for _, entry := range entries {
		ownership := EvaluateOwnership(root, entry, readFile, readDir, resolvePath)
		supersededBy := projectStatusSupersededBy(registry, entry)
		if supersededBy == "" {
			supersededBy = "-"
		}
		if ownership.AgentOwned {
			fmt.Fprintf(stdout, "%s rev:%d owner:agent superseded-by:%s\n", entry.ID, entry.Revision, supersededBy)
			continue
		}
		fmt.Fprintf(stdout, "%s rev:%d owner:human (%s) superseded-by:%s\n", entry.ID, entry.Revision, ownership.Reason, supersededBy)
	}
	exit(0)
}

// projectStatusSupersededBy reports the first global registry path matching
// the project entry's id or the last slug of its candidate key. MatchCandidate
// is an existence/identity lookup here: it does not claim that the global
// skill covers the project skill's content.
func projectStatusSupersededBy(registry Registry, entry ProjectLockEntry) string {
	if matched, skillPath := MatchCandidate(registry, entry.ID); matched {
		return skillPath
	}
	candidateSlug := path.Base(entry.Candidate)
	if matched, skillPath := MatchCandidate(registry, candidateSlug); matched {
		return skillPath
	}
	return ""
}
