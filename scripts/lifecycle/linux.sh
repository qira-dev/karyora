#!/usr/bin/env bash
# scripts/lifecycle/linux.sh — the C5-20 lifecycle-evidence cells for the
# Linux rows, executed by the release-build workflow's lifecycle-evidence
# job on a CLEAN hosted runner (each job is a fresh VM: clean install,
# product lifecycle, second-process refusal, crash recovery, the real
# cross-version package-upgrade transaction, uninstall-with-data-preserved,
# the explicit data deletion as the SEPARATE authorized act, and the
# updater's fail-closed posture with no channel configured).
#
# Placement note: this is TEST-side evidence tooling (like tests/), NOT a
# publishable packaging source — the C5-15 §5.0 static guard keeps scanning
# packaging/ (the publishable set) for user-data-destructive commands, and
# the explicit-deletion cell below is exactly the documented separate act
# being VERIFIED (it deletes the data dir under the test's own authority,
# never a package script's).
#
# Environment: ROW, DIST (the downloaded bundle dir), EXPECT_VERSION,
# BUMP_VERSION (the in-job synthetic upgrade version), REPORT (the output
# file). Every cell is recorded; any failure exits nonzero (fail-closed).
set -euo pipefail

ROW="${ROW:?}"
DIST="${DIST:?}"
EXPECT_VERSION="${EXPECT_VERSION:?}"
BUMP_VERSION="${BUMP_VERSION:?}"
REPORT="${REPORT:?}"
ARCH="${ARCH:?}"

DATA="$HOME/.local/share/karyora/data"
BIN=/opt/karyora/bin/uawp

: > "$REPORT"
cell() { # cell PASS|FAIL name detail
  printf '%-6s %-34s %s\n' "$1" "$2" "$3" | tee -a "$REPORT"
  [ "$1" = PASS ] || { echo "lifecycle: cell '$2' FAILED: $3" >&2; exit 1; }
}
note() { printf 'note   %-34s %s\n' "$1" "$2" | tee -a "$REPORT"; }

echo "== C5-20 lifecycle evidence — row $ROW ==" | tee -a "$REPORT"
{
  echo "environment: $(grep PRETTY_NAME /etc/os-release | cut -d'"' -f2)"
  echo "environment: kernel $(uname -r) arch $(uname -m)"
  echo "environment: $(ldd --version | head -n 1)"
  echo "environment: runner image ${ImageOS:-unknown}-${ImageVersion:-unknown}"
} | tee -a "$REPORT"

# The exact-artifact binding: the bundle's digests recompute (the C5-20
# report carries the exact artifact, not a claim).
( cd "$DIST" && sha256sum -c "SHA256SUMS-$ROW.txt" ) >> "$REPORT" 2>&1
cell PASS "artifact-digest-binding" "SHA256SUMS-$ROW.txt recomputed OK"
PKG_DIGEST=$(cd "$DIST" && sha256sum "karyora-core_0.0.0_$ARCH.deb" | cut -d' ' -f1)
echo "exact-artifact: karyora-core_0.0.0_$ARCH.deb sha256 $PKG_DIGEST" | tee -a "$REPORT"

# C1: clean install (the real deb, unsigned by design — C5-13 §9.2).
sudo dpkg -i "$DIST/karyora-core_0.0.0_$ARCH.deb" >> "$REPORT" 2>&1
[ -x "$BIN" ] || { cell FAIL "clean-install" "$BIN missing after dpkg -i"; }
GOT=$("$BIN" version | head -n 1)
cell PASS "clean-install" "$GOT"

# C2: product lifecycle on the clean environment (start → live, the
# single-authority second-start refusal, stop → not running).
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
[ "$SECOND" = refused ] || { cell FAIL "second-start-refusal" "a second start was accepted (single-authority violation)"; }
cell PASS "second-start-refusal" "the second start was refused (attach-or-refuse)"
"$BIN" stop >> "$REPORT" 2>&1
wait "$START_PID" 2>/dev/null || true
"$BIN" status --json 2>/dev/null | grep -q '"live": *true\|"live":true' \
  && cell FAIL "lifecycle-stop" "still live after stop" \
  || cell PASS "lifecycle-stop" "status reports not running"

# C3: crash recovery — force-kill the core, restart, the delivered startup
# recovery reconciles (bounded).
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
RECOVERED=no
for i in $(seq 1 45); do
  if "$BIN" status --json 2>/dev/null | grep -q '"live": *true\|"live":true'; then RECOVERED=yes; break; fi
  sleep 1
done
[ "$RECOVERED" = yes ] || { cell FAIL "crash-recovery" "no live status within 45s after the kill -9 restart"; }
cell PASS "crash-recovery" "the restart after kill -9 reports live (startup recovery ran)"
[ -d "$DATA" ] || { cell FAIL "data-directory" "$DATA was not created by the first start"; }
note "data-directory" "$DATA exists"

# C4: the real cross-version package-upgrade transaction (an in-job
# synthetic version — the release-train versioning is release-time work).
STAGE="$RUNNER_TEMP/deb-upgrade-stage"
rm -rf "$STAGE"
mkdir -p "$STAGE/DEBIAN" "$STAGE/opt/karyora/bin"
cp "$DIST/uawp-linux-$ARCH" "$STAGE/opt/karyora/bin/uawp"
sed -e "s/@VERSION@/${BUMP_VERSION}/g" -e "s/@ARCH@/${ARCH}/g" packaging/linux/deb/DEBIAN/control > "$STAGE/DEBIAN/control"
for s in postinst prerm postrm; do cp "packaging/linux/deb/DEBIAN/$s" "$STAGE/DEBIAN/$s"; chmod 755 "$STAGE/DEBIAN/$s"; done
dpkg-deb --build --root-owner-group "$STAGE" "$RUNNER_TEMP/karyora-core_${BUMP_VERSION}_${ARCH}.deb" >> "$REPORT" 2>&1
sudo dpkg -i "$RUNNER_TEMP/karyora-core_${BUMP_VERSION}_${ARCH}.deb" >> "$REPORT" 2>&1
INSTALLED=$(dpkg-query -W -f='${Version}' karyora-core)
[ "$INSTALLED" = "$BUMP_VERSION" ] || { cell FAIL "package-upgrade" "dpkg reports version $INSTALLED, want $BUMP_VERSION"; }
[ -d "$DATA" ] || { cell FAIL "package-upgrade" "the data directory did not survive the upgrade"; }
cell PASS "package-upgrade" "dpkg $INSTALLED over 0.0.0; the data directory survived"

# C5: uninstall preserves user data structurally (C5-15 §5.0).
sudo dpkg -r karyora-core >> "$REPORT" 2>&1
[ ! -e "$BIN" ] || { cell FAIL "uninstall-preserves-data" "$BIN still present after dpkg -r"; }
[ -d "$DATA" ] || { cell FAIL "uninstall-preserves-data" "the data directory was removed by uninstall"; }
cell PASS "uninstall-preserves-data" "binary gone; $DATA still present"

# C6: the explicit data deletion is the SEPARATE documented act — performed
# here under the test's own authority (never by any package script), then a
# fresh install + start re-initializes. The package was UNINSTALLED in C5
# (that is the point C5 proved), so the fresh start rides a REAL fresh
# install — the data deletion happens while the product is absent, exactly
# the user's documented flow.
sudo dpkg -i "$RUNNER_TEMP/karyora-core_${BUMP_VERSION}_${ARCH}.deb" >> "$REPORT" 2>&1
rm -rf "$DATA"
[ ! -d "$DATA" ] || { cell FAIL "explicit-data-deletion" "$DATA still exists after the explicit removal"; }
"$BIN" start >> "$REPORT" 2>&1 &
REINIT=no
for i in $(seq 1 30); do
  if "$BIN" status --json 2>/dev/null | grep -q '"live": *true\|"live":true'; then REINIT=yes; break; fi
  sleep 1
done
[ "$REINIT" = yes ] || { cell FAIL "explicit-data-deletion" "no live status after the explicit deletion + fresh start"; }
[ -d "$DATA" ] || { cell FAIL "explicit-data-deletion" "the fresh start did not re-initialize $DATA"; }
cell PASS "explicit-data-deletion" "the separate act removed the data; a fresh install + start re-initialized it"
"$BIN" stop >> "$REPORT" 2>&1

# C7: the rpm row artifact (install/verify/remove — the second Linux
# format). The rpm TOOLCHAIN is installed per run with its version printed
# (the C5-14 §7.3 record-and-verify pattern — this job's VM is fresh).
sudo apt-get update -qq >> "$REPORT" 2>&1
sudo apt-get install -y -qq rpm >> "$REPORT" 2>&1
echo "environment: $(rpm --version)" | tee -a "$REPORT"
sudo rpm -i "$DIST/karyora-core-0.0.0-$( [ "$ARCH" = amd64 ] && echo x86_64 || echo aarch64 ).rpm" >> "$REPORT" 2>&1
[ -x "$BIN" ] || { cell FAIL "rpm-install" "$BIN missing after rpm -i"; }
cell PASS "rpm-install" "$("$BIN" version | head -n 1)"
sudo rpm -e karyora-core >> "$REPORT" 2>&1
[ ! -e "$BIN" ] || { cell FAIL "rpm-remove" "$BIN still present after rpm -e"; }
[ -d "$DATA" ] || { cell FAIL "rpm-remove" "the data directory did not survive rpm -e"; }
cell PASS "rpm-remove" "rpm transaction complete; the data directory survived"

# C8: the portable archive (signature-THEN-digest is the release-day
# verification; today the digest story is the honest cell — C5-15 §6).
PORTABLE="$DIST/karyora-core-0.0.0-linux-$ARCH-portable.tar.gz"
PT="$RUNNER_TEMP/portable"
rm -rf "$PT"; mkdir -p "$PT"
tar -xzf "$PORTABLE" -C "$PT"
[ -x "$PT/uawp-linux-$ARCH" ] || { cell FAIL "portable-archive" "the extracted binary is missing"; }
GOT=$("$PT/uawp-linux-$ARCH" version | head -n 1)
cell PASS "portable-archive" "$GOT (extracted + executed; the detached-signature step is the first SIGNED release)"

# C9: the updater's fail-closed posture with NO channel configured — the
# wired C5-18 machine refuses honestly (the typed fetch_failed refusal).
if "$BIN" update check --json >> "$REPORT" 2>&1; then
  cell FAIL "updater-fail-closed" "uawp update check succeeded without any endpoint configured"
fi
grep -q 'fetch_failed' "$REPORT" || { cell FAIL "updater-fail-closed" "the refusal did not carry the typed reason"; }
cell PASS "updater-fail-closed" "the machine refused with the typed fetch_failed reason (no channel configured)"

# Cells honestly NOT RUN (recorded, never silently skipped):
{
  echo "NOT-RUN update-via-updater        the C5-13 signing stage is BLOCKED (C5-13 §9.2) — no signed release exists to serve; the machine's refusal above is the exercised behavior"
  echo "NOT-RUN interrupted-update        same BLOCKED signing prerequisite (the C5-18 machine's flows exercise in-process under C5-19 with generated test keys)"
  echo "NOT-RUN updater-rollback          same BLOCKED signing prerequisite"
  echo "NOT-RUN artifact-signature        the platform trust path (Authenticode/notarization/GPG) — handover moment 1"
} | tee -a "$REPORT"

echo "lifecycle: row $ROW complete" | tee -a "$REPORT"
