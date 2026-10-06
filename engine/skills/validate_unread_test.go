package skills

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// Decision 3 of the owner (2026-10-05): 'validate' is the verb a CI uses to detect drift, so it
// does not pass a registry the reader did not read whole. Every other verb that only reads goes on
// with a warning.

// runValidateOverUnreadRegistry runs validate over the aligned fixture with a key the reader does
// not know added at its root, and the manifest and the disk the given rows say.
func runValidateOverUnreadRegistry(t *testing.T, manifest string, onDisk []string) (code int, stdout, stderr string) {
	t.Helper()
	regPath, mfPath := mustWriteValidateFixture(t, manifest)
	data, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(regPath, append([]byte("extra: 1\n"), data...), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	RenderValidateCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", "unused"},
		os.ReadFile, testRegistries(os.ReadFile), stubScan(onDisk), &out, &errBuf,
		func(c int) { code = c },
	)
	return code, out.String(), errBuf.String()
}

func TestValidateFailsOnARegistryTheReaderLeftFieldsOutOf(t *testing.T) {
	code, stdout, stderr := runValidateOverUnreadRegistry(t, "sdd-spec/SKILL.md managed\n", []string{"sdd-spec/SKILL.md"})
	if code != 1 {
		t.Errorf("exit code = %d, want 1 for a registry with a key the reader does not know; stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "aligned") {
		t.Errorf("stdout %q says the overlay is aligned, want nothing said of agreement", stdout)
	}
	for _, want := range []string{
		`warning: registry fields left unread: line 1: unknown top-level key "extra"`,
		"error: skills: the registry has fields this program does not read, and validate cannot vouch for a registry it read in part: line 1: unknown top-level key \"extra\"",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr %q should contain %q", stderr, want)
		}
	}
}

// The reason comes after every divergence the run found: a run that fails for the unread fields
// still tells the divergences in the same run.
func TestValidateTellsEveryDivergenceAndThenWhyItFailsOnUnreadFields(t *testing.T) {
	code, _, stderr := runValidateOverUnreadRegistry(t, "sdd-spec/SKILL.md managed\n", []string{"sdd-spec/SKILL.md", "orphan/notes.md"})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	divergence := strings.Index(stderr, "UNREGISTERED_ON_DISK")
	reason := strings.Index(stderr, "error: skills: the registry has fields")
	if divergence < 0 || reason < 0 || divergence > reason {
		t.Errorf("stderr %q should name the divergence, and then the reason", stderr)
	}
	if got := strings.Count(stderr, "\n"); got != 3 {
		t.Errorf("stderr has %d lines, want the warning, the divergence and the reason: %q", got, stderr)
	}
}

// Everything else that only reads a registry keeps its policy: it warns and goes on. (list is one
// of them; the golden files hold the others.)
func TestARegistryReadWholeIsStillValidatedWithExitZero(t *testing.T) {
	regPath, mfPath := mustWriteValidateFixture(t, "sdd-spec/SKILL.md managed\n")
	var out, errBuf bytes.Buffer
	code := 0
	RenderValidateCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", "unused"},
		os.ReadFile, testRegistries(os.ReadFile), stubScan([]string{"sdd-spec/SKILL.md"}), &out, &errBuf,
		func(c int) { code = c },
	)
	if code != 0 || errBuf.Len() != 0 {
		t.Errorf("exit %d, stderr %q, want 0 and nothing said", code, errBuf.String())
	}
}

// --- the three questions the domain asks of a registry that was read in part ---

// The rule that a registry the reader did not read whole is not used for what needs all of it has
// one owner and one skeleton: each question says what it would cost to go on.
func TestTheChecksOfARegistryReadInPartShareOneSkeleton(t *testing.T) {
	whole := unreadRegistry()
	reg := unreadRegistry(`line 3: unknown key "color" in skill entry`, "line 9: unknown key \"mirror\" in source")
	const left = `line 3: unknown key "color" in skill entry (and 1 more)`
	for name, tc := range map[string]struct {
		check func(Registry) error
		want  string
	}{
		"writing":   {Registry.CheckWritable, `skills: the registry has fields this program does not read, and rewriting it would drop them: ` + left},
		"building":  {Registry.CheckBuildable, `skills: the registry has fields this program does not read, and a package built from it would be built from a partial read: ` + left},
		"verifying": {Registry.CheckVerifiable, `skills: the registry has fields this program does not read, and validate cannot vouch for a registry it read in part: ` + left},
	} {
		t.Run(name, func(t *testing.T) {
			if err := tc.check(whole); err != nil {
				t.Errorf("the check of a registry read whole = %v, want nil", err)
			}
			if err := tc.check(reg); err == nil || err.Error() != tc.want {
				t.Errorf("the check = %v, want %q", err, tc.want)
			}
		})
	}
}
