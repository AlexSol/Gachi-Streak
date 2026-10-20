import type { ComponentChildren } from "preact";
import { useEffect, useRef, useState } from "preact/hooks";
import {
  api,
  ApiError,
  type FriendRequest,
  type FriendRequestsResponse,
  type LeaderFilter,
  type LeaderRow,
  type LeaderScope,
  type LeaderSort,
  type Presence,
  type UsersResponse,
} from "./api";
import type { Dict, Lang } from "./i18n";
import { fmtDur } from "./Activity";
import { useNow } from "./Dashboard";
import { RankBadge } from "./Progress";

const POLL_MS = 30_000;
const LIMIT = 50;
const DEBOUNCE_MS = 250;
const SORTS: LeaderSort[] = ["streak", "best", "xp", "name", "seen"];

/** Friends (me + accepted friends), everyone, or pending requests. */
type View = LeaderScope | "requests";
type Act = "add" | "accept" | "decline" | "cancel" | "remove";

const ACTS: Record<Act, (name: string) => Promise<unknown>> = {
  add: api.addFriend,
  accept: api.acceptFriend,
  decline: api.declineFriend,
  cancel: api.cancelFriend,
  remove: api.removeFriend,
};

/** Friends page: the friends' leaderboard, everyone, and friend requests. */
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
  const [view, setView] = useState<View>("friends");
  // Requests view keeps the friends board loaded: it feeds the tab counters.
  const scope: LeaderScope = view === "all" ? "all" : "friends";
  const [reload, setReload] = useState(0);
  const [busy, setBusy] = useState<string | null>(null);
  const [actErr, setActErr] = useState("");
  const [reqs, setReqs] = useState<FriendRequestsResponse | null>(null);
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
        const r = await api.users({ scope, q, filter, sort, page, limit: LIMIT }, c.signal);
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
  }, [active, scope, q, filter, sort, page, reload, onUnauthorized]);

  // Requests are only fetched while their view is open.
  useEffect(() => {
    if (!active || view !== "requests") return;
    const c = new AbortController();
    const load = () =>
      api
        .friendRequests(c.signal)
        .then((r) => {
          setReqs(r);
          setOffline(false);
        })
        .catch((e) => {
          if (c.signal.aborted) return;
          if (e instanceof ApiError && e.status === 401) onUnauthorized();
          else setOffline(true);
        });
    load();
    const id = setInterval(load, POLL_MS);
    return () => {
      clearInterval(id);
      c.abort();
    };
  }, [active, view, reload, onUnauthorized]);

  // One friend action at a time; every answer reloads the board and requests.
  const act = async (kind: Act, name: string) => {
    if (busy) return;
    if (kind === "remove" && !window.confirm(t.friendRemoveConfirm(name))) return;
    setBusy(name);
    setActErr("");
    try {
      await ACTS[kind](name);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized();
        return;
      }
      setActErr(t.friendErr(e instanceof Error ? e.message : String(e)));
    } finally {
      setBusy(null);
      setReload((n) => n + 1);
    }
  };

  const show = (v: View) => {
    setView(v);
    setPage(1);
  };

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

      {actErr && (
        <div class="banner" role="alert">
          {actErr}
        </div>
      )}

      <div class="friends-head">
        <div class="page-head">
          <p class="eyebrow">{t.friendsEyebrow}</p>
          <h1>{t.friendsTitle}</h1>
        </div>
        <div class="tabs views" role="group" aria-label={t.viewAria}>
          {(
            [
              ["friends", t.viewFriends, data.scopes.friends],
              ["all", t.viewAll, data.scopes.all],
            ] as const
          ).map(([v, label, n]) => (
            <button key={v} class={view === v ? "primary small" : "ghost small"} aria-pressed={view === v} onClick={() => show(v)}>
              {label} · {n}
            </button>
          ))}
          <button
            class={view === "requests" ? "primary small" : "ghost small"}
            aria-pressed={view === "requests"}
            onClick={() => show("requests")}
          >
            {t.viewRequests}
            {data.scopes.incoming > 0 && (
              <span class="count-badge" aria-label={t.reqNew(data.scopes.incoming)}>
                {data.scopes.incoming}
              </span>
            )}
          </button>
        </div>
      </div>

      {view === "requests" ? (
        <RequestsView reqs={reqs} busy={busy} onAct={act} now={now} t={t} lang={lang} />
      ) : (
        <>
          {data.at_risk.length > 0 && (
            <section class="card risk" aria-label={t.riskTitle}>
              <div class="tape" aria-hidden="true" />
              <div class="risk-head">
                <h2>
                  {t.riskTitle} <span class="muted">· {t.riskHint}</span>
                </h2>
                {view === "friends" && data.counts.at_risk > data.at_risk.length && filter !== "risk" && (
                  <button class="ghost small" onClick={() => pick("risk")}>
                    {t.riskShowAll(data.counts.at_risk)}
                  </button>
                )}
              </div>
              <ul class="risk-list">
                {data.at_risk.map((u) => (
                  <li key={u.name} class="risk-item">
                    <Avatar name={u.name} presence={u.presence} />
                    <div class="risk-main">
                      <b>
                        <a class="profile-link" href={`#/u/${encodeURIComponent(u.name)}`}>{u.name}</a>
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
            <div class="tabs filters" role="group" aria-label={t.filterAria}>
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

          {view === "friends" && data.scopes.friends <= 1 && (
            <section class="card alone">
              <div>
                <p>
                  <b>{t.friendsAlone}</b>
                </p>
                <p class="muted">{t.friendsAloneHint}</p>
              </div>
              <button class="primary" onClick={() => show("all")}>
                {t.friendsFind}
              </button>
            </section>
          )}

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
                      <th scope="col" class="c-act">
                        <span class="sr-only">{t.colActions}</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.users.map((u) => (
                      <Row key={u.name} u={u} isMe={u.name === me} weekMax={weekMax} now={now} t={t} lang={lang} busy={busy} onAct={act} />
                    ))}
                  </tbody>
                  {pinMe && data.me_row && (
                    <tbody class="board-pinned">
                      <tr class="board-sep">
                        <th scope="rowgroup" colSpan={9}>{t.myPlace}</th>
                      </tr>
                      <Row u={data.me_row} isMe weekMax={weekMax} now={now} t={t} lang={lang} busy={busy} onAct={act} />
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
        </>
      )}
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
  busy,
  onAct,
}: {
  u: LeaderRow;
  isMe: boolean;
  weekMax: number;
  now: number;
  t: Dict;
  lang: Lang;
  busy: string | null;
  onAct: (kind: Act, name: string) => void;
}) {
  const weekTotal = u.week.reduce((a, b) => a + b, 0);
  return (
    <tr class={`state-${u.state}${isMe ? " is-me" : ""}`}>
      <td class="c-pos mono">{String(u.pos).padStart(2, "0")}</td>
      <td>
        <div class="who">
          <Avatar name={u.name} presence={u.presence} />
          <div class="who-main">
            <b>
              <a class="profile-link" href={`#/u/${encodeURIComponent(u.name)}`}>{u.name}</a>
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
      <td class="c-act">{!isMe && <FriendButton u={u} busy={busy} onAct={onAct} t={t} />}</td>
    </tr>
  );
}

/** The friend action that fits the relation; strangers get "Add". */
function FriendButton({
  u,
  busy,
  onAct,
  t,
}: {
  u: LeaderRow;
  busy: string | null;
  onAct: (kind: Act, name: string) => void;
  t: Dict;
}) {
  const disabled = busy !== null;
  switch (u.relation) {
    case "friend":
      return (
        <button class="ghost small fr-btn" title={t.friendRemove(u.name)} aria-label={t.friendRemove(u.name)} disabled={disabled} onClick={() => onAct("remove", u.name)}>
          {t.friendIs}
        </button>
      );
    case "outgoing":
      return (
        <button class="ghost small fr-btn muted" title={t.friendCancelTitle(u.name)} disabled={disabled} onClick={() => onAct("cancel", u.name)}>
          {t.friendSent}
        </button>
      );
    case "incoming":
      return (
        <button class="primary small fr-btn" disabled={disabled} onClick={() => onAct("accept", u.name)}>
          {t.friendAccept}
        </button>
      );
    default:
      return (
        <button class="ghost small fr-btn fr-add" disabled={disabled} onClick={() => onAct("add", u.name)}>
          + {t.friendAdd}
        </button>
      );
  }
}

function RequestsView({
  reqs,
  busy,
  onAct,
  now,
  t,
  lang,
}: {
  reqs: FriendRequestsResponse | null;
  busy: string | null;
  onAct: (kind: Act, name: string) => void;
  now: number;
  t: Dict;
  lang: Lang;
}) {
  if (!reqs) return <div class="spinner" aria-label="loading" />;
  const disabled = busy !== null;
  const item = (r: FriendRequest, actions: ComponentChildren) => (
    <li key={r.name} class="req-item">
      <Avatar name={r.name} />
      <div class="req-main">
        <b>{r.name}</b>
        <span class="muted">
          {r.streak} {t.days(r.streak)} · {relTime(new Date(r.at).getTime(), now, lang)}
        </span>
      </div>
      <div class="req-actions">{actions}</div>
    </li>
  );
  return (
    <div class="reqs">
      <section class="card">
        <h2>
          {t.reqIncoming} <span class="muted">· {reqs.incoming.length}</span>
        </h2>
        {reqs.incoming.length === 0 ? (
          <p class="muted">{t.reqNoneIn}</p>
        ) : (
          <ul class="req-list">
            {reqs.incoming.map((r) =>
              item(
                r,
                <>
                  <button class="primary small" disabled={disabled} onClick={() => onAct("accept", r.name)}>
                    {t.friendAccept}
                  </button>
                  <button class="ghost small" disabled={disabled} onClick={() => onAct("decline", r.name)}>
                    {t.friendDecline}
                  </button>
                </>,
              ),
            )}
          </ul>
        )}
      </section>
      <section class="card">
        <h2>
          {t.reqOutgoing} <span class="muted">· {reqs.outgoing.length}</span>
        </h2>
        {reqs.outgoing.length === 0 ? (
          <p class="muted">{t.reqNoneOut}</p>
        ) : (
          <ul class="req-list">
            {reqs.outgoing.map((r) =>
              item(
                r,
                <button class="ghost small" title={t.friendCancelTitle(r.name)} disabled={disabled} onClick={() => onAct("cancel", r.name)}>
                  {t.friendCancel}
                </button>,
              ),
            )}
          </ul>
        )}
      </section>
    </div>
  );
}

function Avatar({ name, presence }: { name: string; presence?: Presence }) {
  const p = presence;
  const dot = !p || !p.online ? "off" : p.idle ? "idle" : "on";
  return (
    <span class="avatar" aria-hidden="true">
      {name.slice(0, 1).toUpperCase()}
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
  return relTime(new Date(p.updated_at).getTime(), now, lang);
}

/** "5 minutes ago" / "2 days ago" for a past moment (ms). */
function relTime(at: number, now: number, lang: Lang) {
  const rtf = new Intl.RelativeTimeFormat(lang === "uk" ? "uk" : "en", { numeric: "auto" });
  const min = Math.round((now - at) / 60_000);
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
