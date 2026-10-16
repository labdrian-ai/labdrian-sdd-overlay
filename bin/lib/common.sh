# shellcheck shell=bash
#
# common
#
# The helpers every other module uses and none redefines: the messages (die, info, warn),
# ensure_parent, and path_clean.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# path_clean <path> prints <path> lexically normalized: duplicate separators
# collapsed, "." elements removed, ".." elements resolved against the element
# before them, and any trailing separator dropped. "" becomes ".", and ".."
# above the root of a rooted path is the root.
#
# It reproduces Go's filepath.Clean on purpose, byte for byte, because that
# is the OTHER half of a contract this script cannot see. The engine derives
# the longterm-mem binary path with filepath.Join (which cleans), rebuilds
# each runtime's MCP entry from it, and decides ownership by comparing that
# rebuilt entry against the live one as an EXACT STRING fingerprint. This
# script builds the same path by concatenation (which does not clean) and
# hands that literal to `longterm-mem register --binary`, which is what
# actually gets written into the config. The two therefore agreed only while
# $STATE_DIR happened to be lexically clean already: one trailing slash and
# the overlay wrote "<dir>//bin/longterm-mem" while the engine looked for
# "<dir>/bin/longterm-mem" and reported the overlay's own freshly-written
# entry as unmanaged.
#
# Purely lexical, deliberately: it never touches the filesystem and never
# resolves a symlink, because the Go half it must agree with does not
# either. `realpath` would be the wrong tool here even where it exists —
# resolving symlinks would produce a path the engine never derives.
#
# Defined in this module, which is sourced before the one that derives those
# paths (locations), because they are assigned at source time and a function
# must exist before it is called.
path_clean() {
  local path="${1:-}"
  local rooted="" out="" elem rest

  [[ "$path" == /* ]] && rooted=1
  rest="$path"

  while [[ -n "$rest" ]]; do
    elem="${rest%%/*}"
    if [[ "$rest" == */* ]]; then
      rest="${rest#*/}"
    else
      rest=""
    fi

    case "$elem" in
      '' | '.')
        # A duplicate separator or a "." element names nothing.
        ;;
      '..')
        if [[ -n "$rooted" || ( -n "$out" && "$out" != ".." && "$out" != *"/.." ) ]]; then
          # Pop the previous element. A rooted path with nothing left to
          # pop stays at the root: "/.." is "/".
          if [[ "$out" == */* ]]; then
            out="${out%/*}"
          else
            out=""
          fi
        else
          # A RELATIVE path can lead with ".." elements that nothing
          # cancels; those are part of the answer, not noise.
          out="${out:+$out/}.."
        fi
        ;;
      *)
        out="${out:+$out/}$elem"
        ;;
    esac
  done

  if [[ -n "$rooted" ]]; then
    printf '%s\n' "/$out"
  elif [[ -n "$out" ]]; then
    printf '%s\n' "$out"
  else
    printf '%s\n' "."
  fi
}

die() { echo "ERROR: $*" >&2; exit 1; }
info() { echo "==> $*"; }
warn() { echo "WARN: $*" >&2; }

# Ensure the parent directory of a path exists
ensure_parent() {
  local path="$1"
  local parent
  parent="$(dirname "$path")"
  mkdir -p "$parent"
}
