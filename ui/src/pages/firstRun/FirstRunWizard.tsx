import { useCallback, useEffect, useRef, useState } from "react";

import { useFirstRun } from "../../app/firstRun/firstRunState";
import { useDirtyNavigationGuard } from "../../app/useDirtyNavigationGuard";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import { useT } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./FirstRunWizard.module.css";
import { LocalBootstrapPanel } from "./LocalBootstrapPanel";
import { ProviderSetupForm, type ProviderSetupFormHandle } from "./ProviderSetupForm";

/**
 * The First-Run Setup wizard (C-06; Blueprint §9.1–§9.4, §8.13; J-1).
 *
 * The three-step structure with the no-forced-wizard properties carried
 * exactly: the connection choice (step 1) is local, reversible, and
 * never implies Job migration; provider setup (step 2, C-07) is
 * optional until first model use and can be skipped and revisited; the
 * first-action step (step 3) lists only actions that exist, with their
 * honest availability, and no tutorial carousel anywhere.
 *
 * Honest states this milestone can render:
 * - Step 1's choice applies locally and immediately (it is a local
 *   declaration; the connection itself is established by the Core
 *   bootstrap contracts, which are not wired — the step says so).
 * - Step 1's LOCAL choice additionally carries the IMPL-2 interim
 *   manual bootstrap-exchange panel (C-1): the user pastes the one-time
 *   credential the Core issued through its trusted-local channel and
 *   the panel exchanges it exactly once, surfacing only real outcomes.
 *   The full browser bootstrap UX (auto-launch/deep-link) stays its
 *   recorded open item and is not built here.
 * - Setup COMPLETION is recorded only after authoritative Core success
 *   (C-06). With no Core completion surface the completion command
 *   reports `not submitted`, and the wizard says exactly that — no fake
 *   completed state exists anywhere.
 * - Skip/resume is preserved: skipping keeps the recorded choice and the
 *   session's non-secret draft; re-running setup preserves existing
 *   state (never resets). The API key never survives the form (X-6), and
 *   neither does the one-time bootstrap credential (X-6).
 *
 * All three step panels stay mounted; inactive ones are `hidden`, so
 * going back preserves entries (§8.13) — the preservation policy is
 * stated in the visible copy, never silent.
 */

const STEP_COUNT = 3;

export function FirstRunWizard() {
  const t = useT();
  const { connectionChoice, setConnectionChoice, skipSetup, completeSetup, providerDraft } = useFirstRun();
  const [step, setStep] = useState(0);
  const [completionUnavailable, setCompletionUnavailable] = useState(false);
  const [completionPending, setCompletionPending] = useState(false);
  const formRef = useRef<ProviderSetupFormHandle>(null);
  const headingRefs = [useRef<HTMLHeadingElement>(null), useRef<HTMLHeadingElement>(null), useRef<HTMLHeadingElement>(null)];
  const firstRender = useRef(true);

  const stepNameKeys = ["wizard.stepConnection", "wizard.stepProvider", "wizard.stepFirstAction"] as const;

  const isDirty = useCallback(() => formRef.current?.isDirty() ?? false, []);
  const guard = useDirtyNavigationGuard({
    isDirty,
    onSave: () => formRef.current?.attemptSave(),
    onDiscard: () => formRef.current?.discard(),
  });

  // Step change: move focus to the new step's heading (§7.4); the polite
  // live region announces "Step X of Y" without stealing focus.
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false;
      return;
    }
    headingRefs[step]?.current?.focus();
  }, [step]);

  const handleSkip = useCallback(() => {
    guard.requestLeave(() => skipSetup());
  }, [guard, skipSetup]);

  const attemptCompletion = useCallback(async () => {
    setCompletionPending(true);
    try {
      completeSetup();
    } catch {
      setCompletionUnavailable(true);
    } finally {
      setCompletionPending(false);
    }
  }, [completeSetup]);

  const isResume =
    connectionChoice !== null ||
    providerDraft.presetId !== "" ||
    providerDraft.baseUrl !== "" ||
    providerDraft.modelId !== "" ||
    providerDraft.headers !== "";

  return (
    <PageLayout>
      <PageHeading>{t("wizard.title")}</PageHeading>
      <p className={styles.intro}>{isResume ? t("wizard.resumeIntro") : t("wizard.intro")}</p>
      <p className="visually-hidden" aria-live="polite">
        {t("wizard.stepOf", {
          current: step + 1,
          total: STEP_COUNT,
          name: t(stepNameKeys[step]),
        })}
      </p>

      <ol className={styles.stepList} aria-label={t("wizard.stepsLabel")}>
        {stepNameKeys.map((key, index) => (
          <li
            key={key}
            className={styles.stepItem}
            data-state={step === index ? "current" : step > index ? "completed" : "upcoming"}
            aria-current={step === index ? "step" : undefined}
          >
            <span className={styles.stepMarker} aria-hidden="true">
              {step > index ? "✓" : index + 1}
            </span>
            <span>
              {t(key)}
              <span className="visually-hidden">
                {step === index ? ` — ${t("wizard.stepStateCurrent")}` : step > index ? ` — ${t("wizard.stepStateCompleted")}` : ""}
              </span>
            </span>
          </li>
        ))}
      </ol>

      <div className={styles.panels}>
        <section hidden={step !== 0} aria-labelledby="wizard-step1-heading">
          <h2 ref={headingRefs[0]} id="wizard-step1-heading" tabIndex={-1} className={styles.stepHeading}>
            {t("step1.heading")}
          </h2>
          <p className={styles.stepHelp}>{t("step1.help")}</p>
          <fieldset className={styles.choiceGroup}>
            <legend className="visually-hidden">{t("step1.heading")}</legend>
            <div className={styles.choice}>
              <input
                type="radio"
                id="connection-choice-local"
                name="connection-choice"
                value="local"
                checked={connectionChoice === "local"}
                onChange={() => setConnectionChoice("local")}
                aria-describedby="connection-choice-local-help"
              />
              <label htmlFor="connection-choice-local" className={styles.choiceLabel}>
                {t("step1.localLabel")}
              </label>
            </div>
            <p className={styles.choiceHelp} id="connection-choice-local-help">
              {t("step1.localHelp")}
            </p>
            <div className={styles.choice}>
              <input
                type="radio"
                id="connection-choice-remote"
                name="connection-choice"
                value="remote"
                checked={connectionChoice === "remote"}
                onChange={() => setConnectionChoice("remote")}
                aria-describedby="connection-choice-remote-help"
              />
              <label htmlFor="connection-choice-remote" className={styles.choiceLabel}>
                {t("step1.remoteLabel")}
              </label>
            </div>
            <p className={styles.choiceHelp} id="connection-choice-remote-help">
              {t("step1.remoteHelp")}
            </p>
          </fieldset>
          <p className={styles.stepHelp}>{t("step1.recorded")}</p>
          {connectionChoice === "local" ? <LocalBootstrapPanel /> : null}
        </section>

        <section hidden={step !== 1} aria-labelledby="wizard-step2-heading">
          <h2 ref={headingRefs[1]} id="wizard-step2-heading" tabIndex={-1} className={styles.stepHeading}>
            {t("step2.heading")}
          </h2>
          <p className={styles.stepHelp}>{t("step2.help")}</p>
          <ProviderSetupForm ref={formRef} />
        </section>

        <section hidden={step !== 2} aria-labelledby="wizard-step3-heading">
          <h2 ref={headingRefs[2]} id="wizard-step3-heading" tabIndex={-1} className={styles.stepHeading}>
            {t("step3.heading")}
          </h2>
          <p className={styles.stepHelp}>{t("step3.help")}</p>
          <ul className={styles.actionList}>
            <li className={styles.actionItem}>
              <span className={styles.actionName}>{t("step3.chat")}</span>
              <span className={styles.actionNote}>{t("step3.chatNote")}</span>
            </li>
            <li className={styles.actionItem}>
              <span className={styles.actionName}>{t("step3.import")}</span>
              <span className={styles.actionNote}>{t("step3.importNote")}</span>
            </li>
          </ul>
          <p className={styles.stepHelp}>{t("step3.templateWithheld")}</p>
          <p className={styles.stepHelp}>{t("step3.safeActions")}</p>

          <div className={styles.completion}>
            <p className={styles.stepHelp}>{t("wizard.completion.help")}</p>
            <button
              type="button"
              className={styles.primaryButton}
              disabled={completionPending}
              onClick={() => void attemptCompletion()}
            >
              {t("wizard.completion.button")}
            </button>
            {completionUnavailable ? (
              <RecoveryPanel
                title={t("wizard.completion.unavailableTitle")}
                body={t("wizard.completion.unavailableBody")}
                note={t("wizard.completion.unavailableNote")}
              />
            ) : null}
          </div>
        </section>
      </div>

      <div className={styles.footer}>
        <button
          type="button"
          className={styles.secondaryButton}
          disabled={step === 0}
          onClick={() => setStep((value) => Math.max(0, value - 1))}
        >
          {t("wizard.back")}
        </button>
        {step < STEP_COUNT - 1 ? (
          <button
            type="button"
            className={styles.primaryButton}
            onClick={() => setStep((value) => Math.min(STEP_COUNT - 1, value + 1))}
          >
            {t("wizard.next")}
          </button>
        ) : null}
        <button type="button" className={styles.tertiaryButton} onClick={handleSkip}>
          {t("wizard.skip")}
        </button>
      </div>

      <p className={styles.policy}>{t("wizard.preservationPolicy")}</p>

      {guard.guardDialog}
    </PageLayout>
  );
}
