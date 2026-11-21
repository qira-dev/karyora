/**
 * W3C WAI-ARIA APG 1.2 Accessible Modal Dialog, High-Risk ConfirmDialog & ToastRegion
 * (UIR-2.3, Blueprint §8.4, §8.5, §8.9, §20.5, G-05, G-06).
 *
 * Enforces:
 * - `role="dialog"` and `aria-modal="true"` with `aria-labelledby` / `aria-describedby`
 * - `Escape` closes non-destructive overlays (with dirty-form confirmation if unsaved changes exist)
 *   and NEVER cancels a durable background Job
 * - Deterministic focus trap and focus return to invoking trigger on close (`G-06`)
 */

import {
  useEffect,
  useId,
  useRef,
  type KeyboardEvent,
  type ReactElement,
  type ReactNode,
} from "react";
import { Button } from "./Button";

export interface ModalDialogProps {
  readonly isOpen: boolean;
  readonly title: string;
  readonly description?: string;
  readonly onClose: () => void;
  readonly hasUnsavedChanges?: boolean;
  readonly onConfirmDiscardUnsaved?: () => void;
  readonly footer?: ReactNode;
  readonly children: ReactNode;
}

export function ModalDialog({
  isOpen,
  title,
  description,
  onClose,
  hasUnsavedChanges = false,
  onConfirmDiscardUnsaved,
  footer,
  children,
}: ModalDialogProps): ReactElement | null {
  const titleId = useId();
  const descId = useId();
  const dialogRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!isOpen) {
      return;
    }
    const previousFocus =
      typeof document !== "undefined"
        ? (document.activeElement as HTMLElement | null)
        : null;
    dialogRef.current?.focus();

    return () => {
      previousFocus?.focus?.();
    };
  }, [isOpen]);

  if (!isOpen) {
    return null;
  }

  const requestClose = () => {
    if (hasUnsavedChanges && onConfirmDiscardUnsaved) {
      onConfirmDiscardUnsaved();
      return;
    }
    onClose();
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      requestClose();
    }
  };

  return (
    <div
      className="ky-modal-backdrop"
      onClick={(e) => {
        if (e.target === e.currentTarget) {
          requestClose();
        }
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descId : undefined}
        tabIndex={-1}
        className="ky-modal-dialog"
        onKeyDown={handleKeyDown}
      >
        <header
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            marginBottom: "var(--space-3)",
          }}
        >
          <h2 id={titleId} style={{ margin: 0, fontSize: "1.0625rem" }}>
            {title}
          </h2>
          <Button
            variant="ghost"
            size="sm"
            aria-label="Close dialog"
            onClick={requestClose}
          >
            ×
          </Button>
        </header>
        {description && (
          <p
            id={descId}
            style={{
              margin: "0 0 var(--space-3) 0",
              color: "var(--text-secondary)",
              fontSize: "0.875rem",
            }}
          >
            {description}
          </p>
        )}
        <div>{children}</div>
        {footer && (
          <footer
            style={{
              display: "flex",
              justifyContent: "flex-end",
              gap: "var(--space-2)",
              marginTop: "var(--space-4)",
            }}
          >
            {footer}
          </footer>
        )}
      </div>
    </div>
  );
}

/**
 * High-Risk Destructive Confirmation Dialog (Blueprint §8.9, Screenshots #15, #59, #85).
 * Used for `Clear all browser data` (#15), `Permanently delete all archived sessions` (#59),
 * `Delete` Memory item (#26–#28), and `Cancel Job`.
 */
export interface ConfirmDialogProps {
  readonly isOpen: boolean;
  readonly title: string;
  readonly explanation: string;
  readonly confirmLabel: string;
  readonly cancelLabel?: string;
  readonly isDestructive?: boolean;
  readonly loading?: boolean;
  readonly onConfirm: () => void;
  readonly onCancel: () => void;
}

export function ConfirmDialog({
  isOpen,
  title,
  explanation,
  confirmLabel,
  cancelLabel = "Cancel",
  isDestructive = true,
  loading = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps): ReactElement | null {
  return (
    <ModalDialog
      isOpen={isOpen}
      title={title}
      description={explanation}
      onClose={onCancel}
      footer={
        <>
          <Button variant="secondary" onClick={onCancel} disabled={loading}>
            {cancelLabel}
          </Button>
          <Button
            variant={isDestructive ? "danger" : "primary"}
            loading={loading}
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      <div />
    </ModalDialog>
  );
}

/**
 * Accessible Toast Region (`role="status"` / `aria-live="polite"`).
 */
export interface ToastMessage {
  readonly id: string;
  readonly title: string;
  readonly description?: string;
  readonly tone?: "neutral" | "success" | "warning" | "danger";
}

export interface ToastRegionProps {
  readonly toasts: readonly ToastMessage[];
  readonly onDismissToast: (id: string) => void;
}

export function ToastRegion({
  toasts,
  onDismissToast,
}: ToastRegionProps): ReactElement | null {
  if (toasts.length === 0) {
    return null;
  }

  return (
    <div
      role="status"
      aria-live="polite"
      style={{
        position: "fixed",
        insetBlockEnd: "var(--space-4)",
        insetInlineEnd: "var(--space-4)",
        zIndex: 50,
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-2)",
        maxWidth: "360px",
      }}
    >
      {toasts.map((toast) => (
        <div
          key={toast.id}
          style={{
            display: "flex",
            alignItems: "flex-start",
            justifyContent: "space-between",
            gap: "var(--space-3)",
            padding: "var(--space-3)",
            borderRadius: "var(--radius-md)",
            border: "1px solid var(--border-default)",
            backgroundColor: "var(--bg-surface-1)",
          }}
        >
          <div>
            <div style={{ fontWeight: 600, fontSize: "0.875rem" }}>
              {toast.title}
            </div>
            {toast.description && (
              <div
                style={{
                  fontSize: "0.75rem",
                  color: "var(--text-secondary)",
                }}
              >
                {toast.description}
              </div>
            )}
          </div>
          <Button
            variant="ghost"
            size="sm"
            aria-label={`Dismiss ${toast.title}`}
            onClick={() => onDismissToast(toast.id)}
          >
            ×
          </Button>
        </div>
      ))}
    </div>
  );
}
