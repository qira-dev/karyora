import { Terminal, Shield, Keyboard, Bot } from "lucide-react";

import { useT } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./HelpPage.module.css";

export function HelpPage() {
  const t = useT();

  return (
    <PageLayout>
      <PageHeading>{t("items.helpAndDocumentation")}</PageHeading>

      <PageSection>
        <div className={styles.card}>
          <h2 className={styles.cardTitle}>
            <Shield size={18} style={{ verticalAlign: "middle", marginRight: 8 }} />
            Architecture & Privacy Guarantees
          </h2>
          <p>
            Karyora is built as a single self-contained binary on Linux. The Core daemon runs locally, storing data in SQLite with WAL journaling.
            All AI provider API keys are stored solely in the operating-system keyring or in-memory, and are never committed to disk, URLs, logs, or telemetry.
          </p>
        </div>

        <div className={styles.card}>
          <h2 className={styles.cardTitle}>
            <Bot size={18} style={{ verticalAlign: "middle", marginRight: 8 }} />
            Ask (Chat) vs Run (Jobs)
          </h2>
          <p>
            <strong>Ask Mode (/chat):</strong> Conversational interactive surface for rapid problem exploration and inquiries. Never submits durable background tasks and never executes destructive changes.
          </p>
          <p>
            <strong>Run Task Mode (/jobs/new):</strong> Formal background task execution pipeline with auditable event logs, checkpoints, and explicit permission gates.
          </p>
        </div>

        <div className={styles.card}>
          <h2 className={styles.cardTitle}>
            <Terminal size={18} style={{ verticalAlign: "middle", marginRight: 8 }} />
            CLI Command Reference
          </h2>
          <p>Manage Karyora from your Linux terminal using the compiled binary:</p>
          <pre className={styles.codeBlock}>
{`# Open the Web UI in your default browser
./uawp ui

# Generate a one-time session token for browser authentication
./uawp session token

# Submit a background job
./uawp job submit --input "Task prompt"

# Query job status
./uawp job status <job-id>`}
          </pre>
        </div>

        <div className={styles.card}>
          <h2 className={styles.cardTitle}>
            <Keyboard size={18} style={{ verticalAlign: "middle", marginRight: 8 }} />
            Keyboard Shortcuts
          </h2>
          <table className={styles.shortcutTable}>
            <thead>
              <tr>
                <th>Action</th>
                <th>Shortcut</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Open Search / Command Palette</td>
                <td><kbd className={styles.shortcutKbd}>Ctrl</kbd> + <kbd className={styles.shortcutKbd}>K</kbd></td>
              </tr>
              <tr>
                <td>Send message in Chat</td>
                <td><kbd className={styles.shortcutKbd}>Enter</kbd></td>
              </tr>
              <tr>
                <td>Insert newline in Chat</td>
                <td><kbd className={styles.shortcutKbd}>Shift</kbd> + <kbd className={styles.shortcutKbd}>Enter</kbd></td>
              </tr>
              <tr>
                <td>Close modal / dialog</td>
                <td><kbd className={styles.shortcutKbd}>Esc</kbd></td>
              </tr>
            </tbody>
          </table>
        </div>
      </PageSection>
    </PageLayout>
  );
}
