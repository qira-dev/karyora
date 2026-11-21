import { deepMergeResources } from "../mergeBundle";
import common from "../locales/ja/common.json";
import nav from "../locales/ja/nav.json";
import shell from "../locales/ja/shell.json";
import settings from "../locales/ja/settings.json";
import home from "../locales/ja/home.json";
import firstrun from "../locales/ja/firstrun.json";
import jobs from "../locales/ja/jobs.json";
import run from "../locales/ja/run.json";
import chat from "../locales/ja/chat.json";
import notifications from "../locales/ja/notifications.json";
import activity from "../locales/ja/activity.json";
import knowledge from "../locales/ja/knowledge.json";
import memory from "../locales/ja/memory.json";
import context from "../locales/ja/context.json";
import workflow from "../locales/ja/workflow.json";
import plugin from "../locales/ja/plugin.json";

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
