package review_test

// The declared risk and the report an exemption is made on (own-rdd T4, S5, L15-L16). An exemption stands in for
// a review of a head, so it is held to two things: the Risk line the agent declares, which may not name a
// high-risk item, and the report of the verifier, which must be clear. The risk is declared by the agent and never
// computed by the engine; the engine holds the declaration to the grammar and refuses what it names.

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// The only declaration an exemption is made on is the Risk line of a change that names no item of the high-risk
// list: "Risk: none".
func TestCheckRiskAcceptsTheRiskLineOfAChangeWithNoRisk(t *testing.T) {
	if err := review.CheckRisk("none"); err != nil {
		t.Errorf("CheckRisk(%q) error = %v, want nil", "none", err)
	}
	if review.NoRisk != "none" {
		t.Errorf("NoRisk = %q, want %q", review.NoRisk, "none")
	}
}

// Each of the six items of the high-risk list (data or irreversible effects, security, contracts others consume,
// concurrency, delivery or environment, no test that would catch a regression) keeps a change from being exempt,
// and the refusal names the item and what it is. It is not a declaration the engine cannot read.
func TestCheckRiskRefusesEveryHighRiskItemAndNamesIt(t *testing.T) {
	for item, name := range map[int]string{
		1: "data or irreversible effects", 2: "security", 3: "contracts others consume",
		4: "concurrency", 5: "delivery or environment", 6: "no test would catch a regression",
	} {
		declared := strconv.Itoa(item)
		t.Run(declared, func(t *testing.T) {
			err := review.CheckRisk(declared)

			if !errors.Is(err, review.ErrHighRisk) || errors.Is(err, review.ErrInvalidRisk) {
				t.Fatalf("CheckRisk(%q) error = %v, want one that wraps ErrHighRisk and not ErrInvalidRisk", declared, err)
			}
			for _, want := range []string{declared + " (" + name + ")", "never exempt"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("message %q does not say %q", err, want)
				}
			}
		})
	}
}

// A declaration may name several items; one is enough to refuse it, and every one is named, in the order the
// agent wrote them.
func TestCheckRiskRefusesAListAndNamesEveryItemOfIt(t *testing.T) {
	err := review.CheckRisk("4,2")

	if !errors.Is(err, review.ErrHighRisk) {
		t.Fatalf("CheckRisk(%q) error = %v, want one that wraps ErrHighRisk", "4,2", err)
	}
	if want := "4 (concurrency), 2 (security)"; !strings.Contains(err.Error(), want) {
		t.Errorf("message %q does not say %q", err, want)
	}
}

// What is not "none" and not the item numbers of the Risk line is not a declaration at all: it is refused as one
// the engine cannot read, and not as a high risk, so that "no" or "None" is never taken for the absence of risk.
func TestCheckRiskRefusesWhatIsNotADeclaration(t *testing.T) {
	for name, declared := range map[string]string{
		"an empty declaration":             "",
		"white space":                      " ",
		"another case":                     "None",
		"upper case":                       "NONE",
		"a space before":                   " none",
		"a space after":                    "none ",
		"another word":                     "no",
		"zero":                             "0",
		"an item above the list":           "7",
		"two digits":                       "10",
		"a leading zero":                   "01",
		"a negative number":                "-1",
		"the words of the line":            "item 2",
		"items with a space":               "2 3",
		"items with a comma space":         "2, 3",
		"a trailing comma":                 "2,",
		"a leading comma":                  ",2",
		"an empty item":                    "2,,3",
		"none with an item":                "none,2",
		"an item with none":                "2,none",
		"an item twice":                    "2,2",
		"an item above the list in a list": "1,7",
		"another separator":                "1;2",
		"a digit of another script":        "٢",
	} {
		t.Run(name, func(t *testing.T) {
			err := review.CheckRisk(declared)

			if !errors.Is(err, review.ErrInvalidRisk) || errors.Is(err, review.ErrHighRisk) {
				t.Fatalf("CheckRisk(%q) error = %v, want one that wraps ErrInvalidRisk and not ErrHighRisk", declared, err)
			}
			if want := `"none"`; !strings.Contains(err.Error(), want) {
				t.Errorf("message %q does not say what is accepted (%s)", err, want)
			}
		})
	}
}

func cleanReportText() string {
	return `{"checks":[{"command":"go test ./review/...","result":"pass","output":"ok  review 0.009s"}]}`
}

// The report an exemption is made on is clear, in the shape the verifier returns. A report that is not the object
// of the contract is refused for the rule it breaks, and one that parses but is not clear is refused as not clear:
// the exemption needs a report that says every check passed, and a check that did not run never passes.
func TestCheckReportAcceptsOnlyAClearReportInTheShapeOfTheContract(t *testing.T) {
	for _, tc := range []struct {
		name, report string
		want         error // the reason the error wraps; nil for a report that is accepted
	}{
		{"a clear report", cleanReportText(), nil},
		{"no report", "", reviewverdict.ErrUnparseable},
		{"prose", "Everything passed.", reviewverdict.ErrUnparseable},
		{"a member the contract does not name", `{"checks":[],"outcome":"clear"}`, reviewverdict.ErrUnknownMember},
		{"a result outside its enumeration", `{"checks":[{"command":"c","result":"ok","output":"o"}]}`, reviewverdict.ErrOutsideEnum},
		{"a check without output", `{"checks":[{"command":"c","result":"pass","output":""}]}`, reviewverdict.ErrMissingEvidence},
		{"no check", `{"checks":[]}`, reviewverdict.ErrNotClear},
		{"a check that failed", `{"checks":[{"command":"c","result":"fail","output":"o"}]}`, reviewverdict.ErrNotClear},
		{"a check that did not run", `{"checks":[{"command":"c","result":"not-run","output":"no toolchain"}]}`, reviewverdict.ErrNotClear},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := review.CheckReport([]byte(tc.report))

			if tc.want == nil {
				if err != nil {
					t.Errorf("CheckReport() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("CheckReport() error = %v, want one that wraps %v", err, tc.want)
			}
		})
	}
}
