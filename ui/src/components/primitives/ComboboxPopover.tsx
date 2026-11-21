/**
 * W3C WAI-ARIA APG 1.2 Accessible Combobox Popover (`@ / # $ +`) & Context Menu (`#85`, `#61–#65`)
 * (UIR-2.3, Blueprint §7.3, §8.3, §20.5, G-05, G-06).
 */

import {
  useId,
  useState,
  type KeyboardEvent,
  type ReactElement,
  type ReactNode,
} from "react";

export interface ComboboxOptionItem {
  readonly id: string;
  readonly label: string;
  readonly description?: string;
  readonly category?: string;
  readonly icon?: ReactNode;
  readonly disabled?: boolean;
}

export interface ComboboxPopoverProps {
  readonly ariaLabel: string;
  readonly isOpen: boolean;
  readonly query: string;
  readonly options: readonly ComboboxOptionItem[];
  readonly onQueryChange: (nextQuery: string) => void;
  readonly onSelectOption: (option: ComboboxOptionItem) => void;
  readonly onDismiss: () => void;
  readonly placeholder?: string;
}

export function ComboboxPopover({
  ariaLabel,
  isOpen,
  query,
  options,
  onQueryChange,
  onSelectOption,
  onDismiss,
  placeholder = "Search...",
}: ComboboxPopoverProps): ReactElement {
  const baseId = useId();
  const listboxId = `${baseId}-listbox`;
  const [activeIndex, setActiveIndex] = useState(0);

  const enabledOptions = options.filter((o) => !o.disabled);
  const safeIndex =
    enabledOptions.length > 0
      ? Math.min(activeIndex, enabledOptions.length - 1)
      : -1;
  const activeOption = safeIndex >= 0 ? enabledOptions[safeIndex] : undefined;

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Escape") {
      e.preventDefault();
      onDismiss();
      return;
    }
    if (enabledOptions.length === 0) {
      return;
    }
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActiveIndex((prev) => (prev + 1) % enabledOptions.length);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActiveIndex(
        (prev) => (prev - 1 + enabledOptions.length) % enabledOptions.length,
      );
    } else if ((e.key === "Enter" || e.key === "Tab") && activeOption) {
      e.preventDefault();
      onSelectOption(activeOption);
    }
  };

  return (
    <div style={{ position: "relative", width: "100%" }}>
      <input
        type="text"
        role="combobox"
        aria-label={ariaLabel}
        aria-expanded={isOpen ? "true" : "false"}
        aria-controls={listboxId}
        aria-autocomplete="list"
        aria-activedescendant={
          isOpen && activeOption ? `${baseId}-opt-${activeOption.id}` : undefined
        }
        value={query}
        placeholder={placeholder}
        className="ky-input"
        onChange={(e) => {
          setActiveIndex(0);
          onQueryChange(e.target.value);
        }}
        onKeyDown={handleKeyDown}
      />
      {isOpen && (
        <ul
          id={listboxId}
          role="listbox"
          aria-label={ariaLabel}
          style={{
            margin: "var(--space-1) 0 0 0",
            padding: "var(--space-1)",
            listStyle: "none",
            maxHeight: "260px",
            overflowY: "auto",
            borderRadius: "var(--radius-md)",
            border: "1px solid var(--border-default)",
            backgroundColor: "var(--bg-surface-1)",
          }}
        >
          {enabledOptions.length === 0 ? (
            <li
              role="option"
              aria-selected="false"
              aria-disabled="true"
              style={{
                padding: "var(--space-2) var(--space-3)",
                color: "var(--text-muted)",
                fontSize: "0.8125rem",
              }}
            >
              No matching options
            </li>
          ) : (
            enabledOptions.map((option, idx) => {
              const isSelected = idx === safeIndex;
              return (
                <li
                  key={option.id}
                  id={`${baseId}-opt-${option.id}`}
                  role="option"
                  aria-selected={isSelected ? "true" : "false"}
                  onClick={() => onSelectOption(option)}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: "var(--space-2)",
                    padding: "var(--space-2) var(--space-3)",
                    borderRadius: "var(--radius-sm)",
                    cursor: "pointer",
                    backgroundColor: isSelected
                      ? "var(--bg-surface-2)"
                      : "transparent",
                  }}
                >
                  {option.icon}
                  <span style={{ flex: 1, fontSize: "0.875rem" }}>
                    {option.label}
                  </span>
                  {option.description && (
                    <span
                      style={{
                        fontSize: "0.75rem",
                        color: "var(--text-muted)",
                      }}
                    >
                      {option.description}
                    </span>
                  )}
                </li>
              );
            })
          )}
        </ul>
      )}
    </div>
  );
}

/**
 * Accessible Context Menu / Flyout Menu (`role="menu"`, `role="menuitem"`)
 * Powers the 13-Item Task Context Menu (Screenshot #85) and Profile Popover submenus (#61–#65).
 */
export interface ContextMenuItem {
  readonly id: string;
  readonly label: string;
  readonly icon?: ReactNode;
  readonly shortcutHint?: string;
  readonly danger?: boolean;
  readonly disabled?: boolean;
  readonly separatorBefore?: boolean;
}

export interface ContextMenuProps {
  readonly ariaLabel: string;
  readonly items: readonly ContextMenuItem[];
  readonly onSelectItem: (itemId: string) => void;
  readonly onClose: () => void;
}

export function ContextMenu({
  ariaLabel,
  items,
  onSelectItem,
  onClose,
}: ContextMenuProps): ReactElement {
  return (
    <div
      role="menu"
      aria-label={ariaLabel}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.preventDefault();
          onClose();
        }
      }}
      style={{
        display: "flex",
        flexDirection: "column",
        minWidth: "220px",
        padding: "var(--space-1)",
        borderRadius: "var(--radius-md)",
        border: "1px solid var(--border-default)",
        backgroundColor: "var(--bg-surface-1)",
      }}
    >
      {items.map((item) => (
        <div key={item.id}>
          {item.separatorBefore && (
            <hr
              role="separator"
              style={{
                border: "none",
                borderTop: "1px solid var(--border-subtle)",
                margin: "var(--space-1) 0",
              }}
            />
          )}
          <button
            type="button"
            role="menuitem"
            disabled={item.disabled}
            className={`ky-btn ky-btn--ghost ky-btn--sm ${
              item.danger ? "ky-btn--danger" : ""
            }`}
            style={{
              width: "100%",
              justifyContent: "flex-start",
            }}
            onClick={() => {
              if (!item.disabled) {
                onSelectItem(item.id);
                onClose();
              }
            }}
          >
            {item.icon}
            <span style={{ flex: 1, textAlign: "start" }}>{item.label}</span>
            {item.shortcutHint && (
              <kbd
                style={{
                  fontSize: "0.75rem",
                  color: "var(--text-muted)",
                  fontFamily: "var(--font-mono)",
                }}
              >
                {item.shortcutHint}
              </kbd>
            )}
          </button>
        </div>
      ))}
    </div>
  );
}
