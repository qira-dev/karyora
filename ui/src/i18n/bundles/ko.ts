import { deepMergeResources } from "../mergeBundle";
import common from "../locales/ko/common.json";
import nav from "../locales/ko/nav.json";
import shell from "../locales/ko/shell.json";
import settings from "../locales/ko/settings.json";
import home from "../locales/ko/home.json";
import firstrun from "../locales/ko/firstrun.json";
import jobs from "../locales/ko/jobs.json";
import run from "../locales/ko/run.json";
import chat from "../locales/ko/chat.json";
import notifications from "../locales/ko/notifications.json";
import activity from "../locales/ko/activity.json";
import knowledge from "../locales/ko/knowledge.json";
import memory from "../locales/ko/memory.json";
import context from "../locales/ko/context.json";
import workflow from "../locales/ko/workflow.json";
import plugin from "../locales/ko/plugin.json";

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
