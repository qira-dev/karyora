import { deepMergeResources } from "../mergeBundle";
import common from "../locales/fr/common.json";
import nav from "../locales/fr/nav.json";
import shell from "../locales/fr/shell.json";
import settings from "../locales/fr/settings.json";
import home from "../locales/fr/home.json";
import firstrun from "../locales/fr/firstrun.json";
import jobs from "../locales/fr/jobs.json";
import run from "../locales/fr/run.json";
import chat from "../locales/fr/chat.json";
import notifications from "../locales/fr/notifications.json";
import activity from "../locales/fr/activity.json";
import knowledge from "../locales/fr/knowledge.json";
import memory from "../locales/fr/memory.json";
import context from "../locales/fr/context.json";
import workflow from "../locales/fr/workflow.json";
import plugin from "../locales/fr/plugin.json";

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
