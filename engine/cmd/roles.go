package main

// roles subcommand: 'roles validate --file <path>', 'roles next --project
// --goal --chain', 'roles resume --project --goal --chain', 'roles append
// --project --goal --chain --stdin', and 'roles match-shaper --root
// --handoff'. Every verb is read-only except append, which appends one
// caller-supplied record to the role chain store. No verb launches an agent,
// dispatches work, or grants execution authority; runtime-specific agent
// adapters belong outside this package.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// rolesAuthorityNote is printed on every roles command's successful output.
const rolesAuthorityNote = "Role data carries no execution authority; no roles command launches an agent or dispatches work."

// runRoles implements the 'roles <verb>' subcommand.
func runRoles(args []string) {
	runRolesCore(args, os.Stdin, os.Stdout, os.Stderr, os.Exit)
}

// runRolesCore is the testable core of the roles subcommand. Every exit(n)
// is followed by a return, because tests inject a non-terminating exit.
func runRolesCore(args []string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: roles requires a verb: validate, next, resume, append, match-shaper")
		exit(1)
		return
	}
	switch args[0] {
	case "validate":
		runRolesValidate(args[1:], stdout, stderr, exit)
	case "next":
		runRolesNext(args[1:], stdout, stderr, exit)
	case "resume":
		runRolesResume(args[1:], stdout, stderr, exit)
	case "append":
		runRolesAppend(args[1:], stdin, stdout, stderr, exit)
	case "match-shaper":
		runRolesMatchShaper(args[1:], stdout, stderr, exit)
	default:
		fmt.Fprintf(stderr, "error: roles: unknown verb %q (expected validate, next, resume, append, or match-shaper)\n", args[0])
		exit(1)
	}
}

// rolesOpts are the parsed roles flags. Not every verb uses every field.
type rolesOpts struct {
	file, project, goal, chain, root, handoff string
	stdin                                     bool
}

// parseRolesArgs parses --file/--project/--goal/--chain/--root/--handoff and,
// when allowStdin, --stdin. It fails loud on an unknown flag, a flag without
// its value, or a positional argument.
func parseRolesArgs(args []string, allowStdin bool, required []string) (rolesOpts, error) {
	var o rolesOpts
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--file", "--project", "--goal", "--chain", "--root", "--handoff":
			if i+1 >= len(args) {
				return o, fmt.Errorf("%s requires a value", a)
			}
			i++
			switch a {
			case "--file":
				o.file = args[i]
			case "--project":
				o.project = args[i]
			case "--goal":
				o.goal = args[i]
			case "--chain":
				o.chain = args[i]
			case "--root":
				o.root = args[i]
			case "--handoff":
				o.handoff = args[i]
			}
		case "--stdin":
			if !allowStdin {
				return o, fmt.Errorf("unknown flag %q", a)
			}
			o.stdin = true
		default:
			if strings.HasPrefix(a, "-") {
				return o, fmt.Errorf("unknown flag %q", a)
			}
			return o, fmt.Errorf("unexpected argument %q", a)
		}
	}
	values := map[string]string{
		"--file": o.file, "--project": o.project, "--goal": o.goal, "--chain": o.chain,
		"--root": o.root, "--handoff": o.handoff,
	}
	for _, name := range required {
		if values[name] == "" {
			return o, fmt.Errorf("%s is required", name)
		}
	}
	return o, nil
}

// runRolesValidate implements 'roles validate --file <path>'. It is
// read-only: it strictly parses and validates one RoleHandoff record from
// disk and reports the result. It never touches the chain store.
func runRolesValidate(args []string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseRolesArgs(args, false, []string{"--file"})
	if err != nil {
		fmt.Fprintf(stderr, "error: roles validate: %v\n", err)
		exit(1)
		return
	}
	data, err := os.ReadFile(o.file)
	if err != nil {
		fmt.Fprintf(stderr, "error: roles validate: %v\n", err)
		exit(1)
		return
	}
	h, err := roles.ParseRoleHandoff(data)
	if err != nil {
		fmt.Fprintf(stderr, "error: roles validate: %v\n", err)
		exit(1)
		return
	}
	out := struct {
		Valid     bool   `json:"valid"`
		FromRole  string `json:"from_role"`
		ToRole    string `json:"to_role"`
		Status    string `json:"status"`
		Authority string `json:"authority"`
	}{
		Valid: true, FromRole: string(h.FromRole), ToRole: string(h.ToRole), Status: string(h.Status),
		Authority: rolesAuthorityNote,
	}
	writeRolesJSON(stdout, stderr, out, exit)
}

// runRolesNext implements 'roles next --project --goal --chain'. It is
// read-only.
func runRolesNext(args []string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseRolesArgs(args, false, []string{"--project", "--goal", "--chain"})
	if err != nil {
		fmt.Fprintf(stderr, "error: roles next: %v\n", err)
		exit(1)
		return
	}
	records, err := loadRolesChain(o)
	if err != nil {
		fmt.Fprintf(stderr, "error: roles next: %v\n", err)
		exit(1)
		return
	}
	if len(records) == 0 {
		fmt.Fprintln(stderr, "error: roles next: chain has no records")
		exit(1)
		return
	}
	next, err := roles.Next(records)
	if err != nil {
		fmt.Fprintf(stderr, "error: roles next: %v\n", err)
		exit(1)
		return
	}
	out := struct {
		NextRoles []string `json:"next_roles"`
		Authority string   `json:"authority"`
	}{NextRoles: rolesToStrings(next), Authority: rolesAuthorityNote}
	writeRolesJSON(stdout, stderr, out, exit)
}

// runRolesResume implements 'roles resume --project --goal --chain'. It is
// read-only.
func runRolesResume(args []string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseRolesArgs(args, false, []string{"--project", "--goal", "--chain"})
	if err != nil {
		fmt.Fprintf(stderr, "error: roles resume: %v\n", err)
		exit(1)
		return
	}
	records, err := loadRolesChain(o)
	if err != nil {
		fmt.Fprintf(stderr, "error: roles resume: %v\n", err)
		exit(1)
		return
	}
	if len(records) == 0 {
		fmt.Fprintln(stderr, "error: roles resume: chain has no records")
		exit(1)
		return
	}
	state, err := roles.Resume(records)
	if err != nil {
		fmt.Fprintf(stderr, "error: roles resume: %v\n", err)
		exit(1)
		return
	}
	out := struct {
		Role         string `json:"role"`
		Interrupted  bool   `json:"interrupted"`
		ResumeReason string `json:"resume_reason,omitempty"`
		Terminal     bool   `json:"terminal"`
		Authority    string `json:"authority"`
	}{
		Role: string(state.Role), Interrupted: state.Interrupted, ResumeReason: state.ResumeReason,
		Terminal: state.Terminal, Authority: rolesAuthorityNote,
	}
	writeRolesJSON(stdout, stderr, out, exit)
}

// runRolesAppend implements 'roles append --project --goal --chain --stdin'.
// The record is accepted only from stdin, mirroring the shaper clearance
// record command, so content never arrives through argv.
func runRolesAppend(args []string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	fail := func(format string, a ...any) {
		fmt.Fprintf(stderr, "error: roles append: "+format+"\n", a...)
		exit(1)
	}
	o, err := parseRolesArgs(args, true, []string{"--project", "--goal", "--chain"})
	if err != nil {
		fail("%v", err)
		return
	}
	if !o.stdin {
		fail("--stdin is required: the record is accepted only on stdin, never from argv")
		return
	}
	data, err := io.ReadAll(io.LimitReader(stdin, stdinSizeLimit+1))
	if err != nil {
		fail("read stdin: %v", err)
		return
	}
	if len(data) > stdinSizeLimit {
		fail("stdin exceeds %d bytes", stdinSizeLimit)
		return
	}
	if strings.TrimSpace(string(data)) == "" {
		fail("stdin is empty")
		return
	}
	h, err := roles.ParseRoleHandoff(data)
	if err != nil {
		fail("%v", err)
		return
	}
	for _, f := range []struct{ name, flag, record string }{
		{"project_id", o.project, h.ProjectID},
		{"goal_id", o.goal, h.GoalID},
		{"chain_id", o.chain, h.ChainID},
	} {
		if f.flag != f.record {
			fail("--%s %q does not match the record's %s %q", strings.TrimSuffix(f.name, "_id"), f.flag, f.name, f.record)
			return
		}
	}
	store, err := roles.NewChainStore()
	if err != nil {
		fail("%v", err)
		return
	}
	path, err := store.Append(data)
	if err != nil {
		fail("%v", err)
		return
	}
	fmt.Fprintf(stdout, "roles append: stored record at %s\n%s\n", path, rolesAuthorityNote)
}

// loadRolesChain resolves a ChainStore and loads one chain's records.
func loadRolesChain(o rolesOpts) ([]roles.ChainRecord, error) {
	store, err := roles.NewChainStore()
	if err != nil {
		return nil, err
	}
	return store.LoadChain(o.project, o.goal, o.chain)
}

// runRolesMatchShaper implements 'roles match-shaper --root --handoff'. It
// loads a Shaper handoff strictly inside root using shaper's existing
// contained-read helpers, then reports which of its version 3 roles (if any)
// equal a member of the reusable-role vocabulary. It does not modify Shaper
// handoff v3 semantics: the free-form roles list is left exactly as
// authored.
func runRolesMatchShaper(args []string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseRolesArgs(args, false, []string{"--root", "--handoff"})
	if err != nil {
		fmt.Fprintf(stderr, "error: roles match-shaper: %v\n", err)
		exit(1)
		return
	}
	if !filepath.IsAbs(o.root) {
		fmt.Fprintf(stderr, "error: roles match-shaper: --root must be an absolute path, got %q\n", o.root)
		exit(1)
		return
	}
	resolved, err := filepath.EvalSymlinks(o.root)
	if err != nil {
		fmt.Fprintf(stderr, "error: roles match-shaper: resolve --root: %v\n", err)
		exit(1)
		return
	}
	src, err := shaper.LoadHandoff(filepath.Clean(resolved), o.handoff)
	if err != nil {
		fmt.Fprintf(stderr, "error: roles match-shaper: %v\n", err)
		exit(1)
		return
	}
	type matchJSON struct {
		Role              string `json:"role"`
		Responsibility    string `json:"responsibility"`
		MatchesVocabulary bool   `json:"matches_vocabulary"`
	}
	matches := []matchJSON{}
	for _, r := range src.Handoff.Roles {
		matches = append(matches, matchJSON{
			Role:              r.Role,
			Responsibility:    r.Responsibility,
			MatchesVocabulary: roles.IsKnownRole(roles.Role(r.Role)),
		})
	}
	out := struct {
		HandoffVersion int         `json:"handoff_version"`
		Matches        []matchJSON `json:"matches"`
		Authority      string      `json:"authority"`
	}{HandoffVersion: src.Handoff.Version, Matches: matches, Authority: rolesAuthorityNote}
	writeRolesJSON(stdout, stderr, out, exit)
}

func rolesToStrings(rs []roles.Role) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, string(r))
	}
	return out
}

func writeRolesJSON(stdout, stderr io.Writer, v any, exit func(int)) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: roles: %v\n", err)
		exit(1)
		return
	}
	stdout.Write(append(data, '\n'))
	exit(0)
}
