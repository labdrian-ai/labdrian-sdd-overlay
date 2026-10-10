package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The success path of the same wiring: the address the vault's script prints is the address of the page
// the command writes, and the command says so. (TestPromoteGolden pins the same path over a whole
// scenario; this one names the wiring and fails on it alone.)
func TestCmdPromote_PromotesAtTheAddressTheVaultsScriptPrints(t *testing.T) {
	vaultRoot := t.TempDir()
	scripts := filepath.Join(vaultRoot, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", scripts, err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "allocate-address.sh"), []byte("#!/bin/sh\necho c-000777\n"), 0o755); err != nil {
		t.Fatalf("write the allocator fixture: %v", err)
	}
	dbPath, id := promoteFixtureDB(t, "Gets An Address", "cmd-promote-project")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
	t.Chdir(t.TempDir())

	code, stdout, stderr := runCaptured(t, []string{"promote", "--project", "cmd-promote-project", "--id", strconv.FormatInt(id, 10)})

	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr = %q", code, exitOK, stderr)
	}
	if want := "longterm-mem: promoted c-000777 (created)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if _, err := os.Stat(filepath.Join(vaultRoot, "wiki", "memory", "c-000777.md")); err != nil {
		t.Errorf("the page is not at the address the script printed: %v", err)
	}
}

// The command wires the vault's allocator into promotion (Phase 9, L2), and what an operator reads when
// that allocator cannot give an address is the same as before the wiring moved: the exit code, the
// script, its exit status and what it said. The wording below was recorded from the program as it stood
// before L2 (it says "promote:" twice because the command and the package each prefix it).
func TestCmdPromote_WhenTheVaultCannotGiveAnAddress(t *testing.T) {
	cases := []struct {
		name       string
		script     string // body of scripts/allocate-address.sh; empty means the script is absent
		wantStderr string
	}{
		{
			name:       "the script exits non-zero",
			script:     "#!/bin/sh\necho 'the counter is locked' >&2\nexit 3\n",
			wantStderr: "longterm-mem: promote: promote: scripts/allocate-address.sh exited 3: the counter is locked\n",
		},
		{
			name:       "the script prints nothing",
			script:     "#!/bin/sh\nprintf '  \\n'\n",
			wantStderr: "longterm-mem: promote: promote: scripts/allocate-address.sh produced no address\n",
		},
		{
			name:       "the script is absent",
			wantStderr: "longterm-mem: promote: promote: allocate address: vault: resolve script scripts/allocate-address.sh: ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vaultRoot := t.TempDir()
			if tc.script != "" {
				scripts := filepath.Join(vaultRoot, "scripts")
				if err := os.MkdirAll(scripts, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", scripts, err)
				}
				if err := os.WriteFile(filepath.Join(scripts, "allocate-address.sh"), []byte(tc.script), 0o755); err != nil {
					t.Fatalf("write the allocator fixture: %v", err)
				}
			}
			dbPath, id := promoteFixtureDB(t, "Needs An Address", "cmd-promote-project")
			t.Setenv("HOME", t.TempDir())
			t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)
			t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)
			t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
			t.Chdir(t.TempDir())

			code, stdout, stderr := runCaptured(t, []string{"promote", "--project", "cmd-promote-project", "--id", strconv.FormatInt(id, 10)})

			if code != exitInternal {
				t.Errorf("exit = %d, want %d (internal)", code, exitInternal)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing: no page was promoted", stdout)
			}
			if !strings.HasPrefix(stderr, tc.wantStderr) {
				t.Errorf("stderr = %q, want it to start with %q", stderr, tc.wantStderr)
			}
			if _, err := os.Stat(filepath.Join(vaultRoot, "wiki")); !os.IsNotExist(err) {
				t.Errorf("a promotion with no address wrote into wiki/ (stat err = %v)", err)
			}
		})
	}
}
