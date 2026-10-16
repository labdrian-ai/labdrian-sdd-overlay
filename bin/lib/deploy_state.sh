# shellcheck shell=bash
#
# deploy_state
#
# What was deployed: the digest of a target's managed files, and the state file that records
# the version and the digest of each deployment (read and written here).
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# compute_target_digest <target> <live|ref>
#
# Design decision D5: one aggregate sha256 digest per deployment target,
# over sorted "<repo_rel>:<filehash>" lines -- one line per MANAGED manifest
# row that routes to <target> (custom rows are user-owned and excluded,
# matching overlay-release-state R-002's "managed file" wording). Manifest
# row order is not itself sorted, so lines are sorted with LC_ALL=C before
# hashing: this is what makes the digest invariant to manifest reordering
# while still sensitive to any single file's content.
#
# "live" reads each row's deployed destination file for <target> (what's
# actually installed there right now); "ref" reads the repo-source file
# instead (this checkout's current working tree content, independent of
# whether -- or what -- has been deployed). A missing file degrades to the
# literal "MISSING" token rather than aborting, so a never-deployed target
# still yields a real (if divergent) digest instead of erroring.
compute_target_digest() {
  local target="$1"
  local mode="$2"

  local lines=()
  local rr_repo_rel rr_src rr_dest path filehash
  manifest_routes_load "$target" managed
  while IFS=$'\t' read -r _ _ rr_repo_rel rr_src rr_dest; do
    [[ -z "$rr_dest" ]] && continue

    if [[ "$mode" == "ref" ]]; then
      path="$rr_src"
    else
      path="$rr_dest"
    fi

    if [[ -f "$path" ]]; then
      filehash="$(sha256sum "$path" | awk '{ print $1 }')"
    else
      filehash="MISSING"
    fi

    lines+=("${rr_repo_rel}:${filehash}")
  done < <(manifest_routes_rows)

  printf '%s\n' "${lines[@]}" | LC_ALL=C sort | sha256sum | awk '{ print $1 }'
}

# state_read_target <target>
#
# Design decision D8: tolerant awk-based reader over the canonical JSON
# state file. Emits "<version>\t<digest>\t<applied_at>" for a recorded
# entry, or exactly the token "NEVER_DEPLOYED" when the state file is
# absent, has no entry for <target>, or fails to parse -- an unparseable
# file also prints a WARN rather than crashing (overlay-release-state
# R-001's "never deployed" honesty requirement).
state_read_target() {
  local target="$1"

  [[ -f "$STATE_FILE" ]] || { echo "NEVER_DEPLOYED"; return; }

  local line
  line="$(awk -v t="\"${target}\":" 'index($0, t) { print; exit }' "$STATE_FILE" 2>/dev/null || true)"

  if [[ -z "$line" ]]; then
    if [[ -s "$STATE_FILE" ]]; then
      warn "state file $STATE_FILE has no parseable entry for target '$target' -- reporting never deployed"
    fi
    echo "NEVER_DEPLOYED"
    return
  fi

  local version digest applied_at
  version="$(echo "$line" | awk -F'"version"[[:space:]]*:[[:space:]]*"' '{ print $2 }' | awk -F'"' '{ print $1 }')"
  digest="$(echo "$line" | awk -F'"digest"[[:space:]]*:[[:space:]]*"' '{ print $2 }' | awk -F'"' '{ print $1 }')"
  applied_at="$(echo "$line" | awk -F'"applied_at"[[:space:]]*:[[:space:]]*"' '{ print $2 }' | awk -F'"' '{ print $1 }')"

  if [[ -z "$version" || -z "$digest" ]]; then
    warn "state file $STATE_FILE entry for target '$target' is malformed -- reporting never deployed"
    echo "NEVER_DEPLOYED"
    return
  fi

  printf '%s\t%s\t%s\n' "$version" "$digest" "$applied_at"
}

# state_write_target <target> <version> <digest>
#
# Design decision D8: atomic tmp+mv rewrite of the whole canonical state
# file. Existing entries for OTHER targets are preserved by re-reading them
# through state_read_target -- if the prior file was corrupt, every "other"
# target degrades to never-deployed too (state_read_target's own
# tolerant-degrade behavior), so a corrupt state file self-heals into a
# clean one containing only what this call knows, rather than propagating
# the corruption forever.
state_write_target() {
  local target="$1"
  local version="$2"
  local digest="$3"
  local applied_at
  applied_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  mkdir -p "$STATE_DIR"

  local t existing v d a
  local entries=()
  for t in "${!TARGET_PATHS[@]}"; do
    [[ "$t" == "$target" ]] && continue
    existing="$(state_read_target "$t" 2>/dev/null || true)"
    [[ "$existing" == "NEVER_DEPLOYED" || -z "$existing" ]] && continue
    IFS=$'\t' read -r v d a <<< "$existing"
    entries+=("    \"${t}\": {\"version\": \"${v}\", \"digest\": \"${d}\", \"applied_at\": \"${a}\"}")
  done
  entries+=("    \"${target}\": {\"version\": \"${version}\", \"digest\": \"${digest}\", \"applied_at\": \"${applied_at}\"}")

  local tmp
  tmp="$(mktemp "${STATE_FILE}.XXXXXX")"
  {
    echo "{"
    echo "  \"schema\": 1,"
    echo "  \"targets\": {"
    local i n="${#entries[@]}"
    for ((i = 0; i < n; i++)); do
      if [[ "$i" -lt $((n - 1)) ]]; then
        echo "${entries[$i]},"
      else
        echo "${entries[$i]}"
      fi
    done
    echo "  }"
    echo "}"
  } > "$tmp"
  mv "$tmp" "$STATE_FILE"
}
