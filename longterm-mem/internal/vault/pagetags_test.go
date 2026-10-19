package vault

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
)

// The adapter satisfies the query's port.
var _ query.VaultPageTags = PageTagReader{}

func writePage(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A page's tags are read from its frontmatter, in each shape a page carries them: the block list promote writes
// (quoted items), a plain block list, a flow list, an empty list, and a page with no tags or no frontmatter at all.
func TestPageTagReaderReadsTheFrontmatterTags(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          []string
	}{
		{"promote's block list", "---\ntitle: \"x\"\ntags:\n  - \"memory\"\n  - \"decision\"\n  - \"goal:g-42\"\nstatus: developing\n---\n# x\n", []string{"memory", "decision", "goal:g-42"}},
		{"a plain block list", "---\ntags:\n- memory\n- goal:g-7\n---\nbody\n", []string{"memory", "goal:g-7"}},
		{"a flow list", "---\ntags: [memory, \"goal:g-42\", 'concept']\n---\n", []string{"memory", "goal:g-42", "concept"}},
		{"an empty list", "---\ntags: []\n---\n", nil},
		{"a scalar", "---\ntags: goal:g-42\n---\n", []string{"goal:g-42"}},
		{"no tags", "---\ntitle: x\n---\nbody with tags:\n  - goal:g-7\n", nil},
		{"no frontmatter", "# a page\ntags:\n  - goal:g-7\n", nil},
		{"an escaped quote", "---\ntags:\n  - \"a \\\"b\\\" \\\\c\"\n---\n", []string{`a "b" \c`}},
		{"CRLF lines", "---\r\ntags:\r\n  - \"goal:g-42\"\r\n---\r\n", []string{"goal:g-42"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := writePage(t, root, "wiki/c-000001.md", tc.content)
			got, err := PageTagReader{Root: root}.PageTags(context.Background(), query.VaultCandidate{PageAddress: "c-000001", AbsolutePath: path})
			if err != nil {
				t.Fatalf("PageTags: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("PageTags = %q, want %q", got, tc.want)
			}
		})
	}
}

// A page that cannot be read, whose frontmatter is never closed or whose flow list is never closed, or that lies
// outside the vault, is an error: the query leaves such a page out under a goal.
func TestPageTagReaderRefusesWhatItCannotRead(t *testing.T) {
	root := t.TempDir()
	outside := writePage(t, t.TempDir(), "elsewhere.md", "---\ntags: [memory]\n---\n")
	for _, tc := range []struct {
		name, path string
	}{
		{"a missing page", filepath.Join(root, "wiki/missing.md")},
		{"an unclosed frontmatter", writePage(t, root, "wiki/open.md", "---\ntags:\n  - memory\n")},
		{"an unclosed flow list", writePage(t, root, "wiki/flow.md", "---\ntags: [memory, goal:g-7\n---\n")},
		{"a page outside the vault", outside},
		{"a relative path", "wiki/c-000001.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PageTagReader{Root: root}.PageTags(context.Background(), query.VaultCandidate{PageAddress: "c-000001", AbsolutePath: tc.path})
			if err == nil {
				t.Fatalf("PageTags = %q, nil; want an error", got)
			}
		})
	}
}
