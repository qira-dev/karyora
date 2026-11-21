/**
 * Settings Section 10: Memory (UIR-8.2, Screenshots #26–#28, Blueprint §12.3 & §13.2 (10)).
 *
 * Mounts the shared `ScopedMemorySurface` directly inside Overlay Settings.
 */

import type { ReactElement } from "react";
import {
  ScopedMemorySurface,
  type ScopedMemorySurfaceProps,
} from "../../domains/memory";

export function MemorySettingsSection(
  props: ScopedMemorySurfaceProps,
): ReactElement {
  return (
    <div className="ky-settings-content" data-testid="settings-section-memory">
      <ScopedMemorySurface {...props} />
    </div>
  );
}
