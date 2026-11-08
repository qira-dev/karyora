import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope, KnowledgePackVersionServed } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./KnowledgePackDetailPage.module.css";
import { countOrUnavailable, digestOrUnavailable } from "./knowledgeStates";

/**
 * `/knowledge/packs/:packId/versions/:versionId` — the canonical
 * immutable-version route (B1-U1; P1.1 #9, P4.1 D4's deep link, P4.3
 * F-B's immutability law).
 *
 * The read is the published two-ID version read exactly as served. The
 * immutability law binds the render: this version is immutable,
 * Core-enforced; NO edit affordance, no "edit history", no compare, and
 * no resolve control exists here — those operations are not served by
 * the family and none is invented (§31.5). The claim/evidence members
 * are not served by the adapter (B1-A1 §2.6 item 1), so the page states
 * that honestly rather than rendering fabricated rows.
 */

type VersionPhase = "loading" | "data" | "failure" | "unknown";

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function KnowledgeVersionPage() {
  const { packId = "", versionId = "" } = useParams<{ packId: string; versionId: string }>();
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");

  const [version, setVersion] = useState<KnowledgePackVersionServed | null>(null);
  const [phase, setPhase] = useState<VersionPhase>("loading");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);

  const read = useCallback(() => {
    setPhase("loading");
    setFailure(null);
    void transport.knowledgeVersionGet(packId, versionId).then((outcome) => {
      if (outcome.kind === "data") {
        setVersion(outcome.data.version);
        setPhase("data");
      } else if (outcome.kind === "rejection") {
        setFailure(outcome.envelope);
        setPhase("failure");
      } else {
        setPhase("unknown");
      }
    });
  }, [packId, transport, versionId]);

  useEffect(() => {
    read();
  }, [read]);

  return (
    <PageLayout>
      <p className={styles.backLink}>
        <Link to={`/knowledge/packs/${encodeURIComponent(packId)}`}>{t("knowledge.version.back")}</Link>
      </p>
      <PageHeading>{t("knowledge.version.heading")}</PageHeading>
      <p className={styles.note}>{t("knowledge.version.immutableNote")}</p>

      {phase === "loading" ? <p className={styles.note}>{t("knowledge.list.loading")}</p> : null}

      {phase === "failure" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("knowledge.version.failureTitle")}
            body={t("knowledge.version.failureBody")}
            note={t("knowledge.list.connectionHint")}
            referenceId={failure.correlationId}
            actions={[{ label: t("knowledge.detail.refresh"), onAction: read }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {phase === "unknown" ? (
        <RecoveryPanel
          title={t("knowledge.version.unknownTitle")}
          body={t("knowledge.version.unknownBody")}
          note={t("knowledge.list.connectionHint")}
          actions={[{ label: t("knowledge.detail.refresh"), onAction: read }]}
        />
      ) : null}

      {phase === "data" && version !== null ? (
        <section className={styles.panel}>
          <dl className={styles.facts}>
            <div>
              <dt>{t("knowledge.version.packId")}</dt>
              <dd>
                <code>{version.pack_id}</code>
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.version.versionId")}</dt>
              <dd>
                <code>{version.version_id}</code>
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.version.schemaVersion")}</dt>
              <dd>{version.schema_version}</dd>
            </div>
            <div>
              <dt>{t("knowledge.version.createdAt")}</dt>
              <dd>
                <time dateTime={version.created_at}>{formatTimestamp(new Date(version.created_at), locale)}</time>
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.version.digest")}</dt>
              <dd>
                <code>{digestOrUnavailable(version.pack_root_digest, unavailable)}</code>
                <p className={styles.note}>{t("knowledge.version.digestNote")}</p>
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.version.entryCount")}</dt>
              <dd>{countOrUnavailable(version.entry_count, unavailable)}</dd>
            </div>
            <div>
              <dt>{t("knowledge.version.claimCount")}</dt>
              <dd>{countOrUnavailable(version.claim_count, unavailable)}</dd>
            </div>
            <div>
              <dt>{t("knowledge.version.evidenceCount")}</dt>
              <dd>{countOrUnavailable(version.evidence_count, unavailable)}</dd>
            </div>
            <div>
              <dt>{t("knowledge.version.license")}</dt>
              <dd>{version.license === undefined || version.license === "" ? unavailable : version.license}</dd>
            </div>
            <div>
              <dt>{t("knowledge.version.licenseStatus")}</dt>
              <dd>
                {version.license_status === undefined || version.license_status === "" ? (
                  unavailable
                ) : (
                  <>
                    {version.license_status}
                    {version.license_status === "unknown" ? (
                      <span className={styles.note}> — {t("knowledge.detail.integrity.licenseStatusUnknownNote")}</span>
                    ) : null}
                  </>
                )}
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.version.supersedes")}</dt>
              <dd>
                {version.supersedes === undefined || version.supersedes.length === 0 ? (
                  t("knowledge.version.noSupersession")
                ) : (
                  version.supersedes.join(", ")
                )}
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.version.supersededBy")}</dt>
              <dd>
                {version.superseded_by === undefined || version.superseded_by === "" ? (
                  t("knowledge.version.noSupersession")
                ) : (
                  <code>{version.superseded_by}</code>
                )}
              </dd>
            </div>
          </dl>
          <p className={styles.note}>{t("knowledge.version.claimsEvidenceNote")}</p>
        </section>
      ) : null}
    </PageLayout>
  );
}
