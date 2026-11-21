import { deepMergeResources } from "../mergeBundle";
import common from "../locales/ru/common.json";
import nav from "../locales/ru/nav.json";
import shell from "../locales/ru/shell.json";
import settings from "../locales/ru/settings.json";
import home from "../locales/ru/home.json";
import firstrun from "../locales/ru/firstrun.json";
import jobs from "../locales/ru/jobs.json";
import run from "../locales/ru/run.json";
import chat from "../locales/ru/chat.json";
import notifications from "../locales/ru/notifications.json";
import activity from "../locales/ru/activity.json";
import knowledge from "../locales/ru/knowledge.json";
import memory from "../locales/ru/memory.json";
import context from "../locales/ru/context.json";
import workflow from "../locales/ru/workflow.json";
import plugin from "../locales/ru/plugin.json";

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
