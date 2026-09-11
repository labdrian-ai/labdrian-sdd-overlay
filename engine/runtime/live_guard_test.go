package runtime_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates every test in this package from the developer's live
// machine: the Pi adapter's default constructor reads HOME, STATE_DIR and
// OVERLAY_DIR from the environment and its Install/Uninstall shell out to
// a real `pi` CLI, which once removed a freshly installed labdrian-pi
// package during `go test ./...`. Individual tests may still override
// these with t.Setenv.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "engine-test-home-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(home)
	os.Setenv("HOME", home)
	os.Setenv("STATE_DIR", filepath.Join(home, ".labdrian-overlay"))
	os.Unsetenv("OVERLAY_DIR")
	os.Setenv("LABDRIAN_PI_BIN", filepath.Join(home, "pi-must-not-run"))
	os.Exit(m.Run())
}

// TestLiveGuard_IsolatesHomeAndPiBinary proves the TestMain guard is in
// force for this package: no test can reach the developer's ~/.pi or a
// real pi binary by accident.
func TestLiveGuard_IsolatesHomeAndPiBinary(t *testing.T) {
	real, _ := os.UserHomeDir()
	if _, err := os.Stat(filepath.Join(real, ".pi", "agent", "settings.json")); err == nil {
		t.Fatalf("HOME %q still points at a real Pi installation", real)
	}
	if _, err := os.Stat(os.Getenv("LABDRIAN_PI_BIN")); err == nil {
		t.Fatal("LABDRIAN_PI_BIN must point at a non-existent stub in tests")
	}
}
