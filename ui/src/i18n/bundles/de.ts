import { deepMergeResources } from "../mergeBundle";
import common from "../locales/de/common.json";
import nav from "../locales/de/nav.json";
import shell from "../locales/de/shell.json";
import settings from "../locales/de/settings.json";
import home from "../locales/de/home.json";
import firstrun from "../locales/de/firstrun.json";
import jobs from "../locales/de/jobs.json";
import run from "../locales/de/run.json";
import chat from "../locales/de/chat.json";
import notifications from "../locales/de/notifications.json";
import activity from "../locales/de/activity.json";
import knowledge from "../locales/de/knowledge.json";
import memory from "../locales/de/memory.json";
import context from "../locales/de/context.json";
import workflow from "../locales/de/workflow.json";
import plugin from "../locales/de/plugin.json";

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
