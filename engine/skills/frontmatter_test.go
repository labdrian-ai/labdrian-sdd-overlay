package skills

import (
	"errors"
	"strings"
	"testing"
)

func TestReadFrontmatterReturnsTheTopLevelEntriesInOrder(t *testing.T) {
	fm, err := ReadFrontmatter([]byte("---\nname: demo\ndescription: \"A demo.\"\ntools: '*'\n---\n# Body\nname: not-frontmatter\n"))
	if err != nil {
		t.Fatalf("ReadFrontmatter: %v", err)
	}
	var keys []string
	for _, e := range fm.Entries() {
		keys = append(keys, e.Key)
	}
	if got := strings.Join(keys, ","); got != "name,description,tools" {
		t.Fatalf("keys = %s, want name,description,tools", got)
	}
	name, ok := fm.Entry("name")
	if !ok || name.Value != "demo" || name.Text() != "demo" {
		t.Errorf("name = %+v (found %v)", name, ok)
	}
	description, _ := fm.Entry("description")
	if description.Value != `"A demo."` || description.Text() != "A demo." {
		t.Errorf("description = %+v, want the value as written and the text without its quotes", description)
	}
	tools, _ := fm.Entry("tools")
	if tools.Text() != "*" {
		t.Errorf("tools text = %q, want *", tools.Text())
	}
}

func TestReadFrontmatterToleratesABOMCRLFAndTrailingSpaceOnTheFences(t *testing.T) {
	fm, err := ReadFrontmatter([]byte("\xEF\xBB\xBF---  \r\nname: demo\r\n---\t\r\nbody\r\n"))
	if err != nil {
		t.Fatalf("ReadFrontmatter: %v", err)
	}
	if name, _ := fm.Entry("name"); name.Value != "demo" {
		t.Errorf("name = %q, want demo (no carriage return)", name.Value)
	}
}

func TestReadFrontmatterIgnoresKeysThatAreNotAtTheTopLevel(t *testing.T) {
	fm, err := ReadFrontmatter([]byte("---\nmetadata:\n  name: nested\n  author: someone\ndescription: top\n---\n"))
	if err != nil {
		t.Fatalf("ReadFrontmatter: %v", err)
	}
	if _, ok := fm.Entry("name"); ok {
		t.Error("a key indented under metadata was read as a top-level one")
	}
	if _, ok := fm.Entry("author"); ok {
		t.Error("a key indented under metadata was read as a top-level one")
	}
	if d, _ := fm.Entry("description"); d.Value != "top" {
		t.Errorf("description = %q, want top", d.Value)
	}
}

func TestReadFrontmatterCountsTheListItemsUnderAKey(t *testing.T) {
	cases := map[string]struct {
		text      string
		wantItems int
		wantValue string
	}{
		"indented list":   {"---\ntools:\n  - read\n  - write\n---\n", 2, ""},
		"unindented list": {"---\ntools:\n- read\n- write\n---\n", 2, ""},
		"inline":          {"---\ntools: [read, write]\n---\n", 0, "[read, write]"},
		"both":            {"---\ntools: '*'\n  - read\n---\n", 1, "'*'"},
		"a list after another key does not count": {"---\ntools: x\nmodel: y\n  - read\n---\n", 0, "x"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fm, err := ReadFrontmatter([]byte(c.text))
			if err != nil {
				t.Fatalf("ReadFrontmatter: %v", err)
			}
			tools, ok := fm.Entry("tools")
			if !ok {
				t.Fatal("no tools entry")
			}
			if tools.Items != c.wantItems || tools.Value != c.wantValue {
				t.Errorf("tools = {Value:%q Items:%d}, want {Value:%q Items:%d}", tools.Value, tools.Items, c.wantValue, c.wantItems)
			}
		})
	}
}

func TestReadFrontmatterIgnoresAListItemBeforeAnyKey(t *testing.T) {
	fm, err := ReadFrontmatter([]byte("---\n- stray\n  - also stray\nname: demo\n---\n"))
	if err != nil {
		t.Fatalf("ReadFrontmatter: %v", err)
	}
	if entries := fm.Entries(); len(entries) != 1 || entries[0].Key != "name" || entries[0].Items != 0 {
		t.Errorf("entries = %+v, want only name with no items", entries)
	}
}

func TestReadFrontmatterNamesAMissingFence(t *testing.T) {
	cases := map[string]struct {
		text string
		want FrontmatterFault
	}{
		"empty file":        {"", NoOpeningFence},
		"no opening":        {"name: demo\n---\n", NoOpeningFence},
		"an indented fence": {"  ---\nname: demo\n---\n", NoOpeningFence},
		"no closing":        {"---\nname: demo\n", NoClosingFence},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ReadFrontmatter([]byte(c.text))
			var fault *FrontmatterError
			if !errors.As(err, &fault) {
				t.Fatalf("err = %v, want a *FrontmatterError", err)
			}
			if fault.Fault != c.want {
				t.Errorf("fault = %v, want %v", fault.Fault, c.want)
			}
		})
	}
}

func TestReadFrontmatterAgreesWithSplitSkillFileOnWhatIsFrontmatter(t *testing.T) {
	inputs := []string{"---\nname: a\n---\nbody", "no fence", "---\nname: a\n", "\xEF\xBB\xBF---\nname: a\n---\n"}
	for _, in := range inputs {
		_, _, splitErr := SplitSkillFile([]byte(in))
		_, readErr := ReadFrontmatter([]byte(in))
		if (splitErr == nil) != (readErr == nil) {
			t.Errorf("%q: SplitSkillFile err=%v, ReadFrontmatter err=%v: the two disagree on where the frontmatter is", in, splitErr, readErr)
		}
	}
}
