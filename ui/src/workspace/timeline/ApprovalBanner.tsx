/**
 * Inline Human-in-the-Loop Tool & Durable Job Approval Banner
 * (UIR-4.2, Blueprint §8.1, §20.3 B, E2E-05).
 */

import type { ReactElement } from "react";
import { ChromeIcon } from "../../components/icons";
import { Badge, Button } from "../../components/primitives";

export interface ApprovalBannerProps {
  readonly approvalId: string;
  readonly commandOrTarget: string;
  readonly scopeLabel: string;
  readonly riskLevel: "low" | "medium" | "high";
  readonly diffPreview?: string;
  readonly status: "pending" | "approving" | "approved" | "rejected";
  readonly onDecision: (
    approvalId: string,
    decision: "approve" | "approve-session" | "reject",
  ) => void;
}

export function ApprovalBanner({
  approvalId,
  commandOrTarget,
  scopeLabel,
  riskLevel,
  diffPreview,
  status,
  onDecision,
}: ApprovalBannerProps): ReactElement {
  const isLocked = status !== "pending";

  return (
    <section
      role="region"
      aria-label="Human approval required"
      data-testid={`approval-banner-${approvalId}`}
      className="ky-approval-banner"
    >
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "var(--space-2)",
        }}
      >
        <div style={{ display: "inline-flex", alignItems: "center", gap: "var(--space-2)" }}>
          <ChromeIcon name="shield-security" size={16} />
          <strong style={{ fontSize: "0.875rem" }}>Approval required</strong>
          <Badge
            tone={riskLevel === "high" ? "danger" : riskLevel === "medium" ? "warning" : "info"}
            label={`Risk: ${riskLevel}`}
          />
          <Badge tone="neutral" label={scopeLabel} />
        </div>
        <Badge
          tone={
            status === "approved"
              ? "success"
              : status === "rejected"
                ? "danger"
                : "warning"
          }
          label={status}
        />
      </div>

      <code
        style={{
          padding: "var(--space-2)",
          borderRadius: "var(--radius-xs)",
          backgroundColor: "var(--bg-surface-1)",
          fontFamily: "var(--font-mono)",
          fontSize: "0.8125rem",
          wordBreak: "break-all",
        }}
      >
        {commandOrTarget}
      </code>

      {diffPreview && (
        <pre
          style={{
            margin: 0,
            padding: "var(--space-2)",
            borderRadius: "var(--radius-xs)",
            backgroundColor: "var(--code-bg)",
            color: "var(--code-fg)",
            fontFamily: "var(--font-mono)",
            fontSize: "0.75rem",
            overflowX: "auto",
          }}
        >
          {diffPreview}
        </pre>
      )}

      <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-2)" }}>
        <Button
          variant="primary"
          size="sm"
          disabled={isLocked}
          loading={status === "approving"}
          loadingLabel="Approving..."
          onClick={() => onDecision(approvalId, "approve")}
        >
          Approve
        </Button>
        <Button
          variant="secondary"
          size="sm"
          disabled={isLocked}
          onClick={() => onDecision(approvalId, "approve-session")}
        >
          Approve for session
        </Button>
        <Button
          variant="danger"
          size="sm"
          disabled={isLocked}
          onClick={() => onDecision(approvalId, "reject")}
        >
          Reject
        </Button>
      </div>
    </section>
  );
}
