/**
 * Settings Section 18: Usage Stats — Calibrated 1:1 to ZCode Screenshots #56–#57.
 *
 * Implements:
 * - 5 horizontal KPI summary cards (`Total tokens`, `Peak tokens`, `Longest session`, `Current streak`, `Longest streak`)
 * - 12-month `Token activity` heatmap with month axis (`Dec`..`Nov`) and `Daily` | `Weekly` | `Cumulative` switcher
 * - `Time range` (`Last 7 days` | `Last 30 days`) + `Daily token trend chart`
 * - `Model usage` SVG donut chart with center total & right-hand breakdown + `Refresh` button
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Button } from "../../components/primitives";
import { INITIAL_USAGE_STATS } from "../settingsConfig";

const MONTHS = ["Dec", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov"] as const;

export function UsageStatsSection(): ReactElement {
  const { t } = useTranslation();
  const [heatmapMode, setHeatmapMode] = useState<"daily" | "weekly" | "cumulative">("cumulative");
  const [range, setRange] = useState<"7d" | "30d">("7d");
  const [refreshCount, setRefreshCount] = useState(0);

  const stats = INITIAL_USAGE_STATS;
  const trendData = range === "7d" ? stats.dailyTrend7d : stats.dailyTrend30d;
  const maxTrend = Math.max(...trendData.map((d) => d.tokensK), 1);

  const summaryCards = [
    { label: t("ui.settings.dataGov.totalTokens", { defaultValue: "Total tokens" }), value: stats.totalTokens },
    { label: t("ui.settings.dataGov.peakTokens", { defaultValue: "Peak tokens" }), value: stats.peakTokens },
    { label: t("ui.settings.dataGov.longestSession", { defaultValue: "Longest session" }), value: stats.longestSession },
    { label: t("ui.settings.dataGov.currentStreak", { defaultValue: "Current streak" }), value: stats.currentStreak },
    { label: t("ui.settings.dataGov.longestStreak", { defaultValue: "Longest streak" }), value: stats.longestStreak },
  ];

  const circumference = 2 * Math.PI * 38;
  let cumulativePercent = 0;

  return (
    <div className="ky-settings-content" data-testid="settings-section-usage-stats" data-refresh={refreshCount}>
      <div className="ky-usage-summary-grid" data-testid="usage-summary-cards">
        {summaryCards.map((card) => (
          <div key={card.label} className="ky-usage-stat-box">
            <span className="ky-usage-stat-box__val">{card.value}</span>
            <span className="ky-usage-stat-box__label">{card.label}</span>
          </div>
        ))}
      </div>

      <div className="ky-settings-card" data-testid="usage-token-heatmap">
        <div className="ky-settings-toolbar">
          <h3 className="ky-settings-card__title">
            {t("ui.settings.dataGov.tokenActivity", { defaultValue: "Token activity" })}
          </h3>
          <div className="ky-segmented-tabs" role="tablist">
            {(["daily", "weekly", "cumulative"] as const).map((m) => (
              <button
                key={m}
                type="button"
                role="tab"
                aria-selected={heatmapMode === m}
                className="ky-segmented-tabs__btn"
                data-active={heatmapMode === m ? "true" : "false"}
                onClick={() => setHeatmapMode(m)}
              >
                {m[0].toUpperCase() + m.slice(1)}
              </button>
            ))}
          </div>
        </div>

        <div className="ky-heatmap-grid" role="img" aria-label={`12-month ${heatmapMode} token activity heatmap`}>
          {stats.heatmapLevels.map((lvl, idx) => {
            const adjusted =
              heatmapMode === "cumulative"
                ? Math.min(4, Math.floor((idx / stats.heatmapLevels.length) * 5))
                : heatmapMode === "weekly"
                  ? (lvl + 1) % 5
                  : lvl;
            return (
              <div
                key={`hm-${idx}`}
                className="ky-heatmap-cell"
                data-level={adjusted}
                title={`Slot ${idx + 1}: level ${adjusted}`}
              />
            );
          })}
        </div>

        <div className="ky-heatmap-months" aria-hidden="true">
          {MONTHS.map((m) => (
            <span key={m}>{m}</span>
          ))}
        </div>
      </div>

      <div className="ky-settings-toolbar">
        <span style={{ fontSize: "0.84rem", fontWeight: 600, color: "var(--text-primary)" }}>
          {t("ui.settings.dataGov.dailyTrend", { defaultValue: "Daily token trend chart" })}
        </span>
        <div className="ky-segmented-tabs" role="tablist">
          <button
            type="button"
            role="tab"
            aria-selected={range === "7d"}
            className="ky-segmented-tabs__btn"
            data-active={range === "7d" ? "true" : "false"}
            onClick={() => setRange("7d")}
          >
            {t("ui.settings.dataGov.last7Days", { defaultValue: "Last 7 days" })}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={range === "30d"}
            className="ky-segmented-tabs__btn"
            data-active={range === "30d" ? "true" : "false"}
            onClick={() => setRange("30d")}
          >
            {t("ui.settings.dataGov.last30Days", { defaultValue: "Last 30 days" })}
          </button>
        </div>
      </div>

      <div className="ky-settings-card">
        <div className="ky-bar-chart" role="img" aria-label={`Daily token trend for ${range}`}>
          {trendData.map((d) => (
            <div key={d.day} className="ky-bar-chart__col">
              <div
                className="ky-bar-chart__bar"
                style={{ height: `${Math.max(10, Math.round((d.tokensK / maxTrend) * 95))}%` }}
                title={`${d.day}: ${d.tokensK}K tokens`}
              />
              <span style={{ fontSize: "0.7rem", color: "var(--text-muted)" }}>{d.day}</span>
            </div>
          ))}
        </div>
      </div>

      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">
          {t("ui.settings.dataGov.modelUsage", { defaultValue: "Model usage" })}
        </h3>
        <div style={{ display: "flex", alignItems: "center", gap: "28px", flexWrap: "wrap" }}>
          <div className="ky-donut-wrap">
            <svg width="120" height="120" viewBox="0 0 100 100" role="img" aria-label="Model usage donut chart">
              {stats.modelShares.map((share) => {
                const dashLength = (share.percent / 100) * circumference;
                const dashOffset = -(cumulativePercent / 100) * circumference;
                cumulativePercent += share.percent;
                return (
                  <circle
                    key={share.model}
                    cx="50"
                    cy="50"
                    r="38"
                    fill="transparent"
                    stroke={share.color}
                    strokeWidth="12"
                    strokeDasharray={`${dashLength} ${circumference - dashLength}`}
                    strokeDashoffset={dashOffset}
                    transform="rotate(-90 50 50)"
                  />
                );
              })}
            </svg>
            <div className="ky-donut-center">
              <span className="ky-donut-center__val">{stats.totalTokens}</span>
              <span className="ky-donut-center__sub">tokens</span>
            </div>
          </div>

          <div style={{ display: "flex", flexDirection: "column", gap: "10px", flex: 1, minWidth: "200px" }}>
            {stats.modelShares.map((share) => (
              <div
                key={share.model}
                style={{ display: "flex", alignItems: "center", justifyContent: "space-between", fontSize: "0.84rem" }}
              >
                <span style={{ display: "inline-flex", alignItems: "center", gap: "8px" }}>
                  <span
                    style={{
                      width: "9px",
                      height: "9px",
                      borderRadius: "999px",
                      backgroundColor: share.color,
                    }}
                  />
                  <span>{share.model}</span>
                </span>
                <strong style={{ color: "var(--text-primary)" }}>{share.percent}%</strong>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div style={{ display: "flex", justifyContent: "flex-end" }}>
        <Button size="sm" variant="secondary" onClick={() => setRefreshCount((c) => c + 1)}>
          <ChromeIcon name="refresh" size={14} />
          <span>{t("ui.settings.dataGov.refresh", { defaultValue: "Refresh" })}</span>
        </Button>
      </div>
    </div>
  );
}
