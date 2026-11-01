package main

import (
	"fmt"
	"net"
	"net/http"

	"github.com/qira-dev/karyora/internal/config"
	"github.com/qira-dev/karyora/internal/httplocal"
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
func startLocalHTTPTransport(cfg config.Config, l *lazyStore, bundle *securityBundle, correlationID string) (func() error, error) {
	// The same operation-surface wiring the CLI job path composes: one
	// app.Slice over the opened store, the bundle's guard, and the
	// wired provider set.
	slice, _, err := wireJobSlice(l, bundle)
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
	// Safe metadata only: the loopback address and port, never any token
	// or credential material (R-11).
	bundle.logger.Emit(security.Event{
		Level:         security.LevelInfo,
		Category:      "transport.local_http",
		MessageKey:    "LOCAL_HTTP_TRANSPORT_STARTED",
		CorrelationID: correlationID,
		Fields:        map[string]string{"address": ln.Addr().String()},
	})
	return func() error { return ln.Close() }, nil
}
