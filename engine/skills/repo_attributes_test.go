package skills

// The repository's own .gitattributes, pinned by what git does with it. An
// approval record binds the SHA-256 of the exact bytes of a SKILL.md, so a
// checkout or a commit that rewrote a skill's line endings would make the skill
// stale (APPROVAL_STALE) without anyone having edited it. Nothing here touches the
// real repository: the real .gitattributes is copied into a throwaway repository.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitIn runs git in dir with the user's and the system's configuration ignored,
// plus the extra -c settings, and returns its standard output.
func gitIn(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, stderr.String())
	}
	return out
}

// With the line-ending conversion a Windows checkout turns on (core.autocrlf), a
// file under skills/ is committed byte for byte, while any other text file is
// still converted. The second half is the control: it proves the throwaway
// repository really converts, so the first half cannot pass for the wrong reason.
func TestSkillFilesKeepTheirExactBytesUnderLineEndingConversion(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	attributes, err := os.ReadFile(filepath.Join("..", "..", ".gitattributes"))
	if err != nil {
		t.Fatalf("read .gitattributes: %v", err)
	}

	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), attributes, 0o644); err != nil {
		t.Fatal(err)
	}
	crlf := []byte("---\r\nname: demo\r\n---\r\nBody.\r\n")
	for _, rel := range []string{"skills/demo/SKILL.md", "docs/note.md"} {
		path := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, crlf, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, repo, "-c", "core.autocrlf=true", "-c", "core.safecrlf=false", "add", "-A")

	if got := gitIn(t, repo, "cat-file", "blob", ":skills/demo/SKILL.md"); !bytes.Equal(got, crlf) {
		t.Errorf("skills/demo/SKILL.md was rewritten on add: got %q, want %q", got, crlf)
	}
	control := gitIn(t, repo, "cat-file", "blob", ":docs/note.md")
	if bytes.Equal(control, crlf) {
		t.Fatal("control file docs/note.md kept its CRLF: the throwaway repository does not convert, so the check above proves nothing")
	}
}
