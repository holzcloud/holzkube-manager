// Command holzkube-managerd is the holzkube-manager server: one binary that serves the embedded
// web UI over HTTPS and, from phase 2 onward, talks to Talos.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/audit"
	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/auth/oidc"
	"github.com/holzcloud/holzkube-manager/internal/config"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube-manager/internal/imagefactory"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/tlsx"
)

const (
	readHeaderTimeout = 10 * time.Second
	shutdownGrace     = 10 * time.Second

	// The remaining server timeouts. MaxBytesReader caps how large a body may
	// be; nothing capped how long a caller could take to send one, so a client
	// dribbling a valid request a byte at a time held a connection and a
	// goroutine indefinitely, as did one that never read its response.
	//
	// readTimeout covers headers plus body. writeTimeout is generous because it
	// has to cover the slowest legitimate handler -- an argon2id verification
	// on a slow host, plus the rate limiter parking a login for up to
	// maxInlineDelay -- and cutting one of those off would look like a bug in
	// the login. idleTimeout bounds a kept-alive connection between requests.
	readTimeout    = 30 * time.Second
	writeTimeout   = 60 * time.Second
	idleTimeout    = 120 * time.Second
	maxHeaderBytes = 1 << 16
)

// version is the release this binary was built from. goreleaser overwrites it
// through -ldflags -X main.version=...; a build straight from a working tree
// keeps "dev", which is the honest answer for one.
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "holzkube-managerd:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// The level is behind a LevelVar because the level itself is configuration:
	// the logger has to exist before --log-level has been resolved.
	level := new(slog.LevelVar)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	cfg, err := config.Load(args)
	switch {
	case errors.Is(err, config.ErrHelp):
		config.Usage(os.Stdout)
		return nil
	case errors.Is(err, config.ErrVersion):
		fmt.Println("holzkube-managerd", version)
		return nil
	case err != nil:
		return err
	}
	level.Set(cfg.LogLevel)
	// Packages that log through the package-level slog functions -- the argon2id
	// calibration is one -- would otherwise write through a different handler at
	// a different level, and --log-level would be quietly true only of some of
	// the output.
	slog.SetDefault(logger)

	logger.Info("holzkube-manager starting", slog.String("version", version))
	// Every option, its effective value and where that value came from. A
	// misconfigured option is then visible here rather than in the failure it
	// eventually causes (D-03).
	cfg.LogEffective(logger)

	// Before anything is opened or created: plain HTTP is allowed only where the
	// listener cannot leave this machine (D-04). A refusal here is a start
	// failure with nothing written, not a surprise at the first request.
	if err := tlsx.LoopbackGuard(cfg.Listen, cfg.InsecureHTTP); err != nil {
		return err
	}

	if err := config.EnsureDir(cfg.DataDir); err != nil {
		return err
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

	// The Image Factory client. It holds no credentials and opens no
	// connection until a route asks it to, so constructing it here costs
	// nothing and a bad base URL is a start failure rather than a 502 the first
	// time an operator opens the images screen.
	factory, err := imagefactory.New(imagefactory.DefaultBaseURL)
	if err != nil {
		return err
	}

	// The transport mode. It is built here, at the composition root, because
	// this is the only place that has read the configuration -- and it is
	// carried into the handlers rather than consulted from a package variable,
	// so every future node call has to be handed the mode explicitly and none
	// of them can inherit the wrong one (D-03, FOUND-12).
	talosMode := talos.Mode{DryRun: cfg.DryRun}

	// The identity provider, if one is configured. New performs no network I/O:
	// discovery happens on first use, so that a provider which is down -- quite
	// possibly because it runs on the cluster this tool exists to repair --
	// cannot stop the process from starting and cannot take the local
	// break-glass account down with it.
	var provider *oidc.Provider
	if cfg.OIDCEnabled() {
		provider, err = oidc.New(cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret)
		if err != nil {
			return err
		}
		logger.Info("identity provider configured",
			slog.String("issuer", cfg.OIDCIssuer),
			slog.String("client_id", cfg.OIDCClientID))
	}

	deps := httpapi.Deps{
		Store:      st,
		Audit:      auditLog,
		Auth:       authSvc,
		Logger:     logger,
		SudoWindow: cfg.SudoWindow,
		// Inside the literal, deliberately. Deps is copied by value into each
		// …Routes(deps) call below, so a field assigned after this literal is
		// the zero value inside every handler closure -- a nil dependency with
		// no compile error and no failure until a request arrives. A dry-run
		// flag lost that way would be the worst instance of it: the endpoint
		// would report "live" while the transport refused everything, or the
		// reverse.
		Factory:   factory,
		TalosMode: talosMode,
		// Public strips the directory: this verdict is served by an endpoint
		// that answers before authentication, and chainFile is absolute. The
		// operator-facing copy of the path is the log line above, which stays
		// on this host.
		AuditChain: httpapi.ChainStatus{
			OK:           chainOK,
			BrokenAtLine: brokenLine,
			File:         chainFile,
		}.Public(),
		AllowedHosts: allowedHosts(cfg),
		OIDC:         provider,
		IsSSOOnly:    ssoOnly(cfg),
	}

	// The route table is assembled here, from each handler package's own Routes
	// function. A wave-2 plan adds its routes in its handler file and adds one
	// line here; router.go stays untouched.
	deps.Routes = slices.Concat(
		handlers.SystemRoutes(deps),
		handlers.SetupRoutes(deps),
		handlers.AuthRoutes(deps),
		handlers.OIDCRoutes(deps),
		handlers.AccountRoutes(deps),
		handlers.AuditRoutes(deps),
		handlers.SchematicRoutes(deps),
	)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           httpapi.New(deps),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	if cfg.InsecureHTTP {
		logger.Warn("serving plain HTTP on loopback; session cookies are Secure and browsers will not send them",
			slog.String("url", "http://"+cfg.Listen))
	} else {
		tlsConfig, fingerprint, err := tlsx.Ensure(cfg)
		if err != nil {
			return err
		}
		srv.TLSConfig = tlsConfig
		// The operator will see a browser warning for a self-signed
		// certificate. Logging the fingerprint in the browser's own format is
		// what turns "click through it" into "compare it", which is the only
		// version of that step worth asking for (D-04).
		logger.Info("TLS certificate ready",
			slog.String("sha256_fingerprint", fingerprint),
			slog.String("url", "https://"+cfg.Listen))
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

// allowedHosts is every Host header value this instance answers to.
//
// It closes DNS rebinding: without it, the CSRF preconditions are all
// self-referential, because a victim's browser resolving evil.example to the
// loopback sends a Host, an Origin and a Sec-Fetch-Site that agree with each
// other and name the attacker. See middleware.AllowHosts.
//
// The set is the bind address plus the loopback names, which is the same set
// tlsx puts in the certificate's SANs -- the two have to agree, or a host the
// certificate vouches for is one the server refuses.
// ssoOnly reports, for a Host header, whether the local password is refused
// there. It returns nil when no host is SSO-only, which lets the HTTP layer
// skip the check entirely rather than call a function that always says no.
func ssoOnly(cfg config.Config) func(string) bool {
	if len(cfg.SSOOnlyHosts) == 0 {
		return nil
	}
	return cfg.IsSSOOnly
}

func allowedHosts(cfg config.Config) []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if h := tlsx.ListenHost(cfg.Listen); h != "" {
		hosts = append(hosts, h)
	}
	// The configured names are the ones nothing here can derive: a reverse
	// proxy's public hostname reaches this process in the Host header and
	// nowhere else.
	hosts = append(hosts, cfg.AllowedHosts...)
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		hosts = append(hosts, hostname)
		if !strings.Contains(hostname, ".") {
			// macOS reports a bare hostname while Bonjour resolves it with a
			// .local suffix, matching what tlsx puts in the SANs.
			hosts = append(hosts, hostname+".local")
		}
	}
	return hosts
}
