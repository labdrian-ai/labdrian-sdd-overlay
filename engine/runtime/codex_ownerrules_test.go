package runtime_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// Fixtures shaped like the three live files: their marker pairs (nested pairs, the procedure markers,
// the owner's memorycard block) and unmarked text, with placeholder lines for everything else.
const (
	codexGlobalFixture = "<!-- gentle-ai:persona -->\nplaceholder persona\n<!-- /gentle-ai:persona -->\n\n" +
		"<!-- gentle-ai:orchestrator -->\nplaceholder orchestrator\n" +
		"<!-- authority-first-terminal-procedure:start -->\nplaceholder procedure\n<!-- authority-first-terminal-procedure:end -->\n" +
		"<!-- /gentle-ai:orchestrator -->\n\n" +
		"<!-- gentle-ai:agent-routing -->\nplaceholder routing\n" +
		"<!-- gentle-ai:remote-authorization -->\nplaceholder remote\n<!-- /gentle-ai:remote-authorization -->\n" +
		"<!-- /gentle-ai:agent-routing -->\n\n" +
		"<!-- user:memorycard-store -->\nplaceholder store\n<!-- /user:memorycard-store -->\n"
	codexLowerFixture = "placeholder unmarked line one\nplaceholder unmarked line two\n\n" +
		"<!-- gentle-ai:engram-protocol -->\nplaceholder protocol\n<!-- /gentle-ai:engram-protocol -->\n\n" +
		"<!-- gentle-ai:strict-tdd-mode -->\nplaceholder tdd\n<!-- /gentle-ai:strict-tdd-mode -->\n"
	codexHomeFixture = "<!-- gentle-ai:codegraph-guidance -->\nplaceholder guidance\n<!-- /gentle-ai:codegraph-guidance -->\n\n" +
		"<!-- gentle-ai:orchestrator -->\nplaceholder orchestrator\n<!-- /gentle-ai:orchestrator -->\n\n" +
		"<!-- gentle-ai:agent-routing -->\nplaceholder routing\n" +
		"<!-- gentle-ai:remote-authorization -->\nplaceholder remote\n<!-- /gentle-ai:remote-authorization -->\n" +
		"<!-- /gentle-ai:agent-routing -->\n"
)

// codexSandbox is a home and a Codex root under it, in a temporary directory, and the three files
// the adapter writes the block in.
type codexSandbox struct {
	home, root string
	files      []string // ~/AGENTS.md, <root>/AGENTS.md, <root>/agents.md
}

func newCodexSandbox(t *testing.T) codexSandbox {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".codex")
	return codexSandbox{
		home:  home,
		root:  root,
		files: []string{filepath.Join(home, "AGENTS.md"), filepath.Join(root, "AGENTS.md"), filepath.Join(root, "agents.md")},
	}
}

// seed writes the live-shaped fixtures into the three files.
func (s codexSandbox) seed(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{codexHomeFixture, codexGlobalFixture, codexLowerFixture} {
		if err := os.WriteFile(s.files[i], []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func (s codexSandbox) adapter(source engineRuntime.OwnerRulesSource) engineRuntime.CodexAdapter {
	return engineRuntime.NewCodexAdapter(s.root).WithOwnerRules(source, s.home)
}

func (s codexSandbox) manifest() string {
	return filepath.Join(s.root, "labdrian-runtime-lifecycle.json")
}

func TestCodexInstallWritesTheBlockInThreeFilesAndKeepsEveryOtherByte(t *testing.T) {
	s := newCodexSandbox(t)
	s.seed(t)
	result := s.adapter(&rulesSource{text: rulesV1}).Install()
	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install = %v %q", result.Status, result.Message)
	}
	for i, original := range []string{codexHomeFixture, codexGlobalFixture, codexLowerFixture} {
		if got, want := readFileText(t, s.files[i]), original+"\n"+rulesBlock(rulesV1)+"\n"; got != want {
			t.Errorf("%s = %q, want %q", s.files[i], got, want)
		}
		if !strings.Contains(result.Message, "owner rules written to "+s.files[i]) {
			t.Errorf("the message does not name %s: %q", s.files[i], result.Message)
		}
	}
}

func TestCodexInstallCreatesMissingFilesWithTheBlockAlone(t *testing.T) {
	s := newCodexSandbox(t)
	if result := s.adapter(&rulesSource{text: rulesV1}).Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install = %v %q", result.Status, result.Message)
	}
	for _, path := range s.files {
		if got := readFileText(t, path); got != rulesBlock(rulesV1)+"\n" {
			t.Errorf("%s = %q, want the block alone", path, got)
		}
	}
}

func TestCodexUpdateReplacesAStaleBlockInPlaceInEachFile(t *testing.T) {
	s := newCodexSandbox(t)
	s.seed(t)
	tail := "\ntail of the owner\n"
	for i, text := range []string{codexHomeFixture, codexGlobalFixture, codexLowerFixture} {
		stale := text + "\n" + rulesBlock("old rule\n") + tail
		if err := os.WriteFile(s.files[i], []byte(stale), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result := s.adapter(&rulesSource{text: rulesV2}).Update()
	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Update = %v %q", result.Status, result.Message)
	}
	for i, text := range []string{codexHomeFixture, codexGlobalFixture, codexLowerFixture} {
		if got, want := readFileText(t, s.files[i]), text+"\n"+rulesBlock(rulesV2)+tail; got != want {
			t.Errorf("%s = %q, want %q", s.files[i], got, want)
		}
		if !strings.Contains(result.Message, "owner rules updated in "+s.files[i]) {
			t.Errorf("the message does not say %s was updated: %q", s.files[i], result.Message)
		}
	}
}

func TestCodexSecondInstallIsANoOpOnEveryFile(t *testing.T) {
	s := newCodexSandbox(t)
	s.seed(t)
	adapter := s.adapter(&rulesSource{text: rulesV1})
	adapter.Install()
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	before := map[string]string{}
	for _, path := range s.files {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		before[path] = readFileText(t, path)
	}
	result := adapter.Install()
	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("second Install = %v %q", result.Status, result.Message)
	}
	for _, path := range s.files {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(old) || readFileText(t, path) != before[path] {
			t.Errorf("%s was written by the second install", path)
		}
		if !strings.Contains(result.Message, "owner rules already current in "+path) {
			t.Errorf("the message does not say %s is current: %q", path, result.Message)
		}
	}
}

func TestCodexUninstallGivesEveryFileBackAndLeavesNoMarker(t *testing.T) {
	s := newCodexSandbox(t)
	s.seed(t)
	adapter := s.adapter(&rulesSource{text: rulesV1})
	adapter.Install()
	if result := adapter.Uninstall(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Uninstall = %v %q", result.Status, result.Message)
	}
	for i, original := range []string{codexHomeFixture, codexGlobalFixture, codexLowerFixture} {
		got := readFileText(t, s.files[i])
		if got != original {
			t.Errorf("%s = %q after install and uninstall, want the original %q", s.files[i], got, original)
		}
		if strings.Contains(got, "labdrian:owner-rules") {
			t.Errorf("%s still holds a labdrian marker", s.files[i])
		}
	}
	if _, err := os.Stat(s.manifest()); !os.IsNotExist(err) {
		t.Errorf("the manifest is still there after uninstall: %v", err)
	}
}

func TestCodexUninstallRemovesTheFilesInstallCreated(t *testing.T) {
	s := newCodexSandbox(t)
	adapter := s.adapter(&rulesSource{text: rulesV1})
	adapter.Install()
	if result := adapter.Uninstall(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Uninstall = %v %q", result.Status, result.Message)
	}
	for _, path := range s.files {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s is still there after uninstall: %v", path, err)
		}
	}
}

func TestCodexUninstallWithoutASourceLeavesTheBlocksAlone(t *testing.T) {
	s := newCodexSandbox(t)
	s.adapter(&rulesSource{text: rulesV1}).Install()
	if result := engineRuntime.NewCodexAdapter(s.root).Uninstall(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Uninstall = %v %q", result.Status, result.Message)
	}
	for _, path := range s.files {
		if got := readFileText(t, path); got != rulesBlock(rulesV1)+"\n" {
			t.Errorf("%s = %q, want the block kept by an adapter with no source", path, got)
		}
	}
}

func TestCodexManifestIsTheSameWithAndWithoutTheOwnerRules(t *testing.T) {
	s := newCodexSandbox(t)
	engineRuntime.NewCodexAdapter(s.root).Install()
	plain := readFileText(t, s.manifest())
	if err := os.Remove(s.manifest()); err != nil {
		t.Fatal(err)
	}
	s.adapter(&rulesSource{text: rulesV1}).Install()
	if got := readFileText(t, s.manifest()); got != plain {
		t.Errorf("manifest = %q, want %q", got, plain)
	}
}

func TestCodexStatusNamesEachOfTheThreeFiles(t *testing.T) {
	s := newCodexSandbox(t)
	adapter := s.adapter(&rulesSource{text: rulesV1})
	adapter.Install()
	if err := os.Remove(s.files[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.files[1], []byte(codexGlobalFixture+"\n"+rulesBlock("old\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := adapter.Status()
	reasons := strings.Join(result.Reasons, "\n")
	for path, state := range map[string]string{
		s.files[0]: "owner rules block is missing in " + s.files[0] + "; run 'engine runtime install --target codex'",
		s.files[1]: "owner rules block is stale in " + s.files[1] + "; run 'engine runtime update --target codex'",
		s.files[2]: "owner rules block is current in " + s.files[2],
	} {
		if !strings.Contains(reasons, state) {
			t.Errorf("status does not say %q for %s:\n%s", state, path, reasons)
		}
	}
	if result.Status != core.CapabilityPartial {
		t.Errorf("status = %v, want partial (activation is unverified, a block is missing)", result.Status)
	}
}

func TestCodexStatusKeepsItsOldAnswerWithoutASource(t *testing.T) {
	s := newCodexSandbox(t)
	engineRuntime.NewCodexAdapter(s.root).Install()
	result := engineRuntime.NewCodexAdapter(s.root).Status()
	if len(result.Reasons) != 1 || strings.Contains(strings.Join(result.Reasons, ""), "owner rules") {
		t.Errorf("Reasons = %q, want only the activation reason", result.Reasons)
	}
}

func TestCodexReportsTheFileNoCodexPathReads(t *testing.T) {
	s := newCodexSandbox(t)
	adapter := s.adapter(&rulesSource{text: rulesV1})
	install := adapter.Install()
	note := "Codex reads no file named agents.md"
	if !strings.Contains(install.Message, note) || !strings.Contains(install.Message, s.files[2]) {
		t.Errorf("Install message does not report %s as unread: %q", s.files[2], install.Message)
	}
	if reasons := strings.Join(adapter.Status().Reasons, "\n"); !strings.Contains(reasons, note) {
		t.Errorf("status does not report the unread file:\n%s", reasons)
	}
	if got := readFileText(t, s.files[2]); got != rulesBlock(rulesV1)+"\n" {
		t.Errorf("the unread file was not written all the same: %q", got)
	}
}

func TestCodexInstallWithNoOverlayCheckoutWritesTheManifestAloneWithANote(t *testing.T) {
	s := newCodexSandbox(t)
	result := s.adapter(&rulesSource{err: engineRuntime.ErrNoOverlayCheckout}).Install()
	if result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "owner rules not installed: no overlay checkout found") {
		t.Errorf("Install = %v %q", result.Status, result.Message)
	}
	for _, path := range s.files {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s exists with no checkout: %v", path, err)
		}
	}
	if _, err := os.Stat(s.manifest()); err != nil {
		t.Errorf("the manifest was not written: %v", err)
	}
	if status := s.adapter(&rulesSource{err: engineRuntime.ErrNoOverlayCheckout}).Status(); !strings.Contains(strings.Join(status.Reasons, "\n"), "owner rules not checked") {
		t.Errorf("status reasons = %q, want a note that the rules were not checked", status.Reasons)
	}
}

func TestCodexInstallWithUnreadableRulesIsPartialAndWritesNoFile(t *testing.T) {
	s := newCodexSandbox(t)
	result := s.adapter(&rulesSource{err: errors.New("disk on fire")}).Install()
	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "owner rules could not be read: disk on fire") {
		t.Errorf("Install = %v %q", result.Status, result.Message)
	}
	for _, path := range s.files {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s exists although the rules could not be read: %v", path, err)
		}
	}
}

func TestCodexInstallRefusesAFileWithBrokenMarkersAndWritesTheOthers(t *testing.T) {
	s := newCodexSandbox(t)
	s.seed(t)
	broken := "text\n<!-- labdrian:owner-rules -->\nno close line\n"
	if err := os.WriteFile(s.files[1], []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	result := s.adapter(&rulesSource{text: rulesV1}).Install()
	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, s.files[1]) {
		t.Fatalf("Install = %v %q, want partial naming %s", result.Status, result.Message, s.files[1])
	}
	if got := readFileText(t, s.files[1]); got != broken {
		t.Errorf("the broken file was changed: %q", got)
	}
	for _, i := range []int{0, 2} {
		if !strings.Contains(readFileText(t, s.files[i]), rulesBlock(rulesV1)) {
			t.Errorf("%s did not get the block", s.files[i])
		}
	}
}

func TestCodexInstallLeavesTheManifestRefusalAsItWas(t *testing.T) {
	s := newCodexSandbox(t)
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.manifest(), []byte(`{"managed_by":"other","installed_version":"x","config_root":"/x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	result := s.adapter(&rulesSource{text: rulesV1}).Install()
	if result.Status != core.CapabilityPartial {
		t.Fatalf("Install = %v %q, want partial for a foreign manifest", result.Status, result.Message)
	}
	for _, path := range s.files {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s was written although the manifest was refused: %v", path, err)
		}
	}
}

func TestCodexAdapterWithNoHomeWritesTheRootFilesAndSaysSo(t *testing.T) {
	s := newCodexSandbox(t)
	result := engineRuntime.NewCodexAdapter(s.root).WithOwnerRules(&rulesSource{text: rulesV1}, "").Install()
	if result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "home directory is not known") {
		t.Errorf("Install = %v %q", result.Status, result.Message)
	}
	for _, path := range s.files[1:] {
		if got := readFileText(t, path); got != rulesBlock(rulesV1)+"\n" {
			t.Errorf("%s = %q", path, got)
		}
	}
}

// CODEX_HOME is the home directory: ~/AGENTS.md and <root>/AGENTS.md are one file, handled once.
func TestCodexRootThatIsTheHomeHandlesTheSharedFileOnce(t *testing.T) {
	home := t.TempDir()
	shared := filepath.Join(home, "AGENTS.md")
	if err := os.WriteFile(shared, []byte(codexHomeFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := engineRuntime.NewCodexAdapter(home).WithOwnerRules(&rulesSource{text: rulesV1}, home)
	install := adapter.Install()
	if install.Status != core.CapabilityRestartRequired || strings.Count(install.Message, "owner rules written to "+shared) != 1 {
		t.Errorf("Install = %v %q, want the shared file reported once", install.Status, install.Message)
	}
	if got, want := readFileText(t, shared), codexHomeFixture+"\n"+rulesBlock(rulesV1)+"\n"; got != want {
		t.Errorf("%s = %q, want one block: %q", shared, got, want)
	}
	if n := strings.Count(strings.Join(adapter.Status().Reasons, "\n"), "owner rules block is current in "+shared); n != 1 {
		t.Errorf("status names the shared file %d times, want 1", n)
	}
	update := adapter.Update()
	if strings.Count(update.Message, shared) != 1 {
		t.Errorf("Update names the shared file more than once: %q", update.Message)
	}
	uninstall := adapter.Uninstall()
	if uninstall.Status != core.CapabilityRestartRequired || strings.Count(uninstall.Message, "owner rules removed from "+shared) != 1 {
		t.Errorf("Uninstall = %v %q, want the shared file reported once", uninstall.Status, uninstall.Message)
	}
	if got := readFileText(t, shared); got != codexHomeFixture {
		t.Errorf("%s = %q after uninstall, want it as it was", shared, got)
	}
}

// A file that is another name for a file already handled (a symbolic link) is handled once too, whatever its path.
func TestCodexFileThatIsALinkToAnotherOfTheThreeIsHandledOnce(t *testing.T) {
	s := newCodexSandbox(t)
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.files[0], []byte(codexHomeFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(s.files[0], s.files[1]); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	adapter := s.adapter(&rulesSource{text: rulesV1})
	install := adapter.Install()
	if strings.Contains(install.Message, s.files[1]) || strings.Count(install.Message, "owner rules written to") != 2 {
		t.Errorf("Install = %q, want the linked file once (not %s) and agents.md", install.Message, s.files[1])
	}
	if got, want := readFileText(t, s.files[0]), codexHomeFixture+"\n"+rulesBlock(rulesV1)+"\n"; got != want {
		t.Errorf("%s = %q, want one block", s.files[0], got)
	}
	adapter.Uninstall()
	if got := readFileText(t, s.files[0]); got != codexHomeFixture {
		t.Errorf("%s = %q after uninstall, want it as it was", s.files[0], got)
	}
}

// A link to a file the install created: the uninstall removes that file once, and the link, left
// dangling by the removal, is still known as the same file and not handled again.
func TestCodexLinkToAFileTheInstallCreatedIsRemovedOnce(t *testing.T) {
	s := newCodexSandbox(t)
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(s.files[0], s.files[1]); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	adapter := s.adapter(&rulesSource{text: rulesV1})
	adapter.Install()
	uninstall := adapter.Uninstall()
	if uninstall.Status == core.CapabilityPartial || strings.Contains(uninstall.Message, s.files[1]) {
		t.Errorf("Uninstall = %v %q, want %s handled once through %s", uninstall.Status, uninstall.Message, s.files[1], s.files[0])
	}
	if strings.Count(uninstall.Message, "owner rules removed from") != 2 {
		t.Errorf("Uninstall = %q, want two removals (the home file and agents.md)", uninstall.Message)
	}
}

// The registered Codex adapter reads the owner rules of the overlay the config names, and writes the
// home file under the config's home and the other two under CODEX_HOME.
func TestRegisteredCodexAdapterWritesTheOwnerRulesOfTheConfiguredOverlay(t *testing.T) {
	s := newCodexSandbox(t)
	overlay := writeOverlayRules(t, rulesV1)
	registry := core.NewRegistry()
	if err := engineRuntime.RegisterCodexWithOwnerRules(registry); err != nil {
		t.Fatal(err)
	}
	adapter, err := registry.New(core.TargetCodex, core.Config{Home: s.home, CodexHome: s.root, LabdrianOverlayDir: overlay})
	if err != nil {
		t.Fatal(err)
	}
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install = %v %q", result.Status, result.Message)
	}
	for _, path := range s.files {
		if got := readFileText(t, path); got != rulesBlock(rulesV1)+"\n" {
			t.Errorf("%s = %q", path, got)
		}
	}
}

// RegisterCodex keeps its old answer: the manifest alone.
func TestRegisteredCodexAdapterWithoutTheRulesWritesOnlyTheManifest(t *testing.T) {
	s := newCodexSandbox(t)
	registry := core.NewRegistry()
	if err := engineRuntime.RegisterCodex(registry); err != nil {
		t.Fatal(err)
	}
	adapter, err := registry.New(core.TargetCodex, core.Config{Home: s.home, CodexHome: s.root, LabdrianOverlayDir: writeOverlayRules(t, rulesV1)})
	if err != nil {
		t.Fatal(err)
	}
	adapter.Install()
	for _, path := range s.files {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s was written by the plain Codex adapter: %v", path, err)
		}
	}
}
