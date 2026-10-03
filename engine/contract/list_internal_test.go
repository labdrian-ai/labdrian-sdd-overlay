package contract

import (
	"errors"
	"reflect"
	"testing"
)

// parseList is the one parser of a list in the frontmatter. These pin the rules of an item.
func TestParseList(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  []string
	}{
		{"one item", "[a]", []string{"a"}},
		{"several items", "[a, b, c]", []string{"a", "b", "c"}},
		{"white space around the value", "  [a, b]  ", []string{"a", "b"}},
		{"white space around the items", "[  a  ,\tb ]", []string{"a", "b"}},
		{"the empty list", "[]", nil},
		{"a list of white space", "[   ]", nil},
		{"empty items are dropped", "[a,,b, ,]", []string{"a", "b"}},
		{"double quotes are stripped", `["a", "b"]`, []string{"a", "b"}},
		{"single quotes are stripped", `['a', 'b']`, []string{"a", "b"}},
		{"quotes after the white space", `[ "a" , 'b' ]`, []string{"a", "b"}},
		{"every quote at either end is stripped", `["'a'"]`, []string{"a"}},
		{"an item that is only quotes is empty", `["", '']`, nil},
		{"the case is kept", "[Go, TypeScript]", []string{"Go", "TypeScript"}},
		{"a hyphenated name", "[sdd-apply]", []string{"sdd-apply"}},
		{"a comma inside quotes still splits", `["a, b"]`, []string{"a", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseList("key", tc.value)
			if err != nil {
				t.Fatalf("parseList(%q) = %v", tc.value, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseList(%q) = %#v, want %#v", tc.value, got, tc.want)
			}
		})
	}
}

func TestParseListRefusesWhatIsNotBracketed(t *testing.T) {
	for _, value := range []string{"", "   ", "a", "a, b", "[a, b", "a, b]", "(a, b)", "{a, b}", "- a", "[a] extra", "extra [a]", `"[a]"`,
		// A bracket inside the list is not an item: the value is one list, brackets at the ends only.
		"[a][b]", "[]]", "[[a]]", "[a, [b]]", "[a, b]]", "[[a, b]",
		// Quotes do not make a bracket an item's own: the list has no escape for one, so a
		// quoted bracket is refused as an unquoted one is (nothing a contract names has one;
		// the refusal is loud: 'gate-task' says so on stderr and lets the call through).
		`["a[1]"]`, `["a]"]`, `['[a']`, `["a", "b[1]"]`, `["[a]"]`, `['a', "]"]`} {
		got, err := parseList("the_key", value)
		var malformed *MalformedListError
		if !errors.As(err, &malformed) || malformed.Key != "the_key" || got != nil {
			t.Errorf("parseList(%q) = %#v, %v, want a *MalformedListError for the_key and no items", value, got, err)
		}
	}
}
