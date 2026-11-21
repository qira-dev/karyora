import { deepMergeResources } from "../mergeBundle";
import common from "../locales/id/common.json";
import nav from "../locales/id/nav.json";
import shell from "../locales/id/shell.json";
import settings from "../locales/id/settings.json";
import home from "../locales/id/home.json";
import firstrun from "../locales/id/firstrun.json";
import jobs from "../locales/id/jobs.json";
import run from "../locales/id/run.json";
import chat from "../locales/id/chat.json";
import notifications from "../locales/id/notifications.json";
import activity from "../locales/id/activity.json";
import knowledge from "../locales/id/knowledge.json";
import memory from "../locales/id/memory.json";
import context from "../locales/id/context.json";
import workflow from "../locales/id/workflow.json";
import plugin from "../locales/id/plugin.json";

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
