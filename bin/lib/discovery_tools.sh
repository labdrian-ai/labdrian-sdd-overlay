# shellcheck shell=bash
#
# discovery_tools
#
# The discovery tools (bat rg fd sd eza): their canonical list, and the best-effort provisioner
# and re-check that install-hooks and doctor share.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# Discovery tools — canonical list used by ensure_discovery_tools,
# recheck_discovery_tools, and cmd_doctor. Change once here to update all.

DISCOVERY_TOOLS=(bat rg fd sd eza)

# Best-effort, NON-FATAL, idempotent provisioner for the five CLI tools the
# persona rule mandates: bat rg fd sd eza.
#
# Contract:
#   - PORTABLE: uses only 'command -v' for detection; never calls fd/eza/bat
#     inside the helper itself.
#   - NON-FATAL: a provisioning failure MUST NOT abort the caller. All brew
#     invocations are guarded against 'set -e' via explicit if/then/else.
#   - TRANSPARENT: prints exactly what it will install and what succeeded/failed.
#   - IDEMPOTENT: skips tools already present; re-runs are safe.
#   - NO SIGPIPE: all output captured to vars before any comparison.
#
# Called by:
#   - cmd_install_hooks (best-effort, auto): runs after hooks are wired.
#   - cmd_doctor --fix (explicit opt-in): runs then re-checks tool status.

ensure_discovery_tools() {
  local -a missing=()

  # Probe each of the five mandated tools with 'command -v' — never assume
  # any are present, never use them for the probe itself.
  local t
  for t in "${DISCOVERY_TOOLS[@]}"; do
    command -v "$t" >/dev/null 2>&1 || missing+=("$t")
  done

  if [[ ${#missing[@]} -eq 0 ]]; then
    info "Discovery tools: all present (bat rg fd sd eza) — nothing to install."
    return 0
  fi

  if command -v brew >/dev/null 2>&1; then
    echo "==> Discovery tools missing: ${missing[*]}"
    echo "    Installing via Homebrew (best-effort)..."
    local -a succeeded=()
    local -a failed=()
    local tool
    for tool in "${missing[@]}"; do
      if brew install "$tool"; then
        # Re-probe after install to confirm it landed on PATH.
        if command -v "$tool" >/dev/null 2>&1; then
          succeeded+=("$tool")
        else
          # brew reported success but binary still not on PATH (e.g. PATH not
          # refreshed, cask install, prefix mismatch). Treat as non-fatal warn.
          warn "brew install $tool reported success but '$tool' is still not on PATH. You may need to reload your shell or adjust PATH."
          failed+=("$tool")
        fi
      else
        warn "brew install $tool failed — skipping (non-fatal)."
        failed+=("$tool")
      fi
    done
    if [[ ${#succeeded[@]} -gt 0 ]]; then
      info "Installed: ${succeeded[*]}"
    fi
    if [[ ${#failed[@]} -gt 0 ]]; then
      warn "Could not install: ${failed[*]}"
      warn "Install them manually so agents do not hit the silent-empty footgun."
    fi
  else
    # No brew — warn loudly about the silent-empty trap but stay non-fatal.
    warn "Homebrew not found. The following discovery tools are missing: ${missing[*]}"
    warn "Without them, agents that run e.g. 'fd ... 2>/dev/null' get an empty"
    warn "result (command not found, swallowed) and may misread it as zero skills."
    warn "Install manually — typical options:"
    warn "  cargo install bat ripgrep fd-find sd eza   (Rust/cargo — most reliable)"
    warn "  Debian/Ubuntu (apt): 'bat' installs as 'batcat', 'fd' as 'fd-find' (binary:"
    warn "    'fdfind'); 'sd' and 'eza' are often absent or only in recent repos —"
    warn "    prefer cargo or a newer apt source for those."
    warn "  Fedora/RHEL (dnf): bat ripgrep fd-find eza  (sd may need cargo)"
    warn "  Arch (pacman): bat ripgrep fd sd eza"
  fi

  return 0  # always non-fatal
}

# Re-probes DISCOVERY_TOOLS after a provisioning attempt and prints
# PASS or WARN. Single source of truth — used by both branches of cmd_doctor
# --fix (fail!=0 and fail==0). Always returns 0 (non-fatal).

recheck_discovery_tools() {
  local -a recheck_missing=()
  local t
  for t in "${DISCOVERY_TOOLS[@]}"; do
    command -v "$t" >/dev/null 2>&1 || recheck_missing+=("$t")
  done
  if [[ ${#recheck_missing[@]} -eq 0 ]]; then
    echo "PASS  ${DISCOVERY_TOOLS[*]} all present after provisioning."
  else
    echo "WARN  still missing after provisioning: ${recheck_missing[*]}"
  fi
  return 0
}
