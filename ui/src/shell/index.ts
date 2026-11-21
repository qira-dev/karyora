export {
  ApplicationShell,
  type ApplicationShellProps,
} from "./ApplicationShell";

export {
  CommandCenterModal,
  type CommandCenterModalProps,
} from "./CommandCenterModal";

export { LeftSidebar, type LeftSidebarProps } from "./LeftSidebar";

export {
  NotificationDrawer,
  type NotificationDrawerProps,
} from "./NotificationDrawer";

export {
  OnboardingAndMigrationModal,
  type OnboardingAndMigrationModalProps,
} from "./OnboardingAndMigrationModal";

export {
  ProfilePopoverMenu,
  type ProfilePopoverMenuProps,
} from "./ProfilePopoverMenu";

export {
  ProjectHoverCard,
  type ProjectHoverCardProps,
} from "./ProjectHoverCard";

export {
  ProjectTaskTree,
  type ProjectTaskTreeProps,
} from "./ProjectTaskTree";

export {
  StartupHandshakeGate,
  type StartupHandshakeGateProps,
} from "./StartupHandshakeGate";

export {
  TaskContextMenu,
  type TaskContextActionId,
  type TaskContextMenuProps,
} from "./TaskContextMenu";

export {
  WorkspaceTopBar,
  type WorkspaceTopBarProps,
} from "./WorkspaceTopBar";

export {
  HELP_MENU_ITEMS,
  INTERFACE_ZOOM_MAX,
  INTERFACE_ZOOM_MIN,
  INTERFACE_ZOOM_STEP,
  MIGRATION_GUIDE_STEPS,
  PROFILE_INTERFACE_MODE_OPTIONS,
  PROFILE_LOCALE_OPTIONS,
  PROFILE_THEME_OPTIONS,
  TASK_CONTEXT_MENU_ITEMS,
  clampInterfaceZoom,
  deduplicateNotifications,
  filterCommandCenterEntries,
  filterTasksForSidebar,
  filterVisibleSidebarProjects,
  type ActiveMainSurface,
  type CommandCenterCategory,
  type CommandCenterEntry,
  type CoreNotificationItem,
  type HelpMenuEntry,
  type MigrationGuideStep,
  type ProjectTaskFilterMode,
  type ProjectTaskGroupMode,
  type TaskContextMenuActionDef,
} from "./shellConfig";
