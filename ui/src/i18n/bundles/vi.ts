import { deepMergeResources } from "../mergeBundle";
import common from "../locales/vi/common.json";
import nav from "../locales/vi/nav.json";
import shell from "../locales/vi/shell.json";
import settings from "../locales/vi/settings.json";
import home from "../locales/vi/home.json";
import firstrun from "../locales/vi/firstrun.json";
import jobs from "../locales/vi/jobs.json";
import run from "../locales/vi/run.json";
import chat from "../locales/vi/chat.json";
import notifications from "../locales/vi/notifications.json";
import activity from "../locales/vi/activity.json";
import knowledge from "../locales/vi/knowledge.json";
import memory from "../locales/vi/memory.json";
import context from "../locales/vi/context.json";
import workflow from "../locales/vi/workflow.json";
import plugin from "../locales/vi/plugin.json";

export default function loadBundle(): Record<string, unknown> {
  return deepMergeResources(
    {},
    common,
    nav,
    shell,
    settings,
    home,
    firstrun,
    jobs,
    run,
    chat,
    notifications,
    activity,
    knowledge,
    memory,
    context,
    workflow,
    plugin,
  );
}
