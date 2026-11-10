import { ShieldCheck, Lock, Trash2 } from "lucide-react";
import { useState } from "react";

import { useT } from "../../i18n/useT";
import { usePreferences } from "../../app/preferences";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./AppearancePage.module.css";

export function PrivacyPage() {
  const t = useT();
  const { resetToDefaults } = usePreferences();
  const [clearedNotice, setClearedNotice] = useState(false);

  const handleClearLocalData = () => {
    try {
      window.localStorage.clear();
      resetToDefaults();
      setClearedNotice(true);
      setTimeout(() => setClearedNotice(false), 3000);
    } catch {
      // Ignore private mode errors
    }
  };

  return (
    <PageLayout>
      <PageHeading>{t("groups.privacy.title")}</PageHeading>
      <PageSection>
        <div style={{ marginBottom: "var(--space-6)" }}>
          <h2 style={{ fontSize: "var(--text-section-title-size)", fontWeight: 600 }}>
            <ShieldCheck size={18} style={{ verticalAlign: "middle", marginRight: 8, color: "var(--color-success)" }} />
            Zero-Telemetry & Local Security Guarantees
          </h2>
          <p style={{ color: "var(--color-text-secondary)", lineHeight: 1.6 }}>
            Karyora operates under a strict fail-closed data privacy posture. Your prompts, conversation histories, knowledge documents, and job outputs remain strictly on this local machine inside encrypted or WAL-journaled SQLite databases.
          </p>
        </div>

        <div style={{ marginBottom: "var(--space-6)" }}>
          <h2 style={{ fontSize: "var(--text-section-title-size)", fontWeight: 600 }}>
            <Lock size={18} style={{ verticalAlign: "middle", marginRight: 8, color: "var(--color-primary)" }} />
            Credential Isolation
          </h2>
          <p style={{ color: "var(--color-text-secondary)", lineHeight: 1.6 }}>
            No third-party provider API keys (OpenAI, Anthropic, Gemini, etc.) are ever written to browser storage, URLs, terminal logs, or git commits. Credentials are kept in the Linux OS keyring or runtime memory and wiped immediately upon session revocation.
          </p>
        </div>

        <div style={{ marginTop: "var(--space-6)", paddingTop: "var(--space-4)", borderTop: "1px solid var(--color-border)" }}>
          <h3 style={{ fontSize: "16px", fontWeight: 600, marginBottom: "var(--space-2)" }}>Local Browser Storage</h3>
          <p style={{ color: "var(--color-text-secondary)", fontSize: "14px", marginBottom: "var(--space-3)" }}>
            Clear all cached appearance and local device preferences saved in this browser.
          </p>
          <button
            type="button"
            className={styles.resetButton}
            onClick={handleClearLocalData}
          >
            <Trash2 size={14} style={{ verticalAlign: "middle", marginRight: 6 }} />
            Clear Local Preferences
          </button>
          {clearedNotice && (
            <p style={{ color: "var(--color-success)", fontSize: "13px", marginTop: "var(--space-2)" }}>
              Local browser storage successfully reset to default.
            </p>
          )}
        </div>
      </PageSection>
    </PageLayout>
  );
}
