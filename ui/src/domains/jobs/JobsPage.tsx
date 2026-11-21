/**
 * Durable `Jobs` (`AIJobRelay`) 10,000+ Virtualized Operational Page
 * (UIR-4.3, Blueprint §12.1, §22.1, G-13, E2E-13).
 */

import { useMemo, useState, type ReactElement } from "react";
import { DigitFlipCounter, EmptyStateCard } from "../../components/feedback";
import { Badge, Button, Input, Select } from "../../components/primitives";
import {
  computeVirtualWindow,
  filterDurableJobs,
  type DurableJobRecord,
  type DurableJobStatus,
} from "../../workspace/workspaceConfig";
import { JobDetailDrawer } from "./JobDetailDrawer";

export interface JobsPageProps {
  readonly jobs: readonly DurableJobRecord[];
  readonly onCancelJob: (jobId: string) => void;
  readonly onRetryOrResumeJob: (jobId: string) => void;
  readonly onReconcileS4Job: (jobId: string) => void;
}

const ROW_HEIGHT_PX = 44;
const VIEWPORT_HEIGHT_PX = 420;

export function JobsPage({
  jobs,
  onCancelJob,
  onRetryOrResumeJob,
  onReconcileS4Job,
}: JobsPageProps): ReactElement {
  const [searchQuery, setSearchQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | DurableJobStatus>("all");
  const [typeFilter, setTypeFilter] = useState<"all" | DurableJobRecord["jobType"]>("all");
  const [scrollTopPx, setScrollTopPx] = useState(0);
  const [selectedJobId, setSelectedJobId] = useState<string | null>(
    jobs[0]?.jobId ?? null,
  );

  const filteredJobs = useMemo(
    () => filterDurableJobs(jobs, searchQuery, statusFilter, typeFilter),
    [jobs, searchQuery, statusFilter, typeFilter],
  );

  const windowSlice = computeVirtualWindow(
    filteredJobs.length,
    scrollTopPx,
    ROW_HEIGHT_PX,
    VIEWPORT_HEIGHT_PX,
  );

  const visibleRows = filteredJobs.slice(
    windowSlice.startIndex,
    windowSlice.endIndex,
  );
  const selectedJob =
    jobs.find((j) => j.jobId === selectedJobId) ?? null;

  return (
    <div
      data-testid="durable-jobs-page"
      style={{
        maxWidth: "1100px",
        margin: "0 auto",
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-4)",
      }}
    >
      <header
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "var(--space-3)",
        }}
      >
        <div>
          <h1 style={{ margin: 0, fontSize: "1.25rem" }}>
            Durable Jobs (AIJobRelay)
          </h1>
          <p style={{ margin: "2px 0 0 0", fontSize: "0.8125rem", color: "var(--text-secondary)" }}>
            Virtualized operational view supporting 10,000+ durable Core jobs,
            checkpoints, human approvals, and S4 reconciliation.
          </p>
        </div>
        <Badge
          tone="info"
          label={`${filteredJobs.length.toLocaleString("en-US")} matching`}
        />
      </header>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "1fr 180px 180px auto",
          gap: "var(--space-2)",
          alignItems: "end",
        }}
      >
        <Input
          label="Search Jobs"
          placeholder="Search by job ID, title, project, or correlation ID..."
          value={searchQuery}
          onChange={(e) => {
            setSearchQuery(e.target.value);
            setScrollTopPx(0);
          }}
        />
        <Select
          label="Status"
          value={statusFilter}
          onChange={(e) => {
            setStatusFilter(e.target.value as "all" | DurableJobStatus);
            setScrollTopPx(0);
          }}
          options={[
            { value: "all", label: "All statuses" },
            { value: "running", label: "Running" },
            { value: "queued", label: "Queued" },
            { value: "waiting_approval", label: "Waiting approval" },
            { value: "succeeded", label: "Succeeded" },
            { value: "failed", label: "Failed" },
            { value: "cancelled", label: "Cancelled" },
            { value: "reconciling_s4", label: "Reconciling (S4)" },
          ]}
        />
        <Select
          label="Job Type"
          value={typeFilter}
          onChange={(e) => {
            setTypeFilter(
              e.target.value as "all" | DurableJobRecord["jobType"],
            );
            setScrollTopPx(0);
          }}
          options={[
            { value: "all", label: "All types" },
            { value: "agent-task", label: "Agent task" },
            { value: "workflow-run", label: "Workflow run" },
            { value: "knowledge-ingest", label: "Knowledge ingest" },
            { value: "automation", label: "Automation" },
          ]}
        />
        <Button
          variant="secondary"
          size="md"
          onClick={() => {
            setSearchQuery("");
            setStatusFilter("all");
            setTypeFilter("all");
            setScrollTopPx(0);
          }}
        >
          Reset filters
        </Button>
      </div>

      {filteredJobs.length === 0 ? (
        <EmptyStateCard
          title="No durable jobs match your filters"
          description="Try clearing search text or resetting the status and type filters."
          isFilterEmpty
          onResetFilters={() => {
            setSearchQuery("");
            setStatusFilter("all");
            setTypeFilter("all");
          }}
        />
      ) : (
        <div
          className="ky-virtual-table-viewport"
          onScroll={(e) => setScrollTopPx(e.currentTarget.scrollTop)}
        >
          <div
            style={{
              height: `${windowSlice.totalHeightPx}px`,
              position: "relative",
            }}
          >
            <div
              style={{
                position: "absolute",
                top: `${windowSlice.offsetTopPx}px`,
                left: 0,
                right: 0,
              }}
            >
              {visibleRows.map((job) => (
                <div
                  key={job.jobId}
                  className="ky-job-row"
                  role="button"
                  tabIndex={0}
                  onClick={() => setSelectedJobId(job.jobId)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      setSelectedJobId(job.jobId);
                    }
                  }}
                  style={{
                    cursor: "pointer",
                    backgroundColor:
                      job.jobId === selectedJobId
                        ? "var(--bg-surface-2)"
                        : undefined,
                  }}
                >
                  <code style={{ color: "var(--text-muted)" }}>{job.jobId}</code>
                  <span
                    style={{
                      fontWeight: 500,
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {job.title}
                  </span>
                  <Badge
                    tone={
                      job.status === "succeeded"
                        ? "success"
                        : job.status === "failed"
                          ? "danger"
                          : job.status === "reconciling_s4" ||
                              job.status === "waiting_approval"
                            ? "warning"
                            : "info"
                    }
                    label={job.status}
                  />
                  <span style={{ color: "var(--text-secondary)" }}>
                    {job.jobType}
                  </span>
                  <span style={{ color: "var(--text-muted)" }}>
                    Attempt <DigitFlipCounter value={job.attemptCount} />
                  </span>
                  <span style={{ color: "var(--text-muted)" }}>{job.updatedAt}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      <JobDetailDrawer
        job={selectedJob}
        onClose={() => setSelectedJobId(null)}
        onCancelJob={onCancelJob}
        onRetryOrResumeJob={onRetryOrResumeJob}
        onReconcileS4Job={onReconcileS4Job}
      />
    </div>
  );
}
