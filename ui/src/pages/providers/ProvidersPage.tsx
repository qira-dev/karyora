import { Link } from "react-router-dom";
import { Cpu, ExternalLink, Network } from "lucide-react";

import { useT } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./ProvidersPage.module.css";

interface ProviderCardInfo {
  id: string;
  name: string;
  protocol: string;
  models: string;
  caps: string[];
  status: "active" | "supported";
}

const PROVIDERS: ProviderCardInfo[] = [
  {
    id: "openai",
    name: "OpenAI",
    protocol: "REST (api.openai.com/v1)",
    models: "gpt-4o, o1, gpt-4o-mini",
    caps: ["Chat", "Streaming", "Tools", "Vision", "Structured JSON"],
    status: "supported",
  },
  {
    id: "anthropic",
    name: "Anthropic",
    protocol: "REST (api.anthropic.com/v1)",
    models: "claude-3-5-sonnet, claude-3-5-haiku",
    caps: ["Chat", "Streaming", "Tools", "Vision"],
    status: "supported",
  },
  {
    id: "gemini",
    name: "Google Gemini",
    protocol: "REST (generativelanguage.googleapis.com)",
    models: "gemini-2.0-flash, gemini-1.5-pro",
    caps: ["Chat", "Streaming", "Tools", "Vision", "Structured JSON"],
    status: "supported",
  },
  {
    id: "custom-openai",
    name: "Custom / Local Ollama",
    protocol: "OpenAI-Compatible (e.g. localhost:11434/v1)",
    models: "llama3.3, mistral, deepseek-r1, qwen2.5",
    caps: ["Chat", "Streaming", "Local Inference", "Zero Egress Leak"],
    status: "supported",
  },
  {
    id: "deepseek",
    name: "DeepSeek",
    protocol: "REST (api.deepseek.com)",
    models: "deepseek-chat, deepseek-reasoner",
    caps: ["Chat", "Streaming", "Tools"],
    status: "supported",
  },
  {
    id: "qwen",
    name: "Alibaba Qwen",
    protocol: "DashScope Compatible",
    models: "qwen-max, qwen-plus, qwen-turbo",
    caps: ["Chat", "Streaming", "Tools"],
    status: "supported",
  },
  {
    id: "glm",
    name: "GLM / Z.AI",
    protocol: "REST (api.z.ai/paas/v4)",
    models: "glm-4-plus, glm-4-flash",
    caps: ["Chat", "Streaming", "Tools"],
    status: "supported",
  },
  {
    id: "mock",
    name: "Local Mock Fixture",
    protocol: "In-Memory Test Engine",
    models: "default",
    caps: ["Chat", "Instant Testing", "Deterministic Replies"],
    status: "active",
  },
];

export function ProvidersPage() {
  const t = useT();

  return (
    <PageLayout>
      <PageHeading>{t("items.providersAndModels")}</PageHeading>

      <PageSection>
        <h2 className={styles.sectionTitle}>
          <Network size={20} style={{ verticalAlign: "middle", marginRight: 8 }} />
          {t("groups.buildAndConnect")}
        </h2>
        <p className={styles.description}>
          Karyora connects to foundation model providers through isolated, fail-closed codecs.
          API keys are held in the operating-system keyring and never escape to browser storage, logs, or telemetry.
        </p>

        <div className={styles.grid}>
          {PROVIDERS.map((p) => (
            <div key={p.id} className={styles.card}>
              <div>
                <div className={styles.cardHeader}>
                  <h3 className={styles.providerName}>{p.name}</h3>
                  <span
                    className={`${styles.badge} ${
                      p.status === "active" ? styles.badgeActive : styles.badgeSupported
                    }`}
                  >
                    {p.status === "active" ? "Default Active" : "Supported"}
                  </span>
                </div>
                <p className={styles.description}>
                  <strong>Protocol:</strong> {p.protocol}
                  <br />
                  <strong>Models:</strong> {p.models}
                </p>
                <div className={styles.capabilities}>
                  {p.caps.map((c) => (
                    <span key={c} className={styles.capTag}>
                      {c}
                    </span>
                  ))}
                </div>
              </div>

              <div className={styles.actionRow}>
                <span style={{ fontSize: "12px", color: "var(--color-text-secondary)" }}>
                  <Cpu size={14} style={{ verticalAlign: "middle", marginRight: 4 }} />
                  {p.id}
                </span>
                <Link to="/settings/connections" className={styles.manageLink}>
                  Configure <ExternalLink size={12} style={{ verticalAlign: "middle" }} />
                </Link>
              </div>
            </div>
          ))}
        </div>
      </PageSection>
    </PageLayout>
  );
}
