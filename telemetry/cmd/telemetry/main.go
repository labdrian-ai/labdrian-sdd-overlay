package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/labdrian-ai/labdrian-sdd-overlay/telemetry"
)

const usage = `Usage: telemetry report [--claude-root DIR] [--pi-root DIR] [--json] [--top N]
  --claude-root DIR  Claude logs (default $HOME/.claude/projects)
  --pi-root DIR      Pi logs (default $HOME/.pi/agent/sessions)
  --json             Print exactly Report.JSON(); stats stay on stderr
  --top N            Model/agent/project rows and latest days (default 10; positive)
Missing default roots are skipped; explicit missing roots are errors.
`

func main() {
	home, _ := os.UserHomeDir()
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, home))
}

func run(args []string, stdout, stderr io.Writer, home string) int {
	badUsage := func() int { fmt.Fprint(stderr, usage); return 2 }
	if len(args) == 0 || args[0] != "report" {
		return badUsage()
	}
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	claude := fs.String("claude-root", filepath.Join(home, ".claude/projects"), "")
	pi := fs.String("pi-root", filepath.Join(home, ".pi/agent/sessions"), "")
	jsonOutput := fs.Bool("json", false, "")
	top := fs.Int("top", 10, "")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *top <= 0 {
		return badUsage()
	}
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	var a telemetry.Aggregator
	readable := false
	for _, source := range []struct {
		name, root string
		read       func(string, func(telemetry.Call)) (telemetry.Stats, error)
	}{{"claude", *claude, telemetry.ReadClaude}, {"pi", *pi, telemetry.ReadPi}} {
		stats, err := source.read(source.root, a.Add)
		if err != nil {
			if os.IsNotExist(err) && !explicit[source.name+"-root"] {
				fmt.Fprintf(stderr, "%s: not found: %s (skipped default root)\n", source.name, source.root)
				continue
			}
			fmt.Fprintf(stderr, "%s: %v\n", source.name, err)
			return 1
		}
		fmt.Fprintf(stderr, "%s: files=%d lines=%d calls=%d skipped=%d duplicates=%d unreadable_files=%d\n", source.name, stats.Files, stats.Lines, stats.Calls, stats.Skipped, stats.Duplicates, stats.UnreadableFiles)
		readable = readable || stats.Calls > 0 || stats.Files > stats.UnreadableFiles
	}
	if !readable {
		fmt.Fprintln(stderr, "no readable source")
		return 1
	}
	r := a.Report()
	var err error
	if *jsonOutput {
		var b []byte
		b, err = r.JSON()
		if err == nil {
			_, err = stdout.Write(b)
		}
	} else {
		err = human(stdout, r, *top)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func compact(n int64) string {
	for _, unit := range []struct {
		size   int64
		suffix string
	}{{1e9, "B"}, {1e6, "M"}, {1e3, "k"}} {
		if n >= unit.size {
			v, digits := float64(n)/float64(unit.size), 0
			if v < 10 {
				digits = 2
			} else if v < 100 {
				digits = 1
			}
			s := strconv.FormatFloat(v, 'f', digits, 64)
			if digits > 0 {
				s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
			}
			return s + unit.suffix
		}
	}
	return strconv.FormatInt(n, 10)
}
func ratio(v *float64, percent bool) string {
	if v == nil {
		return "-"
	}
	if percent {
		return fmt.Sprintf("%.1f%%", *v*100)
	}
	return fmt.Sprintf("%.0f", *v)
}

func human(out io.Writer, r telemetry.Report, top int) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', tabwriter.AlignRight)
	sections := []struct {
		name   string
		groups []telemetry.Group
		rank   bool
	}{
		{"Summary", []telemetry.Group{r.Summary}, false},
		{"By role", r.ByRole, false}, {"By model", r.ByModel, true},
		{"By agent type", r.ByAgentType, true}, {"By project", r.ByProject, true},
		{"By source", r.BySource, false}, {"By day", r.ByDay, false},
	}
	for i, section := range sections {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, section.name)
		fmt.Fprintln(w, "Key\tCalls\tContext\tCache read\tCtx/out\tP5\tP50\tP90\tMax\tOutput\tCost\t")
		rows := append([]telemetry.Group(nil), section.groups...)
		cut := 0
		if section.rank {
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].ContextTokens == rows[j].ContextTokens {
					return rows[i].Key < rows[j].Key
				}
				return rows[i].ContextTokens > rows[j].ContextTokens
			})
			if len(rows) > top {
				cut = len(rows) - top
				rows = rows[:top]
			}
		} else if section.name == "By day" && len(rows) > top {
			rows = rows[len(rows)-top:]
		}
		for _, g := range rows {
			cost := "n/a"
			if g.CostCalls > 0 && g.CostUSD != nil {
				cost = fmt.Sprintf("$%.2f (%d calls)", *g.CostUSD, g.CostCalls)
			}
			p := []string{"-", "-", "-", "-"}
			if d := g.ContextPerCallDistribution; d != nil {
				p = []string{compact(d.P5), compact(d.P50), compact(d.P90), compact(d.Max)}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t\n", g.Key, compact(g.Calls), compact(g.ContextTokens), ratio(g.CacheReadShare, true), ratio(g.ContextPerOutput, false), p[0], p[1], p[2], p[3], compact(g.Output), cost)
		}
		if cut > 0 {
			fmt.Fprintf(w, "… and %d more\n", cut)
		}
	}
	return w.Flush()
}
