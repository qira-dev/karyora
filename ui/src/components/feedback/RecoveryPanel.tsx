/**
 * Standardized 18-State & S4 Unknown-Outcome `RecoveryPanel` Component
 * (UIR-2.3, Blueprint §15, §21.1, §21.2, G-05, G-08, G-10).
 *
 * Renders the 5 mandatory elements on every error (`S3`), offline, auth-required,
 * permission-denied, rate-limited, conflict, or unknown-outcome (`S4`) state:
 * 1. Human-readable state title (localized from machine error `code`, never raw backend prose)
 * 2. Plain-language explanation of what happened
 * 3. Draft / Data Safety Assurance (confirming whether unsent prompt/form draft/Job is safe)
 * 4. Primary Safe Next Action button (`Retry`, `Reconcile status`, `Open Model settings`, etc.)
 * 5. Technical Diagnostic Disclosure (expandable RFC 9457 `errorCode`, `httpStatus`, and
 *    one-click `Copy Correlation ID`)
 */

import { useState, type ReactElement, type ReactNode } from "react";
import type { RecoveryPanelDescriptor } from "../../coreclient/types";
import { Button } from "../primitives/Button";

export interface RecoveryPanelProps {
  readonly descriptor: RecoveryPanelDescriptor;
  readonly titleText?: string;
  readonly explanationText?: string;
  readonly draftSafetyText?: string;
  readonly primaryActionLabel?: string;
  readonly onPrimaryAction?: (
    action: RecoveryPanelDescriptor["primaryAction"],
  ) => void;
  readonly onCopyCorrelationId?: (correlationId: string) => void;
  readonly secondaryActions?: ReactNode;
}

const DEFAULT_ACTION_LABELS: Readonly<
  Record<RecoveryPanelDescriptor["primaryAction"], string>
> = {
  "retry": "Retry operation",
  "reconcile-s4": "Reconcile status with Core (S4)",
  "open-model-settings": "Open Model settings",
  "switch-core-connection": "Switch Core connection",
  "reauthenticate": "Re-authenticate session",
  "resolve-conflict": "Review revision conflict",
  "dismiss": "Dismiss",
};

export function RecoveryPanel({
  descriptor,
  titleText,
  explanationText,
  draftSafetyText = "Your unsent draft, local edits, and active background Jobs are preserved safely.",
  primaryActionLabel,
  onPrimaryAction,
  onCopyCorrelationId,
  secondaryActions,
}: RecoveryPanelProps): ReactElement {
  const [showTechnicalDetails, setShowTechnicalDetails] = useState(false);
  const [copied, setCopied] = useState(false);

  const handleCopyCorrelationId = () => {
    onCopyCorrelationId?.(descriptor.correlationId);
    if (
      typeof navigator !== "undefined" &&
      typeof navigator.clipboard?.writeText === "function"
    ) {
      void navigator.clipboard.writeText(descriptor.correlationId);
    }
    setCopied(true);
  };

  return (
    <section
      role="alert"
      aria-live="assertive"
      data-testid="karyora-recovery-panel"
      data-state-kind={descriptor.stateKind}
      data-error-code={descriptor.errorCode}
      className={`ky-recovery-panel ky-recovery-panel--${descriptor.stateKind}`}
    >
      {/* Element 1: Human-readable state title */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "var(--space-2)",
        }}
      >
        <h3 style={{ margin: 0, fontSize: "0.9375rem", fontWeight: 600 }}>
          {titleText ?? descriptor.titleI18nKey}
        </h3>
        <span
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: "0.75rem",
            color: "var(--text-muted)",
          }}
        >
          {descriptor.errorCode}
        </span>
      </div>

      {/* Element 2: Plain-language explanation */}
      <p
        style={{
          margin: 0,
          fontSize: "0.875rem",
          color: "var(--text-secondary)",
        }}
      >
        {explanationText ?? descriptor.explanationI18nKey}
      </p>

      {/* Element 3: Draft / Data Safety Assurance */}
      <div
        data-testid="recovery-draft-safety-assurance"
        style={{
          fontSize: "0.8125rem",
          color: "var(--text-muted)",
          padding: "var(--space-2) var(--space-3)",
          borderRadius: "var(--radius-sm)",
          backgroundColor: "var(--bg-surface-2)",
        }}
      >
        {draftSafetyText}
      </div>

      {/* Element 4: Primary Safe Next Action & Secondary Controls */}
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          gap: "var(--space-2)",
        }}
      >
        <Button
          variant="primary"
          size="sm"
          onClick={() => onPrimaryAction?.(descriptor.primaryAction)}
        >
          {primaryActionLabel ?? DEFAULT_ACTION_LABELS[descriptor.primaryAction]}
        </Button>
        {secondaryActions}
        <Button
          variant="ghost"
          size="sm"
          aria-expanded={showTechnicalDetails ? "true" : "false"}
          onClick={() => setShowTechnicalDetails((prev) => !prev)}
        >
          {showTechnicalDetails
            ? "Hide technical diagnostics"
            : "Show technical diagnostics"}
        </Button>
      </div>

      {/* Element 5: Expandable RFC 9457 Diagnostic Disclosure & Copyable Correlation ID */}
      {showTechnicalDetails && (
        <div
          data-testid="recovery-technical-diagnostics"
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "var(--space-2)",
            padding: "var(--space-2) var(--space-3)",
            borderRadius: "var(--radius-sm)",
            border: "1px solid var(--border-subtle)",
            backgroundColor: "var(--bg-surface-1)",
            fontFamily: "var(--font-mono)",
            fontSize: "0.75rem",
          }}
        >
          <div>
            <span>code={descriptor.errorCode}</span>
            {typeof descriptor.httpStatus === "number" && (
              <span> · status={descriptor.httpStatus}</span>
            )}
            <span> · retryable={String(descriptor.retryable)}</span>
            <span> · correlationId={descriptor.correlationId}</span>
          </div>
          <Button
            variant="secondary"
            size="sm"
            onClick={handleCopyCorrelationId}
          >
            {copied ? "Copied ID" : "Copy Correlation ID"}
          </Button>
        </div>
      )}
    </section>
  );
}

/**
 * Standardized Genuine Empty / Filter-Empty Card (Blueprint §21.1).
 */
export interface EmptyStateCardProps {
  readonly title: string;
  readonly description: string;
  readonly isFilterEmpty?: boolean;
  readonly primaryActionLabel?: string;
  readonly onPrimaryAction?: () => void;
  readonly onResetFilters?: () => void;
}

export function EmptyStateCard({
  title,
  description,
  isFilterEmpty = false,
  primaryActionLabel,
  onPrimaryAction,
  onResetFilters,
}: EmptyStateCardProps): ReactElement {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        textAlign: "center",
        gap: "var(--space-3)",
        padding: "var(--space-8)",
        borderRadius: "var(--radius-lg)",
        border: "1px dashed var(--border-default)",
        backgroundColor: "var(--bg-surface-1)",
      }}
    >
      <h3 style={{ margin: 0, fontSize: "1rem", fontWeight: 600 }}>{title}</h3>
      <p
        style={{
          margin: 0,
          maxWidth: "440px",
          fontSize: "0.875rem",
          color: "var(--text-secondary)",
        }}
      >
        {description}
      </p>
      <div style={{ display: "flex", gap: "var(--space-2)" }}>
        {isFilterEmpty && onResetFilters && (
          <Button variant="secondary" size="sm" onClick={onResetFilters}>
            Reset filters
          </Button>
        )}
        {primaryActionLabel && onPrimaryAction && (
          <Button variant="primary" size="sm" onClick={onPrimaryAction}>
            {primaryActionLabel}
          </Button>
        )}
      </div>
    </div>
  );
}
