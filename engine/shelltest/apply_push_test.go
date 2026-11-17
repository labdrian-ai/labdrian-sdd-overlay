package shelltest

// After it merges, apply pushes main to origin so that main does not diverge from the remote. It does
// that only when origin is a network remote: an origin that is a path on this machine (a scratch clone
// made from the real checkout, a mirror) is not a place main may be published to, and a push there
// landed unmerged commits in the real repository's local main.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// pushSnippet makes a repository whose main is one commit ahead of origin/main, gives its origin the
// URL in "$1", and calls the push step with a git that records a push instead of making one, so that
// no case can reach a remote.
const pushSnippet = `set -e
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
git init -q --bare origin.git
git init -q -b main work
cd work
git commit -q --allow-empty -m one
git remote add origin "$PWD/../origin.git"
git push -q origin main
git commit -q --allow-empty -m two
git remote set-url origin "$1"
LOG="$PWD/../pushes"; : > "$LOG"
git() { if [ "${1:-}" = push ]; then echo "$*" >> "$LOG"; return 0; fi; command git "$@"; }
push_main_to_origin
echo "pushes: $(wc -l < "$LOG" | tr -d ' ')"
`

func TestApplyPushesMainOnlyToANetworkRemote(t *testing.T) {
	overlaytest.RequireBash(t)
	script := overlaytest.Script(t)
	for _, tc := range []struct {
		name, url string
		pushes    bool
	}{
		{"an https remote", "https://github.com/example/overlay.git", true},
		{"an scp-style remote", "git@github.com:example/overlay.git", true},
		{"an ssh remote", "ssh://git@example.com/example/overlay.git", true},
		{"a host that only starts like localhost", "ssh://localhost.example.com/overlay.git", true},
		{"an scp form of a host that only starts like localhost", "git@localhost.example.com:overlay.git", true},
		{"a local name hidden in the path of a network scp form", "example.com:a@localhost:overlay.git", true},
		{"an http git server on this machine", "http://localhost:3000/org/overlay.git", true},
		{"an https git server on 127.0.0.1", "https://127.0.0.1/overlay.git", true},
		{"an ssh URL of localhost in upper case", "SSH://LOCALHOST/srv/overlay.git", false},
		{"a git+ssh URL of localhost", "git+ssh://localhost/srv/overlay.git", false},
		{"an ssh+git URL of 127.0.0.1", "ssh+git://127.0.0.1/srv/overlay.git", false},
		{"an absolute path", "/srv/overlay.git", false},
		{"a relative path", "../origin.git", false},
		{"a file URL", "file:///srv/overlay.git", false},
		{"a path under the home", "~/overlay.git", false},
		{"a file URL in capitals", "FILE:///srv/overlay.git", false},
		{"an scp form of localhost", "localhost:/srv/overlay.git", false},
		{"an scp form of localhost with a user", "git@localhost:overlay.git", false},
		{"an scp form of 127.0.0.1", "127.0.0.1:/srv/overlay.git", false},
		{"an scp form of ::1", "[::1]:/srv/overlay.git", false},
		{"an ssh URL of localhost", "ssh://localhost/srv/overlay.git", false},
		{"an ssh URL of 127.0.0.1 with a user and a port", "ssh://git@127.0.0.1:2222/overlay.git", false},
		{"an ssh URL of ::1", "ssh://[::1]/srv/overlay.git", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			shell := overlaytest.Shell{Env: []string{"HOME=" + dir, "PATH=" + os.Getenv("PATH")}, Dir: dir}

			out, err := overlaytest.Source(t, script, shell, pushSnippet, tc.url)

			if err != nil {
				t.Fatalf("the step failed: %v\n%s", err, out)
			}
			if tc.pushes {
				if !strings.Contains(out, "pushes: 1") || !strings.Contains(out, "Pushed main to origin") {
					t.Errorf("origin %s: want one push of main, got:\n%s", tc.url, out)
				}
				return
			}
			if !strings.Contains(out, "pushes: 0") || !strings.Contains(out, "origin is not a network remote") {
				t.Errorf("origin %s: want no push and the reason, got:\n%s", tc.url, out)
			}
		})
	}
}

// The step keeps what it did before: a repository without an origin is skipped with its message, and a
// main that is not ahead is not pushed.
func TestApplyPushStepKeepsItsOtherBehaviour(t *testing.T) {
	overlaytest.RequireBash(t)
	script := overlaytest.Script(t)
	const snippet = `set -e
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
git init -q -b main work
cd work
git commit -q --allow-empty -m one
push_main_to_origin
`
	dir := t.TempDir()
	shell := overlaytest.Shell{Env: []string{"HOME=" + dir, "PATH=" + os.Getenv("PATH")}, Dir: dir}

	out, err := overlaytest.Source(t, script, shell, snippet)

	if err != nil || !strings.Contains(out, "No 'origin' remote — skipping main push") {
		t.Errorf("without an origin: %q, %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "work", ".git")); err != nil {
		t.Fatal(err)
	}
}
