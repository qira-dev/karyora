/**
 * Durable Job Execution Receipt Modal (UIR-4.3, Blueprint §12.1, E2E-13).
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, ModalDialog } from "../../components/primitives";
import type { DurableJobRecord } from "../../workspace/workspaceConfig";

export interface JobReceiptModalProps {
  readonly job: DurableJobRecord | null;
  readonly onClose: () => void;
}

export function JobReceiptModal({
  job,
  onClose,
}: JobReceiptModalProps): ReactElement | null {
  const [copied, setCopied] = useState(false);

  if (!job) {
    return null;
  }

  const receiptJson = JSON.stringify(
    {
      job_id: job.jobId,
      title: job.title,
      status: job.status,
      attempts: job.attemptCount,
      checkpoint: job.checkpointLabel,
      idempotency_key: job.idempotencyKey,
      correlation_id: job.correlationId,
      receipt_digest: job.receiptDigest ?? "pending-terminal-state",
      updated_at: job.updatedAt,
    },
    null,
    2,
  );

  return (
    <ModalDialog
      isOpen
      title={`Execution Receipt — ${job.jobId}`}
      description="Authoritative Core AIJobRelay receipt summary."
      onClose={onClose}
      footer={
        <>
          <Button
            variant="secondary"
            onClick={() => {
              if (
                typeof navigator !== "undefined" &&
                typeof navigator.clipboard?.writeText === "function"
              ) {
                void navigator.clipboard.writeText(receiptJson);
              }
              setCopied(true);
            }}
          >
            {copied ? "Copied JSON" : "Copy Receipt JSON"}
          </Button>
          <Button variant="primary" onClick={onClose}>
            Close
          </Button>
        </>
      }
    >
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
        <div style={{ display: "flex", gap: "var(--space-2)" }}>
          <Badge tone="info" label={`Status: ${job.status}`} />
          <Badge tone="neutral" label={`Attempts: ${job.attemptCount}`} />
        </div>
        <pre
          data-testid="job-receipt-json"
          style={{
            margin: 0,
            padding: "var(--space-3)",
            borderRadius: "var(--radius-sm)",
            backgroundColor: "var(--code-bg)",
            color: "var(--code-fg)",
            fontFamily: "var(--font-mono)",
            fontSize: "0.75rem",
            overflowX: "auto",
          }}
        >
          {receiptJson}
        </pre>
      </div>
    </ModalDialog>
  );
}
