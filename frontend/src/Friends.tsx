import { useEffect, useRef, useState } from "preact/hooks";
import { api, ApiError, type LeaderFilter, type LeaderRow, type LeaderSort, type UsersResponse } from "./api";
import type { Dict, Lang } from "./i18n";
import { fmtDur } from "./Activity";
import { useNow } from "./Dashboard";
import { RankBadge } from "./Progress";

const POLL_MS = 30_000;
const LIMIT = 50;
const DEBOUNCE_MS = 250;
const SORTS: LeaderSort[] = ["streak", "best", "xp", "name", "seen"];

/** Leaderboard of every participant: search, tabs, sort and pages. */
export function FriendsPage({
  t,
  lang,
  me,
  active,
  onUnauthorized,
}: {
  t: Dict;
  lang: Lang;
  me: string;
  /** The page is on screen: fetch and poll only then. */
  active: boolean;
  onUnauthorized: () => void;
}) {
  const [input, setInput] = useState("");
  const [q, setQ] = useState("");
  const [filter, setFilter] = useState<LeaderFilter>("all");
  const [sort, setSort] = useState<LeaderSort>("streak");
  const [page, setPage] = useState(1);
  const [data, setData] = useState<UsersResponse | null>(null);
  const [offline, setOffline] = useState(false);
  const [loading, setLoading] = useState(false);
  // Pills show minutes, not seconds: a slow tick is enough.
  const now = useNow(15_000);

  // Typing restarts the search after a short pause and goes back to page 1.
  useEffect(() => {
    const id = setTimeout(() => {
      setQ(input.trim());
      setPage(1);
    }, DEBOUNCE_MS);
    return () => clearTimeout(id);
  }, [input]);

  // One request in flight: a newer query aborts the older one.
  const ctrl = useRef<AbortController | null>(null);
  useEffect(() => {
    if (!active) return;
    const load = async () => {
      ctrl.current?.abort();
      const c = new AbortController();
      ctrl.current = c;
      setLoading(true);
      try {
        const r = await api.users({ q, filter, sort, page, limit: LIMIT }, c.signal);
        setData(r);
        setOffline(false);
        if (r.page !== page) setPage(r.page); // the list shrank under us
      } catch (e) {
        if (c.signal.aborted) return;
        if (e instanceof ApiError && e.status === 401) onUnauthorized();
        else setOffline(true);
      } finally {
        if (ctrl.current === c) setLoading(false);
      }
    };
    load();
    const id = setInterval(load, POLL_MS);
    const onFocus = () => load();
    window.addEventListener("focus", onFocus);
    return () => {
      clearInterval(id);
      window.removeEventListener("focus", onFocus);
      ctrl.current?.abort();
    };
  }, [active, q, filter, sort, page, onUnauthorized]);

  const pick = (f: LeaderFilter) => {
    setFilter(f);
    setPage(1);
  };

  if (!data) return <div class="spinner" aria-label="loading" />;

  const pages = Math.max(1, Math.ceil(data.total / data.limit));
  const from = data.total ? (data.page - 1) * data.limit + 1 : 0;
  const to = Math.min(data.page * data.limit, data.total);
  // Bars compare members on the same page, not each one with itself.
  const weekMax = Math.max(1, ...data.users.flatMap((u) => u.week));
  const meShown = data.users.some((u) => u.name === me);
  const pinMe = data.me_row && !meShown && q === "" && filter === "all";

  return (
    <div class="friends">
      {offline && <div class="banner">{t.offline}</div>}

      <div class="friends-head">
        <div class="page-head">
          <p class="eyebrow">{t.friendsEyebrow}</p>
          <h1>
            {t.friendsTitle} <span class="muted">· {data.counts.all}</span>
          </h1>
        </div>
        <div class="tabs" role="group" aria-label={t.friendsTitle}>
          {(
            [
              ["all", t.tabAll, data.counts.all],
              ["online", t.tabOnline, data.counts.online],
              ["risk", t.tabRisk, data.counts.at_risk],
            ] as const
          ).map(([f, label, n]) => (
            <button
              key={f}
              class={filter === f ? "primary small" : `ghost small${f === "risk" && n > 0 ? " tab-risk" : ""}`}
              aria-pressed={filter === f}
              onClick={() => pick(f)}
            >
              {label} · {n}
            </button>
          ))}
        </div>
      </div>

      {data.at_risk.length > 0 && (
        <section class="card risk" aria-label={t.riskTitle}>
          <div class="tape" aria-hidden="true" />
          <div class="risk-head">
            <h2>
              {t.riskTitle} <span class="muted">· {t.riskHint}</span>
            </h2>
            {data.counts.at_risk > data.at_risk.length && filter !== "risk" && (
              <button class="ghost small" onClick={() => pick("risk")}>
                {t.riskShowAll(data.counts.at_risk)}
              </button>
            )}
          </div>
          <ul class="risk-list">
            {data.at_risk.map((u) => (
              <li key={u.name} class="risk-item">
                <Avatar u={u} />
                <div class="risk-main">
                  <b>
                    {u.name}
                    {u.name === me && <span class="me"> · {t.you}</span>}
                  </b>
                  <span class="mono">
                    {u.streak} {t.days(u.streak)} · {t.pillLeft(fmtDur(left(u, now) / 1000, t))}
                  </span>
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}

      <div class="friends-tools">
        <label class="search">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <circle cx="11" cy="11" r="7" />
            <path d="m20 20-3.5-3.5" />
          </svg>
          <span class="sr-only">{t.friendsSearch}</span>
          <input
            type="search"
            value={input}
            maxLength={64}
            placeholder={t.friendsSearchPh}
            onInput={(e) => setInput((e.target as HTMLInputElement).value)}
          />
        </label>
        <label class="sort">
          {t.sortLabel}
          <select
            value={sort}
            onChange={(e) => {
              setSort((e.target as HTMLSelectElement).value as LeaderSort);
              setPage(1);
            }}
          >
            {SORTS.map((s) => (
              <option key={s} value={s}>
                {t.sorts[s]}
              </option>
            ))}
          </select>
        </label>
      </div>

      <section class={`card board${loading ? " is-loading" : ""}`} aria-busy={loading}>
        {data.users.length === 0 ? (
          <div class="board-empty">
            <p class="mono board-empty-err">err != nil</p>
            <p>
              <b>{filter === "risk" && !q ? t.nobodyRisk : filter === "online" && !q ? t.nobodyOnline : t.nobody}</b>
            </p>
            {q && <p class="muted">{t.nobodyHint}</p>}
          </div>
        ) : (
          <div class="board-scroll">
            <table class="board-table">
              <thead>
                <tr>
                  <th scope="col" class="c-pos">{t.cols.pos}</th>
                  <th scope="col" class="c-user">{t.cols.user}</th>
                  <th scope="col">{t.cols.today}</th>
                  <th scope="col" class="c-num" aria-sort={sort === "streak" ? "descending" : undefined}>{t.cols.streak}</th>
                  <th scope="col" class="c-num" aria-sort={sort === "best" ? "descending" : undefined}>{t.cols.best}</th>
                  <th scope="col" aria-sort={sort === "xp" ? "descending" : undefined}>{t.cols.rank}</th>
                  <th scope="col">{t.cols.week}</th>
                  <th scope="col" aria-sort={sort === "seen" ? "descending" : undefined}>{t.cols.seen}</th>
                </tr>
              </thead>
              <tbody>
                {data.users.map((u) => (
                  <Row key={u.name} u={u} isMe={u.name === me} weekMax={weekMax} now={now} t={t} lang={lang} />
                ))}
              </tbody>
              {pinMe && data.me_row && (
                <tbody class="board-pinned">
                  <tr class="board-sep">
                    <th scope="rowgroup" colSpan={8}>{t.myPlace}</th>
                  </tr>
                  <Row u={data.me_row} isMe weekMax={weekMax} now={now} t={t} lang={lang} />
                </tbody>
              )}
            </table>
          </div>
        )}

        {data.total > 0 && (
          <nav class="board-foot" aria-label={t.friendsTitle}>
            <span class="muted">{t.showing(from, to, data.total)}</span>
            {pages > 1 && (
              <div class="pager">
                <button class="ghost small" aria-label={t.prevPage} disabled={data.page <= 1} onClick={() => setPage(data.page - 1)}>
                  ‹
                </button>
                {pageList(data.page, pages).map((p, i) =>
                  p === 0 ? (
                    <span key={`gap${i}`} class="pager-gap" aria-hidden="true">
                      …
                    </span>
                  ) : (
                    <button
                      key={p}
                      class={p === data.page ? "primary small" : "ghost small"}
                      aria-label={t.pageN(p)}
                      aria-current={p === data.page ? "page" : undefined}
                      onClick={() => setPage(p)}
                    >
                      {p}
                    </button>
                  ),
                )}
                <button class="ghost small" aria-label={t.nextPage} disabled={data.page >= pages} onClick={() => setPage(data.page + 1)}>
                  ›
                </button>
              </div>
            )}
          </nav>
        )}
      </section>
    </div>
  );
}

function Row({
  u,
  isMe,
  weekMax,
  now,
  t,
  lang,
}: {
  u: LeaderRow;
  isMe: boolean;
  weekMax: number;
  now: number;
  t: Dict;
  lang: Lang;
}) {
  const weekTotal = u.week.reduce((a, b) => a + b, 0);
  return (
    <tr class={`state-${u.state}${isMe ? " is-me" : ""}`}>
      <td class="c-pos mono">{String(u.pos).padStart(2, "0")}</td>
      <td>
        <div class="who">
          <Avatar u={u} />
          <div class="who-main">
            <b>
              {u.name}
              {isMe && <span class="me"> · {t.you}</span>}
            </b>
            <span class="who-sub">{presenceText(u, t)}</span>
          </div>
        </div>
      </td>
      <td>
        <span class="pill" title={u.state === "warning" ? t.pillLeft(fmtDur(left(u, now) / 1000, t)) : undefined}>
          {pillText(u, now, t)}
        </span>
      </td>
      <td class="c-num">
        <span class="board-streak">{u.streak}</span>
      </td>
      <td class="c-num mono muted">{u.best}</td>
      <td>
        <span class="board-rank" title={`${u.xp} XP`}>
          <RankBadge level={u.rank} size={26} />
          <span>{u.rank_name}</span>
        </span>
      </td>
      <td>
        <div class="spark" role="img" aria-label={t.weekAria(u.name, fmtDur(weekTotal, t))}>
          {u.week.map((s, i) => (
            <span key={i} style={{ height: `${s ? Math.max(12, (s / weekMax) * 100) : 6}%` }} class={s ? "" : "zero"} />
          ))}
        </div>
      </td>
      <td class="muted nowrap">{seenText(u, now, t, lang)}</td>
    </tr>
  );
}

function Avatar({ u }: { u: LeaderRow }) {
  const p = u.presence;
  const dot = !p || !p.online ? "off" : p.idle ? "idle" : "on";
  return (
    <span class="avatar" aria-hidden="true">
      {u.name.slice(0, 1).toUpperCase()}
      <span class={`avatar-dot ${dot}`} />
    </span>
  );
}

const left = (u: LeaderRow, now: number) => Math.max(0, new Date(u.deadline).getTime() - now);

function pillText(u: LeaderRow, now: number, t: Dict) {
  if (u.state === "safe") return t.pillDone;
  if (u.state === "failed") return t.state.failed;
  // Only a burning deadline is worth the countdown; the rest just wait.
  if (u.state === "pending") return t.pillPending;
  return fmtDur(left(u, now) / 1000, t);
}

function presenceText(u: LeaderRow, t: Dict) {
  const p = u.presence;
  if (!p) return t.noClient;
  if (!p.online) return t.offlineUser;
  if (p.idle) return t.afk;
  return p.app ? `${t.nowIn} ${p.app}` : t.now;
}

function seenText(u: LeaderRow, now: number, t: Dict, lang: Lang) {
  const p = u.presence;
  if (!p) return t.seenNever;
  if (p.online) return t.seenNow;
  const rtf = new Intl.RelativeTimeFormat(lang === "uk" ? "uk" : "en", { numeric: "auto" });
  const min = Math.round((now - new Date(p.updated_at).getTime()) / 60_000);
  if (min < 60) return rtf.format(-Math.max(1, min), "minute");
  if (min < 24 * 60) return rtf.format(-Math.round(min / 60), "hour");
  return rtf.format(-Math.round(min / 1440), "day");
}

/** Page buttons: 1 … p-1 p p+1 … last (0 marks a gap). */
function pageList(page: number, pages: number): number[] {
  const want = new Set([1, pages, page - 1, page, page + 1].filter((p) => p >= 1 && p <= pages));
  const sorted = [...want].sort((a, b) => a - b);
  const out: number[] = [];
  for (const p of sorted) {
    if (out.length && p - out[out.length - 1] > 1) out.push(0);
    out.push(p);
  }
  return out;
}
