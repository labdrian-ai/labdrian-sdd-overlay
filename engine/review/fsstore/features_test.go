package fsstore_test

// What the stop hook reads of a working tree before it asks git anything (own-rdd T6a, S6): whether the tree keeps
// features at all, because the hook is installed for every repository of the machine and must say nothing in one that
// has no odd/tasks or odd/review-results, and which features have a results directory, because every one of those
// directories holds reviews of the branch.

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
)

func TestFeaturesSaysWhetherTheTreeKeepsFeaturesAndListsTheOnesWithResults(t *testing.T) {
	for _, tc := range []struct {
		name  string
		dirs  []string          // directories made under the root
		files map[string]string // files made under the root
		links map[string]string // symbolic links made under the root, name to target
		want  app.Features
	}{
		{name: "a tree with no odd directory keeps no features", dirs: []string{"engine"}, want: app.Features{}},
		{name: "odd without tasks or results keeps none either", dirs: []string{"odd"}, want: app.Features{}},
		{name: "odd/tasks alone is a tree that keeps features, with no results yet", dirs: []string{"odd/tasks"}, want: app.Features{Tracked: true}},
		{
			name: "every directory of odd/review-results is a feature, by name",
			dirs: []string{"odd/review-results/own-rdd", "odd/review-results/another"}, files: map[string]string{"odd/review-results/README.md": "x"},
			want: app.Features{Tracked: true, Names: []string{"another", "own-rdd"}},
		},
		{
			name:  "a link, a hidden directory and a name that is not a feature are not features",
			dirs:  []string{"odd/review-results/real", "odd/review-results/.hidden", "odd/review-results/two words", "elsewhere"},
			links: map[string]string{"odd/review-results/linked": "../../elsewhere"},
			want:  app.Features{Tracked: true, Names: []string{"real"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			for name, content := range tc.files {
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			for name, target := range tc.links {
				if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}

			got, err := fsstore.New().Features(root)

			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Features() = %+v, %v; want %+v and no error", got, err, tc.want)
			}
		})
	}
}

// A directory of the path that is a link or a file where a directory belongs is refused, as it is for every other
// call of the store, and nothing is read through it.
func TestFeaturesRefusesALinkOrAFileWhereADirectoryBelongs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{"odd is a link", func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, "elsewhere", "tasks"), 0o755); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if err := os.Symlink("elsewhere", filepath.Join(root, "odd")); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}},
		{"odd/review-results is a file", func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, "odd"), 0o755); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if err := os.WriteFile(filepath.Join(root, "odd", "review-results"), []byte("x"), 0o644); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(t, root)

			got, err := fsstore.New().Features(root)

			if !errors.Is(err, fsstore.ErrNotADirectory) || !reflect.DeepEqual(got, app.Features{}) {
				t.Errorf("Features() = %+v, %v; want nothing and an error that is ErrNotADirectory", got, err)
			}
		})
	}
}
