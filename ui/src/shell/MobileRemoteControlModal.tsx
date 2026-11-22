/**
 * Mobile Remote Control QR Pairing & Continuity Modal
 * (1:1 ZCode v3.18.2 `webRemoteControl.*` parity, Blueprint §23.2).
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../components/icons";
import { Badge, Button } from "../components/primitives";

export interface MobileRemoteControlModalProps {
  readonly isOpen: boolean;
  readonly workspaceName: string;
  readonly onClose: () => void;
  readonly onNotify?: (title: string, detail?: string) => void;
}

const QR_MATRIX_PATTERN: readonly string[] = [
  "11111110101001111111",
  "10000010010101000001",
  "10111010110001011101",
  "10111010001101011101",
  "10111010101001011101",
  "10000010110101000001",
  "11111110101011111111",
  "00000000110000000000",
  "10110111001101101010",
  "01001001101010010101",
  "11010110110111011010",
  "00101101001000100101",
  "10110011101101011011",
  "00000000100101001000",
  "11111110011010110101",
  "10000010100101001010",
  "10111010111011101101",
  "10111010001100010010",
  "10000010110010101101",
  "11111110011101010011",
];

function generatePairingCode(): string {
  const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";
  let out = "";
  for (let i = 0; i < 6; i += 1) {
    out += alphabet[Math.floor(Math.random() * alphabet.length)];
  }
  return `${out.slice(0, 3)}-${out.slice(3)}`;
}

export function MobileRemoteControlModal({
  isOpen,
  workspaceName,
  onClose,
  onNotify,
}: MobileRemoteControlModalProps): ReactElement | null {
  const { t } = useTranslation();
  const [pairingCode, setPairingCode] = useState("KYR-842");
  const [sessionActive, setSessionActive] = useState(true);

  if (!isOpen) return null;

  const pairingUrl = `http://127.0.0.1:5173/?remote_pair=${pairingCode}`;

  return (
    <div
      className="ky-modal-backdrop"
      role="presentation"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={t("ui.remoteControl.title", {
          defaultValue: "Mobile remote control",
        })}
        data-testid="mobile-remote-control-modal"
        className="ky-modal-dialog"
        style={{ maxWidth: "460px", display: "flex", flexDirection: "column", gap: "16px" }}
      >
        <header style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div style={{ display: "inline-flex", alignItems: "center", gap: "8px" }}>
            <ChromeIcon name="qr-code" size={18} />
            <h2 style={{ margin: 0, fontSize: "1rem", fontWeight: 600 }}>
              {t("ui.remoteControl.title", { defaultValue: "Mobile remote control" })}
            </h2>
          </div>
          <Button size="sm" variant="ghost" aria-label="Close" onClick={onClose}>
            <ChromeIcon name="close" size={16} />
          </Button>
        </header>

        <p style={{ margin: 0, fontSize: "0.82rem", color: "var(--text-secondary)" }}>
          {t("ui.remoteControl.subtitle", {
            defaultValue:
              "Scan this QR code from your phone on the same trusted network to monitor tasks, inspect diffs, and approve agent actions remotely.",
          })}
        </p>

        <div
          style={{
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            gap: "12px",
            padding: "18px",
            borderRadius: "12px",
            border: "1px solid var(--border-subtle)",
            backgroundColor: "var(--bg-canvas)",
          }}
        >
          {sessionActive ? (
            <>
              <div
                style={{
                  padding: "12px",
                  borderRadius: "10px",
                  backgroundColor: "#ffffff",
                }}
              >
                <svg
                  width="140"
                  height="140"
                  viewBox="0 0 20 20"
                  role="img"
                  aria-label={`Pairing QR code ${pairingCode}`}
                >
                  {QR_MATRIX_PATTERN.map((row, y) =>
                    row.split("").map((cell, x) =>
                      cell === "1" ? (
                        <rect
                          key={`${x}-${y}`}
                          x={x}
                          y={y}
                          width={1}
                          height={1}
                          fill="#111111"
                        />
                      ) : null,
                    ),
                  )}
                </svg>
              </div>
              <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                <Badge tone="success" dot label="Ready to pair" />
                <code
                  style={{
                    fontFamily: "var(--font-mono)",
                    fontSize: "0.9rem",
                    fontWeight: 700,
                    letterSpacing: "0.08em",
                  }}
                >
                  {pairingCode}
                </code>
              </div>
              <span style={{ fontSize: "0.75rem", color: "var(--text-muted)" }}>
                Workspace: {workspaceName}
              </span>
            </>
          ) : (
            <div style={{ textAlign: "center", padding: "20px 0" }}>
              <Badge tone="neutral" label="Session ended" />
              <p style={{ fontSize: "0.8rem", color: "var(--text-muted)", marginTop: "8px" }}>
                Generate a new pairing code to reconnect a mobile device.
              </p>
            </div>
          )}
        </div>

        <footer style={{ display: "flex", justifyContent: "space-between", gap: "8px", flexWrap: "wrap" }}>
          <div style={{ display: "inline-flex", gap: "6px" }}>
            <Button
              size="sm"
              variant="secondary"
              onClick={() => {
                const next = generatePairingCode();
                setPairingCode(next);
                setSessionActive(true);
                onNotify?.("Generated new pairing code", next);
              }}
            >
              <ChromeIcon name="refresh" size={14} />
              <span>New QR</span>
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={!sessionActive}
              onClick={() => {
                if (typeof navigator !== "undefined" && navigator.clipboard) {
                  void navigator.clipboard.writeText(pairingUrl);
                }
                onNotify?.("Pairing link copied", pairingUrl);
              }}
            >
              <ChromeIcon name="copy" size={14} />
              <span>Copy link</span>
            </Button>
          </div>
          {sessionActive && (
            <Button
              size="sm"
              variant="danger"
              onClick={() => {
                setSessionActive(false);
                onNotify?.("Remote pairing session ended", "Revoked active pairing link.");
              }}
            >
              End session
            </Button>
          )}
        </footer>
      </div>
    </div>
  );
}

export default MobileRemoteControlModal;
