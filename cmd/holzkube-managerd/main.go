// Command holzkube-managerd is the holzkube-manager server: one binary that serves the embedded
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
	"path/filepath"
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
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/metrics"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/nodestream"
	"github.com/holzcloud/holzkube-manager/internal/provision"
	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
	"github.com/holzcloud/holzkube-manager/internal/streamhub"
	"github.com/holzcloud/holzkube-manager/internal/support"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/tlsx"
	"github.com/holzcloud/holzkube-manager/internal/upgrade"
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
	//
	// That argon2id-plus-rate-limiter reasoning is still true and is still a
	// reason this value cannot be small. It was never the whole of it, and the
	// sentence it never had is this one: writeTimeout must also cover the
	// largest upstream budget any handler declares, plus room for the handler's
	// own work after the last upstream call returns. The two Factory routes
	// declare theirs as CreateRouteBudget and AssetsRouteBudget in
	// internal/httpapi/handlers/schematics.go, and nothing anywhere added them
	// to this number -- which is how a route with a 60.000s worst case came to
	// meet a 60s response budget and flush a problem document to an expired
	// socket (`status=502 duration=1m0.002907792s`).
	//
	// 130s is derived from that: the smallest multiple of ten strictly greater
	// than CreateRouteBudget + budgetSlack = 125s, and comfortably above the
	// argon2id-plus-rate-limiter floor that produced the old 60s.
	//
	// cmd/holzkube-managerd/budget_test.go is the assertion that keeps the
	// numbers composed, and it fails in both directions. Whoever moves this
	// value goes to that table rather than moving it alone.
	readTimeout    = 30 * time.Second
	writeTimeout   = 130 * time.Second
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
	// The subcommands first. An invocation with no subcommand -- which is what
	// every existing systemd unit and Docker entrypoint does -- falls through
	// to the server below, so adding these changed nothing for anybody already
	// running one.
	if handled, err := dispatch(args); handled {
		if errors.Is(err, config.ErrHelp) {
			return nil
		}
		return err
	}

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
	//
	// The drift observer is where the Factory's schema additions become
	// visible. imagefactory decodes past a field it does not know rather than
	// refusing the response -- one additive upstream field used to take the
	// whole Images screen down -- so this line is the only thing that makes
	// such an addition something anybody finds out about.
	factory, err := imagefactory.New(cfg.ImageFactoryURL,
		imagefactory.WithDriftObserver(func(path, field string) {
			logger.Warn("the Image Factory answered with a field this build does not know",
				slog.String("path", path),
				slog.String("field", field),
				slog.String("effect", "the field was ignored and the rest of the response was used"))
		}))
	if err != nil {
		return err
	}

	// The transport mode. It is built here, at the composition root, because
	// this is the only place that has read the configuration -- and it is
	// carried into the handlers rather than consulted from a package variable,
	// so every future node call has to be handed the mode explicitly and none
	// of them can inherit the wrong one (D-03, FOUND-12).
	talosMode := talos.Mode{DryRun: cfg.DryRun, AllowPreRelease: cfg.AllowPreRelease}

	// The transport. It is built here for the same reason the mode is: this is
	// the only place that has read the configuration, and a dialer constructed
	// inside a handler would be a second answer to "how do we reach a node"
	// that nothing keeps in step with the first.
	dialer := talos.NewDirectDialer(talos.ApidPort)

	// The inventory. Its supervisors are started after the store and the audit
	// log are open and before the server listens, so that the first request to
	// arrive finds an inventory that has already begun observing rather than
	// one that starts observing because somebody looked (D-17).
	inv := inventory.New(inventory.Deps{
		Store:  st,
		Dialer: dialer,
		Mode:   talosMode,
		Logger: logger,
	})
	defer inv.Close()

	// The stream fan-out and the readers that feed it.
	//
	// The hub is one reader per topic and a bounded ring buffer, so a browser
	// tab that stops reading loses events -- visibly, as a gap -- rather than
	// applying backpressure to a Talos node. The manager reference-counts the
	// readers, so four panels on one node's kubelet log are one follow stream
	// and not four.
	hub := streamhub.New()
	defer hub.Close()

	nodeStreams := nodestream.New(nodestream.Deps{
		Hub:    hub,
		Logger: logger,
		Open: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
			return inv.Connect(ctx, id)
		},
	})
	defer nodeStreams.Close()

	// The job engine. Node actions are jobs because they take minutes and can
	// be interrupted, and a record is the only thing that can say where one
	// was when the process died.
	engine := jobs.New(jobs.Deps{
		Store:  st,
		Logger: logger,
		Hub:    hub,
	})
	defer engine.Close()

	jobs.RegisterNodeActions(engine, func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
		return inv.Connect(ctx, id)
	})

	// The confirmation signing key is generated here and lives only in memory.
	// A confirmation that survived a restart would be a decision about a fleet
	// that may since have changed.
	confirmer, err := jobs.NewConfirmer()
	if err != nil {
		return err
	}

	// The config domain. It takes the same connector the jobs take, and the
	// transport mode, because whether this process may apply anything is a
	// property of the process rather than of the request.
	configSvc := machineconfig.New(machineconfig.Deps{
		Connect: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
			return inv.Connect(ctx, id)
		},
		Mode: talosMode,
	})

	// Provisioning: the wizard's reads, and the etcd bootstrap lease.
	//
	// The lease directory lives in the data directory rather than in /tmp or
	// in memory, and that is the whole of mechanism 2 and 3: a lease that did
	// not survive a restart would not be a lease, and an intent record that
	// did not survive a crash would not be a record of the one thing a crash
	// makes unknowable.
	bootstrapper, err := provision.NewBootstrapper(filepath.Join(cfg.DataDir, "bootstrap"))
	if err != nil {
		return err
	}

	// A machine in maintenance mode has no cluster PKI, so there is nothing to
	// verify against and the connection verifies nothing. The fingerprint the
	// operator read off the machine's console is carried on Creds for the pin
	// the transport seam performs; MaintenanceWarning is what the screen says
	// about what this is and is not worth (PROV-04).
	maintenanceCreds := func(fingerprint string) talos.Creds {
		return talos.Creds{
			Kind:        talos.CredMaintenance,
			Fingerprint: fingerprint,
			TLS: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // maintenance mode has no PKI; see above
				MinVersion:         tls.VersionTLS12,
			},
		}
	}

	provisionSvc := provision.NewService(dialer, maintenanceCreds, inv.KnownAt, bootstrapper,
		inv.ControlPlaneCount)

	provision.Register(engine, provision.Deps{
		Dialer:           dialer,
		MaintenanceCreds: maintenanceCreds,
		ClusterCreds:     inv.ClusterCreds,
		Secrets: func(ctx context.Context, id model.ClusterID) (model.ClusterSecrets, error) {
			return st.ClusterSecrets().Get(ctx, id)
		},
		Cluster: func(ctx context.Context, id model.ClusterID) (model.Cluster, error) {
			return st.Clusters().Get(ctx, id)
		},
		PatchBodies: func(ctx context.Context, ids []string) ([]string, error) {
			out := make([]string, 0, len(ids))
			for _, id := range ids {
				rec, err := st.Patches().Get(ctx, model.PatchID(id))
				if err != nil {
					return nil, err
				}
				if rec.Superseded {
					return nil, fmt.Errorf(
						"provision: patch %s version %d has been superseded; name the current version",
						rec.Name, rec.Version)
				}
				out = append(out, rec.Body)
			}
			return out, nil
		},
		// A machine that has come back as a cluster node is filed by the same
		// path an operator's manual add takes: it connects with the cluster's
		// credentials, reads the node's own facts, and records what the node
		// said. Writing the record from what the wizard was told instead would
		// be filing the plan rather than the machine.
		Record: func(ctx context.Context, cluster model.ClusterID, addr string, _ bool) error {
			_, err := inv.AddManual(ctx, cluster, addr)
			return err
		},
		// The cluster's own Kubernetes version, read off the nodes it already
		// has. A cluster with no observed node yet -- the very first one --
		// has no answer, and the job fails with that sentence rather than
		// generating a configuration at a version nobody chose.
		KubernetesVersion: inv.KubernetesVersion,
	})

	// Rolling upgrades and etcd management.
	//
	// The gate is built from the same connector everything else uses and from
	// the inventory's own control-plane list, because the question it answers
	// -- "does this cluster survive losing that node" -- is about every
	// control-plane node and not about the one being upgraded.
	upgradeDeps := upgrade.Deps{
		Connect: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
			return inv.Connect(ctx, id)
		},
		Machines: inv.MachinesOf,
		RoleOf: func(ctx context.Context, id model.MachineID) (model.MachineRole, error) {
			m, err := inv.Machine(ctx, id)
			if err != nil {
				return "", err
			}
			return m.Role, nil
		},
		Record: func(ctx context.Context, id model.MachineID) error {
			inv.Refresh(ctx, id)
			return nil
		},

		// Resolving an installer needs the Image Factory and the stored
		// schematic, and neither belongs to the upgrade package. It is wired
		// here, in the one place that holds both.
		ResolveInstaller: resolveInstaller(factory, st),
	}
	upgradeDeps.Gate = upgrade.NewGate(upgradeDeps.Connect, inv.ControlPlanesOf)

	upgrade.Register(engine, upgradeDeps)

	// The release list comes from the Image Factory, which is also where the
	// installer images come from -- so the versions offered and the versions
	// installable are the same set by construction rather than by agreement.
	upgradeSvc := upgrade.NewService(upgradeDeps, func(ctx context.Context) ([]string, error) {
		if factory == nil {
			return nil, errors.New("this instance was started without an Image Factory client")
		}
		return factory.Versions(ctx)
	})

	// The support-bundle collector. It uses the same connector, the same
	// inventory and the same audit reader everything else does -- a bundle
	// assembled from its own reads would be a bundle describing a cluster
	// nobody else sees.
	supportCollector := support.New(support.Deps{
		Connect: func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error) {
			return inv.Connect(ctx, id)
		},
		Machines:  inv.MachinesOf,
		Clusters:  func(ctx context.Context) ([]model.Cluster, error) { return st.Clusters().List(ctx) },
		AuditTail: support.AuditTailFrom(filepath.Join(cfg.DataDir, audit.DirName)),
		Instance: func() map[string]any {
			return map[string]any{
				"version":     version,
				"dry_run":     cfg.DryRun,
				"talos_range": talos.MinSupportedVersion + " to " + talos.MaxSupportedVersion,
				"prerelease":  cfg.AllowPreRelease,
				"taken_by":    "the running instance",
			}
		},
	})

	// The Prometheus export (V2-API-02). It reads the same read model the API
	// serves rather than a second one of its own: a metric that disagreed with
	// the screen would send an operator looking for a fault in the cluster
	// that was actually a fault here.
	//
	// chainOK is captured by value on purpose. It is the startup verification
	// verdict, and D-15 wants exactly that: a break found at startup has to
	// stay reported, not stop being reported because nothing re-checked.
	metricsExporter := metrics.New(metrics.Deps{
		Machines:         inv.Machines,
		Clusters:         inv.Clusters,
		Jobs:             engine.List,
		AuditChainIntact: func() bool { return chainOK },
	})

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
		Factory:     factory,
		TalosMode:   talosMode,
		Inventory:   inv,
		Hub:         hub,
		NodeStreams: nodeStreams,
		Jobs:        engine,
		Confirmer:   confirmer,
		Config:      configSvc,
		Provision:   provisionSvc,
		Upgrade:     upgradeSvc,
		Support:     supportCollector,
		Metrics:     metricsExporter,
		// The per-cluster read-only lock, read by the route middleware rather
		// than by each handler (D-22). Inside the literal for the reason the
		// comment above states: Deps is copied by value into every …Routes
		// call below, and a lock function assigned afterwards would be nil in
		// every handler closure, with no compile error -- which is to say the
		// lock would silently not exist.
		ClusterLocked: func(r *http.Request, cluster string) error {
			return inv.CheckLock(r.Context(), model.ClusterID(cluster))
		},
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

	// The route table is assembled by routeTable, which is the one place it is
	// written down.
	deps.Routes = routeTable(deps)

	// Start observing before the listener opens. A supervisor that only runs
	// while somebody is looking makes the dashboard slow exactly when it is
	// needed, and makes stale_since meaningless on the first load.
	if err := inv.Start(context.Background()); err != nil {
		return err
	}

	// Resume before the listener opens, so that a job interrupted by the last
	// shutdown is continued or parked before anybody can submit a second one
	// against the same cluster. This is the single most consequential call in
	// the startup sequence: it is where "a kill -9 never produces a doubled
	// side effect" is actually decided.
	if err := engine.Resume(context.Background()); err != nil {
		return err
	}

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

// resolveInstaller builds the function an upgrade resolves its installer
// reference with.
//
// It replaced a string assembled from parts, which was wrong in three ways at
// once and silent about all of them: SecureBoot was not in it, the repository
// name was assumed rather than resolved, and the Factory host was hard-coded
// so an installation pointed at a private Factory upgraded nodes from the
// public one.
//
// The architecture comes from the stored schematic record and never from a
// constant (FACT-03). A schematic this installation does not hold is a refusal
// rather than a default: without it there is no architecture to ask about, and
// the reference that a default produced would be one nobody chose.
func resolveInstaller(factory *imagefactory.Client, st *fsstore.Store) upgrade.ResolveInstaller {
	return func(ctx context.Context, schematicID, version string, secureBoot bool) (string, error) {
		if factory == nil {
			return "", errors.New("this installation is not configured with an Image Factory, " +
				"so the installer for a schematic cannot be resolved")
		}

		rec, err := st.Schematics().Get(ctx, model.SchematicID(schematicID))
		if err != nil {
			return "", fmt.Errorf("this installation does not hold schematic %s, so the "+
				"architecture to resolve its installer for is unknown: %w", schematicID, err)
		}

		arch := imagefactory.Arch(rec.Arch)
		if !arch.Valid() {
			return "", fmt.Errorf("schematic %s names architecture %q, which is not one the "+
				"Image Factory builds assets for", schematicID, rec.Arch)
		}

		ref, _, err := factory.InstallerImage(ctx, imagefactory.AssetRequest{
			SchematicID: schematicID,
			Version:     version,
			Arch:        arch,
			Platform:    imagefactory.PlatformMetal,
			SecureBoot:  secureBoot,
		})
		return ref, err
	}
}

// routeTable assembles the whole HTTP surface, from each handler package's own
// Routes function.
//
// It is a function rather than an expression inside run() because it is not
// only main that needs the list: allowlist_test.go walks every route to assert
// that each audited action has an entry in the redaction allowlist, and a
// second copy of this list in the test was a second place to forget a route
// set. It already had been forgotten once -- the guard passed over a handler
// nobody had added to it, which is the exact shape of the failure it exists to
// catch.
//
// A new plan adds its routes in its handler file and adds one line here;
// router.go stays untouched.
func routeTable(deps httpapi.Deps) []httpapi.Route {
	return slices.Concat(
		handlers.SystemRoutes(deps),
		handlers.SetupRoutes(deps),
		handlers.AuthRoutes(deps),
		handlers.OIDCRoutes(deps),
		handlers.AccountRoutes(deps),
		handlers.UserRoutes(deps),
		handlers.AuditRoutes(deps),
		handlers.SchematicRoutes(deps),
		handlers.InventoryRoutes(deps),
		handlers.LabelRoutes(deps),
		handlers.TemplateRoutes(deps),
		handlers.ScaleRoutes(deps),
		handlers.RotateRoutes(deps),
		handlers.StreamRoutes(deps),
		handlers.JobRoutes(deps),
		handlers.ConfigRoutes(deps),
		handlers.ProvisionRoutes(deps),
		handlers.UpgradeRoutes(deps),
		handlers.SupportRoutes(deps),
		handlers.MetricsRoutes(deps),
	)
}
