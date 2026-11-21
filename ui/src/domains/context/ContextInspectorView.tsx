/**
 * `Context Inspector` Surface (UIR-7.3, Blueprint §12.4, E2E-16).
 *
 * Mountable both as the dedicated `/context` surface and inside the Side-Pane `Context Inspector` tab.
 * Displays exact assembled `ContextSnapshot` digest, visual token budget bar, included items
 * with trust zones, omitted items with exact omission reasons (`Token budget exceeded`,
 * `Scope excluded`, `Policy filtered`, `Forgotten memory`), and honest cost reporting
 * (`Reported` | `Estimated` | `Not reported`).
 */

import { useMemo, useState, type ReactElement } from "react";
import { ChromeIcon } from "../../components/icons";
import { Badge, Select } from "../../components/primitives";
import {
  computeTokenBudgetBreakdown,
  type ContextSnapshotRecord,
} from "./contextConfig";

export interface ContextInspectorViewProps {
  readonly snapshots: readonly ContextSnapshotRecord[];
  readonly compact?: boolean;
}

export function ContextInspectorView({
  snapshots,
  compact = false,
}: ContextInspectorViewProps): ReactElement {
  const [selectedSnapshotId, setSelectedSnapshotId] = useState<string>(
    snapshots[0]?.snapshotId ?? "",
  );

  const snapshot =
    snapshots.find((s) => s.snapshotId === selectedSnapshotId) ?? snapshots[0] ?? null;

  const breakdown = useMemo(
    () => (snapshot ? computeTokenBudgetBreakdown(snapshot) : []),
    [snapshot],
  );

  if (!snapshot) {
    return (
      <section className="ky-domain-page" data-testid="context-inspector-empty">
        <p style={{ color: "var(--text-muted)" }}>No ContextSnapshot captured yet.</p>
      </section>
    );
  }

  return (
    <section
      className="ky-domain-page"
      data-testid="context-inspector-view"
      data-compact={compact ? "true" : "false"}
      aria-label="Context Snapshot Inspector"
    >
      <header className="ky-domain-page__header">
        <div className="ky-domain-page__title-group">
          <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", flexWrap: "wrap" }}>
            <ChromeIcon name="cpu" size={18} />
            <h2 className="ky-domain-page__title">Context Inspector</h2>
            <Badge tone="info" label="Exact-Only Snapshot" />
          </div>
          <code className="ky-kp-digest">Digest: {snapshot.assembledDigest}</code>
        </div>

        <div style={{ display: "flex", gap: "var(--space-2)", alignItems: "center", flexWrap: "wrap" }}>
          <Select
            aria-label="Select turn context snapshot"
            value={snapshot.snapshotId}
            options={snapshots.map((s) => ({
              value: s.snapshotId,
              label: s.turnLabel,
            }))}
            onChange={(e) => setSelectedSnapshotId(e.target.value)}
          />
          <Badge
            tone={
              snapshot.costStatus === "Reported"
                ? "success"
                : snapshot.costStatus === "Estimated"
                  ? "warning"
                  : "neutral"
            }
            label={`Cost: ${snapshot.costStatus}`}
          />
        </div>
      </header>

      {/* Visual Token Budget Bar */}
      <div className="ky-gov-section" data-testid="context-token-budget-panel">
        <div style={{ display: "flex", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-2)" }}>
          <strong>
            Token Budget: {snapshot.usedTokens.toLocaleString()} /{" "}
            {snapshot.totalTokenBudget.toLocaleString()} tokens
          </strong>
          <span style={{ fontSize: "0.78rem", color: "var(--text-secondary)" }}>
            Model: {snapshot.modelId} · {snapshot.costDisplayValue}
          </span>
        </div>

        <div
          className="ky-token-bar"
          role="img"
          aria-label={`Used ${snapshot.usedTokens} of ${snapshot.totalTokenBudget} tokens`}
        >
          {breakdown.map((seg) => (
            <div
              key={seg.category}
              className="ky-token-bar__seg"
              data-cat={seg.category}
              style={{ width: `${Math.max(seg.tokens > 0 ? 1.5 : 0, seg.percent)}%` }}
              title={`${seg.label}: ${seg.tokens.toLocaleString()} tokens (${seg.percent}%)`}
            />
          ))}
        </div>

        <div className="ky-token-legend">
          {breakdown.map((seg) => (
            <span key={seg.category}>
              <strong>{seg.label}:</strong> {seg.tokens.toLocaleString()} ({seg.percent}%)
            </span>
          ))}
        </div>
      </div>

      {/* Included vs Omitted Side-by-Side Columns */}
      <div className="ky-context-columns">
        <div className="ky-gov-section" data-testid="context-included-items">
          <strong>Included Context Items ({snapshot.includedItems.length})</strong>
          {snapshot.includedItems.map((item) => (
            <div key={item.itemId} className="ky-gov-perm-row">
              <div>
                <div style={{ fontWeight: 600 }}>{item.label}</div>
                <span style={{ fontSize: "0.72rem", color: "var(--text-muted)" }}>
                  Zone: {item.trustZone} · Category: {item.category}
                </span>
              </div>
              <Badge tone="success" label={`${item.tokens.toLocaleString()} tok`} />
            </div>
          ))}
        </div>

        <div className="ky-gov-section" data-testid="context-omitted-items">
          <strong>Omitted Items & Exact Reasons ({snapshot.omittedItems.length})</strong>
          {snapshot.omittedItems.map((item) => (
            <div key={item.itemId} className="ky-gov-perm-row">
              <div>
                <div style={{ fontWeight: 600 }}>{item.label}</div>
                <span style={{ fontSize: "0.72rem", color: "var(--text-secondary)" }}>
                  {item.detail}
                </span>
              </div>
              <Badge
                tone={
                  item.reason === "Policy filtered"
                    ? "danger"
                    : item.reason === "Token budget exceeded"
                      ? "warning"
                      : "neutral"
                }
                label={item.reason}
              />
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
