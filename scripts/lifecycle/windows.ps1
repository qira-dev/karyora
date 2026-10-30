# scripts/lifecycle/windows.ps1 — the C5-20 lifecycle-evidence cells for the
# Windows rows on a CLEAN hosted runner. See linux.sh's header for the
# placement/authority note (test-side evidence tooling; the C5-15 §5.0
# guard keeps binding packaging/; the explicit-deletion cell is the
# documented separate act under the test's own authority).
#
# Environment: ROW, DIST, EXPECT_VERSION, BUMP_VERSION, REPORT, ARCH.
# Every cell is recorded; a failure exits nonzero (fail-closed).
$ErrorActionPreference = 'Stop'

$row          = $env:ROW;          if (-not $row)          { throw "ROW is required" }
$dist         = $env:DIST;         if (-not $dist)         { throw "DIST is required" }
$report       = $env:REPORT;       if (-not $report)       { throw "REPORT is required" }
$bumpVersion  = $env:BUMP_VERSION; if (-not $bumpVersion)  { throw "BUMP_VERSION is required" }
$arch         = $env:ARCH;         if (-not $arch)         { throw "ARCH is required" }

$bin  = Join-Path $env:LOCALAPPDATA 'Programs\Karyora\bin\uawp.exe'
$data = Join-Path $env:APPDATA 'Karyora\data'
$msi  = Join-Path $dist "karyora-core-0.0.0-windows-$arch.msi"

Set-Content -Path $report -Value '' -NoNewline
function Cell([string]$verdict, [string]$name, [string]$detail) {
  $line = ('{0,-6} {1,-34} {2}' -f $verdict, $name, $detail)
  Add-Content -Path $report -Value $line
  Write-Output $line
  if ($verdict -ne 'PASS') { throw "lifecycle: cell '$name' FAILED: $detail" }
}
function Note([string]$name, [string]$detail) {
  $line = ('note   {0,-34} {1}' -f $name, $detail)
  Add-Content -Path $report -Value $line
  Write-Output $line
}

Add-Content -Path $report -Value "== C5-20 lifecycle evidence — row $row =="
Add-Content -Path $report -Value "environment: $([System.Environment]::OSVersion.VersionString) arch $env:PROCESSOR_ARCHITECTURE"
Add-Content -Path $report -Value "environment: runner image ${env:ImageOS}-${env:ImageVersion}"

# The exact-artifact binding: the bundle's digest recompute. The windows
# rows emit binary-mode sha256sum lines (`*name`); field 1 is the digest.
$sums = Join-Path $dist "SHA256SUMS-$row.txt"
$sumLine = (Get-Content $sums | Select-Object -First 1)
$expect  = ($sumLine -split '\s+')[0]
$msiDigest = (Get-FileHash -Algorithm SHA256 $msi).Hash.ToLower()
if ($msiDigest -ne $expect.ToLower()) { Cell 'FAIL' 'artifact-digest-binding' "msi digest $msiDigest != $expect" }
Cell 'PASS' 'artifact-digest-binding' "msi sha256 $msiDigest recomputed OK"
Add-Content -Path $report -Value "exact-artifact: karyora-core-0.0.0-windows-$arch.msi sha256 $msiDigest"

# C1: clean install (the real MSI, unsigned by design — C5-13 §9.2).
$install = Start-Process msiexec -ArgumentList "/i", "`"$msi`"", "/qn", "/norestart", "/l*vw", "`"$env:RUNNER_TEMP\msi-install.log`"" -Wait -PassThru
if ($install.ExitCode -ne 0 -and $install.ExitCode -ne 3010) { Cell 'FAIL' 'clean-install' "msiexec exit $($install.ExitCode)" }
if (-not (Test-Path $bin)) { Cell 'FAIL' 'clean-install' "$bin missing after msiexec /i" }
$got = (& $bin version | Select-Object -First 1)
Cell 'PASS' 'clean-install' "$got"

function Wait-Live([int]$seconds) {
  for ($i = 0; $i -lt $seconds; $i++) {
    $out = & $bin status --json 2>$null | Out-String
    if ($out -match '"live"\s*:\s*true') { return $true }
    Start-Sleep -Seconds 1
  }
  return $false
}

# C2: product lifecycle.
Start-Process -FilePath $bin -ArgumentList 'start' -WindowStyle Hidden | Out-Null
if (-not (Wait-Live 30)) { Cell 'FAIL' 'lifecycle-start' 'the core did not report live within 30s' }
Cell 'PASS' 'lifecycle-start' 'status reports live'
& $bin start *> $null
$secondExit = $LASTEXITCODE
if ($secondExit -eq 0) { Cell 'FAIL' 'second-start-refusal' 'a second start was accepted (exit 0)' }
Cell 'PASS' 'second-start-refusal' "the second start was refused (exit $secondExit)"
& $bin stop *> $null
if (Wait-Live 5) { Cell 'FAIL' 'lifecycle-stop' 'still live after stop' }
Cell 'PASS' 'lifecycle-stop' 'status reports not running'

# C3: crash recovery.
Start-Process -FilePath $bin -ArgumentList 'start' -WindowStyle Hidden | Out-Null
if (-not (Wait-Live 30)) { Cell 'FAIL' 'crash-recovery' 'no live status before the kill' }
Stop-Process -Name uawp -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2
Start-Process -FilePath $bin -ArgumentList 'start' -WindowStyle Hidden | Out-Null
if (-not (Wait-Live 45)) { Cell 'FAIL' 'crash-recovery' 'no live status within 45s after the force-kill restart' }
Cell 'PASS' 'crash-recovery' 'the restart after the force-kill reports live (startup recovery ran)'
if (-not (Test-Path $data)) { Cell 'FAIL' 'data-directory' "$data was not created by the first start" }
Note 'data-directory' "$data exists"

# C4: the real cross-version major-upgrade transaction — rebuild the MSI at
# the bumped synthetic version from the checked-out packaging sources over
# the SAME binary (WiX 3.14.1, digest-pinned — the recorded toolchain).
$wixZip = Join-Path $env:RUNNER_TEMP 'wix314-binaries.zip'
Invoke-WebRequest -Uri 'https://github.com/wixtoolset/wix3/releases/download/wix3141rtm/wix314-binaries.zip' -OutFile $wixZip
$actualDigest = (Get-FileHash -Algorithm SHA256 $wixZip).Hash.ToLower()
if ($actualDigest -ne $env:WIX314_DIGEST) { Cell 'FAIL' 'package-upgrade' 'wix zip digest mismatch' }
Expand-Archive -Path $wixZip -DestinationPath (Join-Path $env:RUNNER_TEMP 'wix314') -Force
$wix = Join-Path $env:RUNNER_TEMP 'wix314'
$wixArch = 'x64'; if ($arch -eq 'arm64') { $wixArch = 'arm64' }
& (Join-Path $wix 'candle.exe') -arch $wixArch -dProductVersion=$bumpVersion -dBinaryPath="$dist\uawp-windows-$arch.exe" -out "$env:RUNNER_TEMP\uawp-upgrade.wixobj" packaging/windows/uawp.wxs *> $null
if ($LASTEXITCODE -ne 0) { Cell 'FAIL' 'package-upgrade' 'candle failed' }
& (Join-Path $wix 'light.exe') -out "$env:RUNNER_TEMP\karyora-core-$bumpVersion-windows-$arch.msi" "$env:RUNNER_TEMP\uawp-upgrade.wixobj" *> $null
if ($LASTEXITCODE -ne 0) { Cell 'FAIL' 'package-upgrade' 'light failed' }
$upgrade = Start-Process msiexec -ArgumentList "/i", "`"$env:RUNNER_TEMP\karyora-core-$bumpVersion-windows-$arch.msi`"", "/qn", "/norestart", "/l*vw", "`"$env:RUNNER_TEMP\msi-upgrade.log`"" -Wait -PassThru
if ($upgrade.ExitCode -ne 0 -and $upgrade.ExitCode -ne 3010) { Cell 'FAIL' 'package-upgrade' "upgrade msiexec exit $($upgrade.ExitCode)" }
if (-not (Test-Path $bin)) { Cell 'FAIL' 'package-upgrade' 'the binary vanished after the upgrade' }
if (-not (Test-Path $data)) { Cell 'FAIL' 'package-upgrade' 'the data directory did not survive the upgrade' }
Cell 'PASS' 'package-upgrade' "the major-upgrade transaction installed $bumpVersion over 0.0.0; the data directory survived"

# C5: uninstall preserves user data structurally. NOTE: the /x targets the
# UPGRADED product (the bump replaced 0.0.0 via the MajorUpgrade
# transaction), so the uninstall resolves the product by its UpgradeCode —
# msiexec /x accepts the ORIGINAL msi path only when its product code
# matches; the upgraded product is removed through the upgrade MSI.
$uninstall = Start-Process msiexec -ArgumentList "/x", "`"$env:RUNNER_TEMP\karyora-core-$bumpVersion-windows-$arch.msi`"", "/qn", "/norestart", "/l*vw", "`"$env:RUNNER_TEMP\msi-uninstall.log`"" -Wait -PassThru
if ($uninstall.ExitCode -ne 0 -and $uninstall.ExitCode -ne 3010) { Cell 'FAIL' 'uninstall-preserves-data' "msiexec /x exit $($uninstall.ExitCode)" }
if (Test-Path $bin) { Cell 'FAIL' 'uninstall-preserves-data' "$bin still present after uninstall" }
if (-not (Test-Path $data)) { Cell 'FAIL' 'uninstall-preserves-data' 'the data directory was removed by uninstall' }
Cell 'PASS' 'uninstall-preserves-data' "binary gone; $data still present"

# C6: the explicit data deletion (the separate act) + fresh re-init.
Remove-Item -LiteralPath $data -Recurse -Force
if (Test-Path $data) { Cell 'FAIL' 'explicit-data-deletion' "$data still exists after the explicit removal" }
Start-Process -FilePath $bin -ArgumentList 'start' -WindowStyle Hidden | Out-Null
if (-not (Wait-Live 30)) { Cell 'FAIL' 'explicit-data-deletion' 'no live status after the explicit deletion + fresh start' }
if (-not (Test-Path $data)) { Cell 'FAIL' 'explicit-data-deletion' "the fresh start did not re-initialize $data" }
Cell 'PASS' 'explicit-data-deletion' 'the separate act removed the data; a fresh start re-initialized it'
& $bin stop *> $null

# C7: the updater's fail-closed posture with NO channel configured.
& $bin update check --json *> $env:RUNNER_TEMP\update-check.json
if ($LASTEXITCODE -eq 0) { Cell 'FAIL' 'updater-fail-closed' 'uawp update check succeeded without any endpoint configured' }
$updateOut = Get-Content $env:RUNNER_TEMP\update-check.json -Raw
if ($updateOut -notmatch 'fetch_failed') { Cell 'FAIL' 'updater-fail-closed' 'the refusal did not carry the typed reason' }
Cell 'PASS' 'updater-fail-closed' 'the machine refused with the typed fetch_failed reason (no channel configured)'

# Cells honestly NOT RUN (recorded, never silently skipped):
Add-Content -Path $report -Value 'NOT-RUN update-via-updater        the C5-13 signing stage is BLOCKED (C5-13 §9.2) — no signed release exists to serve'
Add-Content -Path $report -Value 'NOT-RUN interrupted-update        same BLOCKED signing prerequisite'
Add-Content -Path $report -Value 'NOT-RUN updater-rollback          same BLOCKED signing prerequisite'
Add-Content -Path $report -Value 'NOT-RUN artifact-signature        the Authenticode trust path — handover moment 1 (the cloud custody purchase)'
Add-Content -Path $report -Value 'NOT-RUN smartscreen-reputation    rides the Authenticode cell above'
Add-Content -Path $report -Value 'lifecycle: row complete'

Write-Output "lifecycle: row $row complete"
