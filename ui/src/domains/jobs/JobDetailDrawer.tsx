/**
 * Durable Job Detail Drawer (6 Canonical Tabs: Overview, Attempts, Checkpoints, Live Events,
 * Human Approvals, Result & Receipt + S4 Reconciliation) (UIR-4.3, Blueprint §12.1, E2E-13).
 */

import { useState, type ReactElement } from "react";
import { RecoveryPanel } from "../../components/feedback";
import { Badge, Button, ConfirmDialog, Tabs } from "../../components/primitives";
import { toUnknownOutcomeS4RecoveryDescriptor } from "../../coreclient";
import type { DurableJobRecord } from "../../workspace/workspaceConfig";
import { JobReceiptModal } from "./JobReceiptModal";

export interface JobDetailDrawerProps {
  readonly job: DurableJobRecord | null;
  readonly onClose: () => void;
  readonly onCancelJob: (jobId: string) => void;
  readonly onRetryOrResumeJob: (jobId: string) => void;
  readonly onReconcileS4Job: (jobId: string) => void;
}

export function JobDetailDrawer({
  job,
  onClose,
  onCancelJob,
  onRetryOrResumeJob,
  onReconcileS4Job,
}: JobDetailDrawerProps): ReactElement | null {
  const [activeTab, setActiveTab] = useState("overview");
  const [confirmCancel, setConfirmCancel] = useState(false);
  const [receiptOpen, setReceiptOpen] = useState(false);

  if (!job) {
    return null;
  }

  const isTerminal =
    job.status === "succeeded" ||
    job.status === "failed" ||
    job.status === "cancelled";

  return (
    <section
      aria-label={`Durable Job Detail ${job.jobId}`}
      data-testid={`job-detail-drawer-${job.jobId}`}
      style={{
        padding: "var(--space-4)",
        borderRadius: "var(--radius-md)",
        border: "1px solid var(--border-default)",
        backgroundColor: "var(--bg-surface-1)",
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-3)",
      }}
    >
      <header
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "var(--space-2)",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
          <h3 style={{ margin: 0, fontSize: "1rem" }}>{job.title}</h3>
          <code style={{ fontSize: "0.75rem", color: "var(--text-muted)" }}>
            {job.jobId}
          </code>
          <Badge
            tone={
              job.status === "succeeded"
                ? "success"
                : job.status === "failed"
                  ? "danger"
                  : job.status === "reconciling_s4" || job.status === "waiting_approval"
                    ? "warning"
                    : "info"
            }
            label={job.status}
          />
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-2)" }}>
          {!isTerminal && (
            <Button
              variant="danger"
              size="sm"
              onClick={() => setConfirmCancel(true)}
            >
              Safe cancel
            </Button>
          )}
          {(job.status === "failed" || job.status === "cancelled") && (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => onRetryOrResumeJob(job.jobId)}
            >
              Retry / Resume
            </Button>
          )}
          <Button
            variant="secondary"
            size="sm"
            onClick={() => setReceiptOpen(true)}
          >
            Receipt
          </Button>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close
          </Button>
        </div>
      </header>

      {job.status === "reconciling_s4" && (
        <RecoveryPanel
          descriptor={toUnknownOutcomeS4RecoveryDescriptor(job.correlationId)}
          titleText="Reconciling Job state (S4)"
          explanationText="The last mutation response for this durable Job was lost in transit. Querying authoritative Core state before permitting retry."
          primaryActionLabel="Reconcile with Core now"
          onPrimaryAction={() => onReconcileS4Job(job.jobId)}
        />
      )}

      <Tabs
        ariaLabel="Job inspection sections"
        activeTabId={activeTab}
        onSelectTab={setActiveTab}
        tabs={[
          { id: "overview", label: "Overview" },
          { id: "attempts", label: "Attempts", badgeCount: job.attemptCount },
          { id: "checkpoints", label: "Checkpoints" },
          { id: "events", label: "Live Events", badgeCount: job.events.length },
          { id: "approvals", label: "Human Approvals" },
          { id: "receipt", label: "Result & Receipt" },
        ]}
      >
        <div style={{ paddingTop: "var(--space-3)", fontSize: "0.875rem" }}>
          {activeTab === "overview" && (
            <dl
              style={{
                display: "grid",
                gridTemplateColumns: "160px 1fr",
                gap: "var(--space-2)",
                margin: 0,
              }}
            >
              <dt style={{ color: "var(--text-muted)" }}>Project</dt>
              <dd style={{ margin: 0 }}>{job.projectName}</dd>
              <dt style={{ color: "var(--text-muted)" }}>Type</dt>
              <dd style={{ margin: 0 }}>{job.jobType}</dd>
              <dt style={{ color: "var(--text-muted)" }}>Idempotency Key</dt>
              <dd style={{ margin: 0, fontFamily: "var(--font-mono)" }}>
                {job.idempotencyKey}
              </dd>
              <dt style={{ color: "var(--text-muted)" }}>Correlation ID</dt>
              <dd style={{ margin: 0, fontFamily: "var(--font-mono)" }}>
                {job.correlationId}
              </dd>
            </dl>
          )}
          {activeTab === "attempts" && (
            <div>
              Recorded execution attempts: <strong>{job.attemptCount}</strong> (each
              retry preserves prior attempt audit logs and leases).
            </div>
          )}
          {activeTab === "checkpoints" && (
            <div>
              Latest durable checkpoint: <code>{job.checkpointLabel}</code>
            </div>
          )}
          {activeTab === "events" && (
            <ul style={{ margin: 0, paddingInlineStart: "var(--space-4)" }}>
              {job.events.map((ev) => (
                <li key={ev.seq} style={{ marginBottom: "4px" }}>
                  <code>#{ev.seq}</code> [{ev.kind}] {ev.summary}{" "}
                  <span style={{ color: "var(--text-muted)" }}>({ev.timestamp})</span>
                </li>
              ))}
            </ul>
          )}
          {activeTab === "approvals" && (
            <div>
              {job.status === "waiting_approval"
                ? "Awaiting explicit human approval decision recorded in Activity Audit."
                : "No pending approval gates for this Job."}
            </div>
          )}
          {activeTab === "receipt" && (
            <div>
              Receipt digest:{" "}
              <code>{job.receiptDigest ?? "Pending terminal Core commit"}</code>
            </div>
          )}
        </div>
      </Tabs>

      <ConfirmDialog
        isOpen={confirmCancel}
        title={`Cancel durable Job ${job.jobId}?`}
        explanation="Sends a safe cancellation signal to Core AIJobRelay. The UI transitions to Cancelled only after Core emits the terminal cancellation event."
        confirmLabel="Confirm safe cancel"
        isDestructive
        onConfirm={() => {
          setConfirmCancel(false);
          onCancelJob(job.jobId);
        }}
        onCancel={() => setConfirmCancel(false)}
      />

      <JobReceiptModal
        job={receiptOpen ? job : null}
        onClose={() => setReceiptOpen(false)}
      />
    </section>
  );
}
