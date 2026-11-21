/**
 * Settings Section 7: Keyboard Shortcuts (UIR-8.1, Screenshots #21–#22, Blueprint §13.1 (7)).
 *
 * Search, `Reset all to defaults`, editable Global/Composer keybinding table with live
 * conflict detection ensuring `Escape` never cancels a durable Job.
 */

import { useMemo, useState, type ReactElement } from "react";
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
      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-2)" }}>
          <Input
            aria-label="Search keyboard shortcuts"
            placeholder="Search shortcuts..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <Button
            size="sm"
            variant="secondary"
            onClick={() => setBindings(platformDefaults)}
          >
            Reset all to defaults
          </Button>
        </div>

        <table className="ky-step-table">
          <thead>
            <tr>
              <th>Scope</th>
              <th>Action</th>
              <th>Keybinding</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((b) => {
              const err = conflictMap.get(b.id);
              return (
                <tr key={b.id}>
                  <td>
                    <Badge tone="neutral" label={b.scope} />
                  </td>
                  <td>{b.action}</td>
                  <td>
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
                  </td>
                  <td>
                    <Badge
                      tone={err ? "danger" : "success"}
                      label={err ? "Conflict" : "Valid"}
                    />
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
