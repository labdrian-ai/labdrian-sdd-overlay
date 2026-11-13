package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memorycard"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// cmdMigrateCardPages implements `longterm-mem migrate-card-pages --project P [--vault DIR] [--apply]`, the one-time
// migration of S6 (c) of the store-agnostic promotion (promote.MigrateCardPages): every page of the project that
// carries legacy identity keys (engram_id or memorycard_id and their companions) takes the store-agnostic keys of its
// memorycard memory, keeping its address, created date, body and every other key. A page naming an Engram
// observation takes the identity of the memory memorycard imported from it (its import_map row); no Engram database
// is read. Without --apply it reports what it would do and writes nothing. With --apply it first copies the pages it
// will rewrite and the precedence sidecar to a directory it names, then rewrites them. A page it cannot migrate is
// refused by name with its reason on stderr and the run goes on; any refusal makes the exit code 6
// (registration_conflict), the code of every refusal that leaves a page alone.
func (c commands) cmdMigrateCardPages(args []string) int {
	flags := flag.NewFlagSet("migrate-card-pages", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	project := flags.String("project", "", projectFlagUsage)
	vaultDir := flags.String("vault", "", "vault path override")
	apply := flags.Bool("apply", false, "rewrite the pages (the default only reports what would be rewritten); the pages and the precedence sidecar are copied to a directory under .vault-meta first, and the run says where")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if rest := flags.Args(); len(rest) != 0 {
		fmt.Fprintf(os.Stderr, "longterm-mem: migrate-card-pages: unexpected argument %q: the command takes flags only\n", rest[0])
		return exitUsage
	}
	a := c.newApp("migrate-card-pages")
	// Without --apply the command promises not to change what it inspects, the registry included.
	readOnly := !*apply
	resolvedProject, exit := a.project(*project, readOnly)
	if exit != exitOK {
		return exit
	}
	vaultRoot, exit := a.vaultRoot(resolvedProject, *vaultDir, readOnly)
	if exit != exitOK {
		return exit
	}
	cards, err := memorycard.Open(os.Getenv(memorycardDBEnvVar))
	if err != nil {
		return a.refuse(err, exitEngramUnavailable)
	}
	defer a.closeStore("memorycard", cards)
	writer, err := newPromoteWriter(vaultRoot, c.clock)
	if err != nil {
		return a.refuse(err, exitInternal)
	}

	report, err := promote.MigrateCardPages(cards, writer, vaultBackup{root: vaultRoot, clock: c.clock}, resolvedProject, *apply)
	return reportMigration(a, report, err, vaultRoot, *apply)
}

// reportMigration says what the migration did, or with apply false would, and answers the exit code. The pages it
// moved or would move are named on stdout, each refusal on stderr with its reason, and the summary counts the pages
// that carry legacy keys and each kind of refusal, so that a dry run says how many pages are left to migrate.
func reportMigration(a app, report promote.CardMigrationReport, err error, vaultRoot string, apply bool) int {
	if apply && report.Backup != "" {
		fmt.Printf("longterm-mem: migrate-card-pages: backed up the pages to rewrite and the precedence sidecar to %s\n", report.Backup)
	}
	verb := "migrated"
	if !apply {
		verb = "would migrate"
	}
	for _, p := range report.Migrated {
		if p.EngramID != 0 {
			fmt.Printf("%s: %s (Engram observation %d, memorycard memory %d)\n", verb, p.Address, p.EngramID, p.MemoryID)
			continue
		}
		fmt.Printf("%s: %s (memorycard memory %d)\n", verb, p.Address, p.MemoryID)
	}
	for _, r := range report.Refused {
		fmt.Fprintf(os.Stderr, "longterm-mem: migrate-card-pages: refused %s: %s\n", r.Address, r.Reason)
	}
	if err != nil {
		return a.refuse(err, exitInternal)
	}
	if apply {
		fmt.Printf("longterm-mem: migrate-card-pages: migrated %d of %d page(s) carrying legacy keys in %s; refused %d\n", len(report.Migrated), report.Legacy, vaultRoot, len(report.Refused))
	} else {
		fmt.Printf("longterm-mem: migrate-card-pages --dry-run: %d page(s) carry legacy keys in %s: would migrate %d, would refuse %d\n", report.Legacy, vaultRoot, len(report.Migrated), len(report.Refused))
	}
	counts := report.RefusalCounts()
	for _, kind := range promote.RefusalKinds {
		if n := counts[kind]; n > 0 || kind != promote.RefusedOther {
			fmt.Printf("  refused, %s: %d\n", kind, n)
		}
	}
	if !apply {
		fmt.Println("nothing was written (pass --apply to migrate)")
	}
	if len(report.Refused) > 0 {
		return exitRegistrationConflict
	}
	return exitOK
}

// vaultBackup is promote.MigrationBackup over the files of the vault at root: it copies the pages to rewrite and the
// precedence sidecar, keeping their vault-relative paths, into a directory of its own under .vault-meta named by the
// instant it was asked, so that putting the files back is a copy of that directory over the vault. A directory
// that already exists is an error, never merged into.
type vaultBackup struct {
	root  string
	clock promote.Clock
}

func (b vaultBackup) BackUp(addresses []string) (string, error) {
	dir := filepath.Join(b.root, filepath.FromSlash(path.Dir(vaultlayout.SyncStateFile)), "card-page-migration-"+b.clock.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return "", err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", err
	}
	rels := []string{vaultlayout.PrecedenceFile}
	for _, address := range addresses {
		rels = append(rels, vaultlayout.PageFile(address))
	}
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(b.root, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) && rel == vaultlayout.PrecedenceFile {
			// A vault that never tracked a page has no sidecar to lose.
			continue
		}
		if err != nil {
			return "", err
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			return "", err
		}
	}
	return dir, nil
}
