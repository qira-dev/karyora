#!/usr/bin/env bash
# scripts/lifecycle/macos.sh — the C5-20 lifecycle-evidence cells for the
# macOS rows on a CLEAN hosted runner. See linux.sh's header for the
# placement/authority note (test-side evidence tooling; the C5-15 §5.0
# guard keeps binding packaging/; the explicit-deletion cell is the
# documented separate act under the test's own authority).
#
# Environment: ROW, DIST, EXPECT_VERSION, BUMP_VERSION, REPORT, ARCH.
set -euo pipefail
# Any unguarded failure names its own line (the honest-iteration discipline).
trap 'echo "lifecycle: FAILED at line $LINENO (exit $?)" >&2' ERR

ROW="${ROW:?}"
DIST="${DIST:?}"
EXPECT_VERSION="${EXPECT_VERSION:?}"
BUMP_VERSION="${BUMP_VERSION:?}"
REPORT="${REPORT:?}"
ARCH="${ARCH:?}"

DATA="$HOME/Library/Application Support/Karyora/data"
BIN=/usr/local/bin/uawp

: > "$REPORT"
cell() {
  printf '%-6s %-34s %s\n' "$1" "$2" "$3" | tee -a "$REPORT"
  [ "$1" = PASS ] || { echo "lifecycle: cell '$2' FAILED: $3" >&2; exit 1; }
}
note() { printf 'note   %-34s %s\n' "$1" "$2" | tee -a "$REPORT"; }

echo "== C5-20 lifecycle evidence — row $ROW ==" | tee -a "$REPORT"
{
  echo "environment: $(sw_vers -productVersion) ($(sw_vers -buildVersion)) arch $(uname -m)"
  echo "environment: runner image ${ImageOS:-unknown}-${ImageVersion:-unknown}"
} | tee -a "$REPORT"

# Downloaded artifacts are not normally quarantined, but the strip is
# cheap insurance for the runner's execution of the unsigned package
# (signing/notarization is the BLOCKED release-day stage — C5-13 §9.2).
xattr -dr com.apple.quarantine "$DIST" 2>/dev/null || true

( cd "$DIST" && shasum -a 256 -c "SHA256SUMS-$ROW.txt" ) >> "$REPORT" 2>&1
cell PASS "artifact-digest-binding" "SHA256SUMS-$ROW.txt recomputed OK"
PKG_DIGEST=$(cd "$DIST" && shasum -a 256 "karyora-core-0.0.0-macos-$ARCH.pkg" | cut -d' ' -f1)
echo "exact-artifact: karyora-core-0.0.0-macos-$ARCH.pkg sha256 $PKG_DIGEST" | tee -a "$REPORT"

# C1: clean install (the real pkg; the bounded data-preserving postinstall).
sudo installer -pkg "$DIST/karyora-core-0.0.0-macos-$ARCH.pkg" -target / >> "$REPORT" 2>&1
[ -x "$BIN" ] || { cell FAIL "clean-install" "$BIN missing after installer -pkg"; }
GOT=$("$BIN" version | head -n 1)
cell PASS "clean-install" "$GOT"

# C2: product lifecycle.
"$BIN" start >> "$REPORT" 2>&1 &
START_PID=$!
LIVE=no
for i in $(seq 1 30); do
  if "$BIN" status --json 2>/dev/null | grep -q '"live": *true\|"live":true'; then LIVE=yes; break; fi
  sleep 1
done
[ "$LIVE" = yes ] || { cell FAIL "lifecycle-start" "the core did not report live within 30s"; }
cell PASS "lifecycle-start" "status reports live"
"$BIN" start >> "$REPORT" 2>&1 && SECOND=accepted || SECOND=refused
[ "$SECOND" = refused ] || { cell FAIL "second-start-refusal" "a second start was accepted"; }
cell PASS "second-start-refusal" "the second start was refused (attach-or-refuse)"
"$BIN" stop >> "$REPORT" 2>&1
wait "$START_PID" 2>/dev/null || true
"$BIN" status --json 2>/dev/null | grep -q '"live": *true\|"live":true' \
  && cell FAIL "lifecycle-stop" "still live after stop" \
  || cell PASS "lifecycle-stop" "status reports not running"

# C3: crash recovery.
"$BIN" start >> "$REPORT" 2>&1 &
for i in $(seq 1 30); do
  "$BIN" status --json 2>/dev/null | grep -q '"live": *true\|"live":true' && break
  sleep 1
done
CORE_PID=$(pgrep -x uawp | head -n 1)
[ -n "$CORE_PID" ] || { cell FAIL "crash-recovery" "the core pid was not found"; }
kill -9 "$CORE_PID"
wait "$START_PID" 2>/dev/null || true
"$BIN" start >> "$REPORT" 2>&1 &
RESTART_PID=$!
RECOVERED=no
for i in $(seq 1 45); do
  if "$BIN" status --json 2>/dev/null | grep -q '"live": *true\|"live":true'; then RECOVERED=yes; break; fi
  sleep 1
done
[ "$RECOVERED" = yes ] || { cell FAIL "crash-recovery" "no live status within 45s after the kill -9 restart"; }
cell PASS "crash-recovery" "the restart after kill -9 reports live (startup recovery ran)"
[ -d "$DATA" ] || { cell FAIL "data-directory" "$DATA was not created by the first start"; }
note "data-directory" "$DATA exists"
# The package operations that follow (C4–C6) run against a STOPPED product
# — the realistic package-management flow — so the recovered core stops
# here and its graceful stop is itself an asserted cell. The background
# start is REAPPED before the status probe: a status hitting the dying
# listener mid-shutdown is a race, not evidence (C2's reaped block is the
# proven shape).
if ! STOP_OUT=$("$BIN" stop 2>&1); then
  echo "stop-failed-output: $STOP_OUT" >&2
  cell FAIL "lifecycle-stop-recovery" "uawp stop exited nonzero: $STOP_OUT"
fi
printf '%s\n' "$STOP_OUT" >> "$REPORT"
wait "$RESTART_PID" 2>/dev/null || true
STOPPED=no
for i in $(seq 1 10); do
  "$BIN" status --json 2>/dev/null | grep -q '"live": *true\|"live":true' || { STOPPED=yes; break; }
  sleep 1
done
[ "$STOPPED" = yes ] || { cell FAIL "lifecycle-stop-recovery" "still live after the post-recovery stop"; }
cell PASS "lifecycle-stop-recovery" "the core is stopped before the package operations"

# C4: the real cross-version pkg upgrade (pkgbuild from the checked-out
# packaging sources over the SAME binary; the overlay transaction).
PKGROOT="$RUNNER_TEMP/pkgroot-upgrade/bin"
rm -rf "$RUNNER_TEMP/pkgroot-upgrade"
mkdir -p "$PKGROOT"
cp "$DIST/uawp-darwin-$ARCH" "$PKGROOT/uawp"
chmod 755 "$PKGROOT/uawp"
pkgbuild --identifier com.qira-dev.karyora-core \
  --version "${BUMP_VERSION}" \
  --root "$RUNNER_TEMP/pkgroot-upgrade" \
  --scripts packaging/macos \
  --install-location /usr/local \
  "$RUNNER_TEMP/karyora-core-${BUMP_VERSION}-macos-$ARCH.pkg" >> "$REPORT" 2>&1
sudo installer -pkg "$RUNNER_TEMP/karyora-core-${BUMP_VERSION}-macos-$ARCH.pkg" -target / >> "$REPORT" 2>&1
RECEIPT=$(pkgutil --pkg-info com.qira-dev.karyora-core | awk '/^version:/{print $2}')
[ "$RECEIPT" = "$BUMP_VERSION" ] || { cell FAIL "package-upgrade" "pkg receipt version $RECEIPT, want $BUMP_VERSION"; }
[ -d "$DATA" ] || { cell FAIL "package-upgrade" "the data directory did not survive the upgrade"; }
cell PASS "package-upgrade" "pkg receipt $RECEIPT over 0.0.0; the data directory survived"

# C5: uninstall preserves user data (the pkg story: the documented
# uninstall removes the payload; pkgutil --forget clears the receipt).
sudo rm -f "$BIN"
sudo pkgutil --forget com.qira-dev.karyora-core >> "$REPORT" 2>&1
[ ! -e "$BIN" ] || { cell FAIL "uninstall-preserves-data" "$BIN still present after the payload removal"; }
[ -d "$DATA" ] || { cell FAIL "uninstall-preserves-data" "the data directory was removed by uninstall"; }
cell PASS "uninstall-preserves-data" "payload gone; $DATA still present"

# C6: the explicit data deletion (the separate act) + fresh re-init. The
# payload was REMOVED in C5 (that is the point C5 proved), so the fresh
# start rides a REAL fresh install — the data deletion happens while the
# product is absent, exactly the user's documented flow.
sudo installer -pkg "$RUNNER_TEMP/karyora-core-${BUMP_VERSION}-macos-$ARCH.pkg" -target / >> "$REPORT" 2>&1
rm -rf "$DATA"
[ ! -d "$DATA" ] || { cell FAIL "explicit-data-deletion" "$DATA still exists after the explicit removal"; }
"$BIN" start >> "$REPORT" 2>&1 &
REINIT_PID=$!
REINIT=no
for i in $(seq 1 30); do
  if "$BIN" status --json 2>/dev/null | grep -q '"live": *true\|"live":true'; then REINIT=yes; break; fi
  sleep 1
done
[ "$REINIT" = yes ] || { cell FAIL "explicit-data-deletion" "no live status after the explicit deletion + fresh start"; }
[ -d "$DATA" ] || { cell FAIL "explicit-data-deletion" "the fresh start did not re-initialize $DATA"; }
cell PASS "explicit-data-deletion" "the separate act removed the data; a fresh install + start re-initialized it"
if ! REINIT_STOP=$("$BIN" stop 2>&1); then
  echo "stop-failed-output: $REINIT_STOP" >&2
  cell FAIL "lifecycle-stop-fresh" "the fresh core's stop exited nonzero: $REINIT_STOP"
fi
printf '%s\n' "$REINIT_STOP" >> "$REPORT"
wait "$REINIT_PID" 2>/dev/null || true

# C7: the updater's fail-closed posture with NO channel configured.
if "$BIN" update check --json >> "$REPORT" 2>&1; then
  cell FAIL "updater-fail-closed" "uawp update check succeeded without any endpoint configured"
fi
grep -q 'fetch_failed' "$REPORT" || { cell FAIL "updater-fail-closed" "the refusal did not carry the typed reason"; }
cell PASS "updater-fail-closed" "the machine refused with the typed fetch_failed reason (no channel configured)"

{
  echo "NOT-RUN update-via-updater        the C5-13 signing stage is BLOCKED (C5-13 §9.2) — no signed release exists to serve"
  echo "NOT-RUN interrupted-update        same BLOCKED signing prerequisite"
  echo "NOT-RUN updater-rollback          same BLOCKED signing prerequisite"
  echo "NOT-RUN artifact-signature        the Apple trust path (Developer ID + notarization) — handover moment 1 (the owner's §15 deferral)"
  echo "NOT-RUN gatekeeper-assessment     Gatekeeper acceptance rides the notarization cell above"
} | tee -a "$REPORT"

echo "lifecycle: row $ROW complete" | tee -a "$REPORT"
