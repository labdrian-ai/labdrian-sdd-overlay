package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// cmdDoctor implements `longterm-mem doctor --project P [--vault DIR]
// [--json]` (R-011, R-064): run the eight read-only diagnostic checks --
// R-011's original vault diagnostics plus R-064's embedding-index
// diagnostics -- and report each one individually. ops.Doctor always runs
// and reports all eight checks regardless of any single one's own result
// (slice 7's review
// finding: a per-item failure must never abort a whole run) -- this
// command mirrors that at its own layer: it never returns on the first
// FAIL, it always lets Doctor finish and prints every check's result
// before deciding the exit code.
//
// It prints whatever Doctor returns rather than a list of its own, so the
// count above is documentation, not behaviour -- which is exactly how it
// went on claiming a count one short of the truth once a fifth diagnostic
// was added, through a repair round that reported the contradiction
// resolved after correcting only the spec file. That is why
// TestCmdDoctor_DocumentedCheckCountMatchesOpsDoctor reads the number out
// of a real ops.Doctor run instead of trusting any file's prose.
//
// A FAIL exits exitDoctorChecksFailed (9) and nothing else does: every
// branch below that reports a failure of doctor's OWN -- an unresolvable
// vault registry, an ops.Doctor error, a JSON encode failure -- exits
// exitInternal (1). A caller therefore reads 9 as "your vault has a named
// problem, go read the report" and 1 as "doctor could not tell you
// anything".
func (c commands) cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", projectFlagUsage)
	vaultDir := fs.String("vault", "", "vault path override")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	a := c.newApp("doctor")
	resolvedProject, exit := a.project(*project, true)
	if exit != exitOK {
		return exit
	}
	vaultRoot, exit := a.vaultRoot(resolvedProject, *vaultDir)
	if exit != exitOK {
		return exit
	}

	report, err := ops.Doctor(context.Background(), a.doctorDeps(vaultRoot), resolvedProject)
	if err != nil {
		return a.refuse(err, exitInternal)
	}

	if *asJSON {
		if exit := a.printJSON(report); exit != exitOK {
			return exit
		}
	} else {
		printDoctor(report)
	}
	return doctorExitCode(report)
}

// doctorDeps is what the diagnostics of the vault at vaultRoot read: the vault's files, the embedding index under
// the state directory, the live observations of Engram and the embedding backend the commands were composed with.
//
// EmbeddingIndex, LiveObservationIDs and EmbeddingBackendCheck back the three embedding-index checks (R-064).
// This command takes no --embed-endpoint/--embed-model flags of its own (unlike `index --embeddings`), so
// embedding-backend-reachable probes the same defaults `index --embeddings` uses when none are given.
func (a app) doctorDeps(vaultRoot string) ops.DoctorDeps {
	vaultFiles := openVault(vaultRoot)
	return ops.DoctorDeps{
		VaultRoot:             vaultRoot,
		Root:                  vaultFiles,
		Index:                 vaultFiles,
		Log:                   vaultFiles,
		Precedence:            vaultFiles,
		Pages:                 vaultFiles,
		AddressMap:            vaultFiles,
		PrerequisitePresent:   vault.PrerequisitePresent,
		EmbeddingIndex:        embeddingIndexReader(defaultStateDir()),
		LiveObservationIDs:    liveObservationIDs,
		EmbeddingBackendCheck: a.probeEmbeddingBackend,
	}
}

// liveObservationIDs is the id of each live observation of project, in the order Engram lists them. It opens the
// database for the question and closes it.
func liveObservationIDs(project string) ([]int64, error) {
	store, err := engram.Open(os.Getenv(engramDBEnvVar))
	if err != nil {
		return nil, err
	}
	defer store.Close()
	// R-020: ListObservations already excludes soft-deleted rows.
	observations, err := store.ListObservations(project)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(observations))
	for i, o := range observations {
		ids[i] = o.ID
	}
	return ids, nil
}

// probeEmbeddingBackend asks the embedding backend the commands were composed with to embed one text, and says
// why it could not.
func (a app) probeEmbeddingBackend(ctx context.Context) error {
	client, err := a.newEmbedClient(embed.Config{Model: vecindex.DefaultModel})
	if err != nil {
		return err
	}
	_, err = client.Embed(ctx, "longterm-mem doctor: embedding-backend-reachable probe")
	return err
}

// printDoctor prints the text form of a doctor report: every check, whatever it found.
func printDoctor(report ops.DoctorReport) {
	fmt.Printf("longterm-mem: doctor for %s\n", report.Project)
	for _, check := range report.Checks {
		fmt.Printf("  [%s] %s%s\n", check.Status, check.Name, detailSuffix(check.Detail))
	}
}

// doctorExitCode is 9 when any check failed and 0 when none did: the verdict is about the vault, not about the
// program.
func doctorExitCode(report ops.DoctorReport) int {
	for _, check := range report.Checks {
		if check.Status == ops.CheckFailed {
			return exitDoctorChecksFailed
		}
	}
	return exitOK
}
