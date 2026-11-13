package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
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
// (registration_conflict), the code of every refusal that leaves a page alone. A page with a legacy key and no project
// is no project's page: each run prints one note about it on stdout and it makes no exit code.
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
		fmt.Fprintf(os.Stderr, "longterm-mem: migrate-card-pages: refused %s: %s\n", r.Address, refusalReason(vaultRoot, r))
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
		if n := counts[kind]; n > 0 || (kind != promote.RefusedOther && kind != promote.RefusedNoMemory && kind != promote.RefusedHalfMigrated && kind != promote.RefusedUnreadable) {
			fmt.Printf("  refused, %s: %d\n", kind, n)
		}
	}
	if len(report.NoProject) > 0 {
		names := make([]string, len(report.NoProject))
		for i, r := range report.NoProject {
			names[i] = r.Address
		}
		// A note, on stdout only: no project owns these pages, so they are no refusal of this run and make no exit code.
		fmt.Printf("note: %d page(s) carry legacy keys and name no project, so no project's run counts them: %s; add a `project:` line to each by hand, then run again\n", len(names), strings.Join(names, ", "))
	}
	if !apply {
		fmt.Println("nothing was written (pass --apply to migrate)")
	}
	if len(report.Refused) > 0 {
		return exitRegistrationConflict
	}
	return exitOK
}

// backupDirectoryPrefix starts the name of every directory vaultBackup makes under the vault's .vault-meta.
const backupDirectoryPrefix = "card-page-migration-"

// refusalReason is the reason of a refusal as the command prints it. A half-migrated page's way back is a backup of this
// migration, so its reason lists the backup directories the vault has, as paths relative to the vault root.
func refusalReason(vaultRoot string, r promote.MigrationRefusal) string {
	if r.Kind != promote.RefusedHalfMigrated {
		return r.Reason
	}
	meta := path.Dir(vaultlayout.SyncStateFile)
	entries, err := os.ReadDir(filepath.Join(vaultRoot, filepath.FromSlash(meta)))
	var found []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), backupDirectoryPrefix) {
			found = append(found, meta+"/"+entry.Name()+"/")
		}
	}
	switch {
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return r.Reason + "; backups present: could not be listed (" + err.Error() + ")"
	case len(found) == 0:
		return r.Reason + "; backups present: none"
	}
	return r.Reason + "; backups present: " + strings.Join(found, ", ")
}

// vaultBackup is promote.MigrationBackup over the files of the vault at root: it copies the pages to rewrite and the
// precedence sidecar, keeping their vault-relative paths, into a directory of its own under .vault-meta named by the
// instant it was asked, so that putting the files back is a copy of that directory over the vault. A directory
// that already exists is never merged into: a second backup of the same second takes a numbered name, and a copy
// that fails halfway removes its directory, so a directory that is there is a whole copy.
type vaultBackup struct {
	root  string
	clock promote.Clock
	// write and removeAll are the file system calls the copy makes; nil means durable.WriteFile and os.RemoveAll. A
	// test replaces them to fail a write or a cleanup on demand.
	write     func(path string, data []byte, perm fs.FileMode) error
	removeAll func(path string) error
	// syncDir syncs one directory; nil means syncDirectory, which ignores a file system that cannot sync a directory.
	syncDir func(path string) error
}

func (b vaultBackup) BackUp(addresses []string) (string, error) {
	dir, err := b.newDirectory()
	if err != nil {
		return "", err
	}
	if err := b.copyInto(dir, addresses); err != nil {
		// A directory that holds part of the copy would read as a complete one: it goes, whole.
		removeAll := b.removeAll
		if removeAll == nil {
			removeAll = os.RemoveAll
		}
		if removeErr := removeAll(dir); removeErr != nil {
			return "", errors.Join(err, fmt.Errorf("remove the partial copy %s: %w", dir, removeErr))
		}
		// The removal is made durable like the copy was: the directory that held it is synced, so a crash cannot bring the
		// partial copy back.
		syncDir := b.syncDir
		if syncDir == nil {
			syncDir = syncDirectory
		}
		if syncErr := syncDir(filepath.Dir(dir)); syncErr != nil {
			return "", errors.Join(err, fmt.Errorf("sync %s after removing the partial copy: %w", filepath.Dir(dir), syncErr))
		}
		return "", err
	}
	return dir, nil
}

// newDirectory makes the directory of this backup, named by the instant it was asked; a second backup of the same
// second takes the next free suffix, so a retry never meets the copy of the run before it.
func (b vaultBackup) newDirectory() (string, error) {
	base := filepath.Join(b.root, filepath.FromSlash(path.Dir(vaultlayout.SyncStateFile)), backupDirectoryPrefix+b.clock.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(filepath.Dir(base), 0o700); err != nil {
		return "", err
	}
	for n := 1; ; n++ {
		dir := base
		if n > 1 {
			dir = fmt.Sprintf("%s-%d", base, n)
		}
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
}

// copyInto copies the sidecar and each page into dir, under the same vault-relative paths, with the mode each had,
// and makes the copy durable before it returns: every file is synced as it is written, and so is every directory the
// copy made, since the copy is the only way back once the pages are rewritten.
func (b vaultBackup) copyInto(dir string, addresses []string) error {
	write := b.write
	if write == nil {
		write = durable.WriteFile
	}
	rels := []string{vaultlayout.PrecedenceFile}
	for _, address := range addresses {
		rels = append(rels, vaultlayout.PageFile(address))
	}
	for _, rel := range rels {
		// One handle for the stat and the read: the mode and the bytes are of the same file.
		data, mode, err := readWithMode(filepath.Join(b.root, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) && rel == vaultlayout.PrecedenceFile {
			// A vault that never tracked a page has no sidecar to lose.
			continue
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		// durable.WriteFile applies the mode exactly, through the handle, to a file it creates, and the directory is new.
		if err := write(target, data, mode); err != nil {
			return err
		}
	}
	return b.syncDirectories(dir)
}

// readWithMode reads the file at path and answers its bytes and permission bits, both through one open handle.
func readWithMode(path string) ([]byte, fs.FileMode, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	data, err := io.ReadAll(f)
	return data, info.Mode().Perm(), err
}

// syncDirectories syncs dir, every directory under it, the directory that holds it (.vault-meta, which the copy may
// have made) and the vault root that holds that, so the entries of the copy survive a crash and not only the bytes of
// its files.
func (b vaultBackup) syncDirectories(dir string) error {
	var dirs []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return err
	})
	if err != nil {
		return err
	}
	dirs = append(dirs, filepath.Dir(dir), b.root)
	syncDir := b.syncDir
	if syncDir == nil {
		syncDir = syncDirectory
	}
	for _, d := range dirs {
		if err := syncDir(d); err != nil {
			return fmt.Errorf("sync %s: %w", d, err)
		}
	}
	return nil
}

// toleratedDirectorySyncError reports whether err, the error of syncing a directory on goos, is no failure of the copy: a
// platform or file system that cannot sync a directory (Windows, or an answer of EINVAL, ENOTSUP or ErrUnsupported) has
// no way to do more than the sync of the copy's files. It has no error to tolerate when err is nil.
func toleratedDirectorySyncError(goos string, err error) bool {
	if err == nil {
		return false
	}
	return goos == "windows" || errors.Is(err, errors.ErrUnsupported) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP)
}

// syncDirectory fsyncs one directory; the errors toleratedDirectorySyncError names are not failures. Any other error is
// returned.
func syncDirectory(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := handle.Sync()
	if closeErr := handle.Close(); syncErr == nil {
		syncErr = closeErr
	}
	if toleratedDirectorySyncError(runtime.GOOS, syncErr) {
		return nil
	}
	return syncErr
}
