// Command holzkubed is the holzkube server: one binary that serves the embedded
// web UI over HTTPS and, from phase 2 onward, talks to Talos.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/holzcloud/holzkube/internal/audit"
	"github.com/holzcloud/holzkube/internal/auth"
	"github.com/holzcloud/holzkube/internal/config"
	"github.com/holzcloud/holzkube/internal/httpapi"
	"github.com/holzcloud/holzkube/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube/internal/store/fsstore"
	"github.com/holzcloud/holzkube/internal/tlsx"
)

const (
	dataDirPerm       = 0o700
	readHeaderTimeout = 10 * time.Second
	shutdownGrace     = 10 * time.Second
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "holzkubed:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	logger.Info("configuration resolved",
		slog.String("listen", cfg.Listen),
		slog.String("data_dir", cfg.DataDir),
		slog.Bool("insecure_http", cfg.InsecureHTTP),
		slog.Duration("sudo_window", cfg.SudoWindow),
		slog.Duration("session_lifetime", cfg.SessionLifetime),
	)

	if err := os.MkdirAll(cfg.DataDir, dataDirPerm); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	st, err := fsstore.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	auditLog, err := audit.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer auditLog.Close()

	// Verify the chain at startup rather than behind a button: a hash chain
	// nobody checks is theatre (D-15). Verify covers the current day's file and
	// the one rotated before it, and names the file the break is in -- which is
	// not necessarily today's. The verdict is passed to the handlers as an
	// immutable snapshot, so a break found here stays reported for the life of
	// the process instead of disappearing behind a later, luckier check.
	chainOK, chainFile, brokenLine, err := auditLog.Verify(context.Background())
	if err != nil {
		return fmt.Errorf("verify audit chain: %w", err)
	}
	if chainOK {
		chainFile = auditLog.CurrentFile()
	} else {
		logger.Error("audit hash chain does not verify",
			slog.String("file", chainFile),
			slog.Int("broken_at_line", brokenLine))
	}

	authSvc, err := auth.New(st, cfg.SessionLifetime)
	if err != nil {
		return err
	}

	deps := httpapi.Deps{
		Store:      st,
		Audit:      auditLog,
		Auth:       authSvc,
		Logger:     logger,
		SudoWindow: cfg.SudoWindow,
		AuditChain: httpapi.ChainStatus{
			OK:           chainOK,
			BrokenAtLine: brokenLine,
			File:         chainFile,
		},
	}

	// The route table is assembled here, from each handler package's own Routes
	// function. A wave-2 plan adds its routes in its handler file and adds one
	// line here; router.go stays untouched.
	deps.Routes = slices.Concat(
		handlers.SystemRoutes(deps),
		handlers.SetupRoutes(deps),
		handlers.AuthRoutes(deps),
		handlers.AccountRoutes(deps),
		handlers.AuditRoutes(deps),
	)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           httpapi.New(deps),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	if !cfg.InsecureHTTP {
		var cert tls.Certificate
		var fingerprint string
		if cfg.TLSCert != "" {
			cert, fingerprint, err = tlsx.Load(cfg.TLSCert, cfg.TLSKey)
		} else {
			cert, fingerprint, err = tlsx.EnsureCertificate(cfg.DataDir)
		}
		if err != nil {
			return err
		}
		srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		// The operator will see a browser warning for a self-signed
		// certificate. Logging the fingerprint is what turns "click through it"
		// into "compare it", which is the only version of that step worth
		// asking for (D-04).
		logger.Info("TLS certificate ready",
			slog.String("sha256_fingerprint", fingerprint),
			slog.String("url", "https://"+cfg.Listen))
	} else {
		logger.Warn("serving plain HTTP; session cookies require TLS and will not be sent by browsers",
			slog.String("url", "http://"+cfg.Listen))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		var serveErr error
		if cfg.InsecureHTTP {
			serveErr = srv.ListenAndServe()
		} else {
			serveErr = srv.ListenAndServeTLS("", "")
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
