package skills

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// CopyOp is a single file-tree copy directive: copy the Src/ tree to Dst/.
type CopyOp struct {
	SkillID string // entry id, used for output messages
	Src     string // <sourceRoot>/<entry.Path>
	Dst     string // <targetRoot>/.claude/skills/<entry.ID>
}

// PlanInstall filters reg for project-scoped skills allowed for projectID,
// building one CopyOp per admitted entry. Pure: no filesystem access.
// Declaration order from reg.Skills is preserved in the returned slice.
func PlanInstall(reg Registry, projectID, sourceRoot, targetRoot string) ([]CopyOp, error) {
	var ops []CopyOp
	for _, e := range reg.Skills {
		if e.Install.DefaultScope != "project" {
			continue
		}
		if !containsString(e.Install.AllowedProjects, projectID) {
			continue
		}
		ops = append(ops, CopyOp{
			SkillID: e.ID,
			Src:     filepath.Join(sourceRoot, e.Path),
			Dst:     filepath.Join(targetRoot, ".claude", "skills", e.ID),
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

// ExecuteInstall performs the filesystem side of an install plan.
// It first verifies all Src directories exist (fail-loud, full scan), then
// recursively copies each Src tree into its Dst, removing Dst first (clean
// overwrite). Prints "installed: <id>\n" to stdout per skill.
func ExecuteInstall(plan []CopyOp, stdout, stderr io.Writer) error {
	// First pass: verify all sources exist (R-053 full-scan fail-loud).
	var missing []CopyOp
	for _, op := range plan {
		info, err := os.Stat(op.Src)
		if err != nil || !info.IsDir() {
			fmt.Fprintf(stderr, "error: skill %s: source dir not found: %s\n", op.SkillID, op.Src)
			missing = append(missing, op)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%d source director(ies) missing", len(missing))
	}

	// Second pass: copy each op.
	for _, op := range plan {
		if err := os.RemoveAll(op.Dst); err != nil {
			fmt.Fprintf(stderr, "error: skill %s: removing dst %s: %v\n", op.SkillID, op.Dst, err)
			return err
		}
		if err := copyTree(op.Src, op.Dst); err != nil {
			fmt.Fprintf(stderr, "error: skill %s: copying to %s: %v\n", op.SkillID, op.Dst, err)
			return err
		}
		fmt.Fprintf(stdout, "installed: %s\n", op.SkillID)
	}
	return nil
}

// copyTree recursively copies the directory tree rooted at src into dst,
// preserving file mode bits. dst is created if absent.
// Symlinks are not followed (plain files and directories only).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip symlinks.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			info, ierr := d.Info()
			if ierr != nil {
				return ierr
			}
			return os.MkdirAll(target, info.Mode())
		}

		return copyFile(path, target, d)
	})
}

// copyFile copies a single file from src to dst preserving its mode bits.
func copyFile(src, dst string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	return out.Close()
}

// RenderInstallCore is the testable CLI entry for `engine skills install`.
// cwdFn is injected for testability (production callers pass os.Getwd).
func RenderInstallCore(args []string, readFile readFileFn, cwdFn func() (string, error), stdout, stderr io.Writer, exit func(int)) {
	registryPath := "skills.registry.yaml"
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
	cwd, err := cwdFn()
	if err != nil {
		fmt.Fprintf(stderr, "error: resolving project identity: %v\n", err)
		exit(1)
		return
	}
	targetRoot := cwd

	if projectID == "" {
		projectID = filepath.Base(cwd)
	}

	// Read and parse registry.
	data, err := readFile(registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading registry %q: %v\n", registryPath, err)
		exit(1)
		return
	}
	reg, err := ParseRegistry(bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(stderr, "error: parsing registry: %v\n", err)
		exit(1)
		return
	}

	// Plan.
	plan, err := PlanInstall(reg, projectID, sourceRoot, targetRoot)
	if err != nil {
		fmt.Fprintf(stderr, "error: planning install: %v\n", err)
		exit(1)
		return
	}

	if len(plan) == 0 {
		fmt.Fprintf(stdout, "no project-scoped skills admitted for project %q\n", projectID)
		exit(0)
		return
	}

	// Execute.
	if err := ExecuteInstall(plan, stdout, stderr); err != nil {
		exit(1)
		return
	}
	exit(0)
}
