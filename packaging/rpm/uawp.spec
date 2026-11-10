# Karyora Core rpm spec (C5-15 §3.3 Option A — the owner's §14 Linux
# selection: deb + rpm + portable). Built by the release-build workflow's
# packaging step: rpmbuild -bb with the version/arch defines; the prebuilt
# binary arrives as SOURCES/uawp-<goarch> (no build in the spec — the
# binary is the C5-14 §6 stage-2 output).
#
# Binding rules carried here:
# - The data directory is not a packaged component; %post/%postun perform
#   NO user-data operation (C5-15 §5.0; explicit deletion stays the
#   separate documented act, Blueprint §3.7).
# - UNSIGNED on purpose: GPG signing of packages and repo metadata is the
#   BLOCKED release-day stage (C5-13 §9.2; handover moment 1).
# - License stays an honest non-conclusion until C5-22/D-31.
# - Warn-and-document (C5-15 §3.5 Option A): no floor check, no fake
#   support claim below the C5-11 floors.
Name:           karyora-core
Version:        %{_uawp_version}
Release:        1
Summary:        Karyora Core — the Unified AI Work Platform desktop product (uawp)
License:        UNDECIDED (license selection pending; task C5-22, gate D-31)
URL:            https://github.com/qira-dev/karyora
Source0:        uawp-%{_uawp_arch_go}
BuildArch:      %{_uawp_arch_rpm}
AutoReqProv:    no

%description
Local-mode AI work platform core; the binary is self-contained (the
CGO-linked SQLite engine ships inside it — zero runtime prerequisites,
D-07). User data lives at OS-standard user application-data locations
and is NEVER touched by install, upgrade or uninstall (C5-15 §5.0).

%prep
# Nothing to prepare: the pipeline prebuilt the binary.

%install
mkdir -p %{buildroot}/opt/karyora/bin
mkdir -p %{buildroot}/opt/karyora/share/applications
mkdir -p %{buildroot}/opt/karyora/share/icons
mkdir -p %{buildroot}/opt/karyora/share/systemd
install -m 755 "%{_sourcedir}/uawp-%{_uawp_arch_go}" %{buildroot}/opt/karyora/bin/uawp
install -m 644 packaging/linux/karyora.desktop %{buildroot}/opt/karyora/share/applications/karyora.desktop
install -m 644 packaging/linux/karyora.svg %{buildroot}/opt/karyora/share/icons/karyora.svg
install -m 644 packaging/linux/systemd/karyora.service %{buildroot}/opt/karyora/share/systemd/karyora.service

%files
/opt/karyora/bin/uawp
/opt/karyora/share/applications/karyora.desktop
/opt/karyora/share/icons/karyora.svg
/opt/karyora/share/systemd/karyora.service

%post
# C5-15 §5.0: no user-data operation; the binary's mode is fixed at
# install by the %install directive above.
if [ -d /usr/share/applications ]; then
    install -m 644 /opt/karyora/share/applications/karyora.desktop /usr/share/applications/karyora.desktop || true
fi
if [ -d /usr/share/icons/hicolor/scalable/apps ]; then
    install -m 644 /opt/karyora/share/icons/karyora.svg /usr/share/icons/hicolor/scalable/apps/karyora.svg || true
fi
if [ -d /etc/systemd/system ]; then
    install -m 644 /opt/karyora/share/systemd/karyora.service /etc/systemd/system/karyora.service || true
fi
exit 0

%postun
# C5-15 §5.0: uninstall preserves user data structurally.
rm -f /usr/share/applications/karyora.desktop || true
rm -f /usr/share/icons/hicolor/scalable/apps/karyora.svg || true
rm -f /etc/systemd/system/karyora.service || true
exit 0
