package memorycard

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// hashFixture is kinFixture plus memories whose stored content hash, the one memorycard computed, decides a mirror:
//
//   - Observation 950 is memory 60, "Pinned Decision" / "A decision somebody pinned."; memory 61 is a hand mirror of
//     it that moved the words "Decision" from the title to the text, which memorycard hashes the same (its hash joins
//     the title and the text and splits on white space) and ContentKey, which keeps the title apart, does not.
//   - Observation 951 was merged by the importer into memory 62, whose own title is "Shared title" while the
//     observation's was "Shared": memorycard keeps the title of the memory that was there. Memory 63 is the hand
//     mirror that kept the observation's own title and text, "Shared" / "title w x".
//   - Observation 952 is memory 64 ("a" / "bc") and 65 is a hand mirror titled "ab" with the text "c": different
//     words to memorycard's hash, so no mirror.
//   - Memories 67 (another project's import of the text of hand mirror 68) and 69 (mapped by another source, the text
//     of hand mirror 70) are not mirrors either.
func hashFixture(t *testing.T) string {
	t.Helper()
	path := kinFixture(t)
	h := memory.CardContentHash
	writeFixture(t, path,
		fmt.Sprintf(`insert into memories(id, project, type, title, body, status, content_hash) values
			(60, 'p', 'decision', 'Pinned Decision', 'A decision somebody pinned.', 'active', '%s'),
			(61, 'p', 'decision', 'Pinned', 'Decision A decision somebody pinned.', 'active', '%s'),
			(62, 'p', 'decision', 'Shared title', 'w x', 'active', '%s'),
			(63, 'p', 'decision', 'Shared', 'title w x', 'active', '%s'),
			(64, 'p', 'decision', 'a', 'bc', 'active', '%s'),
			(65, 'p', 'decision', 'ab', 'c', 'active', '%s'),
			(67, 'other', 'decision', 'Elsewhere', 'elsewhere body', 'active', '%s'),
			(68, 'p', 'decision', 'Elsewhere', 'elsewhere body', 'active', '%s'),
			(69, 'p', 'decision', 'Noted', 'noted body', 'active', '%s'),
			(70, 'p', 'decision', 'Noted', 'noted body', 'active', '%s')`,
			h("Pinned Decision", "A decision somebody pinned."), h("Pinned Decision", "A decision somebody pinned."),
			h("Shared", "title w x"), h("Shared", "title w x"),
			h("a", "bc"), h("ab", "c"),
			h("Elsewhere", "elsewhere body"), h("Elsewhere", "elsewhere body"),
			h("Noted", "noted body"), h("Noted", "noted body")),
		`insert into import_map values ('engram', '950', 1, 60), ('engram', '951', 1, 62), ('engram', '952', 1, 64),
			('engram', '953', 1, 67), ('notes', 'n9', 1, 69)`,
	)
	return path
}

// A hand mirror whose stored content hash is the stored content hash of an imported part is a mirror of that
// observation, whatever the way its title and text divide the words: memorycard hashes them as one run (S10 (c), "the
// memories of its project with the same content hash"). The hash is memorycard's own, so a title and a text that
// trade characters inside a word do not share one and are no mirror of each other.
func TestSourceMirrorsAreTheObservationsWhoseImportedPartHasTheSameStoredHash(t *testing.T) {
	source := openSource(t, hashFixture(t))

	for id, want := range map[string][]memory.Identity{
		// The words moved from the title to the text.
		"61": {card("60"), engramObservation("950")},
		// The imported memory kept its own title when the importer merged the observation into it; the hand mirror
		// holds the observation's.
		"63": {card("62"), engramObservation("951")},
		// "ab" / "c" and "a" / "bc" are different words to memorycard's hash.
		"65": nil,
		// Another project's import and a memory mapped by another source hold the hash and are no observation of the
		// project.
		"68": nil,
		"70": nil,
	} {
		if got := memoryOf(t, source, id).Mirrored; !reflect.DeepEqual(got, want) {
			t.Errorf("memory %s mirrors = %v, want %v", id, got, want)
		}
	}
}

// A listing finds the same mirrors by hash as ByID does.
func TestSourceListingFindsTheMirrorsByStoredHashAsByIDDoes(t *testing.T) {
	source := openSource(t, hashFixture(t))

	all, err := source.Memories("p")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, listed := range all {
		if want := memoryOf(t, source, listed.Identity.ID); !reflect.DeepEqual(listed, want) {
			t.Errorf("listed memory %s = %+v, ByID says %+v", listed.Identity.ID, listed, want)
		}
		if listed.Identity.ID == "61" || listed.Identity.ID == "63" {
			found += len(listed.Mirrored)
		}
	}
	if found != 4 {
		t.Errorf("the listing found %d mirror identities for memories 61 and 63, want 4", found)
	}
}
