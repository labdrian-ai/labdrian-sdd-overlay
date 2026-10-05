package skills

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// CopyOp is one skill to install: where its source tree is and where the first
// runtime's copy of it goes. The second runtime's copy (.agents/skills/<id>) is the
// same tree in the projectTargets table; PlanInstallOwnership derives both.
type CopyOp struct {
	SkillID string // entry id, used for output messages
	Src     string // <sourceRoot>/<entry.Path>
	Dst     string // <targetRoot>/.claude/skills/<entry.ID>
}

// PlanInstall filters reg for project-scoped skills allowed for projectID,
// building one CopyOp per admitted entry. Pure: no filesystem access.
// Declaration order from reg.Skills is preserved in the returned slice.
// Returns a non-nil error if any entry's id or path contains a traversal
// sequence that would place Dst outside <targetRoot>/.claude/skills/ or
// Src outside sourceRoot (R-055).
func PlanInstall(reg Registry, projectID, sourceRoot, targetRoot string) ([]CopyOp, error) {
	// Pre-compute clean containment roots for traversal checks. withinRoot
	// (pathguard.go) requires already-cleaned arguments.
	srcRoot := filepath.Clean(sourceRoot)
	dstRoot := filepath.Clean(filepath.Join(targetRoot, ".claude", "skills"))

	var ops []CopyOp
	for _, e := range reg.Skills {
		if e.Install.DefaultScope != "project" {
			continue
		}
		if !containsString(e.Install.AllowedProjects, projectID) {
			continue
		}

		src := filepath.Clean(filepath.Join(sourceRoot, e.Path))
		dst := filepath.Clean(filepath.Join(targetRoot, ".claude", "skills", e.ID))

		// R-055: reject traversal in both src and dst (fail-loud, pure).
		// The containment test itself is withinRoot (pathguard.go), shared
		// with resolveTarget and EvaluateOwnership; the two branches keep one
		// message each so a test can prove which one it reached
		// (review-d89971d41a526146 precedent).
		if !withinRoot(srcRoot, src) {
			return nil, fmt.Errorf("skill %q: path %q escapes source root — possible traversal", e.ID, e.Path)
		}
		if !withinRoot(dstRoot, dst) {
			return nil, fmt.Errorf("skill %q: id %q escapes target skills root — possible traversal", e.ID, e.ID)
		}

		ops = append(ops, CopyOp{
			SkillID: e.ID,
			Src:     src,
			Dst:     dst,
		})
	}
	return ops, nil
}

// containsString reports whether slice contains s (case-sensitive).
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// isWriterTempFile reports whether d is a regular file that writeFileAtomic made
// and has not yet renamed into place: the temporary-file prefix followed by its
// unique suffix. It is half of a write another verb is doing in the source tree,
// not skill content. install also holds the overlay lock, which keeps those verbs
// out while it reads the tree; this is the second defence, for a writer that did not
// take it. The prefix alone, with no suffix, is not a name writeFileAtomic makes, and
// a directory with such a name is walked like any other.
func isWriterTempFile(d fs.DirEntry) bool {
	name := d.Name()
	return !d.IsDir() && len(name) > len(atomicTempPrefix) && name[:len(atomicTempPrefix)] == atomicTempPrefix
}

// installEnv is everything install and adopt touch outside their own arguments, so
// that a test can replace any of it. Production wires the real filesystem.
type installEnv struct {
	registries  RegistryRepository                // the registry
	readProject readFileFn                        // the project lock, and anything else in the project
	cwd         func() (string, error)            // the directory to install into
	stat        func(string) (fs.FileInfo, error) // the project's paths
	readDir     func(string) ([]fs.DirEntry, error)
	resolve     func(string) (string, error)       // symlink resolution, for containment
	fsys        projectFS                          // the writes
	readSource  func(string) ([]SourceFile, error) // a skill's source tree
}

// productionInstallEnv is the real filesystem. Registry reads go through readRegistry,
// so the registry can be injected; the project is always read from disk.
func productionInstallEnv(registries RegistryRepository, cwd func() (string, error)) installEnv {
	return installEnv{
		registries:  registries,
		readProject: os.ReadFile,
		cwd:         cwd,
		stat:        os.Stat,
		readDir:     os.ReadDir,
		resolve:     resolvePathKeepingMissing,
		fsys:        osProjectFS{},
		readSource:  readSkillSource,
	}
}

// RenderInstallCore is the testable CLI entry for `engine skills install`.
// cwdFn is injected for testability (production callers pass os.Getwd).
func RenderInstallCore(args []string, registries RegistryRepository, cwdFn func() (string, error), stdout, stderr io.Writer, exit func(int)) {
	renderInstall(productionInstallEnv(registries, cwdFn), args, stdout, stderr, exit)
}

// installContext is what install and adopt have read and decided before either
// looks at the project: who the project is, which skills are admitted, their source
// files, and the project lock as it is.
type installContext struct {
	projectID  string
	root       string
	skills     []InstallSkill
	lockData   []byte
	lockExists bool
}

// prepareInstall parses the arguments the two verbs share (--registry,
// --source-root, --project-id), resolves the project, and reads the registry, the
// source trees of the admitted skills, and the project lock. It reports false, having
// already printed and exited, when the verb has nothing more to do: a failure, or no
// skill admitted for the project.
func prepareInstall(verb string, env installEnv, args []string, stdout, stderr io.Writer, exit func(int)) (installContext, bool) {
	registryPath := defaultRegistryPath
	sourceRoot := ""
	projectID := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--registry":
			if i+1 < len(args) {
				registryPath = args[i+1]
				i++
			}
		case "--source-root":
			if i+1 < len(args) {
				sourceRoot = args[i+1]
				i++
			}
		case "--project-id":
			if i+1 < len(args) {
				projectID = args[i+1]
				i++
			}
		}
	}

	// Resolve cwd (targetRoot + basename-derived projectID).
	cwd, err := env.cwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: skills %s: resolving project identity: %v\n", verb, err)
		exit(1)
		return installContext{}, false
	}
	targetRoot := filepath.Clean(cwd)

	if projectID == "" {
		projectID = filepath.Base(cwd)
	}

	// Read the registry.
	reg, ok := readRegistryForVerb(env.registries, registryPath, false, stderr, exit)
	if !ok {
		return installContext{}, false
	}

	// Plan.
	plan, err := PlanInstall(reg, projectID, sourceRoot, targetRoot)
	if err != nil {
		fmt.Fprintf(stderr, "error: planning %s: %v\n", verb, err)
		exit(1)
		return installContext{}, false
	}

	if len(plan) == 0 {
		fmt.Fprintf(stdout, "no project-scoped skills admitted for project %q\n", projectID)
		exit(0)
		return installContext{}, false
	}

	// Verify all sources exist (R-053 full-scan fail-loud), then read them.
	var missing []CopyOp
	for _, op := range plan {
		info, err := env.stat(op.Src)
		if err != nil || !info.IsDir() {
			fmt.Fprintf(stderr, "error: skill %s: source dir not found: %s\n", op.SkillID, op.Src)
			missing = append(missing, op)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(stderr, "error: %d source director(ies) missing\n", len(missing))
		exit(1)
		return installContext{}, false
	}
	skills := make([]InstallSkill, 0, len(plan))
	for _, op := range plan {
		files, err := env.readSource(op.Src)
		if err != nil {
			fmt.Fprintf(stderr, "error: skill %s: reading its source %s: %v\n", op.SkillID, op.Src, err)
			exit(1)
			return installContext{}, false
		}
		skills = append(skills, InstallSkill{ID: op.SkillID, Files: files})
	}

	// The lock is optional: the first install in a project creates it. A lock that
	// exists and cannot be read must never be replaced by an empty one, which would
	// disown everything recorded in it.
	lockData, err := env.readProject(filepath.Join(targetRoot, filepath.FromSlash(ProjectLockRelPath)))
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(stderr, "error: reading project lock %q: %v\n", ProjectLockRelPath, err)
		exit(1)
		return installContext{}, false
	}
	return installContext{projectID: projectID, root: targetRoot, skills: skills, lockData: lockData, lockExists: err == nil}, true
}

func (c installContext) input(env installEnv) InstallInput {
	return InstallInput{
		ProjectRoot: c.root,
		ProjectID:   c.projectID,
		Skills:      c.skills,
		LockData:    c.lockData,
		LockExists:  c.lockExists,
		ReadFile:    env.readProject,
		Stat:        env.stat,
		ResolvePath: env.resolve,
		ReadDir:     env.readDir,
	}
}

func renderInstall(env installEnv, args []string, stdout, stderr io.Writer, exit func(int)) {
	renderPlanned("install", "installed", PlanInstallOwnership, env, args, stdout, stderr, exit)
}

// renderAdopt is `skills adopt`: the same arguments as install, and the same
// ownership rules, used to record what is already there instead of writing it.
func renderAdopt(env installEnv, args []string, stdout, stderr io.Writer, exit func(int)) {
	renderPlanned("adopt", "adopted", PlanAdopt, env, args, stdout, stderr, exit)
}

// renderPlanned runs a verb that is a plan followed by its execution: a refusal
// prints every reason and writes nothing; otherwise the plan is executed all or
// nothing and each skill's outcome is printed, then the notes.
func renderPlanned(verb, did string, plan func(InstallInput) (InstallPlan, []string), env installEnv, args []string, stdout, stderr io.Writer, exit func(int)) {
	ctx, ok := prepareInstall(verb, env, args, stdout, stderr, exit)
	if !ok {
		return
	}

	in := ctx.input(env)
	in.Verb = verb
	p, refusals := plan(in)
	if len(refusals) > 0 {
		for _, r := range refusals {
			fmt.Fprintf(stderr, "error: %s\n", r)
		}
		fmt.Fprintf(stderr, "error: skills %s: refused, so nothing was %s\n", verb, did)
		exit(1)
		return
	}
	if err := ExecuteInstallPlan(p, ctx.root, env.fsys, stderr); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}
	for _, o := range p.Skills {
		fmt.Fprintf(stdout, "%s: %s\n", o.Status, o.ID)
	}
	for _, n := range p.Notes {
		fmt.Fprintf(stdout, "note: %s\n", n)
	}
	exit(0)
}
