import { deepMergeResources } from "../mergeBundle";
import common from "../locales/pt-BR/common.json";
import nav from "../locales/pt-BR/nav.json";
import shell from "../locales/pt-BR/shell.json";
import settings from "../locales/pt-BR/settings.json";
import home from "../locales/pt-BR/home.json";
import firstrun from "../locales/pt-BR/firstrun.json";
import jobs from "../locales/pt-BR/jobs.json";
import run from "../locales/pt-BR/run.json";
import chat from "../locales/pt-BR/chat.json";
import notifications from "../locales/pt-BR/notifications.json";
import activity from "../locales/pt-BR/activity.json";
import knowledge from "../locales/pt-BR/knowledge.json";
import memory from "../locales/pt-BR/memory.json";
import context from "../locales/pt-BR/context.json";
import workflow from "../locales/pt-BR/workflow.json";
import plugin from "../locales/pt-BR/plugin.json";

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
