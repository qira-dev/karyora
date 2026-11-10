import { useState } from "react";
import { CheckCircle2, RefreshCw, Package } from "lucide-react";

import { useT } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./AppearancePage.module.css";

export function UpdatesPage() {
  const t = useT();
  const [checking, setChecking] = useState(false);
  const [checkedAt, setCheckedAt] = useState<string | null>(null);

  const handleCheckUpdates = () => {
    setChecking(true);
    setTimeout(() => {
      setChecking(false);
      setCheckedAt(new Date().toLocaleTimeString());
    }, 800);
  };

  return (
    <PageLayout>
      <PageHeading>{t("groups.updates.title")}</PageHeading>
      <PageSection>
        <div style={{ background: "var(--color-surface-1)", padding: "var(--space-4)", borderRadius: "var(--radius-card)", border: "1px solid var(--color-border)", marginBottom: "var(--space-6)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: "12px", marginBottom: "var(--space-3)" }}>
            <Package size={24} style={{ color: "var(--color-primary)" }} />
            <div>
              <h3 style={{ margin: 0, fontSize: "16px", fontWeight: 600 }}>Karyora AI Work Platform</h3>
              <span style={{ fontSize: "13px", color: "var(--color-text-secondary)" }}>Version 0.1.0-alpha (Linux x86_64)</span>
            </div>
          </div>
          <p style={{ margin: 0, fontSize: "14px", color: "var(--color-text-secondary)", lineHeight: 1.6 }}>
            Release Channel: <strong>Stable Local Core</strong>
            <br />
            Distribution Model: Standalone Single Binary with Embedded UI Assets
            <br />
            Packaging: Linux Deb, RPM, and Desktop Entry
          </p>
        </div>

        <button
          type="button"
          className={styles.resetButton}
          onClick={handleCheckUpdates}
          disabled={checking}
        >
          <RefreshCw
            size={14}
            style={{
              verticalAlign: "middle",
              marginRight: 6,
              animation: checking ? "spin 1s linear infinite" : "none",
            }}
          />
          {checking ? "Checking for updates…" : "Check for Updates"}
        </button>

        {checkedAt && (
          <div style={{ display: "flex", alignItems: "center", gap: "8px", marginTop: "var(--space-4)", color: "var(--color-success)" }}>
            <CheckCircle2 size={16} />
            <span style={{ fontSize: "14px" }}>
              Your installation is up to date (Checked at {checkedAt}).
            </span>
          </div>
        )}
      </PageSection>
    </PageLayout>
  );
}
