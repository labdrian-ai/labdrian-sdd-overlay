package promote

import "testing"

// rewrittenIdentity puts the four store-agnostic keys where the first identity key was and takes every other
// identity key away, whichever family it belongs to, and touches no other line.
func TestRewrittenIdentity(t *testing.T) {
	for name, tc := range map[string]struct {
		block, revision, kind, want string
	}{
		"an engram page": {
			block:    "---\ntitle: \"T\"\nsources: []\nengram_id: 4\nengram_sync_id: s\nengram_type: note\nengram_revision: 2\nproject: p\n---\n",
			revision: "7", kind: "decision",
			want: "---\ntitle: \"T\"\nsources: []\nsource: memorycard\nsource_id: 9\nsource_type: decision\nsource_revision: 7\nproject: p\n---\n",
		},
		"a page that carries both spellings is rewritten once": {
			block:    "---\nsource: memorycard\nsource_id: 9\nmemorycard_id: 9\nmemorycard_revision: 3\nproject: p\n---\n",
			revision: "3", kind: "note",
			want: "---\nsource: memorycard\nsource_id: 9\nsource_type: note\nsource_revision: 3\nproject: p\n---\n",
		},
		"a value that is not known writes no line for it": {
			block:    "---\nmemorycard_id: 9\nproject: p\n---\n",
			revision: "", kind: "",
			want: "---\nsource: memorycard\nsource_id: 9\nproject: p\n---\n",
		},
		"keys that only start like an identity key are not identity keys": {
			block:    "---\nsources: []\nsource_note: x\nmemorycard_id: 9\n---\n",
			revision: "1", kind: "n",
			want: "---\nsources: []\nsource_note: x\nsource: memorycard\nsource_id: 9\nsource_type: n\nsource_revision: 1\n---\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := rewrittenIdentity(tc.block, 9, tc.revision, tc.kind); got != tc.want {
				t.Errorf("rewrittenIdentity =\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}
