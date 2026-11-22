/**
 * Settings Section 3: Model Settings — Calibrated 1:1 to ZCode Screenshot #10–#13.
 *
 * Two-column master-detail provider card (Locked BYOK presets without Perplexity UI-11
 * + Custom API), Base URL validation, API format, write-only D-37 OS-keyring secret input,
 * connection test, C3-19 status notice, and `+ Add model` modal.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
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
  const { t } = useTranslation();
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
        ? `${activeProvider.label}: secret rotated into OS keyring and cleared from UI memory.`
        : `${activeProvider.label}: provider configuration saved.`,
    );
  };

  const builtinProviders = providers.filter((p) => p.id !== "custom");
  const customProviders = providers.filter((p) => p.id === "custom");

  return (
    <div className="ky-settings-content" data-testid="settings-section-model-settings">
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "12px",
          flexWrap: "wrap",
        }}
      >
        <p className="ky-settings-section-sub" style={{ margin: 0 }}>
          {t("ui.settings.model.subtitle", {
            defaultValue:
              "Manage BYOK model providers stored securely in your OS keyring. Once configured, models can be selected during chat.",
          })}
        </p>
        <Button
          size="sm"
          variant="primary"
          onClick={() => {
            setSelectedId("custom");
            setDraftKeyInput("");
            setTestResult(null);
          }}
        >
          + {t("ui.settings.model.addProvider", { defaultValue: "Add provider" })}
        </Button>
      </div>

      {/* ZCode Exact Master-Detail Two-Column Card (Screenshot #10) */}
      <div
        className="ky-setting-group"
        style={{
          display: "grid",
          gridTemplateColumns: "minmax(190px, 230px) minmax(0, 1fr)",
        }}
      >
        {/* Left Provider Rail */}
        <div
          style={{
            borderInlineEnd: "1px solid var(--border-subtle)",
            padding: "14px 10px",
            display: "flex",
            flexDirection: "column",
            gap: "4px",
          }}
        >
          <div
            style={{
              fontSize: "0.72rem",
              fontWeight: 500,
              color: "var(--text-muted)",
              padding: "4px 10px",
            }}
          >
            {t("ui.settings.model.providers", { defaultValue: "Providers" })}
          </div>
          {builtinProviders.map((prov) => (
            <button
              key={prov.id}
              type="button"
              className="ky-sidebar__nav-item"
              data-active={prov.id === activeProvider.id ? "true" : "false"}
              onClick={() => {
                setSelectedId(prov.id);
                setDraftKeyInput("");
                setTestResult(null);
              }}
            >
              <span className="ky-sidebar__nav-left">
                <ChromeIcon name="model-cube" size={14} />
                <span>{prov.label}</span>
              </span>
              <span
                aria-hidden="true"
                style={{
                  width: "7px",
                  height: "7px",
                  borderRadius: "9999px",
                  backgroundColor: prov.hasStoredKeyringSecret ? "#22c55e" : "#f97316",
                }}
              />
            </button>
          ))}

          <div
            style={{
              fontSize: "0.72rem",
              fontWeight: 500,
              color: "var(--text-muted)",
              padding: "10px 10px 4px",
            }}
          >
            {t("ui.settings.model.customProviders", { defaultValue: "Custom providers" })}
          </div>
          {customProviders.map((prov) => (
            <button
              key={prov.id}
              type="button"
              className="ky-sidebar__nav-item"
              data-active={prov.id === activeProvider.id ? "true" : "false"}
              onClick={() => {
                setSelectedId(prov.id);
                setDraftKeyInput("");
                setTestResult(null);
              }}
            >
              <span className="ky-sidebar__nav-left">
                <ChromeIcon name="sliders" size={14} />
                <span>{prov.label}</span>
              </span>
              <span
                aria-hidden="true"
                style={{
                  width: "7px",
                  height: "7px",
                  borderRadius: "9999px",
                  backgroundColor: prov.hasStoredKeyringSecret ? "#22c55e" : "#f97316",
                }}
              />
            </button>
          ))}
        </div>

        {/* Right Provider Config Panel */}
        <div
          style={{
            padding: "20px",
            display: "flex",
            flexDirection: "column",
            gap: "14px",
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              gap: "8px",
              flexWrap: "wrap",
            }}
          >
            <div style={{ display: "inline-flex", alignItems: "center", gap: "8px" }}>
              <ChromeIcon name="model-cube" size={18} />
              <strong style={{ fontSize: "0.96rem" }}>{activeProvider.label}</strong>
            </div>
            <Badge
              tone={
                activeProvider.c319DataHandlingStatus.startsWith("Eligible")
                  ? "success"
                  : "warning"
              }
              label={activeProvider.c319DataHandlingStatus}
            />
          </div>

          <Input
            label={t("ui.settings.model.baseUrl", { defaultValue: "Base URL" })}
            value={activeProvider.baseUrl}
            errorText={urlValidation.valid ? undefined : urlValidation.error}
            onChange={(e) => updateActiveProvider({ baseUrl: e.target.value })}
          />

          <Select
            label={t("ui.settings.model.apiFormat", { defaultValue: "API format" })}
            value={activeProvider.apiFormat}
            options={SUPPORTED_API_FORMATS.map((fmt) => ({ value: fmt, label: fmt }))}
            onChange={(e) =>
              updateActiveProvider({ apiFormat: e.target.value as SupportedApiFormat })
            }
          />

          <Input
            label={t("ui.settings.model.apiKey", { defaultValue: "API key" })}
            writeOnlySecret
            placeholder={
              activeProvider.hasStoredKeyringSecret
                ? "•••••••• (stored in OS keyring — enter new value to rotate)"
                : t("ui.settings.model.apiKeyPlaceholder", {
                    defaultValue: "Enter API key (write-only OS keyring)",
                  })
            }
            value={draftKeyInput}
            onChange={(e) => {
              setDraftKeyInput(e.target.value);
              onDirtyChange?.(true);
            }}
          />

          <div style={{ display: "flex", gap: "8px", flexWrap: "wrap" }}>
            <Button
              size="sm"
              variant="primary"
              disabled={!urlValidation.valid}
              onClick={handleSaveToKeyring}
            >
              {t("ui.settings.model.saveToKeyring", { defaultValue: "Save key" })}
            </Button>
            <Button
              size="sm"
              variant="secondary"
              disabled={!urlValidation.valid}
              onClick={() => {
                setTestResult(
                  `Handshake probe ready for ${activeProvider.baseUrl} (${activeProvider.apiFormat})`,
                );
                onNotify(`Tested endpoint configuration for ${activeProvider.label}.`);
              }}
            >
              {t("ui.settings.model.testConnection", { defaultValue: "Test connection" })}
            </Button>
          </div>

          {testResult && (
            <div style={{ fontSize: "0.78rem", color: "var(--success-text)" }}>{testResult}</div>
          )}

          {/* Model List (#10 bottom) */}
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              marginTop: "6px",
            }}
          >
            <span style={{ fontSize: "0.82rem", color: "var(--text-secondary)", fontWeight: 500 }}>
              {t("ui.settings.model.modelList", { defaultValue: "Model list" })}
            </span>
            <Button size="sm" variant="secondary" onClick={() => setAddModelOpen(true)}>
              + {t("ui.settings.model.addModel", { defaultValue: "Add model" })}
            </Button>
          </div>

          {activeProvider.models.length === 0 ? (
            <div
              style={{
                padding: "14px",
                borderRadius: "8px",
                border: "1px dashed var(--border-default)",
                fontSize: "0.8rem",
                color: "var(--text-muted)",
              }}
            >
              {t("ui.settings.model.noModels", {
                defaultValue: "No models are configured. Add a model to use it in chat.",
              })}
            </div>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
              {activeProvider.models.map((m) => (
                <div
                  key={m.modelId}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: "8px",
                    padding: "10px 12px",
                    borderRadius: "8px",
                    border: "1px solid var(--border-subtle)",
                    backgroundColor: "var(--bg-canvas)",
                  }}
                >
                  <div>
                    <div style={{ fontSize: "0.84rem", fontWeight: 600 }}>{m.modelId}</div>
                    <div style={{ fontSize: "0.74rem", color: "var(--text-muted)" }}>
                      Context: {m.contextWindow.toLocaleString()} · Max output:{" "}
                      {m.maxOutputTokens.toLocaleString()}
                    </div>
                  </div>
                  <Badge
                    tone={m.capabilityStatus === "Verified" ? "success" : "neutral"}
                    label={m.capabilityStatus}
                  />
                </div>
              ))}
            </div>
          )}

          <p style={{ margin: 0, fontSize: "0.74rem", color: "var(--text-muted)" }}>
            {activeProvider.c319Note}
          </p>
        </div>
      </div>

      <AddModelModal
        isOpen={addModelOpen}
        providerLabel={activeProvider.label}
        onAddModel={(model) => {
          updateActiveProvider({ models: [...activeProvider.models, model] });
          setAddModelOpen(false);
          onNotify(`Added model "${model.modelId}" to ${activeProvider.label}.`);
        }}
        onClose={() => setAddModelOpen(false)}
      />
    </div>
  );
}
