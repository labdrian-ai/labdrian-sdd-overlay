package ownership_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership"
)

func TestWellFormedIsABareWordOfAtMost64Bytes(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"claude", true},
		{"a", true},
		{"A1", true},
		{"my-runtime_2.0", true},
		{"a.b", true},
		{"9lives", true},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
		{"", false},
		{".hidden", false},
		{"-x", false},
		{"_x", false},
		{"claude ", false},
		{" claude", false},
		{"clau de", false},
		{"claude\r", false},
		{"cla\x00ude", false},
		{"\x01", false},
		{"a/b", false},
		{"a\nb", false},
		{"caf\xc3\xa9", false},
	} {
		if got := ownership.WellFormed(tc.name); got != tc.want {
			t.Errorf("WellFormed(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestParseKeepsEveryWellFormedNameOnceAndSortedAndDropsOnlyWhatIsNotOne(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    []string
	}{
		{"nothing", "", nil},
		{"one name", "claude\n", []string{"claude"}},
		{"sorted and once", "opencode\nclaude\nclaude\n", []string{"claude", "opencode"}},
		{"sorted by bytes", "b\nB\na\n", []string{"B", "a", "b"}},
		{"a name only a newer build knows", "zed\n", []string{"zed"}},
		{"a name cut short at the end of the file", "claude\nopenc", []string{"claude"}},
		{"only a name cut short", "clau", nil},
		{"a NUL byte makes a name garbage", "\x00zed\n", nil},
		{"a NUL byte in the middle", "ze\x00d\n", nil},
		{"garbage and blank lines around a name", "\x00garbage\n{not-a-target}\n\nclaude\n", []string{"claude"}},
		{"a carriage return is not part of a name", "claude\r\ncodex\n", []string{"codex"}},
		{"padding is not part of a name", " claude\nclaude \ncodex\n", []string{"codex"}},
		{"a name of 65 bytes", strings.Repeat("a", 65) + "\nclaude\n", []string{"claude"}},
		{"only blank lines", "\n\n\n", nil},
		{"a file that ends without a line end after valid lines", "claude\ncodex", []string{"claude"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ownership.Parse([]byte(tc.content)).Names()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Parse(%q) = %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}

func TestTargetsAnswerWhatTheyHoldAndWriteThemselvesBackAsARecord(t *testing.T) {
	targets := ownership.Parse([]byte("opencode\nclaude\nzed\n"))
	if !targets.Contains("zed") || !targets.Contains("claude") || targets.Contains("clau") || targets.Contains("") {
		t.Errorf("Contains: %q holds zed and claude and no other name", targets.Names())
	}
	if got := string(targets.Content()); got != "claude\nopencode\nzed\n" {
		t.Errorf("Content() = %q, want each name on a line of its own, sorted", got)
	}
	if ownership.Parse(nil).Content() == nil || len(ownership.Parse(nil).Content()) != 0 {
		t.Errorf("Content() of no name = %v, want an empty record that is not nil", ownership.Parse(nil).Content())
	}
	if !ownership.Parse(nil).Empty() || targets.Empty() {
		t.Error("Empty() is true for no name and false for three")
	}
}

func TestWithoutTakesOutOnlyTheWholeNameAndKeepsTheOthersUnknownOnesIncluded(t *testing.T) {
	targets := ownership.Parse([]byte("claude\nclaude-next\nmy-claude\nzed\n"))
	got := targets.Without("claude").Names()
	if want := []string{"claude-next", "my-claude", "zed"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Without(claude) = %q, want %q", got, want)
	}
	if got := targets.Without("absent").Names(); !reflect.DeepEqual(got, targets.Names()) {
		t.Errorf("Without(absent) = %q, want the set as it was", got)
	}
	if before := targets.Names(); len(before) != 4 {
		t.Errorf("Without changed the set it was called on: %q", before)
	}
}

func TestNamesIsACopy(t *testing.T) {
	targets := ownership.Parse([]byte("claude\ncodex\n"))
	targets.Names()[0] = "changed"
	if !targets.Contains("claude") {
		t.Error("changing the slice Names returned changed the set")
	}
}

func TestAdmitRefusesWhatTheVocabularyDoesNotKnow(t *testing.T) {
	knows := func(name string) bool { return name == "claude" || name == "pi" }
	if err := ownership.Admit("claude", knows); err != nil {
		t.Errorf("Admit(claude) = %v, want nil", err)
	}
	err := ownership.Admit("zed", knows)
	var unknown *ownership.UnknownTargetError
	if !errors.As(err, &unknown) || unknown.Name != "zed" {
		t.Fatalf("Admit(zed) = %v, want an *UnknownTargetError naming zed", err)
	}
	if !strings.Contains(err.Error(), "zed") {
		t.Errorf("the error %q does not name the target", err)
	}
	if err := ownership.Admit("claude", nil); err == nil {
		t.Error("a vocabulary that is not there knows no name, and Admit(claude) = nil")
	}
}

func TestObserveReadsTheContentOnlyOfARecordItCouldRead(t *testing.T) {
	read := ownership.Observe(ownership.Readable, []byte("claude\n"))
	if read.State != ownership.Readable || !read.Known() || !read.Targets.Contains("claude") {
		t.Errorf("a readable record = %+v, want its names", read)
	}
	for _, state := range []ownership.State{ownership.Absent, ownership.Unreadable} {
		got := ownership.Observe(state, []byte("claude\n"))
		if !got.Targets.Empty() {
			t.Errorf("Observe(%v, content) holds %q, want no name: only a record that was read vouches for any", state, got.Targets.Names())
		}
	}
	if ownership.Observe(ownership.Absent, nil).Known() {
		t.Error("an absent record is known")
	}
	if !ownership.Observe(ownership.Unreadable, nil).Known() {
		t.Error("a record that exists and cannot be read is not unknown: it is there")
	}
}

// observation builds a record as found: a state and the names it holds.
func observation(state ownership.State, names ...string) ownership.Observation {
	return ownership.Observation{State: state, Targets: ownership.Parse([]byte(strings.Join(names, "\n") + "\n"))}
}

func TestSettleKeepsTheRegistrationRecordAndTheBinaryUnlessNothingDependsOnThem(t *testing.T) {
	absent := ownership.Observation{State: ownership.Absent}
	unreadable := ownership.Observation{State: ownership.Unreadable}
	empty := ownership.Observation{State: ownership.Readable}
	holding := observation(ownership.Readable, "claude")
	for _, tc := range []struct {
		name string
		in   ownership.Intent
		obs  ownership.Observation
		want ownership.Verdict
	}{
		{"a record that holds nothing", ownership.Intent{}, empty, ownership.Retire},
		{"a record that holds a target", ownership.Intent{}, holding, ownership.StillInstalled},
		{"no record is not an empty one", ownership.Intent{}, absent, ownership.NoRecord},
		{"a record nobody can read", ownership.Intent{}, unreadable, ownership.UnreadableRecord},
		{"an unregister that failed, over a record that holds nothing", ownership.Intent{UnregisterFailed: true}, empty, ownership.FailedUnregister},
		{"an unregister that failed, over a record that holds a target", ownership.Intent{UnregisterFailed: true}, holding, ownership.FailedUnregister},
		{"an unregister that failed, over no record", ownership.Intent{UnregisterFailed: true}, absent, ownership.FailedUnregister},
		{"an unregister that failed outranks an unreadable record", ownership.Intent{UnregisterFailed: true}, unreadable, ownership.FailedUnregister},
		{"purge over a record that holds a target", ownership.Intent{Purge: true}, holding, ownership.Retire},
		{"purge over no record", ownership.Intent{Purge: true}, absent, ownership.Retire},
		{"purge over a record nobody can read", ownership.Intent{Purge: true}, unreadable, ownership.Retire},
		{"purge through an unregister that failed", ownership.Intent{Purge: true, UnregisterFailed: true}, holding, ownership.Retire},
		{"purge through an unregister that failed over an unreadable record", ownership.Intent{Purge: true, UnregisterFailed: true}, unreadable, ownership.Retire},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ownership.Settle(tc.in, tc.obs); got != tc.want {
				t.Errorf("Settle(%+v, state %v with %q) = %v, want %v", tc.in, tc.obs.State, tc.obs.Targets.Names(), got, tc.want)
			}
		})
	}
}

func TestDisposeRemovesTheBinaryOnlyWhenPurgedOrWhenTheRecordIsThereAndEmpty(t *testing.T) {
	absent := ownership.Observation{State: ownership.Absent}
	unreadable := ownership.Observation{State: ownership.Unreadable}
	empty := ownership.Observation{State: ownership.Readable}
	holding := observation(ownership.Readable, "opencode")
	for _, tc := range []struct {
		name  string
		purge bool
		obs   ownership.Observation
		want  ownership.Disposal
	}{
		{"a record that is there and holds nothing", false, empty, ownership.DisposeOfBinary},
		{"a record that holds a target", false, holding, ownership.KeepBinary},
		{"no record: whether anything depends on the binary is not known", false, absent, ownership.KeepBinary},
		{"a record nobody can read", false, unreadable, ownership.HoldBinaryUnreadable},
		{"purge over a record that holds a target", true, holding, ownership.DisposeOfBinary},
		{"purge over no record", true, absent, ownership.DisposeOfBinary},
		{"purge over a record nobody can read", true, unreadable, ownership.DisposeOfBinary},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ownership.Dispose(tc.purge, tc.obs); got != tc.want {
				t.Errorf("Dispose(purge=%v, state %v with %q) = %v, want %v", tc.purge, tc.obs.State, tc.obs.Targets.Names(), got, tc.want)
			}
		})
	}
}
