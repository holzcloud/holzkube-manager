package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/audit"
	"github.com/holzcloud/holzkube-manager/internal/config"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/store/migrate/backup"
)

// The subcommands, which are the operational half of this binary (OPS-01,
// OPS-02).
//
// They are subcommands of the same binary rather than a second tool, and the
// reason is the one property that makes them worth having: the backup format,
// the permission rules and the audit chain's hashing all live in this build,
// and a separate tool would be a second implementation of each that nothing
// keeps in step. A backup written by one version and refused by another is not
// a backup.
//
// Every one of them takes the data directory the server takes, through the
// same configuration loader, so an operator who runs the server with
// `HOLZKUBE_MANAGER_DATA_DIR` set runs these against the same directory
// without saying so twice.

// commands are the subcommands this binary answers to. `serve` is the default
// and is what an invocation with no subcommand does, so that the existing
// systemd units and Docker entrypoints keep working.
var commands = map[string]func(args []string) error{
	"backup":       cmdBackup,
	"restore":      cmdRestore,
	"backups":      cmdBackups,
	"verify-audit": cmdVerifyAudit,
}

// dispatch routes the first argument to a subcommand, or to the server.
//
// A word that is not a subcommand falls through to the server, which then
// refuses it as an unknown flag. That is deliberate: a typo'd subcommand must
// not silently start a server, and the flag parser's error names the word.
func dispatch(args []string) (handled bool, err error) {
	if len(args) == 0 {
		return false, nil
	}
	if cmd, ok := commands[args[0]]; ok {
		return true, cmd(args[1:])
	}
	return false, nil
}

// cmdBackup writes a tarball of the data directory (OPS-01).
//
// It is safe to run against a *running* instance, and that is the design
// decision worth stating: it takes no lock. Every record in the store is
// written atomically -- write a temporary file, fsync, rename -- so a tarball
// taken mid-write captures either the old record or the new one and never half
// of one. The alternative, refusing to back up while the server runs, is a
// backup subcommand nobody runs.
func cmdBackup(args []string) error {
	cfg, err := loadFor(args, "backup")
	if err != nil {
		return err
	}

	label := backup.Manual
	for _, a := range args {
		if strings.HasPrefix(a, "--label=") {
			label = strings.TrimPrefix(a, "--label=")
		}
	}

	path, err := backup.CreateNamed(cfg.DataDir, label)
	if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	fmt.Printf("wrote %s (%s)\n", path, humanBytes(info.Size()))
	fmt.Println("It contains every secret in the data directory verbatim, and it is 0600 inside " +
		"a 0700 directory. Copying it somewhere else copies those secrets with it.")
	return nil
}

// cmdBackups lists what is there.
func cmdBackups(args []string) error {
	cfg, err := loadFor(args, "backups")
	if err != nil {
		return err
	}

	entries, err := backup.List(cfg.DataDir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No backups in", filepath.Join(cfg.DataDir, backup.DirName))
		return nil
	}

	for _, e := range entries {
		fmt.Printf("%s  %8s  %s\n",
			e.ModTime.UTC().Format(time.RFC3339), humanBytes(e.Size), e.Name)
	}
	return nil
}

// cmdRestore unpacks a backup over the data directory (OPS-01).
//
// Two refusals stand in front of it, and both are about not making a bad
// situation worse:
//
//  1. **The directory must not be in use.** It takes the store's own process
//     lock to find out, because that lock is what makes "one writer" true --
//     restoring underneath a running instance would replace the files it has
//     open with different ones carrying the same names.
//  2. **What is there now is backed up first.** A restore that went wrong
//     without one would have replaced a working installation with a broken one
//     and left nothing to go back to.
func cmdRestore(args []string) error {
	if len(args) == 0 {
		return errors.New("restore: name the archive to restore, e.g. " +
			"`holzkube-managerd restore ~/.holzkube-manager/backups/manual-20260911T120000Z.tar.gz`")
	}

	archive := args[0]
	cfg, err := loadFor(args[1:], "restore")
	if err != nil {
		return err
	}

	if _, err := os.Stat(archive); err != nil {
		return fmt.Errorf("restore: %s: %w", archive, err)
	}

	// The lock, taken and released. Holding it across the restore would be
	// correct and is not possible: the restore replaces the directory the lock
	// file lives in. What this proves is that no *other* process holds it at
	// the moment the restore starts, which is the check worth having -- the
	// alternative is no check at all.
	if err := ensureNotInUse(cfg.DataDir); err != nil {
		return err
	}

	safety, err := backup.Restore(archive, cfg.DataDir, "pre-restore")
	if safety != "" {
		fmt.Printf("what was there has been backed up to %s\n", safety)
	}
	if err != nil {
		return err
	}

	fmt.Printf("restored %s into %s\n", archive, cfg.DataDir)

	// The chain is verified after a restore rather than left for the next
	// start, because a restore is exactly the operation that can produce an
	// archive that does not verify -- and finding out now is the difference
	// between choosing another backup and discovering it during an incident.
	return reportChain(cfg.DataDir)
}

// cmdVerifyAudit checks the audit log's hash chain (OPS-02).
//
// It exits non-zero on a break, so that a cron entry or a monitoring check can
// use it without parsing output. The server verifies at startup as well; this
// is the same check on demand, because "is my audit log intact" is a question
// an operator asks between restarts.
func cmdVerifyAudit(args []string) error {
	cfg, err := loadFor(args, "verify-audit")
	if err != nil {
		return err
	}
	return reportChain(cfg.DataDir)
}

// reportChain verifies the chain and prints what it found.
func reportChain(dir string) error {
	al, err := audit.Open(dir)
	if err != nil {
		return err
	}
	defer al.Close() //nolint:errcheck // the verdict is Verify's

	ok, file, line, err := al.Verify(context.Background())
	if err != nil {
		return err
	}

	if ok {
		fmt.Println("the audit hash chain verifies")
		return nil
	}

	// Named as an error so the exit code is non-zero, and carrying the file
	// and line because "the chain is broken" without them is a statement
	// nobody can act on.
	return fmt.Errorf("the audit hash chain is broken at %s line %d. Every record before that "+
		"line still verifies; the record at it and everything after are not covered by a chain "+
		"this build can check. That is either tampering or a restore of a partial file, and both "+
		"are worth finding out about before anything else is done to this installation",
		file, line)
}

// ensureNotInUse reports whether another process holds the data directory.
func ensureNotInUse(dir string) error {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		// Nothing there to be in use, and Restore creates it.
		return nil
	}

	st, err := fsstore.Open(dir)
	if err != nil {
		return fmt.Errorf("%w: %s could not be opened, which usually means a holzkube-manager is "+
			"running against it. Stop it first: %w", backup.ErrDirectoryInUse, dir, err)
	}
	return st.Close()
}

// loadFor resolves the data directory for a subcommand.
//
// It goes through the same configuration loader the server uses, so
// HOLZKUBE_MANAGER_DATA_DIR and --data-dir mean the same thing here as there.
// The flags a subcommand does not use are accepted and ignored rather than
// refused: an operator pasting their systemd ExecStart line in front of
// `backup` should get a backup, not a lecture.
func loadFor(args []string, name string) (config.Config, error) {
	kept := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "--label=") {
			continue
		}
		kept = append(kept, a)
	}

	cfg, err := config.Load(kept)
	switch {
	case errors.Is(err, config.ErrHelp):
		usageFor(os.Stdout, name)
		return config.Config{}, err
	case err != nil:
		return config.Config{}, err
	}
	return cfg, nil
}

func usageFor(w io.Writer, name string) {
	fmt.Fprintf(w, "holzkube-managerd %s [--data-dir DIR]\n\n", name)
	config.Usage(w)
}

// humanBytes renders a size the way a directory listing would.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
