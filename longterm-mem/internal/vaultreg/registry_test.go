package vaultreg

import (
	"errors"
	"strings"
	"testing"
)

// home is an Environment's way to tell the home directory when a case has one to tell.
func home(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

// registryOf is a registry that came from a place called /registry/vaults.json and holds a row for each project.
func registryOf(rows map[string]string) Registry {
	reg := Registry{Vaults: map[string]Entry{}, Origin: "/registry/vaults.json"}
	for project, path := range rows {
		reg.Vaults[project] = Entry{Path: path}
	}
	return reg
}

// The registry is the vault of a project, whichever way it is written: an absolute path as it stands, a path in the
// home directory with its ~ expanded.
func TestResolve_TheRowOfTheProjectIsItsVault(t *testing.T) {
	env := Environment{Home: home("/home/someone")}
	reg := registryOf(map[string]string{
		"absolute": "/vaults/absolute",
		"homely":   "~/brain",
		"home":     "~",
	})

	for project, want := range map[string]string{
		"absolute": "/vaults/absolute",
		"homely":   "/home/someone/brain",
		"home":     "/home/someone",
	} {
		got, err := Resolve("", env, reg, project)
		if err != nil || got != want {
			t.Errorf("Resolve(%q) = (%q, %v), want %q", project, got, err, want)
		}
	}
}

// The precedence of D5: the flag over the environment over the row. Each source is the answer when it is the first
// one that is set, and a source that is empty is as good as unset.
func TestResolve_FlagThenEnvironmentThenRow(t *testing.T) {
	reg := registryOf(map[string]string{"proj": "/vaults/row"})
	env := Environment{Vault: "/vaults/env", Home: home("/home/someone")}

	for name, tc := range map[string]struct {
		flag string
		env  Environment
		want string
	}{
		"the row when neither the flag nor the environment is set": {"", Environment{Home: env.Home}, "/vaults/row"},
		"the environment over the row":                             {"", env, "/vaults/env"},
		"the flag over the environment and the row":                {"/vaults/flag", env, "/vaults/flag"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := Resolve(tc.flag, tc.env, reg, "proj")
			if err != nil || got != tc.want {
				t.Errorf("Resolve = (%q, %v), want %q", got, err, tc.want)
			}
		})
	}
}

// The flag and the environment need no registry: a project that has no row has the vault it is told it has.
func TestResolve_ANamedVaultNeedsNoRowForTheProject(t *testing.T) {
	got, err := Resolve("/vaults/flag", Environment{}, Registry{}, "no-such-project")
	if err != nil || got != "/vaults/flag" {
		t.Errorf("Resolve with the flag = (%q, %v), want the flag's path", got, err)
	}
	got, err = Resolve("", Environment{Vault: "/vaults/env"}, Registry{}, "no-such-project")
	if err != nil || got != "/vaults/env" {
		t.Errorf("Resolve with the environment's vault = (%q, %v), want the environment's path", got, err)
	}
}

// The flag and the environment are expanded and checked as a row is.
func TestResolve_ANamedVaultIsExpandedAndCheckedLikeARow(t *testing.T) {
	env := Environment{Vault: "~/from-env", Home: home("/home/someone")}

	if got, err := Resolve("", env, Registry{}, "p"); err != nil || got != "/home/someone/from-env" {
		t.Errorf("Resolve with ~ in the environment = (%q, %v), want it expanded", got, err)
	}
	if got, err := Resolve("~/from-flag", env, Registry{}, "p"); err != nil || got != "/home/someone/from-flag" {
		t.Errorf("Resolve with ~ in the flag = (%q, %v), want it expanded", got, err)
	}
	if _, err := Resolve("relative/vault", env, Registry{}, "p"); err == nil || !strings.Contains(err.Error(), `must be absolute after expansion`) {
		t.Errorf("Resolve with a relative flag = %v, want it refused as not absolute", err)
	}
}

// A project with no row is not configured, never a guess (R-023), and the refusal says where it looked.
func TestResolve_AProjectWithNoRowIsNotConfigured(t *testing.T) {
	_, err := Resolve("", Environment{Home: home("/home/someone")}, registryOf(map[string]string{"other": "/vaults/other"}), "proj")

	if !errors.Is(err, ErrVaultNotConfigured) {
		t.Fatalf("Resolve = %v, want ErrVaultNotConfigured", err)
	}
	if want := `vaultreg: vault not configured: project "proj" has no vault-registry entry in /registry/vaults.json`; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

// A row that is not an absolute path once expanded is refused with the row as written, and the same goes for a
// registry with no rows at all.
func TestResolve_ARowThatIsNotAbsoluteAfterExpansionIsRefused(t *testing.T) {
	env := Environment{Home: home("/home/someone")}

	_, err := Resolve("", env, registryOf(map[string]string{"p": "relative/vault"}), "p")

	if want := `vaultreg: vault path "relative/vault" must be absolute after expansion`; err == nil || err.Error() != want {
		t.Errorf("Resolve = %v, want %q", err, want)
	}
	if _, err := Resolve("", env, Registry{}, "p"); !errors.Is(err, ErrVaultNotConfigured) {
		t.Errorf("Resolve over an empty registry = %v, want ErrVaultNotConfigured", err)
	}
}

// The home directory is asked for only when a path begins with ~: a vault named by an absolute path is no worse off
// on a machine that cannot tell its home.
func TestResolve_TheHomeDirectoryIsAskedOnlyForAPathThatBeginsWithATilde(t *testing.T) {
	cannotTell := errors.New("$HOME is not defined")
	asked := 0
	env := Environment{Home: func() (string, error) { asked++; return "", cannotTell }}

	if got, err := Resolve("/vaults/flag", env, Registry{}, "p"); err != nil || got != "/vaults/flag" || asked != 0 {
		t.Errorf("Resolve of an absolute path = (%q, %v) after %d questions of the home, want the path and none", got, err, asked)
	}

	for _, path := range []string{"~", "~/brain"} {
		asked = 0
		_, err := Resolve(path, env, Registry{}, "p")

		if !errors.Is(err, cannotTell) || asked != 1 {
			t.Errorf("Resolve(%q) = %v after %d questions, want the failure to tell the home, asked once", path, err, asked)
		}
		if want := "vaultreg: resolve home directory: "; err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("Resolve(%q) = %v, want it to begin %q", path, err, want)
		}
	}
}

// An environment built with no way to tell the home is a mistake of whoever built it, and is said as one, not
// answered with a crash.
func TestResolve_AnEnvironmentWithNoHomeSaysSoForAPathThatNeedsOne(t *testing.T) {
	_, err := Resolve("~/brain", Environment{}, Registry{}, "p")

	if !errors.Is(err, errNoHome) {
		t.Errorf("Resolve = %v, want the missing source of the home to be said", err)
	}
}

// A path that begins with ~ and something else (~someone/brain) is no home path: it is relative, and refused.
func TestResolve_OnlyTheTildeAloneOrFollowedByASlashIsTheHome(t *testing.T) {
	_, err := Resolve("~someone/brain", Environment{Home: home("/home/someone")}, Registry{}, "p")

	if err == nil || !strings.Contains(err.Error(), "must be absolute after expansion") {
		t.Errorf("Resolve(~someone/brain) = %v, want it refused as not absolute", err)
	}
}

func TestOverridden_IsTrueForTheFlagAndForTheEnvironmentAndFalseOtherwise(t *testing.T) {
	for name, tc := range map[string]struct {
		flag string
		env  Environment
		want bool
	}{
		"neither":          {"", Environment{}, false},
		"the flag":         {"/v", Environment{}, true},
		"the environment":  {"", Environment{Vault: "/v"}, true},
		"both":             {"/v", Environment{Vault: "/w"}, true},
		"only a home path": {"", Environment{Home: home("/h")}, false},
	} {
		if got := Overridden(tc.flag, tc.env); got != tc.want {
			t.Errorf("%s: Overridden = %v, want %v", name, got, tc.want)
		}
	}
}
