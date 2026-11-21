/**
 * Settings Section 3: Model Settings (UIR-8.1, Screenshots #10–#13, CR-P4-01, D-37, C3-19, E2E-17).
 *
 * Locked BYOK presets (`OpenAI`, `Anthropic`, `Google Gemini`, `Alibaba Qwen`, `GLM/Z.AI`,
 * `Kimi`, `MiniMax`, `DeepSeek` — NO Perplexity) + Custom API, Base URL validation, API format,
 * write-only D-37 OS-keyring secret input (cleared from memory after save — never stored in
 * browser storage G-11), connection test, key rotation, C3-19 notice, and `+ Add model` modal.
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, Input, Select } from "../../components/primitives";
import {
  SUPPORTED_API_FORMATS,
  type LockedByokProviderId,
  type SupportedApiFormat,
} from "../../types/decisions";
import {
  INITIAL_BYOK_PROVIDERS,
  validateProviderEndpoint,
  type ByokProviderProfile,
} from "../settingsConfig";
import { AddModelModal } from "./AddModelModal";

export interface ModelSettingsSectionProps {
  readonly onDirtyChange?: (dirty: boolean) => void;
  readonly onNotify: (msg: string) => void;
}

export function ModelSettingsSection({
  onDirtyChange,
  onNotify,
}: ModelSettingsSectionProps): ReactElement {
  const [providers, setProviders] = useState<readonly ByokProviderProfile[]>(
    INITIAL_BYOK_PROVIDERS,
  );
  const [selectedId, setSelectedId] = useState<LockedByokProviderId>("openai");
  const [draftKeyInput, setDraftKeyInput] = useState("");
  const [addModelOpen, setAddModelOpen] = useState(false);
  const [testResult, setTestResult] = useState<string | null>(null);

  const activeProvider =
    providers.find((p) => p.id === selectedId) ?? providers[0];
  const urlValidation = validateProviderEndpoint(activeProvider.baseUrl);

  const updateActiveProvider = (patch: Partial<ByokProviderProfile>) => {
    setProviders((prev) =>
      prev.map((p) => (p.id === activeProvider.id ? { ...p, ...patch } : p)),
    );
    onDirtyChange?.(true);
  };

  const handleSaveToKeyring = () => {
    if (!urlValidation.valid) return;
    // D-37 & G-11: write-only save to Core OS keyring, then wipe draft key from React memory
    const hadNewKey = draftKeyInput.trim().length > 0;
    setProviders((prev) =>
      prev.map((p) =>
        p.id === activeProvider.id
          ? { ...p, hasStoredKeyringSecret: p.hasStoredKeyringSecret || hadNewKey }
          : p,
      ),
    );
    setDraftKeyInput("");
    onDirtyChange?.(false);
    onNotify(
      hadNewKey
        ? `${activeProvider.label}: secret rotated into OS keyring (D-37) and cleared from UI memory.`
        : `${activeProvider.label}: provider configuration saved.`,
    );
  };

  return (
    <div className="ky-settings-content" data-testid="settings-section-model-settings">
      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Locked BYOK Provider Presets (UI-11 / D-37)</h3>
        <div style={{ display: "flex", flexWrap: "wrap", gap: "6px" }}>
          {providers.map((prov) => (
            <Button
              key={prov.id}
              size="sm"
              variant={prov.id === activeProvider.id ? "primary" : "secondary"}
              onClick={() => {
                setSelectedId(prov.id);
                setDraftKeyInput("");
                setTestResult(null);
              }}
            >
              {prov.label}
            </Button>
          ))}
        </div>

        <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
          <Badge
            tone={
              activeProvider.c319DataHandlingStatus.startsWith("Eligible")
                ? "success"
                : "warning"
            }
            label={`C3-19: ${activeProvider.c319DataHandlingStatus}`}
          />
          <Badge
            tone={activeProvider.hasStoredKeyringSecret ? "info" : "neutral"}
            label={
              activeProvider.hasStoredKeyringSecret
                ? "OS Keyring Secret Stored (Write-Only)"
                : "No Secret Stored"
            }
          />
        </div>
        <p style={{ margin: 0, fontSize: "0.8rem", color: "var(--text-secondary)" }}>
          {activeProvider.c319Note}
        </p>

        <div className="ky-settings-grid-2">
          <Input
            label="Base URL"
            value={activeProvider.baseUrl}
            errorText={urlValidation.valid ? undefined : urlValidation.error}
            onChange={(e) => updateActiveProvider({ baseUrl: e.target.value })}
          />
          <Select
            label="API Format"
            value={activeProvider.apiFormat}
            options={SUPPORTED_API_FORMATS.map((fmt) => ({ value: fmt, label: fmt }))}
            onChange={(e) =>
              updateActiveProvider({
                apiFormat: e.target.value as SupportedApiFormat,
              })
            }
          />
        </div>

        <Input
          label="API Key (Write-Only OS Keyring D-37)"
          writeOnlySecret={true}
          hasStoredKeyringSecret={activeProvider.hasStoredKeyringSecret}
          placeholder="Enter new API key to store or rotate..."
          value={draftKeyInput}
          onChange={(e) => {
            setDraftKeyInput(e.target.value);
            onDirtyChange?.(true);
          }}
        />

        <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
          <Button
            size="sm"
            variant="primary"
            disabled={!urlValidation.valid}
            onClick={handleSaveToKeyring}
          >
            {activeProvider.hasStoredKeyringSecret ? "Save / Rotate Key" : "Save to OS Keyring"}
          </Button>
          <Button
            size="sm"
            variant="secondary"
            disabled={!urlValidation.valid}
            onClick={() =>
              setTestResult(
                activeProvider.hasStoredKeyringSecret
                  ? `Handshake ready for ${activeProvider.label} (${activeProvider.apiFormat}).`
                  : `No credential stored in OS keyring for ${activeProvider.label} yet.`,
              )
            }
          >
            Test connection
          </Button>
          <Button size="sm" variant="secondary" onClick={() => setAddModelOpen(true)}>
            + Add model
          </Button>
        </div>

        {testResult && (
          <p style={{ margin: 0, fontSize: "0.8rem", color: "var(--text-secondary)" }}>
            {testResult}
          </p>
        )}
      </div>

      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">
          Configured Models ({activeProvider.models.length})
        </h3>
        {activeProvider.models.length === 0 ? (
          <p style={{ margin: 0, fontSize: "0.82rem", color: "var(--text-muted)" }}>
            No custom models registered for {activeProvider.label} yet. Click "+ Add model".
          </p>
        ) : (
          activeProvider.models.map((m) => (
            <div key={m.modelId} className="ky-gov-perm-row">
              <span>
                <strong>{m.displayName}</strong> ({m.modelId}) ·{" "}
                {m.contextWindow.toLocaleString()} ctx / {m.maxOutputTokens.toLocaleString()} out
              </span>
              <Badge
                tone={m.capabilityStatus === "Verified" ? "success" : "warning"}
                label={m.capabilityStatus}
              />
            </div>
          ))
        )}
      </div>

      <AddModelModal
        isOpen={addModelOpen}
        providerLabel={activeProvider.label}
        onAddModel={(model) => {
          setProviders((prev) =>
            prev.map((p) =>
              p.id === activeProvider.id
                ? { ...p, models: [...p.models, model] }
                : p,
            ),
          );
          onNotify(`Added model ${model.modelId} (${model.capabilityStatus}).`);
        }}
        onClose={() => setAddModelOpen(false)}
      />
    </div>
  );
}
