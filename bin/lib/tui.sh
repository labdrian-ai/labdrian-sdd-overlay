# shellcheck shell=bash
# shellcheck disable=SC2164  # errexit is on (set -e in bin/labdrian-overlay): a failed cd ends the run
#
# tui
#
# The tui command, and the install-alias command that makes the name to start it by.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

cmd_tui() {
  command -v go >/dev/null 2>&1 || die "go is not installed; required to run the TUI"
  [[ -d "$OVERLAY_DIR/tui" ]] || die "tui/ not found at $OVERLAY_DIR/tui"
  # First-run convenience: a fresh clone only checks out `main`, but sync-check and
  # the dashboard need a local `upstream` ref. Materialize it from origin/upstream if
  # missing, so the user can go straight from `install-alias` to `tui` and do
  # everything else (capture / apply / install-hooks) from inside the dashboard.
  if ! git -C "$OVERLAY_DIR" show-ref --verify --quiet refs/heads/upstream \
     && git -C "$OVERLAY_DIR" show-ref --verify --quiet refs/remotes/origin/upstream; then
    git -C "$OVERLAY_DIR" branch --force upstream origin/upstream >/dev/null 2>&1 \
      && info "Materialized local 'upstream' branch from origin/upstream."
  fi
  # Export repo root so the Go program can locate bin/labdrian-overlay reliably.
  export OVERLAY_DIR
  # Run from inside the module dir (repo root is not a Go module).
  cd "$OVERLAY_DIR/tui"
  exec go run .
}

cmd_install_alias() {
  local name="${1:-labdrian}"
  mkdir -p "$HOME/.local/bin"
  ln -sf "$OVERLAY_DIR/bin/labdrian-overlay" "$HOME/.local/bin/$name"
  echo "Linked '$name' -> overlay. Run '$name tui' (needs ~/.local/bin on PATH)."
}
