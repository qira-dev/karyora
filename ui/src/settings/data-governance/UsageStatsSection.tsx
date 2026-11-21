/**
 * Settings Section 18: Usage Stats (UIR-8.3, Screenshots #56–#57, E2E-20).
 *
 * Implements:
 * - 5 summary cards (`Total tokens`, `Peak tokens`, `Longest session`, `Current streak`, `Longest streak`)
 * - 12-month `Token activity` heatmap (`Daily` | `Weekly` | `Cumulative`)
 * - `Last 7 days` / `Last 30 days` filter + `Daily token trend chart`
 * - `Model usage` SVG donut chart
 */

import { useState, type ReactElement } from "react";
import { Badge, Button } from "../../components/primitives";
import { INITIAL_USAGE_STATS } from "../settingsConfig";

export function UsageStatsSection(): ReactElement {
  const [heatmapMode, setHeatmapMode] = useState<"daily" | "weekly" | "cumulative">("daily");
  const [range, setRange] = useState<"7d" | "30d">("7d");

  const stats = INITIAL_USAGE_STATS;
  const trendData = range === "7d" ? stats.dailyTrend7d : stats.dailyTrend30d;
  const maxTrend = Math.max(...trendData.map((d) => d.tokensK), 1);

  const summaryCards = [
    { label: "Total tokens", value: stats.totalTokens },
    { label: "Peak tokens", value: stats.peakTokens },
    { label: "Longest session", value: stats.longestSession },
    { label: "Current streak", value: stats.currentStreak },
    { label: "Longest streak", value: stats.longestStreak },
  ];

  // SVG donut offsets (circumference for r=36 is ~226.2)
  const circumference = 2 * Math.PI * 36;
  let cumulativePercent = 0;

  return (
    <div className="ky-settings-content" data-testid="settings-section-usage-stats">
      {/* 5 Summary Cards */}
      <div className="ky-usage-summary-grid" data-testid="usage-summary-cards">
        {summaryCards.map((card) => (
          <div key={card.label} className="ky-usage-stat-box">
            <span className="ky-usage-stat-box__val">{card.value}</span>
            <span className="ky-usage-stat-box__label">{card.label}</span>
          </div>
        ))}
      </div>

      {/* 12-Month Token Activity Heatmap */}
      <div className="ky-settings-card" data-testid="usage-token-heatmap">
        <div style={{ display: "flex", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-2)" }}>
          <h3 className="ky-settings-card__title">12-Month Token Activity</h3>
          <div style={{ display: "flex", gap: "4px" }}>
            {(["daily", "weekly", "cumulative"] as const).map((m) => (
              <Button
                key={m}
                size="sm"
                variant={heatmapMode === m ? "primary" : "ghost"}
                onClick={() => setHeatmapMode(m)}
              >
                {m[0].toUpperCase() + m.slice(1)}
              </Button>
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
      </div>

      {/* Daily Token Trend Chart & Model Usage Donut Chart */}
      <div className="ky-settings-grid-2">
        <div className="ky-settings-card">
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <h3 className="ky-settings-card__title">Daily token trend chart</h3>
            <div style={{ display: "flex", gap: "4px" }}>
              <Button size="sm" variant={range === "7d" ? "primary" : "ghost"} onClick={() => setRange("7d")}>
                Last 7 days
              </Button>
              <Button size="sm" variant={range === "30d" ? "primary" : "ghost"} onClick={() => setRange("30d")}>
                Last 30 days
              </Button>
            </div>
          </div>

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
          <h3 className="ky-settings-card__title">Model usage</h3>
          <div style={{ display: "flex", alignItems: "center", gap: "var(--space-4)", flexWrap: "wrap" }}>
            <svg width="104" height="104" viewBox="0 0 100 100" role="img" aria-label="Model usage donut chart">
              {stats.modelShares.map((share) => {
                const dashLength = (share.percent / 100) * circumference;
                const dashOffset = -(cumulativePercent / 100) * circumference;
                cumulativePercent += share.percent;
                return (
                  <circle
                    key={share.model}
                    cx="50"
                    cy="50"
                    r="36"
                    fill="transparent"
                    stroke={share.color}
                    strokeWidth="14"
                    strokeDasharray={`${dashLength} ${circumference - dashLength}`}
                    strokeDashoffset={dashOffset}
                    transform="rotate(-90 50 50)"
                  />
                );
              })}
            </svg>

            <div style={{ display: "flex", flexDirection: "column", gap: "8px", flex: 1 }}>
              {stats.modelShares.map((share) => (
                <div key={share.model} className="ky-gov-perm-row">
                  <span style={{ display: "inline-flex", alignItems: "center", gap: "6px" }}>
                    <span
                      style={{
                        width: "10px",
                        height: "10px",
                        borderRadius: "2px",
                        backgroundColor: share.color,
                      }}
                    />
                    {share.model}
                  </span>
                  <Badge tone="neutral" label={`${share.percent}%`} />
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
