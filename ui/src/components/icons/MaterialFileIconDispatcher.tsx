/**
 * Material File, Language & Folder SVG Icon Dispatcher Component (UIR-2.2, UI-36, G-04).
 *
 * Resolves any filename, path, extension, folder name, or code theme across all
 * 1,147 Material SVG icons (`src/assets/material-icons/<iconId>.svg`).
 * Used across:
 * - Conversation stream tool execution rows (`Viewed`, `Edited`, `Created`, `Changed` #75–#77, #84)
 * - Left Sidebar Project & Task rows (#84)
 * - Side-Pane `Files` tree & `Review` diff tab (#78)
 * - Composer `@` file/folder attachment chips (#68)
 */

import type { ReactElement } from "react";
import { resolveCodeThemePalette } from "../../styles/codeThemeRegistry";
import type { DarkCodeTheme, LightCodeTheme } from "../../types/decisions";
import {
  getMaterialIconAssetPath,
  resolveMaterialIconId,
  type MaterialIconId,
  type ResolveMaterialIconOptions,
} from "./materialIconManifest";

export interface MaterialFileIconProps {
  readonly filename: string;
  readonly isFolder?: boolean;
  readonly isOpen?: boolean;
  readonly codeTheme?: LightCodeTheme | DarkCodeTheme;
  readonly themeVariant?: ResolveMaterialIconOptions["themeVariant"];
  readonly size?: 16 | 20 | 24;
  readonly className?: string;
  readonly title?: string;
  readonly decorative?: boolean;
}

export function resolveIconThemeVariant(
  codeTheme?: LightCodeTheme | DarkCodeTheme,
  explicitVariant?: ResolveMaterialIconOptions["themeVariant"],
): ResolveMaterialIconOptions["themeVariant"] {
  if (explicitVariant) {
    return explicitVariant;
  }
  if (!codeTheme) {
    return "default";
  }
  return resolveCodeThemePalette(codeTheme).iconThemeVariant;
}

export function MaterialFileIcon({
  filename,
  isFolder = false,
  isOpen = false,
  codeTheme,
  themeVariant,
  size = 16,
  className,
  title,
  decorative = true,
}: MaterialFileIconProps): ReactElement {
  const effectiveVariant = resolveIconThemeVariant(codeTheme, themeVariant);
  const iconId: MaterialIconId = resolveMaterialIconId(filename, {
    isFolder,
    isOpen,
    themeVariant: effectiveVariant,
  });
  const assetPath = getMaterialIconAssetPath(iconId);

  return (
    <img
      src={assetPath}
      width={size}
      height={size}
      alt={decorative ? "" : title ?? filename}
      aria-hidden={decorative ? "true" : undefined}
      title={title}
      className={className}
      data-icon-id={iconId}
      data-filename={filename}
      style={{
        display: "inline-block",
        verticalAlign: "middle",
        flexShrink: 0,
        width: `${size}px`,
        height: `${size}px`,
      }}
    />
  );
}

export { MaterialFileIcon as FileIcon };
