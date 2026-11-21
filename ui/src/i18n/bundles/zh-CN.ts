import { deepMergeResources } from "../mergeBundle";
import common from "../locales/zh-CN/common.json";
import nav from "../locales/zh-CN/nav.json";
import shell from "../locales/zh-CN/shell.json";
import settings from "../locales/zh-CN/settings.json";
import home from "../locales/zh-CN/home.json";
import firstrun from "../locales/zh-CN/firstrun.json";
import jobs from "../locales/zh-CN/jobs.json";
import run from "../locales/zh-CN/run.json";
import chat from "../locales/zh-CN/chat.json";
import notifications from "../locales/zh-CN/notifications.json";
import activity from "../locales/zh-CN/activity.json";
import knowledge from "../locales/zh-CN/knowledge.json";
import memory from "../locales/zh-CN/memory.json";
import context from "../locales/zh-CN/context.json";
import workflow from "../locales/zh-CN/workflow.json";
import plugin from "../locales/zh-CN/plugin.json";

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
