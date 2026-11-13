#!/usr/bin/env bash
# Rehearsal of `longterm-mem migrate-card-pages` (T6 of the store-agnostic promotion) on COPIES of the live vault, the
# live memorycard database and the live Engram database. The owner runs it; nothing else does. It reads the live
# files with `cp -a` and never runs the binary against them: every path the binary sees is inside one scratch
# directory, and the script aborts before running anything if a resolved path is under a live one. It prints a digest
# of the live vault's pages and sidecar before and after, so a change to the live vault is visible.
#
# The Engram database is copied only because `doctor`'s embedding checks still read it (until T12 of the next phase);
# migrate-card-pages reads no Engram database, and the run sets LONGTERM_MEM_ENGRAM_DB to the copy for `doctor` alone.
#
# Run: bash scripts/rehearse-card-page-migration.sh [project]
# Then read: the refusal counts of the first dry run, the "legacy pages" count after the apply (the pages that are
# left), the two `sync --dry-run` comparisons and the doctor comparison.
set -euo pipefail

project="${1:-labdrian-sdd-overlay}"
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../longterm-mem" && pwd)"
live_vault=/home/labdrian/labdrian-brain
live_card_dir=/home/labdrian/.memorycard
live_engram_dir=/home/labdrian/.engram
cache=/home/labdrian/.cache

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
        echo "ABORT: $p resolves under the live path $live" >&2
        exit 1
      fi
    done
    if ! under "$p" "$work"; then
      echo "ABORT: $p is not inside the scratch directory $work" >&2
      exit 1
    fi
  done
}
live_digest() { # a digest of what the migration would rewrite in the live vault: its pages and its precedence sidecar
  (cd "$live_vault" && find wiki/memory .raw/.longterm-mem-manifest.json -type f -print0 2>/dev/null | sort -z | xargs -0 sha256sum | sha256sum)
}

echo "scratch directory: $work"
guard "$vault" "$card_db" "$engram_db" "$home" "$work/state" "$work/tmp" "$bin"
live_before="$(live_digest)"
echo "live vault digest before: $live_before"
cp -a "$live_vault/." "$vault/"
cp -a "$live_card_dir/memory.db" "$card_db"
cp -a "$live_engram_dir/engram.db" "$engram_db"
for suffix in -wal -shm; do
  if [[ -e "$live_card_dir/memory.db$suffix" ]]; then cp -a "$live_card_dir/memory.db$suffix" "$card_db$suffix"; fi
  if [[ -e "$live_engram_dir/engram.db$suffix" ]]; then cp -a "$live_engram_dir/engram.db$suffix" "$engram_db$suffix"; fi
done

# Every path and variable the binary can read is the copy's: the vault registry and the state directory are in the
# scratch directory, and the vault and both databases are named explicitly.
export HOME="$home"
export TMPDIR="$work/tmp"
export LONGTERM_MEM_VAULT="$vault"
export LONGTERM_MEM_VAULTS_FILE="$home/vaults.json"
export LONGTERM_MEM_STATE_DIR="$work/state"
export LONGTERM_MEM_MEMORYCARD_DB="$card_db"
export MEMORYCARD_DB="$card_db"
export LONGTERM_MEM_ENGRAM_DB="$engram_db"
guard "$LONGTERM_MEM_VAULT" "$LONGTERM_MEM_VAULTS_FILE" "$LONGTERM_MEM_STATE_DIR" "$LONGTERM_MEM_MEMORYCARD_DB" "$LONGTERM_MEM_ENGRAM_DB" "$HOME" "$TMPDIR"
cd "$work"

# The copied .git is dropped (it could be a link to the live repository, and its hooks and config are the live
# ones): the baseline is a fresh repository of the copy. A link inside the copy must stay inside the scratch directory.
rm -rf "${vault:?}/.git"
find "$vault" -type l -print0 | while IFS= read -r -d '' link; do guard "$link"; done
git -C "$vault" init -q
git -C "$vault" add -A
git -c user.name=rehearsal -c user.email=rehearsal@example.invalid -c commit.gpgsign=false -c core.hooksPath=/dev/null \
  -C "$vault" commit -q --allow-empty --no-verify -m "rehearsal baseline: the copy of the live vault"
echo "baseline commit: $(git -C "$vault" rev-parse --short HEAD)"

# The build uses the real HOME for the Go caches only (it reads and writes no vault or database).
(cd "$repo" && HOME=/home/labdrian GOTOOLCHAIN=go1.26.1 go build -o "$bin" ./cmd/longterm-mem)
echo "built: $bin (from $(git -C "$repo" rev-parse --short HEAD) plus its working tree)"

lm() { "$bin" "$@" --project "$project" --vault "$vault"; }
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

show "sync --dry-run BEFORE the migration" lm sync --dry-run | tee "$work/sync-before.txt"
findings >"$work/doctor-before.txt" || true
show "migrate-card-pages (dry run, the default): the legacy pages and the count of each refusal" lm migrate-card-pages | tee "$work/dry-before.txt"
show "migrate-card-pages --apply" lm migrate-card-pages --apply | tee "$work/apply.txt"

echo
echo "=== git -C <copy> diff --stat (tracked files: the migrated pages and the precedence sidecar) ==="
git -C "$vault" diff --stat | tail -5
echo "=== git -C <copy> status --short (the backup directory is new, so it shows as ??) ==="
git -C "$vault" status --short | rg -v '^ M wiki/memory/' | head -20 || true

echo
echo "=== second run: nothing may change ==="
before="$(git -C "$vault" status --short | sha256sum; git -C "$vault" diff | sha256sum)"
show "migrate-card-pages --apply (second run)" lm migrate-card-pages --apply
after="$(git -C "$vault" status --short | sha256sum; git -C "$vault" diff | sha256sum)"
if [[ "$before" == "$after" ]]; then echo "second run: the copy is unchanged"; else echo "SECOND RUN CHANGED THE COPY"; fi

echo
echo "=== promote --source memorycard --id N for each migrated memory: each must say already promoted ==="
ids="$(perl -ne 'print "$1\n" if /^migrated: .* memorycard memory (\d+)\)/' "$work/apply.txt" | sort -un | head -20)"
if [[ -z "$ids" ]]; then echo "no page was migrated: nothing to promote"; fi
for id in $ids; do
  show "promote --source memorycard --id $id" lm promote --source memorycard --id "$id"
done
echo
echo "=== diff --stat after the promotes (still only the migrated pages and the sidecar) ==="
git -C "$vault" diff --stat | tail -1

echo
echo "=== sync --dry-run AFTER the migration: the promotions must be the same as BEFORE ==="
show "sync --dry-run AFTER" lm sync --dry-run | tee "$work/sync-after.txt"
promotions() { rg '^  would promote:|would promote [0-9]+ memory' "$1" | perl -pe 's/, patch .*//; s/ into .*//'; }
if diff <(promotions "$work/sync-before.txt") <(promotions "$work/sync-after.txt") >/dev/null; then
  echo "sync dry run: the same promotions before and after"
else
  echo "sync dry run: THE PROMOTIONS DIFFER (a migrated page's memory would be promoted again):"
  diff <(promotions "$work/sync-before.txt") <(promotions "$work/sync-after.txt") || true
fi

echo
echo "=== doctor: the findings must be the same before and after ==="
findings >"$work/doctor-after.txt" || true
if diff "$work/doctor-before.txt" "$work/doctor-after.txt" >/dev/null; then
  echo "doctor: the same findings before and after"
else
  echo "doctor: THE FINDINGS DIFFER:"
  diff "$work/doctor-before.txt" "$work/doctor-after.txt" || true
fi

echo
echo "=== sync (a real sync of the COPY: the index rebuild runs on the migrated vault) ==="
show "sync" lm sync
echo
show "migrate-card-pages (dry run after everything): the legacy pages that are left" lm migrate-card-pages

live_after="$(live_digest)"
echo
echo "live vault digest after:  $live_after"
if [[ "$live_before" == "$live_after" ]]; then echo "the live vault is unchanged"; else echo "THE LIVE VAULT CHANGED (something else wrote it while this ran)"; fi
echo
echo "done. The copies are in $work (remove with: rm -rf \"$work\")"
