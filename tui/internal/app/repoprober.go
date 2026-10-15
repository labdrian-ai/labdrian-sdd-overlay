package app

import "github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"

// RepoProber tells how far the repository is behind, from the launch-time
// probe the TUI shows at the top of every screen.
type RepoProber struct {
	probe domain.RepoProbe
}

// NewRepoProber returns the prober over the probe that reads the repository.
func NewRepoProber(probe domain.RepoProbe) RepoProber {
	return RepoProber{probe: probe}
}

// Behind reads how many commits the repository is behind origin/main and how
// many it is behind the latest release tag, from one report (D2: no second run
// is spent to pick up the release count, and the report is read once). Either
// is domain.RepoBehindOriginNA when the report does not say, which is not the
// same as "up to date": a report that cannot be read must never look like a
// repository that is current.
func (p RepoProber) Behind() domain.RepoBehind {
	return domain.BehindOf(p.probe.Report())
}
