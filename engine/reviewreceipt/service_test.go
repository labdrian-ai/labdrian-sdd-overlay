package reviewreceipt_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt/receipttest"
)

// world is every port of the service answered from memory. It records the order of the
// questions it was asked, so a test can say that the service asked nothing it did not need.
type world struct {
	stores    []reviewreceipt.Store
	storesErr error
	// docs and docsErr are what each store holds, or why it cannot be read.
	docs    map[reviewreceipt.Store][]reviewreceipt.Document
	docsErr map[reviewreceipt.Store]error

	// changes is the directories under openspec/changes, artifacts what each one holds.
	changes    []string
	changesErr error
	artifacts  map[string][]string

	// persisted is the bytes under "<change>/<name>"; the errors are what Read and Write
	// answer for a key.
	persisted map[string][]byte
	readErr   map[string]error
	writeErr  map[string]error

	calls  []string
	writes []string
}

func newWorld() *world {
	return &world{
		docs: map[reviewreceipt.Store][]reviewreceipt.Document{}, docsErr: map[reviewreceipt.Store]error{},
		artifacts: map[string][]string{}, persisted: map[string][]byte{},
		readErr: map[string]error{}, writeErr: map[string]error{},
	}
}

func (w *world) service() *reviewreceipt.Service {
	return reviewreceipt.NewService(reviewreceipt.Ports{Stores: w, Source: w, Sink: w, Changes: w})
}

func (w *world) Stores() ([]reviewreceipt.Store, error) {
	w.calls = append(w.calls, "Stores")
	return w.stores, w.storesErr
}

func (w *world) Documents(store reviewreceipt.Store) ([]reviewreceipt.Document, error) {
	w.calls = append(w.calls, "Documents "+string(store))
	if err := w.docsErr[store]; err != nil {
		return nil, err
	}
	return w.docs[store], nil
}

func (w *world) Changes() ([]string, error) {
	w.calls = append(w.calls, "Changes")
	return w.changes, w.changesErr
}

func (w *world) HasArtifact(change, name string) bool {
	w.calls = append(w.calls, "HasArtifact "+change+"/"+name)
	for _, a := range w.artifacts[change] {
		if a == name {
			return true
		}
	}
	return false
}

func key(change, name string) string { return change + "/" + name }

func (w *world) Read(change, name string) ([]byte, error) {
	w.calls = append(w.calls, "Read "+key(change, name))
	if err := w.readErr[key(change, name)]; err != nil {
		return nil, err
	}
	data, ok := w.persisted[key(change, name)]
	if !ok {
		return nil, fmt.Errorf("read %s: %w", w.Location(change, name), reviewreceipt.ErrNotPersisted)
	}
	return data, nil
}

func (w *world) Write(change, name string, data []byte) error {
	w.calls = append(w.calls, "Write "+key(change, name))
	if err := w.writeErr[key(change, name)]; err != nil {
		return err
	}
	w.persisted[key(change, name)] = data
	w.writes = append(w.writes, key(change, name))
	return nil
}

func (w *world) Location(change, name string) string {
	return "/project/openspec/changes/" + change + "/review-receipts/" + name
}

// the documents a store holds
func receiptDoc(lineage, schema, terminal string) reviewreceipt.Document {
	return reviewreceipt.Document{Shape: reviewreceipt.ShapeReceipt, Data: []byte(receipttest.ReceiptDocument(lineage, schema, terminal))}
}

func approvedReceiptDoc(lineage string) reviewreceipt.Document {
	return receiptDoc(lineage, receipttest.ReceiptSchema, receipttest.Approved)
}

func stateDoc(lineage, state string) reviewreceipt.Document {
	return reviewreceipt.Document{Shape: reviewreceipt.ShapeState, Data: []byte(receipttest.StateDocument(lineage, state))}
}

func (w *world) calledNothing(t *testing.T) {
	t.Helper()
	if len(w.calls) != 0 {
		t.Errorf("the service asked %v, want it to ask nothing", w.calls)
	}
}

func lineages(captured []reviewreceipt.Captured) []string {
	var out []string
	for _, c := range captured {
		out = append(out, c.LineageID)
	}
	return out
}

// Capture persists each approved receipt once, in the order the stores list them (store by
// store, lineage by lineage, the legacy receipt before the lifecycle state), under the name
// its shape gives it, and reports where each is.
func TestCapturePersistsEachApprovedReceiptOnceInStoreOrder(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"private", "common"}
	w.docs["private"] = []reviewreceipt.Document{approvedReceiptDoc("review-a"), stateDoc("review-a", "approved"), approvedReceiptDoc("review-shared")}
	w.docs["common"] = []reviewreceipt.Document{approvedReceiptDoc("review-shared"), stateDoc("review-b", "approved")}

	captured, err := w.service().Capture("my-change")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	wantWrites := []string{"my-change/review-a.json", "my-change/review-a.review-state.json", "my-change/review-shared.json", "my-change/review-b.review-state.json"}
	if !reflect.DeepEqual(w.writes, wantWrites) {
		t.Errorf("writes = %v, want %v", w.writes, wantWrites)
	}
	if want := []string{"review-a", "review-a", "review-shared", "review-b"}; !reflect.DeepEqual(lineages(captured), want) {
		t.Errorf("captured lineages = %v, want %v", lineages(captured), want)
	}
	if want := w.Location("my-change", "review-a.review-state.json"); captured[1].Path != want {
		t.Errorf("captured[1].Path = %q, want where the sink says it is, %q", captured[1].Path, want)
	}
	if string(w.persisted["my-change/review-shared.json"]) != string(approvedReceiptDoc("review-shared").Data) {
		t.Error("the persisted receipt is not the exact bytes of the document")
	}
}

// The two shapes of one lineage are two receipts, and the same shape of one lineage in two
// stores is one.
func TestCaptureCountsALineageOncePerShape(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"a", "b"}
	w.docs["a"] = []reviewreceipt.Document{approvedReceiptDoc("review-x"), stateDoc("review-x", "approved")}
	w.docs["b"] = []reviewreceipt.Document{approvedReceiptDoc("review-x"), stateDoc("review-x", "approved")}

	captured, err := w.service().Capture("c")
	if err != nil || len(captured) != 2 {
		t.Fatalf("Capture = %v, %v, want two receipts, one per shape", lineages(captured), err)
	}
}

// Only an approved receipt of a known schema that names its lineage is worth keeping.
func TestCaptureSkipsWhatIsNotAnApprovedReceipt(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"s"}
	w.docs["s"] = []reviewreceipt.Document{
		receiptDoc("review-v1", "gentle-ai.review-receipt/v1", "approved"),
		receiptDoc("review-declined", receipttest.ReceiptSchema, "declined"),
		receiptDoc("", receipttest.ReceiptSchema, "approved"),
		stateDoc("review-reviewing", "reviewing"),
		stateDoc("", "approved"),
		{Shape: reviewreceipt.ShapeReceipt, Data: []byte("not json")},
		{Shape: reviewreceipt.ShapeState, Data: []byte("{}")},
		approvedReceiptDoc("review-ok"),
	}

	captured, err := w.service().Capture("c")
	if err != nil || !reflect.DeepEqual(lineages(captured), []string{"review-ok"}) {
		t.Fatalf("Capture = %v, %v, want only review-ok", lineages(captured), err)
	}
}

// What is already persisted, byte for byte, is left alone and still reported; a different
// file at the name is never replaced, and what came before it stays captured.
func TestCaptureIsIdempotentAndNeverReplacesADifferingFile(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"s"}
	w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-a"), approvedReceiptDoc("review-b"), approvedReceiptDoc("review-c")}
	w.persisted["c/review-a.json"] = approvedReceiptDoc("review-a").Data
	w.persisted["c/review-b.json"] = []byte(`{"different":true}`)

	captured, err := w.service().Capture("c")

	wantErr := "reviewreceipt: " + w.Location("c", "review-b.json") + " already exists with different content"
	if err == nil || err.Error() != wantErr {
		t.Fatalf("Capture error = %v, want %q", err, wantErr)
	}
	if !reflect.DeepEqual(lineages(captured), []string{"review-a"}) {
		t.Errorf("captured = %v, want the one that was already there, reported", lineages(captured))
	}
	if len(w.writes) != 0 {
		t.Errorf("wrote %v; the identical file is left alone and the differing one is never replaced", w.writes)
	}
	if string(w.persisted["c/review-b.json"]) != `{"different":true}` {
		t.Error("the differing file was replaced")
	}
	for _, call := range w.calls {
		if call == "Read c/review-c.json" {
			t.Errorf("the service went on to the next receipt after a refusal: %v", w.calls)
		}
	}
}

// A failure of the sink is reported in the sink's words behind the package's name, and
// nothing after it is written.
func TestCaptureStopsAtTheFirstFailureOfTheSink(t *testing.T) {
	for _, tc := range []struct {
		name string
		arm  func(w *world)
		want string
	}{
		{"a read", func(w *world) { w.readErr["c/review-b.json"] = errors.New("read x: boom") }, "reviewreceipt: read x: boom"},
		{"a write", func(w *world) { w.writeErr["c/review-b.json"] = errors.New("create y: no space") }, "reviewreceipt: create y: no space"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld()
			w.stores = []reviewreceipt.Store{"s"}
			w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-a"), approvedReceiptDoc("review-b"), approvedReceiptDoc("review-c")}
			tc.arm(w)

			captured, err := w.service().Capture("c")
			if err == nil || err.Error() != tc.want {
				t.Fatalf("Capture error = %v, want %q", err, tc.want)
			}
			if !reflect.DeepEqual(lineages(captured), []string{"review-a"}) || !reflect.DeepEqual(w.writes, []string{"c/review-a.json"}) {
				t.Errorf("captured %v and wrote %v, want only review-a", lineages(captured), w.writes)
			}
		})
	}
}

// Stores are read one at a time and what one holds is persisted before the next is read, so
// a store that cannot be read stops the capture with the stores before it already kept.
func TestCaptureReadsAStoreOnlyWhenItGetsToIt(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"first", "second", "third"}
	w.docs["first"] = []reviewreceipt.Document{approvedReceiptDoc("review-a")}
	w.docsErr["second"] = errors.New("read second: permission denied")

	captured, err := w.service().Capture("c")

	if err == nil || err.Error() != "reviewreceipt: read second: permission denied" {
		t.Fatalf("Capture error = %v", err)
	}
	if !reflect.DeepEqual(lineages(captured), []string{"review-a"}) {
		t.Errorf("captured = %v, want what the first store held", lineages(captured))
	}
	for _, call := range w.calls {
		if call == "Documents third" {
			t.Error("the third store was read after the second failed")
		}
	}
}

func TestCaptureReportsStoresThatCannotBeListed(t *testing.T) {
	w := newWorld()
	w.storesErr = errors.New("git rev-parse --show-toplevel: exit status 128")

	captured, err := w.service().Capture("c")
	if err == nil || err.Error() != "reviewreceipt: git rev-parse --show-toplevel: exit status 128" || captured != nil {
		t.Fatalf("Capture = %v, %v", captured, err)
	}
}

// A blank change name is refused before anything is asked.
func TestCaptureRequiresAChangeName(t *testing.T) {
	for _, change := range []string{"", "  ", "\t\n"} {
		w := newWorld()
		captured, err := w.service().Capture(change)
		if err == nil || err.Error() != "reviewreceipt: change name is required" || captured != nil {
			t.Errorf("Capture(%q) = %v, %v, want the refusal", change, captured, err)
		}
		w.calledNothing(t)
	}
}

// A change name that is not one path component is refused before anything is asked, and the
// refusal names it: the receipts folder is joined from it, so a name with a separator or a
// dot segment would put a receipt outside the folder, and one with white space at an end
// would put it in a sibling that is not the change the caller meant.
func TestCaptureRefusesAChangeNameThatIsNotOnePathComponent(t *testing.T) {
	for _, change := range []string{"../x", "a/b", "/etc", "..", ".", " padded", "padded ", "padded\n", `a\b`} {
		w := newWorld()
		captured, err := w.service().Capture(change)
		var unsafe *reviewreceipt.UnsafeNameError
		if !errors.As(err, &unsafe) || unsafe.Kind != "change name" || unsafe.Name != change || captured != nil {
			t.Errorf("Capture(%q) = %v, %v, want an *UnsafeNameError for the change name", change, captured, err)
		}
		if err != nil && !strings.HasPrefix(err.Error(), "reviewreceipt: change name ") {
			t.Errorf("Capture(%q) error = %q, want it to name the change name", change, err)
		}
		w.calledNothing(t)
	}
}

// The name a receipt is persisted under is made of the lineage id the review tool wrote,
// which is read from a document in a directory another program owns. A lineage that would
// put the file outside the folder is refused, before the sink is asked to read or write it;
// every other approved receipt, before or after it, is still persisted, and the capture then
// fails naming the refused one, so one bad document blocks nothing else.
func TestCaptureRefusesAReceiptWhoseFileNameIsNotOnePathComponent(t *testing.T) {
	for _, lineage := range []string{"../../escaped", "a/b", `a\b`, " padded", "line\nbreak"} {
		w := newWorld()
		w.stores = []reviewreceipt.Store{"s"}
		w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-fine"), approvedReceiptDoc(lineage), approvedReceiptDoc("review-after")}

		captured, err := w.service().Capture("c1")
		var unsafe *reviewreceipt.UnsafeNameError
		if !errors.As(err, &unsafe) || unsafe.Kind != "receipt file name" || unsafe.Name != lineage+".json" {
			t.Errorf("lineage %q: Capture error = %v, want an *UnsafeNameError for the receipt file name", lineage, err)
		}
		if got := lineages(captured); !reflect.DeepEqual(got, []string{"review-fine", "review-after"}) {
			t.Errorf("lineage %q: captured %v, want every receipt but the refused one", lineage, got)
		}
		for _, call := range w.calls {
			if strings.Contains(call, lineage+".json") {
				t.Errorf("lineage %q: the sink was asked %q", lineage, call)
			}
		}
		if !reflect.DeepEqual(w.writes, []string{"c1/review-fine.json", "c1/review-after.json"}) {
			t.Errorf("lineage %q: wrote %v, want every receipt but the refused one", lineage, w.writes)
		}
	}
}

// A visit that fails after a receipt was refused still stops the walk, and its error keeps
// every refusal collected before it: none is dropped because another failure came later.
func TestAFailedVisitKeepsTheRefusalsCollectedBeforeIt(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"s"}
	bad := approvedReceiptDoc("../one")
	bad.Origin = "s/review-one/review-receipt.json"
	w.docs["s"] = []reviewreceipt.Document{bad, approvedReceiptDoc("review-b"), approvedReceiptDoc("review-c")}
	w.writeErr["c1/review-b.json"] = errors.New("create y: no space")

	_, err := w.service().Capture("c1")
	var unusable *reviewreceipt.UnusableReceiptError
	if !errors.As(err, &unusable) || unusable.Origin != bad.Origin {
		t.Errorf("Capture error = %v, want it to keep the refusal of %s", err, bad.Origin)
	}
	if err == nil || !strings.Contains(err.Error(), "create y: no space") {
		t.Errorf("Capture error = %v, want it to keep the failed write", err)
	}
	if strings.Contains(strings.Join(w.writes, " "), "review-c") {
		t.Errorf("wrote %v, want the walk stopped at the failed write", w.writes)
	}
}

// Every refused receipt is named, not only the first, so one run tells a person all that has
// to be corrected.
func TestCaptureNamesEveryRefusedReceipt(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"s"}
	first, second := approvedReceiptDoc("../one"), approvedReceiptDoc("a/two")
	first.Origin, second.Origin = "s/review-one/review-receipt.json", "s/review-two/review-receipt.json"
	w.docs["s"] = []reviewreceipt.Document{first, approvedReceiptDoc("review-fine"), second}

	captured, err := w.service().Capture("c1")
	if got := lineages(captured); !reflect.DeepEqual(got, []string{"review-fine"}) {
		t.Errorf("captured %v, want the one receipt that can be kept", got)
	}
	for _, origin := range []string{first.Origin, second.Origin} {
		if err == nil || !strings.Contains(err.Error(), "the approved receipt in "+origin+" cannot be kept") {
			t.Errorf("Capture error = %v, want it to name %s", err, origin)
		}
	}
}

// The refusal stops every capture until the document is dealt with, so it names the document
// as its source reported it and says how to recover. It stays an *UnsafeNameError underneath.
func TestARefusedApprovedReceiptNamesItsDocumentAndTheRemedy(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"s"}
	bad := approvedReceiptDoc("../out")
	bad.Origin = "s/review-bad/review-receipt.json"
	w.docs["s"] = []reviewreceipt.Document{bad}

	_, err := w.service().Capture("c1")
	var unusable *reviewreceipt.UnusableReceiptError
	var unsafe *reviewreceipt.UnsafeNameError
	if !errors.As(err, &unusable) || unusable.Origin != bad.Origin || !errors.As(err, &unsafe) {
		t.Fatalf("Capture error = %v, want an *UnusableReceiptError for %s wrapping an *UnsafeNameError", err, bad.Origin)
	}
	want := `reviewreceipt: the approved receipt in s/review-bad/review-receipt.json cannot be kept: reviewreceipt: receipt file name "../out.json" is not a safe path component: it holds a path separator; correct or remove that lineage in the review tool's store, then run the command again`
	if err.Error() != want {
		t.Errorf("Capture error =\n%s\nwant\n%s", err, want)
	}
	if _, err := w.service().AllSurvivingApprovedPersisted([]string{"c1"}); !errors.As(err, &unusable) || unusable.Origin != bad.Origin {
		t.Errorf("AllSurvivingApprovedPersisted error = %v, want the same *UnusableReceiptError", err)
	}
}

// The survey that lets a retried acknowledgement through reads the sink too, so it refuses
// the same names, and an unsafe change in its list is not asked about.
func TestAllSurvivingApprovedPersistedRefusesNamesThatAreNotOnePathComponent(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"s"}
	w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("../out")}
	got, err := w.service().AllSurvivingApprovedPersisted([]string{"c1"})
	var unsafe *reviewreceipt.UnsafeNameError
	if !errors.As(err, &unsafe) || unsafe.Kind != "receipt file name" || got {
		t.Errorf("unsafe lineage: AllSurvivingApprovedPersisted = %v, %v, want a refusal", got, err)
	}

	w = newWorld()
	w.stores = []reviewreceipt.Store{"s"}
	w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-a")}
	got, err = w.service().AllSurvivingApprovedPersisted([]string{"c1", "../c2"})
	if !errors.As(err, &unsafe) || unsafe.Kind != "change name" || unsafe.Name != "../c2" || got {
		t.Errorf("unsafe change: AllSurvivingApprovedPersisted = %v, %v, want a refusal naming the change", got, err)
	}
	for _, call := range w.calls {
		if strings.Contains(call, "../c2") {
			t.Errorf("the sink was asked %q", call)
		}
	}
}

// The hook denies, naming the name, whichever way an unsafe name reaches the service: an
// acknowledgement it cannot capture safely is one it must not let burn the receipt.
func TestHookDeniesAnUnsafeNameInsteadOfWritingOutsideTheFolder(t *testing.T) {
	w := newWorld()
	w.changes, w.artifacts = []string{"only"}, map[string][]string{"only": {"tasks.md"}}
	w.stores = []reviewreceipt.Store{"s"}
	escaped := approvedReceiptDoc("../../escaped")
	escaped.Origin = "s/review-escaped/review-receipt.json"
	w.docs["s"] = []reviewreceipt.Document{escaped}

	v := w.service().CheckCommand(ackCommand)
	want := `review-receipt: capture failed: reviewreceipt: the approved receipt in s/review-escaped/review-receipt.json cannot be kept: reviewreceipt: receipt file name "../../escaped.json" is not a safe path component: it holds a path separator; correct or remove that lineage in the review tool's store, then run the command again`
	if !v.Deny || v.Reason != want {
		t.Errorf("CheckCommand = %+v, want a denial %q", v, want)
	}
	if len(w.writes) != 0 {
		t.Errorf("the hook wrote %v", w.writes)
	}

	w = newWorld()
	w.changes, w.artifacts = []string{"b", "a "}, map[string][]string{"a ": {"tasks.md"}, "b": {"tasks.md"}}
	w.stores = []reviewreceipt.Store{"s"}
	w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-1")}
	w.persisted["b/review-1.json"] = approvedReceiptDoc("review-1").Data
	v = w.service().CheckCommand(ackCommand)
	if !v.Deny || !strings.Contains(v.Reason, `change name "a "`) {
		t.Errorf("padded change among several: CheckCommand = %+v, want a denial naming the change", v)
	}
}

// DetectActiveChange: a directory under openspec/changes is an active change when it is
// not the archive and holds one of the SDD artifacts.
func TestDetectActiveChange(t *testing.T) {
	for _, tc := range []struct {
		name      string
		changes   []string
		artifacts map[string][]string
		want      string
		wantMulti []string
	}{
		{"no changes at all", nil, nil, "", nil},
		{"a directory with no artifact", []string{"stray"}, map[string][]string{"stray": {"notes.txt"}}, "", nil},
		{"the archive is never a change", []string{"archive"}, map[string][]string{"archive": {"tasks.md"}}, "", nil},
		{"one change among strays", []string{"stray", "mine"}, map[string][]string{"mine": {"tasks.md"}}, "mine", nil},
		{"several, reported in name order", []string{"zeta", "alpha", "archive"}, map[string][]string{"zeta": {"proposal.md"}, "alpha": {"entry.json"}, "archive": {"design.md"}}, "", []string{"alpha", "zeta"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld()
			w.changes, w.artifacts = tc.changes, tc.artifacts

			got, err := w.service().DetectActiveChange()
			var multi *reviewreceipt.MultipleActiveChangesError
			switch {
			case tc.wantMulti != nil:
				if !errors.As(err, &multi) || !reflect.DeepEqual(multi.Changes, tc.wantMulti) {
					t.Fatalf("DetectActiveChange = %q, %v, want the changes %v", got, err, tc.wantMulti)
				}
			case err != nil || got != tc.want:
				t.Fatalf("DetectActiveChange = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestEveryArtifactMarksAChange(t *testing.T) {
	for _, artifact := range []string{"tasks.md", "design.md", "proposal.md", "entry.json"} {
		w := newWorld()
		w.changes, w.artifacts = []string{"c"}, map[string][]string{"c": {artifact}}
		if got, err := w.service().DetectActiveChange(); err != nil || got != "c" {
			t.Errorf("a change holding only %s: DetectActiveChange = %q, %v, want it active", artifact, got, err)
		}
	}
	w := newWorld()
	w.changes, w.artifacts = []string{"c"}, map[string][]string{"c": {"state.yaml", "README.md"}}
	if got, _ := w.service().DetectActiveChange(); got != "" {
		t.Errorf("a change holding only state.yaml is active (%q): it is not an SDD artifact marker", got)
	}
}

func TestDetectActiveChangeReportsAListingThatFails(t *testing.T) {
	w := newWorld()
	w.changesErr = errors.New("read /project/openspec/changes: permission denied")
	if got, err := w.service().DetectActiveChange(); err == nil || err.Error() != "reviewreceipt: read /project/openspec/changes: permission denied" || got != "" {
		t.Errorf("DetectActiveChange = %q, %v", got, err)
	}
}

func TestMultipleActiveChangesErrorNamesTheRemedy(t *testing.T) {
	err := &reviewreceipt.MultipleActiveChangesError{Changes: []string{"a", "b"}}
	want := "reviewreceipt: multiple active changes (a, b); run `review-receipt capture --change <name>` before acknowledging"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

// AllSurvivingApprovedPersisted: every approved receipt that survives must be persisted
// byte for byte under some change; none surviving is true.
func TestAllSurvivingApprovedPersisted(t *testing.T) {
	a, b := approvedReceiptDoc("review-a"), stateDoc("review-b", "approved")
	for _, tc := range []struct {
		name      string
		persisted map[string][]byte
		want      bool
	}{
		{"nothing is persisted", nil, false},
		{"one of two is persisted", map[string][]byte{"c1/review-a.json": a.Data}, false},
		{"both, under different changes", map[string][]byte{"c1/review-a.json": a.Data, "c2/review-b.review-state.json": b.Data}, true},
		{"the right name with different bytes", map[string][]byte{"c1/review-a.json": []byte("{}"), "c2/review-b.review-state.json": b.Data}, false},
		{"the right bytes under the other shape's name", map[string][]byte{"c1/review-a.json": a.Data, "c1/review-b.json": b.Data}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld()
			w.stores = []reviewreceipt.Store{"s"}
			w.docs["s"] = []reviewreceipt.Document{a, b, receiptDoc("review-declined", receipttest.ReceiptSchema, "declined")}
			for k, v := range tc.persisted {
				w.persisted[k] = v
			}

			got, err := w.service().AllSurvivingApprovedPersisted([]string{"c1", "c2"})
			if err != nil || got != tc.want {
				t.Errorf("AllSurvivingApprovedPersisted = %v, %v, want %v", got, err, tc.want)
			}
		})
	}
}

func TestAllSurvivingApprovedPersistedIsTrueWhenNothingSurvives(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"s"}
	if got, err := w.service().AllSurvivingApprovedPersisted([]string{"c1"}); err != nil || !got {
		t.Errorf("AllSurvivingApprovedPersisted with no receipts = %v, %v, want true", got, err)
	}
	for _, call := range w.calls {
		if strings.HasPrefix(call, "Read ") {
			t.Errorf("asked the sink for %q although no receipt survives", call)
		}
	}
}

func TestAllSurvivingApprovedPersistedReportsWhatFails(t *testing.T) {
	for _, tc := range []struct {
		name string
		arm  func(w *world)
		want string
	}{
		{"stores", func(w *world) { w.storesErr = errors.New("no repository") }, "reviewreceipt: no repository"},
		{"a store", func(w *world) { w.docsErr["s"] = errors.New("read s: denied") }, "reviewreceipt: read s: denied"},
		{"a read", func(w *world) { w.readErr["c1/review-a.json"] = errors.New("read c1: denied") }, "reviewreceipt: read c1: denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld()
			w.stores = []reviewreceipt.Store{"s"}
			w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-a")}
			tc.arm(w)
			got, err := w.service().AllSurvivingApprovedPersisted([]string{"c1"})
			if err == nil || err.Error() != tc.want || got {
				t.Errorf("AllSurvivingApprovedPersisted = %v, %v, want a refusal %q", got, err, tc.want)
			}
		})
	}
}

const ackCommand = "gentle-ai review acknowledge-approved --cwd /repo --lineage review-x"

// A command that is not an acknowledgement, and the empty command of a call that has none, pass
// without the service asking anything: the check runs before every shell command.
func TestCheckCommandPassesWhatIsNotAnAcknowledgementWithoutAsking(t *testing.T) {
	for name, command := range map[string]string{
		"another command":               "git status",
		"no command":                    "",
		"white space":                   " \n\t",
		"the review tool, another verb": "gentle-ai review status --cwd /repo",
		"the verb in another case":      "GENTLE-AI REVIEW ACKNOWLEDGE-APPROVED",
		"the words in another order":    "gentle-ai acknowledge-approved review",
	} {
		w := newWorld()
		if v := w.service().CheckCommand(command); v.Deny || v.Reason != "" {
			t.Errorf("%s: CheckCommand = %+v, want an allow", name, v)
		}
		w.calledNothing(t)
	}
}

// Without an active change there is nothing to attach a receipt to: the hook allows, and it
// never asks where the stores are (so a directory that is not a repository costs nothing).
func TestHookAllowsWithoutAnActiveChangeAndNeverListsTheStores(t *testing.T) {
	w := newWorld()
	w.changes = []string{"stray"}
	w.storesErr = errors.New("not a repository")

	if v := w.service().CheckCommand(ackCommand); v.Deny || v.Reason != "" {
		t.Errorf("CheckCommand = %+v, want an allow", v)
	}
	for _, call := range w.calls {
		if call == "Stores" {
			t.Error("the hook asked for the stores although there is no change to capture into")
		}
	}
}

func TestHookCapturesIntoTheSingleActiveChange(t *testing.T) {
	w := newWorld()
	w.changes, w.artifacts = []string{"only"}, map[string][]string{"only": {"tasks.md"}}
	w.stores = []reviewreceipt.Store{"s"}
	w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-solo")}

	if v := w.service().CheckCommand(ackCommand); v.Deny || v.Reason != "" {
		t.Fatalf("CheckCommand = %+v, want an allow", v)
	}
	if !reflect.DeepEqual(w.writes, []string{"only/review-solo.json"}) {
		t.Errorf("writes = %v", w.writes)
	}
}

func TestHookDeniesWhenCaptureFails(t *testing.T) {
	w := newWorld()
	w.changes, w.artifacts = []string{"only"}, map[string][]string{"only": {"tasks.md"}}
	w.storesErr = errors.New("git rev-parse --show-toplevel: exit status 128")

	v := w.service().CheckCommand(ackCommand)
	if want := "review-receipt: capture failed: reviewreceipt: git rev-parse --show-toplevel: exit status 128"; !v.Deny || v.Reason != want {
		t.Errorf("CheckCommand = %+v, want a denial %q", v, want)
	}
}

func TestHookDeniesAnAmbiguousChangeUnlessEveryReceiptIsPersisted(t *testing.T) {
	setup := func() *world {
		w := newWorld()
		w.changes, w.artifacts = []string{"b", "a"}, map[string][]string{"a": {"tasks.md"}, "b": {"tasks.md"}}
		w.stores = []reviewreceipt.Store{"s"}
		w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-1")}
		return w
	}

	w := setup()
	v := w.service().CheckCommand(ackCommand)
	wantDeny := "reviewreceipt: multiple active changes (a, b); run `review-receipt capture --change <name>` before acknowledging"
	if !v.Deny || v.Reason != wantDeny {
		t.Errorf("unpersisted: CheckCommand = %+v, want a denial %q", v, wantDeny)
	}
	if len(w.writes) != 0 {
		t.Errorf("the hook guessed a change and wrote %v", w.writes)
	}

	w = setup()
	w.persisted["a/review-1.json"] = approvedReceiptDoc("review-1").Data
	if v := w.service().CheckCommand(ackCommand); v.Deny || v.Reason != "" {
		t.Errorf("persisted: CheckCommand = %+v, want an allow", v)
	}

	w = setup()
	w.storesErr = errors.New("no repository")
	if v := w.service().CheckCommand(ackCommand); !v.Deny || v.Reason != "review-receipt: reviewreceipt: no repository" {
		t.Errorf("survey fails: CheckCommand = %+v, want a denial %q", v, "review-receipt: reviewreceipt: no repository")
	}
}

func TestHookDeniesWhenTheChangesCannotBeListed(t *testing.T) {
	w := newWorld()
	w.changesErr = errors.New("read changes: denied")
	if v := w.service().CheckCommand(ackCommand); !v.Deny || v.Reason != "review-receipt: reviewreceipt: read changes: denied" {
		t.Errorf("CheckCommand = %+v", v)
	}
}

// A service that is missing a port cannot do its work, which it says, never by guessing: the
// hook denies an acknowledgement it cannot guard and still passes every other command.
func TestAServiceMissingAPortRefusesInsteadOfGuessing(t *testing.T) {
	w := newWorld()
	for _, svc := range []*reviewreceipt.Service{
		{},
		reviewreceipt.NewService(reviewreceipt.Ports{Source: w, Sink: w, Changes: w}),
		reviewreceipt.NewService(reviewreceipt.Ports{Stores: w, Sink: w, Changes: w}),
		reviewreceipt.NewService(reviewreceipt.Ports{Stores: w, Source: w, Changes: w}),
		reviewreceipt.NewService(reviewreceipt.Ports{Stores: w, Source: w, Sink: w}),
	} {
		if _, err := svc.Capture("c"); err == nil || !strings.Contains(err.Error(), "port") {
			t.Errorf("Capture = %v, want a refusal naming the missing port", err)
		}
		if _, err := svc.DetectActiveChange(); err == nil {
			t.Error("DetectActiveChange accepted a service with a missing port")
		}
		if _, err := svc.AllSurvivingApprovedPersisted([]string{"c"}); err == nil {
			t.Error("AllSurvivingApprovedPersisted accepted a service with a missing port")
		}
		if v := svc.CheckCommand(ackCommand); !v.Deny || !strings.HasPrefix(v.Reason, "review-receipt: ") {
			t.Errorf("CheckCommand of an acknowledgement = %+v, want a denial", v)
		}
		if v := svc.CheckCommand("ls"); v.Deny || v.Reason != "" {
			t.Errorf("CheckCommand of another command = %+v, want an allow", v)
		}
	}
	w.calledNothing(t)
}
