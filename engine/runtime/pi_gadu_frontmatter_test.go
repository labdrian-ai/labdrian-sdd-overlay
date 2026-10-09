package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installWithGadu installs the Pi package whose agents/GADU.md is content, under a home of its
// own, and returns the message of the Install and whether GADU.md was linked.
func installWithGadu(t *testing.T, content string) (message string, linked bool) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	overlayRoot, registryPath := piFixtureOverlay(t)
	mustWrite(t, filepath.Join(overlayRoot, "agents", "GADU.md"), content)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	result := newPiAdapterAt(t, overlayRoot, registryPath, destDir).Install()
	_, err = os.Lstat(filepath.Join(home, ".pi", "agent", "agents", "GADU.md"))
	return result.Message, err == nil
}

// What Pi needs of the agent file, and the words of the refusal: the cases the link has always
// answered the same way.
func TestInstallChecksTheGaduFrontmatter(t *testing.T) {
	cases := map[string]struct {
		content string
		want    string // a substring of the message; empty means GADU.md is linked
	}{
		"name and description":         {"---\nname: GADU\ndescription: d\n---\nbody\n", ""},
		"an inline tools scalar":       {"---\nname: GADU\ndescription: d\ntools: '*'\n---\nbody\n", ""},
		"a tools list":                 {"---\nname: GADU\ndescription: d\ntools:\n  - Read\n  - Write\n---\nbody\n", ""},
		"an unindented tools list":     {"---\nname: GADU\ndescription: d\ntools:\n- Read\n---\nbody\n", ""},
		"carriage returns":             {"---\r\nname: GADU\r\ndescription: d\r\ntools: '*'\r\n---\r\nbody\r\n", ""},
		"a quoted empty description":   {"---\nname: GADU\ndescription: \"\"\n---\nbody\n", ""},
		"no opening fence":             {"name: GADU\ndescription: d\n---\n", "gadu frontmatter: missing opening --- delimiter"},
		"no closing fence":             {"---\nname: GADU\ndescription: d\n", "gadu frontmatter: missing closing --- delimiter"},
		"no name":                      {"---\ndescription: d\n---\n", "gadu frontmatter: missing required 'name'"},
		"an empty name":                {"---\nname:\ndescription: d\n---\n", "gadu frontmatter: missing required 'name'"},
		"no description":               {"---\nname: GADU\n---\n", "gadu frontmatter: missing required 'description'"},
		"tools as a scalar and a list": {"---\nname: GADU\ndescription: d\ntools: '*'\ntools:\n  - Read\n---\nbody\n", "'tools' declared both as an inline scalar and a YAML list"},
		"a list of another key":        {"---\nname: GADU\ndescription: d\ntools: '*'\nskills:\n  - a\n---\nbody\n", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			message, linked := installWithGadu(t, c.content)
			if c.want == "" {
				if strings.Contains(message, "gadu frontmatter") || !linked {
					t.Fatalf("a valid agent file was refused: linked=%v message=%q", linked, message)
				}
				return
			}
			if !strings.Contains(message, c.want) || linked {
				t.Fatalf("message=%q linked=%v, want a refusal containing %q and no link", message, linked, c.want)
			}
		})
	}
}

// The agent file is read with the reader the skills lint uses (skills.ReadFrontmatter). Three
// inputs the link used to read differently:
func TestInstallReadsTheGaduFrontmatterTheWayTheLintDoes(t *testing.T) {
	cases := map[string]struct {
		content string
		want    string // empty means GADU.md is linked
	}{
		"a byte-order mark is tolerated (it was refused as no opening fence)": {"\xEF\xBB\xBF---\nname: GADU\ndescription: d\n---\nbody\n", ""},
		"a key under another key is not the description (it was read as one)": {"---\nname: GADU\nmetadata:\n  description: d\n---\nbody\n", "gadu frontmatter: missing required 'description'"},
		"an indented fence is not a fence":                                    {"  ---\nname: GADU\ndescription: d\n---\nbody\n", "gadu frontmatter: missing opening --- delimiter"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			message, linked := installWithGadu(t, c.content)
			if c.want == "" {
				if !linked {
					t.Fatalf("a valid agent file was refused: %q", message)
				}
				return
			}
			if !strings.Contains(message, c.want) || linked {
				t.Fatalf("message=%q linked=%v, want a refusal containing %q and no link", message, linked, c.want)
			}
		})
	}
}
