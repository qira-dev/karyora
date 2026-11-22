/**
 * Settings Section 7: Keyboard Shortcuts — Calibrated 1:1 to ZCode Screenshots #21–#22.
 *
 * Exact ZCode layout:
 * - Top bar: `Reset all to defaults` (left), `Search shortcuts` (right)
 * - Clean 4-column table: `Command` | `Keybinding` (styled `<kbd>` chips, click Edit to modify) | `Scope` | `Actions`
 * - Conflict detection guarantees `Escape` never cancels a durable Job.
 */

import { useMemo, useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Badge, Button, Input } from "../../components/primitives";
import {
  detectRuntimePlatform,
  formatPlatformShortcut,
} from "../../platform-bridge";
import {
  DEFAULT_KEYBOARD_SHORTCUTS,
  detectShortcutConflicts,
  type ShortcutBinding,
} from "../settingsConfig";

export function KeyboardShortcutsSettingsSection(): ReactElement {
  const { t } = useTranslation();
  const runtimeTarget = useMemo(() => detectRuntimePlatform(), []);
  const platformDefaults = useMemo<readonly ShortcutBinding[]>(
    () =>
      DEFAULT_KEYBOARD_SHORTCUTS.map((binding) => ({
        ...binding,
        keys: formatPlatformShortcut(binding.keys, runtimeTarget),
      })),
    [runtimeTarget],
  );
  const [bindings, setBindings] = useState<readonly ShortcutBinding[]>(
    platformDefaults,
  );
  const [search, setSearch] = useState("");
  const [editingId, setEditingId] = useState<string | null>(null);

  const conflicts = useMemo(() => detectShortcutConflicts(bindings), [bindings]);
  const conflictMap = useMemo(() => {
    const m = new Map<string, string>();
    for (const c of conflicts) m.set(c.bindingId, c.reason);
    return m;
  }, [conflicts]);

  const filtered = bindings.filter((b) => {
    const q = search.trim().toLowerCase();
    if (!q) return true;
    return (
      b.action.toLowerCase().includes(q) ||
      b.keys.toLowerCase().includes(q) ||
      b.scope.toLowerCase().includes(q)
    );
  });

  return (
    <div className="ky-settings-content" data-testid="settings-section-keyboard-shortcuts">
      <div className="ky-settings-toolbar">
        <div className="ky-settings-toolbar__left">
          <Button
            size="sm"
            variant="secondary"
            onClick={() => {
              setBindings(platformDefaults);
              setEditingId(null);
            }}
          >
            <ChromeIcon name="refresh" size={14} />
            <span>
              {t("ui.settings.shortcuts.resetAll", {
                defaultValue: "Reset all to defaults",
              })}
            </span>
          </Button>
        </div>
        <div className="ky-settings-toolbar__right" style={{ minWidth: "220px" }}>
          <Input
            aria-label={t("ui.settings.shortcuts.searchPlaceholder", {
              defaultValue: "Search shortcuts",
            })}
            placeholder={t("ui.settings.shortcuts.searchPlaceholder", {
              defaultValue: "Search shortcuts",
            })}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
      </div>

      <div className="ky-settings-list">
        <table className="ky-settings-table">
          <thead>
            <tr>
              <th>{t("ui.settings.shortcuts.colCommand", { defaultValue: "Command" })}</th>
              <th>{t("ui.settings.shortcuts.colKeybinding", { defaultValue: "Keybinding" })}</th>
              <th>{t("ui.settings.shortcuts.colScope", { defaultValue: "Scope" })}</th>
              <th style={{ textAlign: "end" }}>
                {t("ui.settings.shortcuts.colActions", { defaultValue: "Actions" })}
              </th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((b) => {
              const err = conflictMap.get(b.id);
              const isEditing = editingId === b.id;
              const keyTokens = b.keys.split("+").map((k) => k.trim()).filter(Boolean);
              return (
                <tr key={b.id}>
                  <td style={{ fontWeight: 500 }}>{b.action}</td>
                  <td>
                    {isEditing ? (
                      <Input
                        aria-label={`Shortcut for ${b.action}`}
                        value={b.keys}
                        errorText={err}
                        onChange={(e) =>
                          setBindings((prev) =>
                            prev.map((item) =>
                              item.id === b.id ? { ...item, keys: e.target.value } : item,
                            ),
                          )
                        }
                      />
                    ) : (
                      <div style={{ display: "flex", alignItems: "center", gap: "8px", flexWrap: "wrap" }}>
                        <span className="ky-kbd-group">
                          {keyTokens.map((tok, i) => (
                            <kbd key={`${b.id}-${i}`} className="ky-kbd">
                              {tok}
                            </kbd>
                          ))}
                        </span>
                        {err && <Badge tone="danger" label={err} />}
                      </div>
                    )}
                  </td>
                  <td style={{ color: "var(--text-muted)", fontSize: "0.8rem" }}>
                    {b.scope}
                  </td>
                  <td style={{ textAlign: "end" }}>
                    <Button
                      size="sm"
                      variant="ghost"
                      aria-label={isEditing ? `Finish editing ${b.action}` : `Edit ${b.action}`}
                      onClick={() => setEditingId(isEditing ? null : b.id)}
                    >
                      <ChromeIcon name={isEditing ? "check" : "edit"} size={14} />
                    </Button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
