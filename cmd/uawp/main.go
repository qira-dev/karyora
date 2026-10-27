// Command uawp is the composition root of the Karyora Core and the Phase 2
// CLI surface (C2-07): start, stop, status, doctor. One lifecycle serves the
// foreground path and the OS service adapters; every command has one machine
// form (--json, stable English keys) and one human form derived from the
// same report object. Human copy resolves from the 12-locale resource set
// with English fallback; JSON mode never carries localized prose.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/platform"
	"github.com/qira-dev/karyora/internal/runtime"
)

// version is the scaffold build identifier. Reproducible build metadata is
// stamped by the build pipeline (-ldflags -X) per the build-evidence
// protocol; this value is not a release version.
var version = "0.0.0-scaffold"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout))
}

// run executes one command and returns the process exit code from the
// C2-07 register.
func run(ctx context.Context, args []string, stdout *os.File) int {
	cmd, flags, ordered := parseArgs(args)
	loc := runtime.NewLocale(flags.locale)
	out := newPrinter(stdout, flags.json, loc)

	switch cmd {
	case "start":
		return cmdStart(ctx, flags, out)
	case "stop":
		return cmdStop(ctx, flags, out)
	case "status":
		return cmdStatus(ctx, flags, out)
	case "doctor":
		return cmdDoctor(ctx, flags, out)
	case "job":
		return cmdJob(ctx, flags, out, ordered)
	case "credential":
		return cmdCredential(ctx, flags, out, ordered)
	case "version":
		out.text("karyora core " + version)
		return int(runtime.ExitOK)
	case "help", "":
		out.text(loc.Text(runtime.KeyHelpUsage))
		return int(runtime.ExitOK)
	default:
		fmt.Fprintf(os.Stderr, "uawp: unknown command %q\n", cmd)
		out.text(loc.Text(runtime.KeyHelpUsage))
		return int(runtime.ExitFailure)
	}
}

type cliFlags struct {
	json      bool
	dataDir   string
	locale    string
	overrides map[string]any
}

// parseArgs splits the command from the shared flags and returns the
// remaining positional words (subcommand + arguments). Value-taking flags
// accept "--flag value" or "--flag=value".
func parseArgs(args []string) (string, cliFlags, []string) {
	var f cliFlags
	var ordered []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			f.json = true
		case "--data-dir", "--locale":
			if i+1 < len(args) {
				if args[i] == "--data-dir" {
					f.dataDir = args[i+1]
				} else {
					f.locale = args[i+1]
				}
				i++
			}
		default:
			if strings.HasPrefix(args[i], "--data-dir=") {
				f.dataDir = strings.TrimPrefix(args[i], "--data-dir=")
			} else if strings.HasPrefix(args[i], "--locale=") {
				f.locale = strings.TrimPrefix(args[i], "--locale=")
			} else {
				ordered = append(ordered, args[i])
			}
		}
	}
	if len(ordered) == 0 {
		return "", f, nil
	}
	return ordered[0], f, ordered[1:]
}

// printer renders one form only: machine JSON or locale-resolved human text.
type printer struct {
	json bool
	loc  runtime.Locale
	out  *os.File
}

func newPrinter(out *os.File, asJSON bool, loc runtime.Locale) *printer {
	return &printer{json: asJSON, loc: loc, out: out}
}

func (p *printer) text(s string) {
	fmt.Fprintln(p.out, s)
}

// statusReport prints a status report in the selected form; both derive from
// the same report object (C2-07 §5).
func (p *printer) statusReport(r runtime.StatusReport) {
	if p.json {
		p.report(r)
		return
	}
	p.text(r.Human(p.loc))
}

// stopOutcome prints the drain outcome in the selected form.
func (p *printer) stopOutcome(o runtime.DrainOutcome) {
	if p.json {
		p.report(o)
		return
	}
	if o.Completed {
		p.text(p.loc.Text(runtime.KeyStopCompleted))
		return
	}
	p.text(p.loc.Text(runtime.KeyStopIncomplete))
}

func (p *printer) report(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	fmt.Fprintln(p.out, string(b))
}

func (p *printer) failure(f *runtime.Failure) {
	if p.json {
		p.report(f.Envelope)
		return
	}
	p.text(p.loc.ErrorText(string(f.Envelope.Code), f.Envelope.CorrelationID))
}

// resolvePaths resolves the storage-class roots with the explicit
// data-dir override (C2-06); failures map to the exit register.
func resolvePaths(flags cliFlags) (platform.DirSet, *runtime.Failure) {
	corr := errs.NewCorrelationID()
	ds, err := platform.Resolve(&platform.Overrides{DataDir: flags.dataDir})
	if err != nil {
		env, _ := errs.New(errs.CodeValidationFailed, corr, map[string]any{"resource": "paths"})
		return platform.DirSet{}, &runtime.Failure{Envelope: env, Exit: runtime.ExitPermission}
	}
	return ds, nil
}

func cmdStart(ctx context.Context, flags cliFlags, out *printer) int {
	ds, f := resolvePaths(flags)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	corr := errs.NewCorrelationID()
	cfg, f := runtime.LoadEffectiveConfig(corr, ds.Config, flags.overrides, os.Getenv)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	l := &lazyStore{opts: buildOptions(cfg, ds.Data)}
	defer l.close()
	journal := &lifecycleJournal{store: l, corrID: corr}
	// Security bundle (C2-14/C2-15): local identity, decision port + guard,
	// tamper-evident audit chain, bounded diagnostics. The audit mirror
	// rides the lifecycle sink so transitions populate the audit slice.
	bundle := newSecurityBundle(l, cfg, ds.Data, ds.State, corr)
	defer bundle.logger.Close()
	sink := func(ev runtime.Event) {
		journal.sink(ev)
		bundle.auditMirror(ev, corr)
	}
	runner := runtime.NewRunner(ds, runnerPorts(l, journal, version, bundle), sink)
	runner.Guard = bundle.guardFunc(corr)
	if f := runner.Start(ctx, corr, version); f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	closeIPC, err := runner.ServeIPC()
	if err != nil {
		env, _ := errs.New(errs.CodeInternal, corr, map[string]any{"reason": "IPC_INFRASTRUCTURE"})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitAuthority})
		return int(runtime.ExitAuthority)
	}
	defer closeIPC()

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-sigCtx.Done()
		// Signals map to the same graceful-stop request; a repeated signal
		// does not bypass the safe order (C2-07 §3).
		runner.StopRequested("signal")
	}()
	<-runner.Done()
	outcome := runner.DrainResult()
	if outcome.Completed {
		return int(runtime.ExitOK)
	}
	return int(runtime.ExitFailure)
}

func cmdStop(ctx context.Context, flags cliFlags, out *printer) int {
	ds, f := resolvePaths(flags)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	rec, token, err := platform.OwnerCredentials(ds.Runtime)
	if err != nil {
		// Already stopped: the stable idempotent no-op (C2-07 §2).
		out.statusReport(runtime.NotRunningReport(nil))
		return int(runtime.ExitOK)
	}
	resp, err := platform.CallIPC(ds.Runtime, platform.IPCOpStop, rec.InstanceUUID, token)
	if err != nil {
		// The owner died between the record read and the call: idempotent.
		out.statusReport(runtime.NotRunningReport(&rec))
		return int(runtime.ExitOK)
	}
	if !resp.OK {
		if resp.Code == "AUTHZ_DENIED" {
			env, _ := errs.New(errs.CodeAuthzDenied, errs.NewCorrelationID(), map[string]any{"resource": "ipc"})
			out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitAuthority})
			return int(runtime.ExitAuthority)
		}
		return int(runtime.ExitFailure)
	}
	var outcome runtime.DrainOutcome
	if err := json.Unmarshal(resp.Body, &outcome); err == nil {
		out.stopOutcome(outcome)
		if outcome.Completed {
			return int(runtime.ExitOK)
		}
		return int(runtime.ExitFailure)
	}
	return int(runtime.ExitOK)
}

func cmdStatus(ctx context.Context, flags cliFlags, out *printer) int {
	ds, f := resolvePaths(flags)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	rec, token, err := platform.OwnerCredentials(ds.Runtime)
	if err != nil {
		out.statusReport(runtime.NotRunningReport(nil))
		return int(runtime.ExitOK)
	}
	resp, err := platform.CallIPC(ds.Runtime, platform.IPCOpStatus, rec.InstanceUUID, token)
	if err != nil {
		// Read-only: a dead owner is a stable not-running report with the
		// recorded diagnostics (never a readiness claim).
		out.statusReport(runtime.NotRunningReport(&rec))
		return int(runtime.ExitOK)
	}
	if !resp.OK {
		env, _ := errs.New(errs.CodeAuthzDenied, errs.NewCorrelationID(), map[string]any{"resource": "ipc"})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitAuthority})
		return int(runtime.ExitAuthority)
	}
	var report runtime.StatusReport
	if err := json.Unmarshal(resp.Body, &report); err != nil {
		return int(runtime.ExitInternal)
	}
	out.statusReport(report)
	return int(runtime.ExitOK)
}

func cmdDoctor(ctx context.Context, flags cliFlags, out *printer) int {
	ds, f := resolvePaths(flags)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	corr := errs.NewCorrelationID()
	cfg, f := runtime.LoadEffectiveConfig(corr, ds.Config, flags.overrides, os.Getenv)
	doctor := runtime.Doctor{Paths: ds, Locale: out.loc}
	if f != nil {
		env := f.Envelope
		doctor.ConfigFailure = &env
	} else {
		opts := buildOptions(cfg, ds.Data)
		doctor.Storage = func(ctx context.Context) runtime.DoctorCheck {
			return storageProbe(ctx, opts)
		}
		// The audit chain check is wired when the database exists; a
		// never-started install has nothing to verify (doctor stays
		// read-only and never initializes anything).
		if _, err := os.Stat(opts.Path); err == nil {
			doctor.Audit = func(ctx context.Context) runtime.DoctorCheck {
				return auditProbe(ctx, opts, cfg)
			}
		}
	}
	report := doctor.Run(ctx)
	if flags.json {
		out.report(report)
	} else {
		out.text(report.Human(out.loc))
	}
	if report.Overall == runtime.DoctorFail {
		return int(runtime.ExitFailure)
	}
	return int(runtime.ExitOK)
}
