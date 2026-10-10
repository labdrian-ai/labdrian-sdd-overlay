package workflow

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// ProfileDriftKind says how the catalog has come to differ from the profile a workflow recorded.
type ProfileDriftKind string

const (
	// ProfileChanged means the catalog has a profile of the recorded name, and it is not the
	// recorded one: some of its declarations differ (ProfileDrift.Fields).
	ProfileChanged ProfileDriftKind = "changed"
	// ProfileRetired means the catalog has no profile of the recorded name any more.
	ProfileRetired ProfileDriftKind = "retired"
)

// ProfileDrift is the finding that a workflow's recorded profile is no longer the catalog's. It is a
// report and nothing else: a workflow of version 2 goes on from its snapshot, so drift changes what
// no verb does. What is done with the finding (shown, warned of, refused) is for the caller.
type ProfileDrift struct {
	Kind ProfileDriftKind
	// Profile is the name the workflow recorded.
	Profile string
	// RecordedDigest is the digest of the snapshot in the log. CurrentDigest is the digest of the
	// catalog's profile of that name, and is empty when it is retired.
	RecordedDigest string
	CurrentDigest  string
	// Fields names, in the order of the snapshot's encoding, the declarations in which the
	// catalog's profile differs from the recorded one (see workflowprofile.Snapshot.DifferingFields);
	// it is empty when the profile is retired.
	Fields []string
}

// DetectProfileDrift says whether the profile recorded in the log of the workflow in state is still
// the one the catalog gives for its name. It reports nothing (nil) for a workflow of version 1, which
// recorded a name and no profile and so is the catalog's by definition, and for a workflow whose
// snapshot is the catalog's. A catalog that refuses with workflowprofile.ErrUnknownProfile has
// retired the profile; a catalog that fails otherwise has said nothing, so that is the error and not a
// finding.
func DetectProfileDrift(state State, catalog ProfileCatalog) (*ProfileDrift, error) {
	if state.ProfileSnapshot == nil {
		return nil, nil
	}
	recorded := *state.ProfileSnapshot
	recordedDigest, err := recorded.Digest()
	if err != nil {
		return nil, fmt.Errorf("workflow profile drift: %w", err)
	}
	current, err := catalog.Resolve(state.Profile)
	if errors.Is(err, workflowprofile.ErrUnknownProfile) {
		return &ProfileDrift{Kind: ProfileRetired, Profile: state.Profile, RecordedDigest: recordedDigest}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("workflow profile drift: %w", err)
	}
	snapshot := current.Snapshot()
	currentDigest, err := snapshot.Digest()
	if err != nil {
		return nil, fmt.Errorf("workflow profile drift: %w", err)
	}
	if currentDigest == recordedDigest {
		return nil, nil
	}
	return &ProfileDrift{
		Kind:           ProfileChanged,
		Profile:        state.Profile,
		RecordedDigest: recordedDigest,
		CurrentDigest:  currentDigest,
		Fields:         recorded.DifferingFields(snapshot),
	}, nil
}

// ProfileDrift says whether the profile the workflow recorded is still this Lifecycle's catalog's (see
// DetectProfileDrift). It reads the workflow and appends nothing; a workflow that is not owned has
// none to report (ErrWorkflowNotOwned).
func (l Lifecycle) ProfileDrift(projectID, workflowID string) (*ProfileDrift, error) {
	loaded, err := l.loadOwned(projectID, workflowID)
	if err != nil {
		return nil, err
	}
	return DetectProfileDrift(loaded.State, l.profiles)
}
