import { useEffect, useState } from "preact/hooks";
import { api, type ActivityResponse, type AppTime, type Presence } from "./api";
import type { Dict, Lang } from "./i18n";

export function fmtDur(sec: number, t: Dict) {
  const s = Math.max(0, Math.round(sec));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (h === 0) return `${m}${t.m}`;
  return `${h}${t.h} ${String(m).padStart(2, "0")}${t.m}`;
}

// A stable color per program name, so GoLand looks the same everywhere.
export function appColor(app: string) {
  let h = 0;
  for (const ch of app) h = (h * 31 + ch.charCodeAt(0)) % 360;
  return `hsl(${h} 70% 58%)`;
}

/** Discord-like "now playing" line. */
export function PresenceLine({ p, t, now }: { p?: Presence; t: Dict; now: number }) {
  if (!p) return <p class="presence off">{t.noClient}</p>;
  if (!p.online) return <p class="presence off"><span class="pdot" /> {t.offlineUser}</p>;
  if (p.idle) return <p class="presence idle"><span class="pdot" /> {t.afk}</p>;
  if (!p.app) return <p class="presence on"><span class="pdot" /> {t.now}</p>;
  const since = p.since ? Math.floor((now - new Date(p.since).getTime()) / 1000) : 0;
  return (
    <p class="presence on">
      <span class="pdot" /> {t.now} {t.nowIn} <b>{p.app}</b>
      {since > 0 && <span class="mono"> · {fmtDur(since, t)}</span>}
    </p>
  );
}

/** Horizontal bars of time per program. */
export function AppBars({ apps, t, limit = 5 }: { apps: AppTime[]; t: Dict; limit?: number }) {
  const shown = apps.slice(0, limit);
  const max = Math.max(1, ...shown.map((a) => a.seconds));
  return (
    <ul class="bars">
      {shown.map((a) => (
        <li key={a.app}>
          <div class="bar-label">
            <span>{a.app}</span>
            <span class="mono">{fmtDur(a.seconds, t)}</span>
          </div>
          <div class="bar-track">
            <div class="bar-fill" style={{ width: `${(a.seconds / max) * 100}%`, background: appColor(a.app) }} />
          </div>
        </li>
      ))}
    </ul>
  );
}

const REFRESH_MS = 5 * 60_000;

/** Week statistics for both users. */
export function WeekStats({ t, lang, me }: { t: Dict; lang: Lang; me: string }) {
  const [data, setData] = useState<ActivityResponse | null>(null);

  useEffect(() => {
    const load = () => api.activity(7).then(setData).catch(() => undefined);
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, []);

  if (!data || data.users.every((u) => u.total_seconds === 0)) return null;
  const users = [...data.users].sort((a, b) => (a.name === me ? -1 : b.name === me ? 1 : 0));
  const dayMax = Math.max(1, ...users.flatMap((u) => u.by_day.map((d) => d.seconds)));
  const locale = lang === "uk" ? "uk-UA" : "en-GB";

  return (
    <section class="card week">
      <div class="tape" aria-hidden="true" />
      <h2>{t.weekTitle}</h2>
      <div class="week-grid">
        {users.map((u) => (
          <div key={u.name} class="week-user">
            <div class="week-head">
              <b>{u.name}</b>
              <span class="muted">
                {t.total}: <span class="mono">{fmtDur(u.total_seconds, t)}</span>
              </span>
            </div>
            <div class="days" role="img" aria-label={`${u.name}: ${fmtDur(u.total_seconds, t)}`}>
              {u.by_day.map((d) => (
                <div key={d.day} class="day" title={`${d.day}: ${fmtDur(d.seconds, t)}`}>
                  <div class="day-bar" style={{ height: `${Math.max(2, (d.seconds / dayMax) * 100)}%` }} />
                  <span class="day-label">
                    {new Date(d.day + "T12:00:00").toLocaleDateString(locale, { weekday: "short" })}
                  </span>
                </div>
              ))}
            </div>
            {u.apps.length > 0 && <AppBars apps={u.apps} t={t} limit={6} />}
          </div>
        ))}
      </div>
    </section>
  );
}
