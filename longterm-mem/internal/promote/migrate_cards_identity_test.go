package promote

import "testing"

func TestWithStoreAgnosticIdentity(t *testing.T) {
	const head = "---\ntitle: \"T\"\n"
	const tail = "project: p\n---\n"
	cases := []struct{ name, block, want string }{
		{"a memorycard page is rewritten", head + "memorycard_id: 7\nmemorycard_type: learning\nmemorycard_revision: 2\n" + tail,
			head + "source: memorycard\nsource_id: 7\nsource_type: learning\nsource_revision: 2\n" + tail},
		{"a store-agnostic page is left", head + "source: memorycard\nsource_id: 7\nsource_type: learning\nsource_revision: 2\n" + tail,
			head + "source: memorycard\nsource_id: 7\nsource_type: learning\nsource_revision: 2\n" + tail},
		{"an Engram page is left", head + "engram_id: 7\nengram_type: learning\nengram_revision: 2\n" + tail,
			head + "engram_id: 7\nengram_type: learning\nengram_revision: 2\n" + tail},
		{"an id that is no number is left", head + "memorycard_id: seven\nmemorycard_revision: 2\n" + tail,
			head + "memorycard_id: seven\nmemorycard_revision: 2\n" + tail},
		{"an indented identity key is left", head + "memorycard_id: 7\n  memorycard_type: learning\n" + tail,
			head + "memorycard_id: 7\n  memorycard_type: learning\n" + tail},
		{"a value that runs onto the next line is left", head + "memorycard_id: 7\nmemorycard_type: >\n  learning\n" + tail,
			head + "memorycard_id: 7\nmemorycard_type: >\n  learning\n" + tail},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withStoreAgnosticIdentity(c.block); got != c.want {
				t.Errorf("got\n%s\nwant\n%s", got, c.want)
			}
		})
	}
}
