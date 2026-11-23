#!/bin/sh
# Keeps the evidence of repeated findings of review (repeated-corrections-4,
# -6): checks the page docs/wiki/evidence.md of a project, counts its evidence,
# and prints its rules. It never writes a file.
# Usage: sh evidence.sh check|count|rules|index [wiki directory]
#        sh evidence.sh rule <kind> [wiki directory]
#        sh evidence.sh reviewed <identifier> [wiki directory]
# The wiki directory is "$CLAUDE_PROJECT_DIR/docs/wiki" when that variable is
# set, else docs/wiki. The playbook's general rules are in evidence-rules.md,
# next to this script.
# The page has the title "# Evidence" and two tables, under "## Kinds" and
# "## Evidence" (evidence-rules.md has only the first):
#   Kinds:    | Kind | Level | Rule | Applies when | Source | Scope |
#   Evidence: | Kind | Level | Review | Date | Finding |
# A kind is kebab-case and unique; a level is a positive integer, 1 meaning
# that the kind has no rule yet; a date is YYYY-MM-DD; a scope is general or
# project; every kind of the evidence is in Kinds.
# - check: prints "FAIL <file>: <problem>" for each problem and exits 1; else
#   prints one line and exits 0. With no page it says so and exits 0.
# - count: one line per kind and level of the evidence: the number of distinct
#   reviews and the last date; a group at or above the threshold starts with
#   "ready". With no page it prints nothing.
# - rules: the rules of level 2 and up, of the page and of evidence-rules.md,
#   one per line under one heading; nothing at all when there are none.
# - index: one line per kind of level 2 and up, same sources and order as
#   rules: "- <kind> (level N): <applies when>"; nothing when there are none.
#   The SubagentStart hook gives this index, not the rules (seed-evidence-31).
# - rule <kind>: the level, rule, applies when, and source of that kind, at any
#   level, from the page first and then evidence-rules.md; when the kind is in
#   neither, a message on stderr and exit 1 (seed-evidence-31).
# - reviewed <identifier>: prints nothing; exits 0 when the page passes every
#   check of "check" and a row of its Evidence table has the identifier as its
#   Review cell, exactly (a cell is trimmed, "\|" in it is read as "|", and the
#   identifier is compared as the bytes given); 1 when none does, when the page
#   fails a check, or when nothing is at the page's path under a directory that
#   can be searched; 4, with one stderr line, for anything else at the path that
#   is not a readable regular file, for a directory on the way that cannot be
#   searched, and for an awk that fails or gives no answer (its own stderr is
#   dropped). evidence-rules.md is not read. An identifier that is missing, or
#   holds a space, a tab or a line break, is a usage error.
# count, rules, and index exit 0 (reviewed also exits 4, see above), but with no
# awk on PATH every mode prints
# "evidence.sh: awk is not on PATH; <mode> cannot run" on stderr and exits 3
# (a usage error still exits 2). A cell may hold "|" written as "\|". Not
# read: a table inside a code block. A row after a line that ends the table
# (a blank line, for one) is still read, and check fails on it.

# Distinct reviews of one kind and level that make it ready to consolidate:
# repeated-corrections-12. Reviews are counted, not findings; adjustable.
threshold=3

usage='usage: sh evidence.sh check|count|rules|index [wiki directory]
       sh evidence.sh rule <kind> [wiki directory]
       sh evidence.sh reviewed <identifier> [wiki directory]'
mode=${1:-}
want=
case $mode in
  check | count | rules | index) dirarg=${2:-} ;;
  rule | reviewed)
    want=${2:-}
    [ -n "$want" ] || { printf '%s\n' "$usage" >&2; exit 2; }
    if [ "$mode" = reviewed ]; then
      case $want in
        *[[:space:]]*) printf '%s\n' "$usage" >&2; exit 2 ;;
      esac
    fi
    dirarg=${3:-}
    ;;
  *) printf '%s\n' "$usage" >&2; exit 2 ;;
esac
command -v awk >/dev/null 2>&1 || {
  printf 'evidence.sh: awk is not on PATH; %s cannot run\n' "$mode" >&2
  exit 3
}
if [ -n "$dirarg" ]; then
  wiki=$dirarg
elif [ -n "${CLAUDE_PROJECT_DIR:-}" ]; then
  wiki=$CLAUDE_PROJECT_DIR/docs/wiki
else
  wiki=docs/wiki
fi
page=$wiki/evidence.md
general="$(cd "$(dirname "$0")" 2>/dev/null && pwd)/evidence-rules.md"

# run_awk <file> <label>: reads one page in the mode of this run.
run_awk() {
  # The identifier or kind goes through the environment, which awk reads without escape processing; -v would turn "\b" into a backspace.
  EVIDENCE_WANT=$want awk -v mode="$mode" -v file="$2" -v threshold="$threshold" '
  function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t\r]+$/, "", s); return s }
  function fail(msg) { bad = 1; if (mode == "check") print "FAIL " file ": " msg }
  # Splits a table row into cells[1..ncell], with "\|" kept as "|".
  function cells(line,    n, i, ph) {
    ph = sprintf("%c", 1)
    gsub(/\\\|/, ph, line)
    n = split(line, raw, "|")
    ncell = 0
    for (i = 2; i < n; i++) { ncell++; cell[ncell] = trim(raw[i]); gsub(ph, "|", cell[ncell]) }
    if (trim(raw[n]) != "") { ncell++; cell[ncell] = trim(raw[n]); gsub(ph, "|", cell[ncell]) }
  }
  function joined(    i, s) { s = ""; for (i = 1; i <= ncell; i++) s = s (i > 1 ? "|" : "") cell[i]; return s }
  function isseparator(line) { line = trim(line); return line ~ /^\|[ \t:|-]+$/ && line ~ /-/ }
  function isdate(d,    y, m, dd, mx) {
    if (d !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/) return 0
    y = substr(d, 1, 4) + 0; m = substr(d, 6, 2) + 0; dd = substr(d, 9, 2) + 0
    if (m < 1 || m > 12 || dd < 1) return 0
    mx = 31
    if (m == 4 || m == 6 || m == 9 || m == 11) mx = 30
    else if (m == 2) mx = ((y % 4 == 0 && y % 100 != 0) || y % 400 == 0) ? 29 : 28
    return dd <= mx
  }
  BEGIN { want = ENVIRON["EVIDENCE_WANT"]; sec = ""; kh = "Kind|Level|Rule|Applies when|Source|Scope"; eh = "Kind|Level|Review|Date|Finding" }
  /^```/ { fence = !fence; next }
  fence { next }
  /^# / { if (trim(substr($0, 3)) == "Evidence") title = 1; next }
  /^## / {
    h = trim(substr($0, 4)); sec = ""
    if (h == "Kinds") { sec = "K"; hasK = 1; rowK = 0; doneK = 0 }
    else if (h == "Evidence") { sec = "E"; hasE = 1; rowE = 0; doneE = 0 }
    next
  }
  /^\|/ {
    if (sec == "") next
    if (sec == "K" && doneK || sec == "E" && doneE) fail("a table row after the table of " (sec == "K" ? "Kinds" : "Evidence") " ended, by a line that is not a row: " $0)
    cells($0)
    if (sec == "K") {
      rowK++
      if (rowK == 1) { if (joined() != kh) fail("the header row of Kinds is not: | " kh " |") ; next }
      if (rowK == 2) { if (isseparator($0)) next; fail("the second row of Kinds is not the separator row") }
      if (ncell != 6) { fail("a row of Kinds has " ncell " cells, not 6: " cell[1]); next }
      k = cell[1]; lv = cell[2]
      if (k !~ /^[a-z0-9]+(-[a-z0-9]+)*$/) fail("kind \"" k "\" is not kebab-case")
      if (k in kind) fail("kind \"" k "\" is repeated in Kinds")
      kind[k] = 1
      na++; ak[na] = k; al[na] = lv; ar[na] = cell[3]; aw[na] = cell[4]; asrc[na] = cell[5]
      if (lv !~ /^[1-9][0-9]*$/) fail("kind \"" k "\" has level \"" lv "\", not a positive integer")
      else if (lv + 0 >= 2 && cell[3] == "") fail("kind \"" k "\" is at level " lv " and has no rule")
      if (cell[6] != "general" && cell[6] != "project") fail("kind \"" k "\" has scope \"" cell[6] "\", not general or project")
      if (lv ~ /^[1-9][0-9]*$/ && lv + 0 >= 2 && cell[3] != "") {
        nr++; rk[nr] = k; rl[nr] = lv; ra[nr] = cell[4]; rr[nr] = cell[3]
      }
    } else {
      rowE++
      if (rowE == 1) { if (joined() != eh) fail("the header row of Evidence is not: | " eh " |"); next }
      if (rowE == 2) { if (isseparator($0)) next; fail("the second row of Evidence is not the separator row") }
      if (ncell != 5) { fail("a row of Evidence has " ncell " cells, not 5: " cell[1]); next }
      k = cell[1]; lv = cell[2]; rv = cell[3]; d = cell[4]
      ne++; ek[ne] = k
      if (rv == want) found = 1
      if (lv !~ /^[1-9][0-9]*$/) fail("evidence of \"" k "\" has level \"" lv "\", not a positive integer")
      if (rv == "") fail("evidence of \"" k "\" has no review")
      if (!isdate(d)) fail("evidence of \"" k "\" has date \"" d "\", not YYYY-MM-DD")
      g = k SUBSEP lv
      if (!(g in last)) { order[++ng] = g; gk[g] = k; gl[g] = lv; last[g] = "" }
      if (!((g SUBSEP rv) in seen)) { seen[g SUBSEP rv] = 1; cnt[g]++ }
      if (d > last[g]) last[g] = d
    }
    next
  }
  { if (sec == "K" && rowK > 0) doneK = 1; if (sec == "E" && rowE > 0) doneE = 1 }
  END {
    if (mode == "check" || mode == "reviewed") {
      if (!title) fail("missing the heading \"# Evidence\"")
      if (!hasK) fail("missing the heading \"## Kinds\"")
      else if (rowK == 0) fail("Kinds has no table")
      if (file != "evidence-rules.md") {
        if (!hasE) fail("missing the heading \"## Evidence\"")
        else if (rowE == 0) fail("Evidence has no table")
        for (i = 1; i <= ne; i++) if (!(ek[i] in kind)) fail("evidence of kind \"" ek[i] "\" has no row in Kinds")
      }
      if (mode == "check") exit bad
    }
    if (mode == "count") {
      for (i = 1; i <= ng; i++) {
        g = order[i]
        printf "%s%s (level %s): %d distinct reviews, last %s\n", (cnt[g] >= threshold ? "ready to consolidate: " : "not ready: "), gk[g], gl[g], cnt[g], last[g]
      }
      exit 0
    }
    if (mode == "index") {
      for (i = 1; i <= nr; i++) printf "- %s (level %s): %s\n", rk[i], rl[i], ra[i]
      exit 0
    }
    if (mode == "rule") {
      for (i = 1; i <= na; i++) if (ak[i] == want) {
        printf "kind: %s\nlevel: %s\nrule: %s\napplies when: %s\nsource: %s\n", ak[i], al[i], ar[i], aw[i], asrc[i]
        exit 0
      }
      exit 0
    }
    if (mode == "reviewed") { print ((found && !bad) ? "yes" : "no"); exit 0 }
    for (i = 1; i <= nr; i++) printf "- %s (level %s; applies when: %s): %s\n", rk[i], rl[i], ra[i], rr[i]
  }' "$1"
}

case $mode in
  check)
    rc=0
    for f in "$general" "$page"; do
      [ -f "$f" ] || continue
      label=$(basename "$f")
      out=$(run_awk "$f" "$label") || rc=1
      [ -z "$out" ] || printf '%s\n' "$out"
    done
    [ "$rc" -eq 0 ] || exit 1
    if [ -f "$page" ]; then
      printf 'evidence check passed: %s\n' "$page"
    else
      printf 'there is no evidence page at %s\n' "$page"
    fi
    ;;
  count)
    [ -f "$page" ] && run_awk "$page" evidence.md
    exit 0
    ;;
  rules)
    out=
    for f in "$page" "$general"; do
      [ -f "$f" ] || continue
      r=$(run_awk "$f" "$(basename "$f")" 2>/dev/null)
      [ -z "$r" ] || out="$out$r
"
    done
    [ -z "$out" ] || printf 'Rules learned from repeated findings; apply those whose "applies when" matches your task:\n%s' "$out"
    exit 0
    ;;
  index)
    for f in "$page" "$general"; do
      [ -f "$f" ] || continue
      run_awk "$f" "$(basename "$f")" 2>/dev/null
    done
    exit 0
    ;;
  reviewed)
    # No page is nothing at the path, under the nearest path above it that exists when that is a directory that
    # can be searched; anything else that is not a readable regular file cannot be answered.
    if [ ! -e "$page" ] && [ ! -L "$page" ]; then
      up=$page
      while :; do
        parent=$(dirname "$up")
        if [ -e "$parent" ] || [ -L "$parent" ]; then
          if [ -d "$parent" ] && [ -x "$parent" ]; then exit 1; fi
          break
        fi
        [ "$parent" != "$up" ] || break
        up=$parent
      done
    fi
    if [ ! -f "$page" ] || [ ! -r "$page" ]; then
      printf 'evidence.sh: cannot read %s; reviewed cannot answer\n' "$page" >&2
      exit 4
    fi
    # awk answers with one word on stdout, yes or no, and exits 0; its exit status alone never answers.
    out=$(run_awk "$page" evidence.md 2>/dev/null)
    rc=$?
    if [ "$rc" -eq 0 ]; then
      case $out in
        yes) exit 0 ;;
        no) exit 1 ;;
      esac
    fi
    printf 'evidence.sh: awk failed on %s (exit %s); reviewed cannot answer\n' "$page" "$rc" >&2
    exit 4
    ;;
  rule)
    for f in "$page" "$general"; do
      [ -f "$f" ] || continue
      r=$(run_awk "$f" "$(basename "$f")" 2>/dev/null)
      [ -z "$r" ] || { printf '%s\n' "$r"; exit 0; }
    done
    printf 'evidence.sh: no kind "%s" in %s or %s\n' "$want" "$page" "$general" >&2
    exit 1
    ;;
esac
