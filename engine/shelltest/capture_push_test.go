package shelltest

// After it commits, capture pushes upstream to origin, and, like apply, only to a network remote: an
// origin that is a path on this machine is skipped with a message.

import (
	"os"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// uploadSnippet makes a repository whose upstream is one commit ahead of origin/upstream, gives its
// origin the URL in "$1", and calls the push step with a git that records a push instead of making one.
// With "$2" = new, origin has no upstream branch yet.
const uploadSnippet = `set -e
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
git init -q --bare origin.git
git init -q -b upstream work
cd work
git commit -q --allow-empty -m one
git remote add origin "$PWD/../origin.git"
if [ "$2" != new ]; then git push -q origin upstream; git commit -q --allow-empty -m two; fi
git remote set-url origin "$1"
LOG="$PWD/../pushes"; : > "$LOG"
git() { if [ "${1:-}" = push ]; then echo "$*" >> "$LOG"; return 0; fi; command git "$@"; }
push_upstream_to_origin
echo "pushes: $(wc -l < "$LOG" | tr -d ' ')"
`

func TestCapturePushesUpstreamOnlyToANetworkRemote(t *testing.T) {
	overlaytest.RequireBash(t)
	script := overlaytest.Script(t)
	for _, tc := range []struct {
		name, url, branch string
		pushes            bool
	}{
		{"an https remote", "https://github.com/example/overlay.git", "ahead", true},
		{"an scp-style remote", "git@github.com:example/overlay.git", "ahead", true},
		{"a new branch on a network remote", "ssh://git@example.com/example/overlay.git", "new", true},
		{"an absolute path", "/srv/overlay.git", "ahead", false},
		{"a relative path", "../origin.git", "ahead", false},
		{"a file URL", "file:///srv/overlay.git", "ahead", false},
		{"a new branch on a local path", "/srv/overlay.git", "new", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			shell := overlaytest.Shell{Env: []string{"HOME=" + dir, "PATH=" + os.Getenv("PATH")}, Dir: dir}

			out, err := overlaytest.Source(t, script, shell, uploadSnippet, tc.url, tc.branch)

			if err != nil {
				t.Fatalf("the step failed: %v\n%s", err, out)
			}
			if tc.pushes {
				if !strings.Contains(out, "pushes: 1") || !strings.Contains(out, "Pushed upstream to origin") {
					t.Errorf("origin %s: want one push of upstream, got:\n%s", tc.url, out)
				}
				return
			}
			if !strings.Contains(out, "pushes: 0") || !strings.Contains(out, "origin is not a network remote") {
				t.Errorf("origin %s: want no push and the reason, got:\n%s", tc.url, out)
			}
		})
	}
}
