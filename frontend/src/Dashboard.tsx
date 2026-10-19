import { useCallback, useEffect, useState } from "preact/hooks";
import { api, ApiError, type Me, type StatusResponse, type UserStatus } from "./api";
import type { Dict, Lang } from "./i18n";
import { AppBars, PresenceLine, WeekStats } from "./Activity";

const POLL_MS = 30_000;

function useNow(intervalMs = 1000) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

function fmtLeft(ms: number) {
  const s = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  return `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
}

function fmtDay(day: string, lang: Lang) {
  if (!day) return "";
  const d = new Date(day + "T12:00:00");
  return d.toLocaleDateString(lang === "uk" ? "uk-UA" : "en-GB", { day: "numeric", month: "long" });
}

const stateIcon: Record<UserStatus["state"], string> = {
  safe: "✅",
  pending: "⏳",
  warning: "⚠️",
  failed: "💀",
};

export function Dashboard({
  t,
  lang,
  me,
  onUnauthorized,
}: {
  t: Dict;
  lang: Lang;
  me: Me;
  onUnauthorized: () => void;
}) {
  const [data, setData] = useState<StatusResponse | null>(null);
  const [offline, setOffline] = useState(false);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);
  const [pinging, setPinging] = useState(false);
  const [toast, setToast] = useState("");
  const [note, setNote] = useState("");

  const load = useCallback(async () => {
    try {
      const s = await api.status();
      setData(s);
      setOffline(false);
      setUpdatedAt(new Date());
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onUnauthorized();
      else setOffline(true);
    }
  }, [onUnauthorized]);

  useEffect(() => {
    load();
    const id = setInterval(load, POLL_MS);
    const onFocus = () => load();
    window.addEventListener("focus", onFocus);
    return () => {
      clearInterval(id);
      window.removeEventListener("focus", onFocus);
    };
  }, [load]);

  const ping = async () => {
    setPinging(true);
    try {
      const r = await api.ping(note.trim() || "web");
      setToast(r.counted ? t.pingCounted : t.pingAgain);
      setNote("");
      await load();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onUnauthorized();
      else setToast(t.errNetwork);
    } finally {
      setPinging(false);
      setTimeout(() => setToast(""), 4000);
    }
  };

  if (!data) return <div class="spinner" aria-label="loading" />;

  const mine = data.users.find((u) => u.name === me.name);
  const ordered = [...data.users].sort((a, b) => (a.name === me.name ? -1 : b.name === me.name ? 1 : 0));

  return (
    <div class="dash">
      {offline && <div class="banner">{t.offline}</div>}

      <div class="grid">
        {ordered.map((u) => (
          <UserCard key={u.name} u={u} isMe={u.name === me.name} t={t} lang={lang} />
        ))}
      </div>

      {mine && (
        <section class="card action">
          <input
            class="note"
            type="text"
            maxLength={140}
            placeholder={t.notePlaceholder}
            value={note}
            onInput={(e) => setNote((e.target as HTMLInputElement).value)}
            disabled={pinging}
          />
          <button class={mine.done_today ? "ghost big" : "primary big"} onClick={ping} disabled={pinging}>
            {pinging ? t.pinging : mine.done_today ? t.pingDone : t.pingBtn}
          </button>
          {toast && (
            <p class="toast" role="status">
              {toast}
            </p>
          )}
        </section>
      )}

      <WeekStats t={t} lang={lang} me={me.name} />

      {updatedAt && (
        <p class="updated">
          {t.updated} {updatedAt.toLocaleTimeString(lang === "uk" ? "uk-UA" : "en-GB")}
        </p>
      )}
    </div>
  );
}

function UserCard({ u, isMe, t, lang }: { u: UserStatus; isMe: boolean; t: Dict; lang: Lang }) {
  const now = useNow();
  const left = new Date(u.deadline).getTime() - now;
  return (
    <article class={`card user state-${u.state}`}>
      <div class="tape" aria-hidden="true" />
      <header class="user-head">
        <h2>
          {u.name}
          {isMe && <span class="me"> · {t.you}</span>}
        </h2>
        <span class="badge">
          {stateIcon[u.state]} {t.state[u.state]}
        </span>
      </header>

      <PresenceLine p={u.presence} t={t} now={now} />

      <div class="streak">
        <span class="fire" aria-hidden="true">
          🔥
        </span>
        <span class="num">{u.streak}</span>
        <span class="unit">{t.days(u.streak)}</span>
      </div>
      <p class="best">
        {t.best}: <b>{u.best}</b> {t.days(u.best)}
      </p>

      <div class="countdown">
        <span class="mono">{fmtLeft(left)}</span>
        <span class="muted"> {t.left}</span>
      </div>

      <dl class="meta">
        <div>
          <dt>{t.lastActive}</dt>
          <dd>{u.last_active_day ? fmtDay(u.last_active_day, lang) : t.never}</dd>
        </div>
        {u.last_failure && (
          <div>
            <dt>{t.lastFail}</dt>
            <dd>{fmtDay(u.last_failure.day, lang)}</dd>
          </div>
        )}
      </dl>

      <div class="today-apps">
        <h3>{t.todayApps}</h3>
        {u.today_apps?.length ? <AppBars apps={u.today_apps} t={t} /> : <p class="muted small">{t.noApps}</p>}
      </div>
    </article>
  );
}
