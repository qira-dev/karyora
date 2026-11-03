import { useT } from "../../i18n/useT";
import { NotificationList } from "./NotificationList";
import { PageHeading, PageLayout } from "../PageLayout";

/**
 * `/settings/notifications` — the notifications group's page (IMPL-14;
 * the ADOPTED S6 supply, P-S6-07's own terms): the delivered entry-state
 * page is replaced by the WIRED notification surface over the delivered
 * Core notification machinery (the same surface the topbar
 * notification-center popover renders, at page width).
 *
 * The PREFERENCES are NOT here, truthfully: the delivered Core contract
 * has no notification preference store (the recorded gap — M-40's
 * surface is the preference-store supply's), so no preference control,
 * no category toggle, and no channel setting exists on this page and
 * none is invented. The page renders the in-app list surface and states
 * the preference absence. The delivery disposition stays decision 6's
 * recorded answers: in-app ON + OS OFF by default — no OS-permission
 * request, no OS delivery, no quiet hours in v1 (the UI-34 FINAL freeze
 * stays its own Checkpoint-C act).
 */
export function NotificationsPage() {
  const t = useT();

  return (
    <PageLayout>
      <PageHeading>{t("groups.notifications.title")}</PageHeading>
      <p>{t("notifications.preferencesAbsent")}</p>
      <NotificationList variant="page" />
    </PageLayout>
  );
}
