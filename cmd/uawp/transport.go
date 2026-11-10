package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/app"
	"github.com/qira-dev/karyora/internal/config"
	"github.com/qira-dev/karyora/internal/httplocal"
	"github.com/qira-dev/karyora/internal/platform"
	"github.com/qira-dev/karyora/internal/runtime"
	"github.com/qira-dev/karyora/internal/security"
)

// Local HTTP transport wiring (IMPL-1): the composition root builds the
// transport over the SAME wiring the CLI foreground session uses (the
// wireJobSlice composition, the security bundle's session service and
// guard) and serves it on the loopback bind only. The transport is opt-in
// through the closed configuration registry
// (transport.local_http.enabled/port; never a hardcoded endpoint), and it
// exists only while the Core runs — the same lifetime as the IPC surface.
// The boundary predicates (loopback bind, Host/Origin allowlists,
// state-changing-GET prohibition, CSRF, credential surfaces) are enforced
// inside internal/httplocal; this root supplies no override for any of
// them and no browser session behavior is promised.

// startLocalHTTPTransport constructs and serves the transport, returning
// the stop function that closes the listener on drain.
func startLocalHTTPTransport(cfg config.Config, l *lazyStore, bundle *securityBundle, runner *runtime.Runner, ds platform.DirSet, correlationID string) (func() error, error) {
	// The same operation-surface wiring the CLI job path composes: one
	// app.Slice over the opened store, the bundle's guard, and the
	// wired provider set.
	slice, runDeps, err := wireJobSlice(l, bundle)
	if err != nil {
		return nil, err
	}
	srv, err := httplocal.New(httplocal.Deps{
		Slice:    slice,
		Sessions: bundle.sessions,
		Guard:    bundle.guard,
		Replay:   l,
		Log: func(ev security.Event) {
			bundle.logger.Emit(ev)
		},
	})
	if err != nil {
		return nil, err
	}
	// The bind host is the adapter's loopback-only default (validated by
	// the boundary predicate — a non-loopback bind is refused outright);
	// only the port is operator-tunable, 0 meaning kernel-assigned.
	if err := httplocal.ValidateBind("127.0.0.1"); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.TransportLocalHTTPPort))
	if err != nil {
		return nil, err
	}
	go func() { _ = http.Serve(ln, srv.Handler()) }()

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	workerHandler := app.NewRelayHandler(runDeps)
	workerCap := aijobrelay.WorkerCapability{
		WorkerID:    "daemon-worker",
		ProviderIDs: app.WiredProviders(runDeps),
		MaxMemoryMB: 1024,
	}
	sched := aijobrelay.NewScheduler(runDeps.Relay, workerCap, workerHandler.Handle, 2, 30*time.Second)
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				sched.Stop()
				return
			case <-ticker.C:
				_, _ = runDeps.Relay.ReleaseExpiredLeases(workerCtx, time.Now())
				if runDeps.Backoff != nil {
					_, _ = runDeps.Backoff.RequeueBackoff(workerCtx, time.Now())
				}
				_, _ = sched.Tick(workerCtx)
			}
		}
	}()

	if runner != nil {
		runner.BootstrapToken = func(origin string) (string, error) {
			cred, err := srv.IssueBootstrapCredential(origin)
			if err != nil {
				return "", err
			}
			return cred.Secret, nil
		}
	}

	// Issue trusted initial bootstrap credentials for standard local Web UI dev origins.
	credLocalhost, _ := srv.IssueBootstrapCredential("http://localhost:5173")
	_, _ = srv.IssueBootstrapCredential("http://127.0.0.1:5173")
	if ds.Runtime != "" {
		_ = os.WriteFile(filepath.Join(ds.Runtime, "http_address"), []byte(ln.Addr().String()), 0o600)
	}
	if credLocalhost.Secret != "" && ds.Runtime != "" {
		_ = os.WriteFile(filepath.Join(ds.Runtime, "bootstrap_token"), []byte(credLocalhost.Secret), 0o600)
		fmt.Printf("\n[karyora] Web UI bootstrap token (http://localhost:5173):\n          %s\n\n", credLocalhost.Secret)
	}

	// Safe metadata only: the loopback address and port, never any token
	// or credential material (R-11).
	bundle.logger.Emit(security.Event{
		Level:         security.LevelInfo,
		Category:      "transport.local_http",
		MessageKey:    "LOCAL_HTTP_TRANSPORT_STARTED",
		CorrelationID: correlationID,
		Fields:        map[string]string{"address": ln.Addr().String()},
	})
	return func() error {
		cancelWorker()
		if ds.Runtime != "" {
			_ = os.Remove(filepath.Join(ds.Runtime, "http_address"))
			_ = os.Remove(filepath.Join(ds.Runtime, "bootstrap_token"))
		}
		return ln.Close()
	}, nil
}
