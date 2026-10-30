# Karyora — Benutzerhandbuch

Dieses Handbuch beschreibt die Desktop-Anwendung Karyora so, wie sie heute ausgeliefert wird. Es nennt nur, was das Produkt tatsächlich tut; Fähigkeiten, die es noch nicht gibt, sind als Einschränkungen aufgeführt und niemals als Versprechen formuliert. Dasselbe Handbuch ist in diesem Ordner in 12 Sprachen verfügbar.

## 1. Was Karyora ist

- Karyora ist eine Desktop-Anwendung mit lokalem Vorrang (local-first). Ihre Wissenspakete, Jobs und Einstellungen werden auf Ihrem eigenen Rechner gespeichert.
- Das Produkt funktioniert ohne jeglichen Cloud-Dienst des Projekts. Der einzige Netzverkehr, den es erzeugen kann, ist von Ihnen veranlasst: Anfragen an Ihre KI-Anbieter, ein von Ihnen angefordertes Importieren eines Wissenspakets und — wenn Sie ihn konfigurieren — der Update-Kanal.
- KI-Funktionen nutzen Ihre eigenen Anbieterkonten (Bring Your Own Key, BYOK). Sie hinterlegen Ihre eigenen API-Schlüssel, und der Anbieter stellt Ihnen direkt in Rechnung. Karyora verkauft die Anbieternutzung niemals weiter und tritt nie als Zwischenhändler für deren Zahlungen auf.

## 2. Community- und offizielle Builds

- Der Quellcode von Karyora wird unter der Apache License 2.0 veröffentlicht. Sie dürfen ihn unter dieser Lizenz verwenden, untersuchen, verändern und weiterverbreiten — einschließlich des eigenen Buildens.
- Das öffentliche Repository ist `https://github.com/qira-dev/karyora`. Jede Version ist durch ein `v*`-Tag gekennzeichnet, und der Quellbaum dieses Tags ist der Quellcode der Version.
- Ein „offizielles“ Release ist eines, das von der eigenen Build-Pipeline des Projekts bei einer getaggten Revision mit aufgezeichneten Nachweisen gebaut wurde. Offizielle und Community-Builds haben denselben Funktionsumfang; der Unterschied liegt darin, wer das Artefakt gebaut hat und wer dafür einsteht — niemals darin, was das Artefakt kann.
- In v1 gibt es noch keine offizielle Downloadseite und keinen offiziellen Update-Kanal. Bis sie veröffentlicht sind, erhalten Sie Karyora aus dem öffentlichen Repository und bauen es aus dem Quellcode.

## 3. Unterstützte Plattformen (v1)

| Betriebssystem | CPU | Untergrenze |
|---|---|---|
| Windows 11 | x86_64 | Windows 11, 23H2 oder neuer |
| Windows 11 | ARM64 | Windows 11 für ARM |
| macOS | x86_64 | macOS 15 (Sequoia) oder neuer |
| macOS | ARM64 | macOS 15 (Sequoia) oder neuer, Apple Silicon |
| Linux | x86_64 | glibc 2.34 oder neuer; Ubuntu LTS 22.04/24.04, Debian 12+, RHEL 9+ |
| Linux | ARM64 | glibc 2.34 oder neuer; dieselben Distributionsfamilien |

In v1 ausdrücklich nicht unterstützt: Windows 10; macOS 14 oder älter; musl-basierte Distributionen wie Alpine; jede oben nicht aufgeführte Kombination. Die Unterstützung folgt dem Sicherheitsfenster des jeweiligen Anbieters und wird bei jedem Release neu geprüft.

## 4. Pakete und ihre Prüfung

- Der erzeugte Paketsatz ist: Windows-MSI, macOS-pkg, Linux-deb, Linux-rpm und ein portables Linux-Archiv — ein Artefakt pro unterstützter CPU-Architektur.
- Jedes Paket entsteht zusammen mit einem `SHA256SUMS`-Manifest sowie SBOM- und Provenienz-Aufzeichnungen je Artefakt. So prüfen Sie ein Paket: Berechnen Sie den sha256-Digest Ihres Downloads und vergleichen Sie ihn mit dem Manifest-Eintrag. Stimmt der Digest nicht überein, installieren Sie das Paket nicht.
- Pakete sind in v1 bewusst nicht digital signiert. Die Schritte zur Signaturprüfung der Plattform (Signaturprüfungen des Betriebssystems, Repository-Schlüssel) werden zusammen mit dem ersten signierten Release dokumentiert. Bis dahin ist der obige Digest-Vergleich die Prüfung, die Sie vornehmen können.
- Die Dateien `LICENSE` und `NOTICE` mit der Projektlizenz und den Drittangaben sind Teil des Quell-Repositorys.

## 5. Updates

- Karyora enthält einen Update-Mechanismus, der in v1 standardmäßig deaktiviert ausgeliefert wird. Es ist kein Update-Kanal konfiguriert, daher meldet eine Update-Prüfung eine typisierte Ablehnung, die besagt, dass kein Endpunkt konfiguriert ist. Das ist beabsichtigt: Der offizielle Update-Kanal wird erst aktiviert, wenn er veröffentlicht ist.
- Bis dahin bedeutet ein Upgrade, das neuere Paket über das ältere zu installieren. Ihre Daten bleiben über Upgrades hinweg erhalten (siehe Abschnitt 6).

## 6. Ihre Daten

- Alle Produktdaten liegen auf Ihrem Rechner. Das standardmäßige Datenverzeichnis ist `%APPDATA%\Karyora\data` unter Windows, `~/Library/Application Support/Karyora/data` unter macOS und `~/.local/share/karyora/data` unter Linux (die XDG-Einstellung `XDG_DATA_HOME` wird beachtet). Mit dem Flag `--data-dir` können Sie einen anderen Ort wählen.
- Das Produkt führt keine geplanten Sicherungen durch. Zum Sichern: Produkt stoppen und das Datenverzeichnis kopieren. Wiederherstellen heißt, das kopierte Verzeichnis zurückzulegen. Eine Cloud-Sicherung gibt es nicht.
- Deinstallieren oder Aktualisieren löscht Ihre Daten nicht. Das Löschen von Daten ist eine gesonderte, ausdrückliche Aktion, die Sie selbst ausführen.
- Daten im Ruhezustand sind nicht verschlüsselt. Schützen Sie das Datenverzeichnis mit eigenen Mitteln, etwa durch Festplattenverschlüsselung. Dies ist eine klar benannte, bekannte Einschränkung.
- Anbieter-Zugangsdaten (Ihre API-Schlüssel) werden ausschließlich im Schlüsselbund Ihres Betriebssystems gespeichert. Sie werden niemals in Dateien, Protokolle oder Prompt-Inhalte geschrieben.

## 7. Datenschutz

- Das Produkt enthält keine Telemetrie: An die Server des Projekts wird nichts gesendet.
- Diagnosedaten entstehen lokal und sind standardmäßig datenschutzfreundlich; Sie entscheiden, ob überhaupt etwas Ihren Rechner verlässt.

## 8. Bekannte Einschränkungen in v1

- Die Ausführung von Drittanbieter-Plugins ist deaktiviert.
- Team- und Fernbetrieb sind nicht verfügbar.
- Einen laufenden Job auf einen anderen Rechner zu übertragen, wird nicht unterstützt.
- Die Synchronisation portabler Wissenspakete ist nicht verfügbar.
- Die Gruppierungsfunktion „Work“ existiert nicht.
- Es gibt keine bezahlten Abonnements und keine Berechtigungen (Entitlements): nichts zu kaufen, keine kostenpflichtigen Stufen.
- Es werden keine numerischen Leistungsversprechen gemacht.
- Daten im Ruhezustand sind nicht verschlüsselt (siehe Abschnitt 6).
- Eine offizielle Downloadseite und ein Update-Kanal gibt es noch nicht (siehe Abschnitte 2 und 5).

## 9. Lizenz

- Karyora wird unter der Apache License 2.0 veröffentlicht. Der vollständige Lizenztext steht in der Datei `LICENSE`, die Drittangaben in der Datei `NOTICE`; beide im Quell-Repository.
- Karyora selbst zu bauen, es zu forken oder lizenzgerecht weiterzuverbreiten bewahrt alle Rechte, die die Lizenz gewährt. Dieses Recht hängt von keinem Kauf ab, weil es nichts zu kaufen gibt.

## 10. Kommandozeilen-Referenz

Das Kommandozeilen-Werkzeug ist `uawp`.

| Befehl | Wirkung |
|---|---|
| `uawp start` | Startet den lokalen Dienst. Ein zweiter Start auf demselben Datenverzeichnis wird abgelehnt, solange einer läuft. |
| `uawp stop` | Stoppt den Dienst mit einem begrenzten, geordneten Herunterfahren. |
| `uawp status` | Meldet, ob der Dienst läuft. |
| `uawp doctor` | Führt lokale Zustands- und Diagnoseprüfungen aus. |
| `uawp update check` / `uawp update apply` / `uawp update status` | Prüft, wendet an oder meldet Updates. In v1 — ohne konfigurierten Kanal — enden check und apply mit einer typisierten Ablehnung, die besagt, dass kein Endpunkt konfiguriert ist; `status` meldet den Update-Zustand. |
| `uawp job submit` / `uawp job status` / `uawp job events` | Reicht einen dauerhaften Job ein, meldet seinen Status oder verfolgt seine Ereignisse. |
| `uawp credential store --provider <id> [--name api-key]` | Speichert Ihren Anbieter-API-Schlüssel im Schlüsselbund des Betriebssystems. |
| `uawp credential status` | Meldet nur, ob ein Zugriffstoken vorhanden ist — niemals seinen Wert. |
| `uawp credential remove` | Entfernt das gespeicherte Zugriffstoken. |
| `uawp version` / `uawp help` | Gibt die Version bzw. die Hilfe aus. |

Ein gespeicherter Schlüssel öffnet für sich allein nichts: Ein echter Anbieteraufruf muss weiterhin die Datenverarbeitungsregeln des Produkts für diesen Anbieter passieren.
