# shellcheck shell=bash
#
# bootstrap
#
# The bootstrap command: initialize the overlay repository and its upstream and main branches
# from a gentle-ai snapshot.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# Known limitations of bootstrap (left out of scope by requirement R-009):
#   - The bootstrap init block's human-readable info echoes (dir creation) remain
#     skills-first. They are a no-op for the role agents (custom rows) and only matter for a
#     hypothetical future managed agent row.
#   - No native Codex agent-definition files are produced (R-009).
#   - No SDD orchestrator auto-spawn wiring (R-009).

# repo_has_commit succeeds when the repository in the current directory has a
# commit and fails when it has none yet; it dies when git cannot say. "No commit
# yet" is a HEAD that names a branch that does not exist, and nothing else:
# a branch whose commit is gone, a ref that is not one, a HEAD that is a commit
# that is gone, a repository git cannot open are all errors, not an empty
# repository, and must not be committed over.
repo_has_commit() {
  if git rev-parse --verify --quiet 'HEAD^{commit}' >/dev/null 2>&1; then
    return 0
  fi
  local head_ref status=0
  if head_ref="$(git symbolic-ref --quiet HEAD 2>/dev/null)"; then
    git show-ref --verify --quiet "$head_ref" >/dev/null 2>&1 || status=$?
    # show-ref exits 1 for a ref that is not there; any other failure is git's.
    [[ "$status" -eq 1 ]] && return 1
  fi
  die "cannot read the history of $(pwd): git cannot tell whether HEAD has a commit (a corrupt repository?); nothing was committed"
}

cmd_bootstrap() {
  # The engine says where each row goes (manifest_routes_load below), and
  # bootstrap writes a repository and commits to it before it asks: make the
  # engine first, so that a machine that cannot (no go, no binary) stops with
  # nothing written, and while the engine source is still in the tree the
  # branches are cut from.
  ensure_engine_binary >&2

  info "Bootstrapping overlay repo at $OVERLAY_DIR"

  # 1. Init git repo if needed
  if [[ ! -d "$OVERLAY_DIR/.git" ]]; then
    git init "$OVERLAY_DIR"
  fi

  # shellcheck disable=SC2164  # errexit is on (set -e in bin/labdrian-overlay): a failed cd ends the run
  cd "$OVERLAY_DIR"

  # 2. Ensure all required directories exist
  mkdir -p bin \
    skills/sdd-spec \
    skills/sdd-tasks \
    skills/sdd-verify/references \
    skills/requirements-from-transcripts

  # 3. Write scaffold files if not present
  if [[ ! -f "$OVERLAY_DIR/README.md" ]]; then
    echo "# labdrian-sdd-overlay" > "$OVERLAY_DIR/README.md"
  fi
  if [[ ! -f "$OVERLAY_DIR/overlay.manifest" ]]; then
    die "overlay.manifest not found — ensure it exists before running bootstrap"
  fi
  if [[ ! -f "$OVERLAY_DIR/.gitignore" ]]; then
    printf '*.swp\n*.swo\n*~\n.DS_Store\n' > "$OVERLAY_DIR/.gitignore"
  fi

  # 4. Stage scaffold (bin/overlay itself is already here; stage it too)
  git add README.md overlay.manifest .gitignore bin/overlay

  # 5. Initial commit (only if none exists). Whether there is a commit is asked of
  # git's plumbing and not of a pipeline: `git log | head -1 | grep -q .` under
  # pipefail loses a race, because head and grep leave as soon as they have a
  # line, and when git log is still writing it is killed by SIGPIPE, the pipeline
  # fails, and the guard reads "no commit yet" for a repository that has them (so
  # a second bootstrap stopped here one run in three, trying to commit what is
  # committed). And `git log ... || true` read any failure as "no commit yet":
  # see repo_has_commit.
  if ! repo_has_commit; then
    git commit -m "chore: initial overlay scaffold"
  fi

  # 6. Create/switch to upstream branch
  git checkout -B upstream

  # 7. Extract managed files from tarball
  info "Extracting managed files from tarball: $TARBALL"
  if [[ ! -f "$TARBALL" ]]; then
    die "Tarball not found: $TARBALL"
  fi

  # Pre-build tar listing once to avoid SIGPIPE issues with set -euo pipefail
  local tar_listing
  tar_listing="$(tar tzf "$TARBALL" 2>/dev/null)"

  # The destination of a row is not read here (the files go into the
  # repository); claude is only the target the engine is asked to answer for.
  local rr_route rr_repo_rel rr_src rr_dest tar_path dest
  manifest_routes_load claude managed
  while IFS=$'\t' read -r _ rr_route rr_repo_rel rr_src _; do
    tar_path="$(route_tar_path "$rr_route" "$rr_repo_rel")"
    dest="$rr_src"
    ensure_parent "$dest"

    if grep -qF "$tar_path" <<< "$tar_listing"; then
      tar xzf "$TARBALL" --to-stdout "$tar_path" > "$dest"
      info "  extracted: ${rr_repo_rel}"
    else
      echo "  WARNING: not found in tarball, skipping: $tar_path"
    fi
  done < <(manifest_routes_rows)

  # 8. Commit upstream baseline (only if there are changes)
  git add skills/
  test -d agents && git add agents/ || true
  test -d opencode && git add opencode/ || true
  if [[ -n "$(git status --porcelain)" ]]; then
    git commit -m "upstream: baseline from gentle-ai $TARBALL_GENTLE_AI_VERSION"
  else
    info "No changes to commit on upstream (already up to date)"
  fi

  # 9. Create main from upstream (AFTER upstream commit)
  git checkout -B main upstream

  # 10. Copy current customized files from live targets onto main
  info "Layering customized files onto main..."
  local rel_path src
  manifest_routes_load claude
  while IFS=$'\t' read -r rel_path _ _ rr_src rr_dest; do
    # Source from the claude live location when applicable (bootstrap layers from claude).
    [[ -z "$rr_dest" ]] && continue

    src="${rr_dest}"
    dest="$rr_src"   # repo dest
    ensure_parent "$dest"

    if [[ -f "$src" ]]; then
      cp "$src" "$dest"
      info "  copied: $rel_path"
    else
      echo "  WARNING: source file not found, skipping: $src"
    fi
  done < <(manifest_routes_rows)

  # 11. Commit overlay
  git add skills/
  test -d agents && git add agents/ || true
  test -d opencode && git add opencode/ || true
  if [[ -n "$(git status --porcelain)" ]]; then
    git commit -m "overlay: EARS notation + stable R-NNN IDs"
  else
    info "No customization diff detected — main is identical to upstream"
  fi

  echo ""
  echo "Bootstrap complete."
  echo "  upstream branch: pristine vendor baseline"
  echo "  main branch:     your customized overlay (current)"
  echo ""
  git log --oneline --all
}
