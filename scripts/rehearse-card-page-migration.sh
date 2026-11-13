#!/usr/bin/env bash
# Rehearsal of `longterm-mem migrate-card-pages` (T6 of the store-agnostic promotion) on COPIES of the live vault, the
# live memorycard database and the live Engram database. The owner runs it; nothing else does. The vault is copied
# with `cp -a`, each database with `sqlite3 -readonly <db> ".backup <copy>"` (a consistent copy even while the
# database is in use), and the binary is never run against a live path: every path it sees is inside one scratch
# directory, and the script aborts before running anything if a resolved path is under a live one. It prints a digest
# of the live vault's pages and sidecar before and after, so a change to the live vault is visible.
#
# The command works on one project, so the rehearsal runs it for EVERY project that has pages in the vault: the list is
# read from the `project:` line of the copy's pages (not hardcoded). Pass project names to rehearse only those.
#
# The Engram database is copied only because `doctor`'s embedding checks still read it (until T12 of the next phase);
# migrate-card-pages reads no Engram database, and the run sets LONGTERM_MEM_ENGRAM_DB to the copy for `doctor` alone.
#
# The real `sync` of the copy runs the vault's own index rebuild (bin/setup-retrieve.sh --no-llm, then
# scripts/contextual-prefix.py --all --no-llm and scripts/bm25-index.py build). Those scripts take the vault from their
# own location (setup-retrieve.sh: the parent of its directory; the two Python scripts: Path(__file__).parent.parent),
# so every write lands in the copy, and --no-llm keeps every page body on the machine. The script also points HOME and
# every XDG directory at the scratch directory and drops ANTHROPIC_API_KEY and OLLAMA_URL, so nothing the rebuild reads
# from the environment leads outside it, and it checks that bin/ and scripts/ of the copy are not links to elsewhere.
#
# Run: bash scripts/rehearse-card-page-migration.sh [project ...]
# Then read: the refusal counts of the first dry runs, the vault-wide count of pages that still carry a legacy key at the
# end (zero means the vault is ready for the retirement of the legacy keys), the `sync --dry-run` comparisons and the
# doctor comparison. It ends by printing the live commands, per project, for the owner.
set -euo pipefail

die() { echo "ABORT: $*" >&2; exit 1; }

real_home="$HOME"
# The build must keep the Go build cache it already has: the XDG directories are pointed at the scratch directory below,
# and Go derives its cache directory from XDG_CACHE_HOME unless GOCACHE is set.
gocache="$(GOTOOLCHAIN=go1.26.1 go env GOCACHE)"
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../longterm-mem" && pwd)"
live_vault="$real_home/labdrian-brain"
live_card_dir="$real_home/.memorycard"
live_engram_dir="$real_home/.engram"
cache="$real_home/.cache"

command -v sqlite3 >/dev/null || die "sqlite3 is needed to copy the databases consistently"
[[ -d "$live_vault/wiki/memory" ]] || die "the live vault has no wiki/memory directory: $live_vault"
[[ -f "$live_card_dir/memory.db" ]] || die "no memorycard database at $live_card_dir/memory.db"
mkdir -p "$cache"

work="$(mktemp -d -p "$cache" t6-rehearsal.XXXXXX)"
vault="$work/labdrian-brain"
home="$work/home"
card_db="$work/memorycard/memory.db"
engram_db="$work/engram/engram.db"
bin="$work/longterm-mem"
mkdir -p "$work/memorycard" "$work/engram" "$work/tmp" "$work/state" "$home" "$vault"

under() { # under <path> <prefix>: true when the resolved path is the prefix or inside it
  local p
  p="$(realpath -m "$1")"
  [[ "$p" == "$2" || "$p" == "$2"/* ]]
}
guard() { # abort when a path the binary will see resolves under a live location, or outside the scratch directory
  local p live
  for p in "$@"; do
    for live in "$live_vault" "$live_card_dir" "$live_engram_dir"; do
      if under "$p" "$live"; then
        die "$p resolves under the live path $live"
      fi
    done
    if ! under "$p" "$work"; then
      die "$p is not inside the scratch directory $work"
    fi
  done
}
live_digest() { # a digest of what the migration would rewrite in the live vault: its pages and its precedence sidecar
  local sidecar=.raw/.longterm-mem-manifest.json paths=(wiki/memory)
  if [[ ! -d "$live_vault/wiki/memory" ]]; then
    echo "ABORT: the live vault has no wiki/memory directory: $live_vault" >&2
    return 1
  fi
  if [[ -e "$live_vault/$sidecar" ]]; then
    paths+=("$sidecar")
  else
    echo "note: the live vault has no precedence sidecar ($sidecar); the digest covers its pages only" >&2
  fi
  (cd "$live_vault" && find "${paths[@]}" -type f -print0 | sort -z | xargs -0 sha256sum | sha256sum)
}
copy_db() { # copy_db <live db> <copy>: a consistent copy of a sqlite database, which may be open by another process
  sqlite3 -readonly "$1" ".backup '$2'"
}
legacy_counts() { # one line per project: the pages of the vault that still carry a legacy identity key, and the total
  # The command reads the .md files directly in wiki/memory whose frontmatter lines end in a line feed (a CRLF page has
  # no frontmatter it can read) and counts a top-level key line only: this does the same, and says how many CRLF pages it skipped.
  perl -e '
    my %n; my $total = 0; my $crlf = 0;
    opendir(my $dh, "$ARGV[0]/wiki/memory") or do { print STDERR "ABORT: cannot read $ARGV[0]/wiki/memory: $!\n"; exit 2; };
    my @files = sort grep { /\.md$/ && -f "$ARGV[0]/wiki/memory/$_" } readdir($dh);
    closedir($dh);
    for my $name (@files) {
      my $f = "$ARGV[0]/wiki/memory/$name";
      open(my $h, "<", $f) or next;
      my ($project, $legacy) = ("", 0);
      while (<$h>) {
        if ($. == 1 && $_ eq "---\r\n") { $crlf++; last; }
        chomp;
        if ($. == 1) { last unless $_ eq "---"; next; }
        last if $_ eq "---";
        $legacy = 1 if /^(engram_id|engram_revision|engram_type|engram_sync_id|memorycard_id|memorycard_revision|memorycard_type):/;
        $project = $1 if /^project:\s*(.*?)\s*$/;
      }
      close($h);
      next unless $legacy;
      $project =~ s/^["\x27]|["\x27]$//g;
      $n{$project eq "" ? "(no project, not counted per project)" : $project}++;
      $total++;
    }
    printf("  %s: %d\n", $_, $n{$_}) for sort keys %n;
    printf("vault-wide: %d page(s) still carry a legacy identity key\n", $total);
    printf("(%d page(s) with CRLF line endings skipped: the command does not read their frontmatter)\n", $crlf) if $crlf;
  ' "$1"
}
projects_of() { # the distinct `project:` values of the pages of the vault at $1, one per line
  perl -e '
    my %seen;
    opendir(my $dh, "$ARGV[0]/wiki/memory") or do { print STDERR "ABORT: cannot read $ARGV[0]/wiki/memory: $!\n"; exit 2; };
    my @files = sort grep { /\.md$/ && -f "$ARGV[0]/wiki/memory/$_" } readdir($dh);
    closedir($dh);
    for my $name (@files) {
      open(my $h, "<", "$ARGV[0]/wiki/memory/$name") or next;
      while (<$h>) {
        chomp;
        if ($. == 1) { last unless $_ eq "---"; next; }
        last if $_ eq "---";
        if (/^project:\s*(.*?)\s*$/) { my $p = $1; $p =~ s/^["\x27]|["\x27]$//g; $seen{$p} = 1 if $p ne ""; last; }
      }
      close($h);
    }
    print "$_\n" for sort keys %seen;
  ' "$1"
}

echo "scratch directory: $work"
guard "$vault" "$card_db" "$engram_db" "$home" "$work/state" "$work/tmp" "$bin"
live_before="$(live_digest)"
echo "live vault digest before: $live_before"
cp -a "$live_vault/." "$vault/"
[[ -d "$vault/wiki/memory" && -r "$vault/wiki/memory" ]] || die "the copy of the vault has no readable wiki/memory directory: $vault"
copy_db "$live_card_dir/memory.db" "$card_db"
if [[ -f "$live_engram_dir/engram.db" ]]; then
  copy_db "$live_engram_dir/engram.db" "$engram_db"
else
  echo "note: no Engram database at $live_engram_dir/engram.db; doctor's embedding checks run without one"
fi

# Every path and variable the binary can read is the copy's: the vault registry and the state directory are in the
# scratch directory, the vault and both databases are named explicitly, and HOME and the XDG directories are the
# scratch home, so nothing the index rebuild resolves from the environment is a live path.
export HOME="$home"
export XDG_CONFIG_HOME="$home/.config" XDG_CACHE_HOME="$home/.cache" XDG_DATA_HOME="$home/.local/share" XDG_STATE_HOME="$home/.local/state"
unset ANTHROPIC_API_KEY OLLAMA_URL
export TMPDIR="$work/tmp"
export LONGTERM_MEM_VAULT="$vault"
export LONGTERM_MEM_VAULTS_FILE="$home/vaults.json"
export LONGTERM_MEM_STATE_DIR="$work/state"
export LONGTERM_MEM_MEMORYCARD_DB="$card_db"
export MEMORYCARD_DB="$card_db"
export LONGTERM_MEM_ENGRAM_DB="$engram_db"
guard "$LONGTERM_MEM_VAULT" "$LONGTERM_MEM_VAULTS_FILE" "$LONGTERM_MEM_STATE_DIR" "$LONGTERM_MEM_MEMORYCARD_DB" "$LONGTERM_MEM_ENGRAM_DB" "$HOME" "$XDG_CONFIG_HOME" "$XDG_CACHE_HOME" "$XDG_DATA_HOME" "$XDG_STATE_HOME" "$TMPDIR"
cd "$work"

# The copied .git is dropped (it could be a link to the live repository, and its hooks and config are the live
# ones): the baseline is a fresh repository of the copy. A link inside the copy must stay inside the scratch directory,
# and the scripts the index rebuild runs must be the copy's own files.
rm -rf "${vault:?}/.git"
find "$vault" -type l -print0 | while IFS= read -r -d '' link; do guard "$link"; done
for dir in bin scripts; do
  if [[ -d "$vault/$dir" ]]; then guard "$vault/$dir"; fi
done
git -C "$vault" init -q
git -C "$vault" add -A
git -c user.name=rehearsal -c user.email=rehearsal@example.invalid -c commit.gpgsign=false -c core.hooksPath=/dev/null \
  -C "$vault" commit -q --allow-empty --no-verify -m "rehearsal baseline: the copy of the live vault"
echo "baseline commit: $(git -C "$vault" rev-parse --short HEAD)"

# The build uses the real HOME for the Go caches only (it reads and writes no vault or database).
(cd "$repo" && HOME="$real_home" GOCACHE="$gocache" GOTOOLCHAIN=go1.26.1 go build -o "$bin" ./cmd/longterm-mem)
echo "built: $bin (from $(git -C "$repo" rev-parse --short HEAD) plus its working tree)"

if (($# > 0)); then
  projects=("$@")
else
  project_list="$(projects_of "$vault")"
  projects=()
  if [[ -n "$project_list" ]]; then mapfile -t projects <<<"$project_list"; fi
fi
((${#projects[@]} > 0)) || die "no page of the vault names a project"
echo "projects to rehearse (${#projects[@]}): ${projects[*]}"
echo
echo "=== pages that carry a legacy identity key BEFORE the migration ==="
legacy_counts "$vault"

project=""
lm() { "$bin" "$@" --project "$project" --vault "$vault"; }
slug() { printf '%02d-%s' "$1" "$(printf '%s' "$2" | tr -c 'A-Za-z0-9._-' '_')"; } # slug <index> <project>: an index number first, so two projects never share a file name
show() { # show <label> <command...>: print the output and the exit code, whatever it is
  local rc=0
  echo
  echo "=== $1 ==="
  shift
  "$@" || rc=$?
  echo "[exit $rc]"
}
findings() { # the findings of doctor: each check and its status, and which page is judged against which memory
  lm doctor --json 2>/dev/null | perl -0ne 'while (/"name":\s*"([^"]+)",\s*"status":\s*"([^"]+)"/g) { print "$1 $2\n" } while (/(c-\d+: memory \d+ is not pinned and has no topic_key)/g) { print "$1\n" }'
}
promotions() { rg '^  would promote:|would promote [0-9]+ memory' "$1" | perl -pe 's/, patch .*//; s/ into .*//'; }

idx=0
for project in "${projects[@]}"; do
  s="$(slug "$idx" "$project")"; idx=$((idx + 1))
  echo
  echo "################ project: $project ################"
  show "[$project] sync --dry-run BEFORE the migration" lm sync --dry-run | tee "$work/sync-before-$s.txt"
  findings >"$work/doctor-before-$s.txt" || true
  show "[$project] migrate-card-pages (dry run, the default): the legacy pages and the count of each refusal" lm migrate-card-pages | tee "$work/dry-before-$s.txt"
  show "[$project] migrate-card-pages --apply" lm migrate-card-pages --apply | tee "$work/apply-$s.txt"
done

echo
echo "=== git -C <copy> diff --stat (tracked files: the migrated pages and the precedence sidecar) ==="
git -C "$vault" diff --stat | tail -5
echo "=== git -C <copy> status --short (the backup directories are new, so they show as ??) ==="
git -C "$vault" status --short | rg -v '^ M wiki/memory/' | head -20 || true

idx=0
for project in "${projects[@]}"; do
  s="$(slug "$idx" "$project")"; idx=$((idx + 1))
  echo
  echo "################ project: $project (checks after the migration) ################"
  before="$(git -C "$vault" status --short | sha256sum; git -C "$vault" diff | sha256sum)"
  show "[$project] migrate-card-pages --apply (second run: nothing may change)" lm migrate-card-pages --apply
  after="$(git -C "$vault" status --short | sha256sum; git -C "$vault" diff | sha256sum)"
  if [[ "$before" == "$after" ]]; then echo "second run: the copy is unchanged"; else echo "SECOND RUN CHANGED THE COPY"; fi

  echo
  echo "=== [$project] promote --source memorycard --id N for each migrated memory: each must say already promoted ==="
  ids="$(perl -ne 'print "$1\n" if /^migrated: .* memorycard memory (\d+)\)/' "$work/apply-$s.txt" | sort -un | head -20)"
  if [[ -z "$ids" ]]; then echo "no page was migrated: nothing to promote"; fi
  for id in $ids; do
    show "[$project] promote --source memorycard --id $id" lm promote --source memorycard --id "$id"
  done
  echo
  echo "=== [$project] diff --stat after the promotes (still only the migrated pages and the sidecar) ==="
  git -C "$vault" diff --stat | tail -1

  echo
  echo "=== [$project] sync --dry-run AFTER the migration: the promotions must be the same as BEFORE ==="
  show "[$project] sync --dry-run AFTER" lm sync --dry-run | tee "$work/sync-after-$s.txt"
  if diff <(promotions "$work/sync-before-$s.txt") <(promotions "$work/sync-after-$s.txt") >/dev/null; then
    echo "sync dry run: the same promotions before and after"
  else
    echo "sync dry run: THE PROMOTIONS DIFFER (a migrated page's memory would be promoted again):"
    diff <(promotions "$work/sync-before-$s.txt") <(promotions "$work/sync-after-$s.txt") || true
  fi

  echo
  echo "=== [$project] doctor: the findings must be the same before and after ==="
  findings >"$work/doctor-after-$s.txt" || true
  if diff "$work/doctor-before-$s.txt" "$work/doctor-after-$s.txt" >/dev/null; then
    echo "doctor: the same findings before and after"
  else
    echo "doctor: THE FINDINGS DIFFER:"
    diff "$work/doctor-before-$s.txt" "$work/doctor-after-$s.txt" || true
  fi
done

echo
echo "=== sync (a real sync of the COPY, one project: the index rebuild runs on the migrated vault) ==="
project="${projects[0]}"
show "[$project] sync" lm sync
for project in "${projects[@]}"; do
  show "[$project] migrate-card-pages (dry run after everything): the legacy pages that are left" lm migrate-card-pages
done

echo
echo "=== pages that still carry a legacy identity key AFTER the migration (the vault-wide count is what S7 needs to be zero) ==="
legacy_counts "$vault"

live_after="$(live_digest)"
echo
echo "live vault digest after:  $live_after"
if [[ "$live_before" == "$live_after" ]]; then echo "the live vault is unchanged"; else echo "THE LIVE VAULT CHANGED (something else wrote it while this ran)"; fi

echo
echo "=== the live run, when the rehearsal is what you expect (one project at a time) ==="
echo "The hook-driven sync has no switch and no lock guards a concurrent sync. Close every agent session that uses the vault, then"
echo "run: pgrep -af '[l]ongterm-mem.* sync'   (any output is a sync that is running; the bracket keeps the check from matching itself)"
for project in "${projects[@]}"; do
  echo "  longterm-mem migrate-card-pages --project '$project'            # dry run, read the counts"
  echo "  longterm-mem migrate-card-pages --project '$project' --apply    # the backup directory it prints is the way back"
done
echo "Then run the dry run for every project again: the vault-wide count of legacy pages is the sum of what is left."
echo
echo "done. The copies are in $work (remove with: rm -rf \"$work\")"
