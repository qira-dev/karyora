import { deepMergeResources } from "../mergeBundle";
import common from "../locales/bn/common.json";
import nav from "../locales/bn/nav.json";
import shell from "../locales/bn/shell.json";
import settings from "../locales/bn/settings.json";
import home from "../locales/bn/home.json";
import firstrun from "../locales/bn/firstrun.json";
import jobs from "../locales/bn/jobs.json";
import run from "../locales/bn/run.json";
import chat from "../locales/bn/chat.json";
import notifications from "../locales/bn/notifications.json";
import activity from "../locales/bn/activity.json";
import knowledge from "../locales/bn/knowledge.json";
import memory from "../locales/bn/memory.json";
import context from "../locales/bn/context.json";
import workflow from "../locales/bn/workflow.json";
import plugin from "../locales/bn/plugin.json";

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
