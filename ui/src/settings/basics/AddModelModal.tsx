/**
 * `+ Add model` Modal for Settings → Model Settings (UIR-8.1, Screenshot #13, E2E-17).
 *
 * Implements Smart configuration, Model ID, Context window, Max output tokens, Input types,
 * Model capabilities (`Verified` vs `Not verified`), and Reasoning levels.
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, Input, ModalDialog, Select, Toggle } from "../../components/primitives";
import type { CustomModelSpec } from "../settingsConfig";

export interface AddModelModalProps {
  readonly isOpen: boolean;
  readonly providerLabel: string;
  readonly onAddModel: (model: CustomModelSpec) => void;
  readonly onClose: () => void;
}

export function AddModelModal({
  isOpen,
  providerLabel,
  onAddModel,
  onClose,
}: AddModelModalProps): ReactElement | null {
  const [smartConfig, setSmartConfig] = useState(true);
  const [modelId, setModelId] = useState("");
  const [contextWindow, setContextWindow] = useState("128000");
  const [maxOutputTokens, setMaxOutputTokens] = useState("16384");
  const [supportsImage, setSupportsImage] = useState(true);
  const [supportsPdf, setSupportsPdf] = useState(false);
  const [capabilityStatus, setCapabilityStatus] = useState<"Verified" | "Not verified">("Not verified");

  if (!isOpen) return null;

  return (
    <ModalDialog
      isOpen={isOpen}
      title={`Add model — ${providerLabel}`}
      description="Declare model limits and capabilities honestly (`Not verified` until live conformance succeeds)."
      onClose={onClose}
    >
      <div className="ky-modal-form" data-testid="add-model-modal">
        <Toggle
          label="Smart configuration"
          description="Infer defaults from Core capability registry when available"
          checked={smartConfig}
          onChange={setSmartConfig}
        />
        <Input
          label="Model ID"
          placeholder="e.g. gpt-5-mini"
          value={modelId}
          onChange={(e) => setModelId(e.target.value)}
        />
        <div className="ky-modal-form__row">
          <Input
            label="Context window"
            type="number"
            value={contextWindow}
            onChange={(e) => setContextWindow(e.target.value)}
          />
          <Input
            label="Max output tokens"
            type="number"
            value={maxOutputTokens}
            onChange={(e) => setMaxOutputTokens(e.target.value)}
          />
        </div>
        <div style={{ display: "flex", gap: "var(--space-3)", flexWrap: "wrap" }}>
          <Toggle label="Image input" checked={supportsImage} onChange={setSupportsImage} />
          <Toggle label="PDF input" checked={supportsPdf} onChange={setSupportsPdf} />
        </div>
        <Select
          label="Model capabilities status"
          value={capabilityStatus}
          options={[
            { value: "Not verified", label: "Not verified (default honest state)" },
            { value: "Verified", label: "Verified via conformance test" },
          ]}
          onChange={(e) =>
            setCapabilityStatus(e.target.value as "Verified" | "Not verified")
          }
        />
        <div style={{ display: "flex", gap: "6px" }}>
          <Badge tone="neutral" label="Reasoning levels: low · medium · high" />
        </div>
        <div className="ky-modal-form__footer">
          <span />
          <div style={{ display: "flex", gap: "var(--space-2)" }}>
            <Button size="sm" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button
              size="sm"
              variant="primary"
              disabled={!modelId.trim()}
              onClick={() => {
                const inputs: ("text" | "image" | "pdf")[] = ["text"];
                if (supportsImage) inputs.push("image");
                if (supportsPdf) inputs.push("pdf");
                onAddModel({
                  modelId: modelId.trim(),
                  displayName: modelId.trim(),
                  contextWindow: Number(contextWindow) || 128_000,
                  maxOutputTokens: Number(maxOutputTokens) || 16_384,
                  inputTypes: inputs,
                  capabilityStatus,
                  reasoningLevels: ["low", "medium", "high"],
                });
                onClose();
              }}
            >
              Add model
            </Button>
          </div>
        </div>
      </div>
    </ModalDialog>
  );
}
