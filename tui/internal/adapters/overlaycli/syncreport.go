package overlaycli

import (
	"bufio"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// parseSyncCheck extracts per-target verdicts from sync-check stdout.
//
// It reads the machine-friendly lines emitted by the backend:
//
//	VERDICT:<target>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M REPO_BEHIND_ORIGIN=<n|NA>
//	ACTION:<target>: <text>
//
// It also tracks `=== sync-check: <target> ===` section headers to set the
// active target scope, and captures per-file `agents/...` status lines
// (format: `  <path>: <STATUS>`) into [domain.TargetVerdict.AgentFiles].
//
// Verdicts are returned in first-seen order, each classified by
// domain.Classify; output that holds none yields none.
func parseSyncCheck(output string) []domain.TargetVerdict {
	order := []string{}
	byTarget := map[string]*domain.TargetVerdict{}

	get := func(name string) *domain.TargetVerdict {
		v, ok := byTarget[name]
		if !ok {
			// RepoBehindOrigin/RepoBehindRelease default to RepoBehindOriginNA
			// until a VERDICT line explicitly sets them — mirrors the
			// backend's NA sentinel and avoids a defensive zero-value
			// collapsing into "0 behind".
			v = &domain.TargetVerdict{Target: name, RepoBehindOrigin: domain.RepoBehindOriginNA, RepoBehindRelease: domain.RepoBehindOriginNA}
			byTarget[name] = v
			order = append(order, name)
		}
		return v
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var currentTarget string // tracks the active section for per-file lines
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Section header: "=== sync-check: <target> (<path>) ==="
		if strings.HasPrefix(line, "=== sync-check: ") {
			rest := strings.TrimPrefix(line, "=== sync-check: ")
			currentTarget, _, _ = strings.Cut(rest, " ")
			continue
		}

		if rest, ok := strings.CutPrefix(line, "VERDICT:"); ok {
			// rest = "<target>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M"
			target, counts, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			v := get(target)
			for _, field := range strings.Fields(counts) {
				key, val, ok := strings.Cut(field, "=")
				if !ok {
					continue
				}
				switch key {
				case "UPSTREAM_CHANGED":
					n, _ := strconv.Atoi(val)
					v.UpstreamChanged = n
				case "OVERLAY_NOT_DEPLOYED":
					n, _ := strconv.Atoi(val)
					v.OverlayNotDeployed = n
				case "REPO_BEHIND_ORIGIN":
					// Dedicated NA pre-check: the field's literal "NA" sentinel
					// MUST be handled before strconv.Atoi. Falling through to a
					// generic `n, _ := strconv.Atoi(val)` (as UPSTREAM_CHANGED/
					// OVERLAY_NOT_DEPLOYED do) would discard the parse error and
					// silently collapse "NA" (or an omitted field) to Go's
					// zero-value 0 — indistinguishable from "confirmed 0 commits
					// behind" and reproducing the exact silent-healthy bug class
					// R-006 exists to eliminate.
					if val == "NA" {
						v.RepoBehindOrigin = domain.RepoBehindOriginNA
					} else if n, err := strconv.Atoi(val); err == nil {
						v.RepoBehindOrigin = n
					}
				case "REPO_BEHIND_RELEASE":
					// Same dedicated NA pre-check as REPO_BEHIND_ORIGIN above,
					// for the same reason (D2 — a new field, same
					// silent-zero-collapse risk).
					if val == "NA" {
						v.RepoBehindRelease = domain.RepoBehindOriginNA
					} else if n, err := strconv.Atoi(val); err == nil {
						v.RepoBehindRelease = n
					}
				case "RECORDED_VERSION":
					v.RecordedVersion = val
				case "DIGEST_MATCH":
					v.DigestMatch = val
				}
			}
			v.Status = domain.Classify(v.UpstreamChanged, v.OverlayNotDeployed, v.RepoBehindOrigin, v.RepoBehindRelease, v.DigestMatch == "no")
			continue
		}

		if rest, ok := strings.CutPrefix(line, "ACTION:"); ok {
			// rest = "<target>: <text>"
			target, text, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			v := get(target)
			v.Action = strings.TrimSpace(text)
			continue
		}

		// Per-file lines: capture agents/ file statuses within the current section.
		for _, s := range []string{"IN_SYNC", "OVERLAY_NOT_DEPLOYED", "UPSTREAM_CHANGED"} {
			if rest, ok := strings.CutPrefix(line, s+": "); ok {
				if strings.HasPrefix(rest, "agents/") && currentTarget != "" {
					path, _, _ := strings.Cut(rest, " ") // strip "(detail)" suffix
					v := get(currentTarget)
					v.AgentFiles = append(v.AgentFiles, domain.AgentFileEntry{Path: path, Status: s})
				}
				break
			}
		}
	}

	result := make([]domain.TargetVerdict, 0, len(order))
	for _, name := range order {
		result = append(result, *byTarget[name])
	}
	return result
}
