# shellcheck shell=bash
#
# hooks
#
# The commands that wire the engine's hooks into ~/.claude/settings.json: install-hooks,
# status-hooks and uninstall-hooks.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# Design decisions:
#   1. SEPARATE from 'labdrian apply'. Skills deployment and hook wiring are
#      distinct operations — apply only copies files, never mutates
#      ~/.claude/settings.json. This keeps apply non-invasive.
#   2. DELIBERATE activation. The user must run 'labdrian install-hooks'
#      intentionally. This satisfies the "INERT until user activates" contract.
#   3. MISSING-BINARY SAFETY. Hook commands emitted by the engine use the form:
#        command -v <binary> &>/dev/null && <binary> ... || true
#      This ensures any hook exits 0 even when the binary is absent, so no
#      Claude Code Task is ever blocked by a missing binary.
#   4. ATOMIC+BACKUP inside the engine. The engine's merge-settings subcommand
#      writes to a temp file and renames; it also copies the original to
#      settings.json.bak before overwriting.

cmd_install_hooks() {
  command -v go >/dev/null 2>&1 || die "go is not installed; required to build the engine"
  command -v gentle-ai >/dev/null 2>&1 || warn "gentle-ai not found on PATH — the overlay layers onto gentle-ai; hooks will install but skill propagation/scoping may be inert until gentle-ai is installed. Run 'labdrian doctor' for a full preflight."
  [[ -d "$ENGINE_SRC" ]] || die "engine source not found at $ENGINE_SRC"

  info "Building engine binary..."
  mkdir -p "$(dirname "$ENGINE_BINARY")"
  (cd "$ENGINE_SRC" && go build -o "$ENGINE_BINARY" ./cmd/) \
    || die "go build failed; binary not deployed"
  info "Engine binary deployed to: $ENGINE_BINARY"

  local settings_path="$HOME/.claude/settings.json"
  info "Merging hooks into: $settings_path"
  "$ENGINE_BINARY" merge-settings \
    --settings "$settings_path" \
    --hook-command "$ENGINE_BINARY" \
    || die "merge-settings failed"

  echo ""
  echo "install-hooks complete."
  echo "  binary  : $ENGINE_BINARY"
  echo "  settings: $settings_path (backup at ${settings_path}.bak)"
  echo "  hooks   : the overlay hook families, wired into UserPromptSubmit, PreToolUse, and SessionEnd:"
  echo "            deterministic scoping (minimalism-contract and anti-generic-design),"
  echo "            the SessionEnd longterm-mem sync, the review-receipt and shaper clearance guards,"
  echo "            the workflow projection hooks, and the skills approve guard"
  echo "            (denies the agent running 'skills approve'; a speed bump, not a security boundary)."
  echo ""
  echo "Deterministic scoping is now ACTIVE for Claude Code sessions once you restart Claude Code,"
  echo "which loads hooks only when it starts (re-run install-hooks after an upgrade that adds a family)."
  echo ""
  echo "Run 'labdrian doctor' to verify the full installation."

  # Best-effort: provision discovery tools so agents don't hit the silent-empty
  # footgun on a fresh install. Non-fatal — a failure here must never abort
  # install-hooks.
  # Set OVERLAY_SKIP_TOOL_INSTALL=1 to opt out (e.g. in CI or managed hosts).
  if [[ -n "${OVERLAY_SKIP_TOOL_INSTALL:-}" ]]; then
    info "OVERLAY_SKIP_TOOL_INSTALL is set — skipping automatic discovery-tool installation."
    warn "Discovery tools (bat rg fd sd eza) were NOT auto-installed."
    warn "Run 'labdrian doctor --fix' later to provision them when ready."
  else
    echo ""
    echo "==> Installing host packages via Homebrew: discovery tools (bat rg fd sd eza)."
    echo "    Set OVERLAY_SKIP_TOOL_INSTALL=1 to skip this step."
    ensure_discovery_tools
  fi
}

# Read-only health check for overlay install status:
#   - Engine binary present + executable at $ENGINE_BINARY
#   - Hooks wired in ~/.claude/settings.json (UserPromptSubmit + PreToolUse/Agent)
#   - Contract readable with valid frontmatter
#   - (best-effort) Scoped block present in CWD's .atl/skill-registry.md
#
# Exits 0 if all checks pass, 1 otherwise.
# Read-only by design: no binary builds/writes.

cmd_status_hooks() {
  if [[ ! -x "$ENGINE_BINARY" ]]; then
    # 'install-hooks' is no longer the narrowest remedy: doctor --fix rebuilds
    # the binary without also rewriting ~/.claude/settings.json. Hook wiring is
    # still named, because an engine that was never installed needs it too.
    die "engine binary not found at $ENGINE_BINARY; run 'labdrian doctor --fix' to build it, or run 'labdrian install-hooks' to build it and wire the hooks"
  fi

  # The engine binary's freshness belongs to THIS status command, not to
  # cmd_status: cmd_status reports per-target file/version sync of deployed
  # skills, while status-hooks is the read-only health check for the overlay's
  # own installation — the engine binary and its hooks. The TUI's "Estado" row
  # folds status-hooks in, so one line here surfaces there with no TUI change.
  # One line, because status is a summary; 'labdrian doctor' is the report.
  if engine_binary_is_stale; then
    warn "$(engine_stale_diagnosis)"
  fi

  exec "$ENGINE_BINARY" status
}

cmd_uninstall_hooks() {
  local settings_path="$HOME/.claude/settings.json"

  if [[ ! -f "$ENGINE_BINARY" ]]; then
    # Binary absent: fall back to a fresh build to run uninstall.
    # If we cannot build, error out — we cannot safely remove entries.
    command -v go >/dev/null 2>&1 || die "engine binary not found at $ENGINE_BINARY and go is not installed; cannot uninstall"
    [[ -d "$ENGINE_SRC" ]] || die "engine source not found at $ENGINE_SRC; cannot build for uninstall"
    info "Engine binary not found; building to perform uninstall..."
    mkdir -p "$(dirname "$ENGINE_BINARY")"
    (cd "$ENGINE_SRC" && go build -o "$ENGINE_BINARY" ./cmd/) \
      || die "go build failed; cannot uninstall hooks"
  fi

  info "Removing hooks from: $settings_path"
  "$ENGINE_BINARY" uninstall-hooks \
    --settings "$settings_path" \
    --hook-command "$ENGINE_BINARY" \
    || die "uninstall-hooks failed"

  echo ""
  echo "uninstall-hooks complete."
  echo "  Every overlay hook entry (scoping, the SessionEnd sync, the review-receipt and shaper guards,"
  echo "  the workflow projection hooks, and the skills approve guard) has been removed from $settings_path"
  echo "  (backup at ${settings_path}.bak if the file was modified)"
  echo ""
  echo "Run 'labdrian doctor' to verify the installation state."
}
