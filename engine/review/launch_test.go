package review_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

func TestCheckModelRefusesWhatCodexCouldReadAsAnOption(t *testing.T) {
	for _, model := range []string{"", "-m", "--x", "a\nb", "a\x00b"} {
		if err := review.CheckModel(model); !errors.Is(err, review.ErrInvalidModel) {
			t.Errorf("CheckModel(%q) = %v, want an error wrapping ErrInvalidModel", model, err)
		}
	}
	for _, model := range []string{"gpt-5.5", "o4-mini"} {
		if err := review.CheckModel(model); err != nil {
			t.Errorf("CheckModel(%q) = %v, want nil", model, err)
		}
	}
}

func TestCheckBriefNamesTheFirstPartOfTheBindingItLacks(t *testing.T) {
	c := review.Candidate{
		Base:       strings.Repeat("a", 40),
		Head:       strings.Repeat("b", 40),
		DiffSHA256: strings.Repeat("c", 64),
	}
	const worktree = "/work/main-review/bbbbbbbbbbbb"
	parts := []struct{ name, value string }{
		{"base", c.Base}, {"head", c.Head}, {"diff_sha256", c.DiffSHA256}, {"worktree", worktree},
	}
	whole := strings.Join([]string{c.Base, c.Head, c.DiffSHA256, worktree}, "\n")
	if err := review.CheckBrief(c, worktree, []byte(whole)); err != nil {
		t.Fatalf("CheckBrief(a brief holding all four) = %v, want nil", err)
	}
	for _, part := range parts {
		brief := strings.Replace(whole, part.value, "", 1)
		want := "the brief does not name the " + part.name + " " + part.value + " of the binding, so it is not the brief of this candidate"
		err := review.CheckBrief(c, worktree, []byte(brief))
		if err == nil || err.Error() != want {
			t.Errorf("CheckBrief(without the %s) = %v, want exactly %q", part.name, err, want)
		}
		if !errors.Is(err, review.ErrInvalidBrief) {
			t.Errorf("CheckBrief(without the %s) = %v, want an error wrapping ErrInvalidBrief", part.name, err)
		}
	}
	for name, brief := range map[string][]byte{
		"empty":     nil,
		"not UTF-8": []byte("\xff"),
		"too large": append([]byte(whole), bytes.Repeat([]byte("x"), review.MaxReplyBytes)...),
	} {
		if err := review.CheckBrief(c, worktree, brief); !errors.Is(err, review.ErrInvalidBrief) {
			t.Errorf("CheckBrief(%s) = %v, want an error wrapping ErrInvalidBrief", name, err)
		}
	}
	exact := append([]byte(whole), bytes.Repeat([]byte("x"), review.MaxReplyBytes-len(whole))...)
	if err := review.CheckBrief(c, worktree, exact); err != nil {
		t.Errorf("CheckBrief(MaxReplyBytes bytes) = %v, want nil", err)
	}
	if err := review.CheckBrief(c, worktree, append(exact, 'x')); !errors.Is(err, review.ErrInvalidBrief) {
		t.Errorf("CheckBrief(MaxReplyBytes+1 bytes) = %v, want an error wrapping ErrInvalidBrief", err)
	}
}

func TestOverLimitErrorSaysHowToSplit(t *testing.T) {
	err := review.OverLimitError(review.Candidate{DiffBytes: 150001})
	const want = "the diff is 150001 bytes, over the 150000-byte limit of one launch; split the candidate into one launch per commit (skills/role-chain/SKILL.md)"
	if err == nil || err.Error() != want {
		t.Errorf("OverLimitError() = %v, want exactly %q", err, want)
	}
	if !errors.Is(err, review.ErrOverLimit) {
		t.Errorf("OverLimitError() = %v, want an error wrapping ErrOverLimit", err)
	}
}
