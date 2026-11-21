import { deepMergeResources } from "../mergeBundle";
import common from "../locales/tr/common.json";
import nav from "../locales/tr/nav.json";
import shell from "../locales/tr/shell.json";
import settings from "../locales/tr/settings.json";
import home from "../locales/tr/home.json";
import firstrun from "../locales/tr/firstrun.json";
import jobs from "../locales/tr/jobs.json";
import run from "../locales/tr/run.json";
import chat from "../locales/tr/chat.json";
import notifications from "../locales/tr/notifications.json";
import activity from "../locales/tr/activity.json";
import knowledge from "../locales/tr/knowledge.json";
import memory from "../locales/tr/memory.json";
import context from "../locales/tr/context.json";
import workflow from "../locales/tr/workflow.json";
import plugin from "../locales/tr/plugin.json";

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
