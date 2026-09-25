package shaper

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// unpresentableRune reports whether r must never appear in a presented view:
// a C0 control other than LF and TAB, DEL, a C1 control, or a Unicode format
// control (category Cf), which includes the bidirectional embedding,
// override, isolate, and mark controls and the zero-width characters. Each
// can hide, reorder, or disguise text on the human's terminal or editor while
// the view digest still binds the hidden bytes. Such content is refused, never
// rewritten: rewriting would present something other than the bound bytes.
func unpresentableRune(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return false
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	}
	return unicode.Is(unicode.Cf, r)
}

// checkPresentableText returns an error naming section and the rune offset
// of the first unpresentable rune in value, or of the first byte that is not
// valid UTF-8 (a lone 0x9b byte is an 8-bit CSI on some terminals).
func checkPresentableText(section string, value []byte) error {
	offset := 0
	for i := 0; i < len(value); offset++ {
		r, size := utf8.DecodeRune(value[i:])
		if r == utf8.RuneError && size == 1 {
			return fmt.Errorf("view section %s at rune offset %d holds byte 0x%02x that is not valid UTF-8; the view is unpresentable and is refused, not rewritten", section, offset, value[i])
		}
		if unpresentableRune(r) {
			return fmt.Errorf("view section %s at rune offset %d holds %U, a terminal control or invisible formatting character; the view is unpresentable and is refused, not rewritten", section, offset, r)
		}
		i += size
	}
	return nil
}

// checkPresentable refuses v when any section it presents holds an
// unpresentable rune, first section in render order wins. Sections are named
// goal, plan, goal_scope, goal_non_goals[i], out_of_scope[i], and
// flags[i].id, .kind, .field, or .item, with 0-based indexes.
func checkPresentable(v PresentedView) error {
	type section struct {
		name  string
		value []byte
	}
	sections := []section{
		{"goal", v.GoalBytes},
		{"plan", v.PlanBytes},
		{"goal_scope", []byte(v.GoalScope)},
	}
	for i, s := range v.GoalNonGoals {
		sections = append(sections, section{fmt.Sprintf("goal_non_goals[%d]", i), []byte(s)})
	}
	for i, s := range v.OutOfScope {
		sections = append(sections, section{fmt.Sprintf("out_of_scope[%d]", i), []byte(s)})
	}
	for i, f := range v.Flags {
		sections = append(sections,
			section{fmt.Sprintf("flags[%d].id", i), []byte(f.ID)},
			section{fmt.Sprintf("flags[%d].kind", i), []byte(f.Kind)},
			section{fmt.Sprintf("flags[%d].field", i), []byte(f.Field)},
			section{fmt.Sprintf("flags[%d].item", i), []byte(f.Item)},
		)
	}
	for _, s := range sections {
		if err := checkPresentableText(s.name, s.value); err != nil {
			return err
		}
	}
	return nil
}

// checkViewBytesPresentable refuses rendered view bytes that hold any
// unpresentable rune. RenderView's own framing is printable ASCII plus LF, so
// this refuses exactly the views whose presented content checkPresentable
// refuses, and it also refuses view bytes RenderView did not produce.
func checkViewBytesPresentable(view []byte) error {
	return checkPresentableText("view", view)
}
