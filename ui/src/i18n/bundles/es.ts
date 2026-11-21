import { deepMergeResources } from "../mergeBundle";
import common from "../locales/es/common.json";
import nav from "../locales/es/nav.json";
import shell from "../locales/es/shell.json";
import settings from "../locales/es/settings.json";
import home from "../locales/es/home.json";
import firstrun from "../locales/es/firstrun.json";
import jobs from "../locales/es/jobs.json";
import run from "../locales/es/run.json";
import chat from "../locales/es/chat.json";
import notifications from "../locales/es/notifications.json";
import activity from "../locales/es/activity.json";
import knowledge from "../locales/es/knowledge.json";
import memory from "../locales/es/memory.json";
import context from "../locales/es/context.json";
import workflow from "../locales/es/workflow.json";
import plugin from "../locales/es/plugin.json";

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
