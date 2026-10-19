import { useCallback, useEffect, useMemo, useState } from "preact/hooks";
import {
  api,
  ApiError,
  type AchCategory,
  type AchievementDef,
  type AchievementState,
  type ProgressResponse,
  type RankDef,
  type UserProgress,
} from "./api";
import type { Dict, Lang } from "./i18n";
import { AchViewer, BriefsIcon, type Look } from "./AchViewer";
import { PackageArt } from "./briefs";
import { EFFECTS, pickEffect, type Effect } from "./Unbox";

// MVP only: buttons on every card that replay each opening effect.
const DEBUG_EFFECTS = true;

const flexTextFor = (xp: number) =>
  xp >= 300 ? "♂ DUNGEON MASTER ♂" : xp >= 50 ? "♂ BOSS OF THIS GYM ♂" : "♂ BOY NEXT DOOR ♂";

function PackageIcon({ id, width = 44 }: { id: string; width?: number }) {
  return (
    <svg width={width} height={(width * 160) / 200} viewBox="0 0 200 160" aria-hidden="true" class="ach-icon pkg-icon">
      <PackageArt id={id} />
    </svg>
  );
}

const POLL_MS = 60_000;

/** Loads /progress, refreshes on focus and every minute. */
function useProgress(onUnauthorized: () => void) {
  const [data, setData] = useState<ProgressResponse | null>(null);
  const [failed, setFailed] = useState(false);
  const load = useCallback(async () => {
    try {
      setData(await api.progress());
      setFailed(false);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onUnauthorized();
      else setFailed(true);
    }
  }, [onUnauthorized]);
  useEffect(() => {
    load();
    const id = setInterval(load, POLL_MS);
    window.addEventListener("focus", load);
    return () => {
      clearInterval(id);
      window.removeEventListener("focus", load);
    };
  }, [load]);
  return { data, failed };
}

function fmtDay(day: string, lang: Lang) {
  const d = new Date(day + "T12:00:00");
  return d.toLocaleDateString(lang === "uk" ? "uk-UA" : "en-GB", { day: "numeric", month: "long", year: "numeric" });
}

const fmtXP = (n: number) => n.toLocaleString("uk-UA").replace(/ /g, " ");

/** Users with "me" first. */
function ordered(users: UserProgress[], me: string) {
  return [...users].sort((a, b) => (a.name === me ? -1 : b.name === me ? 1 : 0));
}

// ---------- rank badge ----------

const rankInk = ["#6e645b", "#8a6b4a", "#a9793f", "#d9a441", "#c9c1b6", "#e0574c", "#f0c46a"];

const glyphs: Record<number, string> = {
  1: "M14 24 L22 18 L30 24",
  2: "M14 20 L22 14 L30 20 M14 28 L22 22 L30 28",
  3: "M14 17 L22 11 L30 17 M14 24 L22 18 L30 24 M14 31 L22 25 L30 31",
  5: "M11 22 H33 M14 16 V28 M30 16 V28 M10 19 V25 M34 19 V25",
  6: "M15 27 C15 18 29 18 29 27 M18 17 L16 13 M26 17 L28 13",
  7: "M12 28 L14 16 L19 22 L22 13 L25 22 L30 16 L32 28 Z",
};

export function RankBadge({ level, size = 44 }: { level: number; size?: number }) {
  const ink = rankInk[level - 1] ?? rankInk[0];
  return (
    <svg class="rank-badge" width={size} height={size} viewBox="0 0 44 44" aria-hidden="true">
      <path d="M22 3 L39 13 V31 L22 41 L5 31 V13 Z" fill="none" stroke={ink} stroke-width="2" />
      {level === 4 ? (
        <>
          <circle cx="22" cy="22" r="8" fill="none" stroke={ink} stroke-width="2.5" />
          <path d="M22 10 V14 M22 30 V34 M10 22 H14 M30 22 H34" stroke={ink} stroke-width="2.5" />
        </>
      ) : (
        <path d={glyphs[level] ?? glyphs[1]} fill="none" stroke={ink} stroke-width="2.5" stroke-linejoin="round" />
      )}
      {level === 6 && (
        <>
          <circle cx="19" cy="24" r="1.6" fill={ink} />
          <circle cx="25" cy="24" r="1.6" fill={ink} />
        </>
      )}
    </svg>
  );
}

// ---------- ranks page ----------

function RankCard({ u, ranks, isMe, t }: { u: UserProgress; ranks: RankDef[]; isMe: boolean; t: Dict }) {
  const rank = ranks.find((r) => r.level === u.rank) ?? ranks[0];
  const next = ranks.find((r) => r.level === u.rank + 1);
  const pct = next ? Math.round(((u.xp - rank.min_xp) / (next.min_xp - rank.min_xp)) * 100) : 100;
  return (
    <article class={`card rank-card${isMe ? " is-me" : ""}`}>
      <div class="rank-card-head">
        <RankBadge level={rank.level} size={64} />
        <div>
          <p class="eyebrow">
            {t.colLevel} {String(rank.level).padStart(2, "0")} · {u.name}
            {isMe && ` · ${t.you}`}
          </p>
          <h2 class="rank-name">{rank.name}</h2>
        </div>
      </div>
      <div
        class="xp-track"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
        aria-label={next ? t.xpToNext(next.min_xp - u.xp, next.name) : t.maxRank}
      >
        <div class="xp-fill" style={{ width: `${Math.max(0, Math.min(100, pct))}%` }} />
      </div>
      <p class="xp-line">
        <span class="mono">
          {fmtXP(u.xp)}
          {next && ` / ${fmtXP(next.min_xp)}`} XP
        </span>
        <span class="muted">{next ? t.xpToNext(next.min_xp - u.xp, next.name) : t.maxRank}</span>
      </p>
      <dl class="xp-breakdown" aria-label={t.xpBreakdown}>
        <div>
          <dt>{t.xpDays(u.active_days)}</dt>
          <dd class="mono">+{fmtXP(u.xp_days)}</dd>
        </div>
        <div>
          <dt>{t.xpAch}</dt>
          <dd class="mono">+{fmtXP(u.xp_achievements)}</dd>
        </div>
        <div class="neg">
          <dt>{t.xpPenalty}</dt>
          <dd class="mono">−{fmtXP(u.xp_penalty)}</dd>
        </div>
      </dl>
    </article>
  );
}

export function RanksPage({ t, me, onUnauthorized }: { t: Dict; me: string; onUnauthorized: () => void }) {
  const { data, failed } = useProgress(onUnauthorized);
  if (!data) return failed ? <div class="banner">{t.errNetwork}</div> : <div class="spinner" aria-label="loading" />;
  const users = ordered(data.users, me);
  const mine = users.find((u) => u.name === me);

  return (
    <div class="dash">
      {failed && <div class="banner">{t.offline}</div>}
      <header class="page-head">
        <p class="eyebrow">{t.ranksEyebrow}</p>
        <h1>{t.ranksTitle}</h1>
        <p class="muted">{t.ranksText(data.xp_per_day, data.xp_per_failure)}</p>
      </header>

      <div class="grid">
        {users.map((u) => (
          <RankCard key={u.name} u={u} ranks={data.ranks} isMe={u.name === me} t={t} />
        ))}
      </div>

      <section class="card ladder" aria-label={t.ranksEyebrow}>
        <ol class="ladder-list">
          {[...data.ranks].reverse().map((r) => {
            const here = users.filter((u) => u.rank === r.level);
            const isMine = mine?.rank === r.level;
            return (
              <li key={r.key} class={`ladder-row${isMine ? " is-me" : ""}${r.level < (mine?.rank ?? 0) ? " passed" : ""}`}>
                <span class="ladder-level mono">{String(r.level).padStart(2, "0")}</span>
                <RankBadge level={r.level} />
                <div class="ladder-main">
                  <div class="ladder-name">
                    {r.name}
                    {here.map((u) => (
                      <span key={u.name} class={`chip${u.name === me ? " chip-me" : ""}`}>
                        {u.name === me ? t.you : u.name}
                      </span>
                    ))}
                  </div>
                  <code class="ladder-code">{r.code}</code>
                </div>
                <div class="ladder-side">
                  <span class="mono ladder-xp">{fmtXP(r.min_xp)} XP</span>
                  <span class="muted small">{t.perks[r.key]}</span>
                </div>
              </li>
            );
          })}
        </ol>
        <p class="muted small ladder-note">{t.perkSoon}</p>
      </section>
    </div>
  );
}

// ---------- achievements page ----------

function lookOf(def: AchievementDef, st: AchievementState, codeTracking: boolean): Look {
  if (st.unlocked) return def.category === "shame" ? "shame" : "done";
  if (def.secret) return "secret";
  if (def.detector === "code" && !codeTracking) return "soon";
  return st.progress > 0 ? "progress" : "locked";
}

const cats: ("all" | AchCategory)[] = ["all", "streak", "go", "tests", "duo", "shame", "secret"];

export function AchievementsPage({
  t,
  lang,
  me,
  onUnauthorized,
  onUnlocked,
}: {
  t: Dict;
  lang: Lang;
  me: string;
  onUnauthorized: () => void;
  /** Called with my unlocks the server flagged as new (once per unlock). */
  onUnlocked?: (defs: AchievementDef[]) => void;
}) {
  const { data, failed } = useProgress(onUnauthorized);
  const [cat, setCat] = useState<"all" | AchCategory>("all");
  const [who, setWho] = useState(me);
  // real = the first unpacking (stored on the server); otherwise a preview.
  const [openState, setOpenState] = useState<{ id: string; effect: Effect | null; real: boolean; n: number } | null>(null);
  const closeViewer = useCallback(() => setOpenState(null), []);
  // Unpacked in this session, before the next /progress poll says so.
  const [unpacked, setUnpacked] = useState<Set<string>>(() => new Set());
  const openId = openState?.id ?? null;
  // Unlocks announced in this session stay highlighted until reload.
  const [fresh, setFresh] = useState<Set<string>>(() => new Set());

  useEffect(() => {
    const news = data?.users.find((u) => u.name === me)?.achievements.filter((a) => a.new) ?? [];
    if (!data || !news.length) return;
    const ids = new Set(news.map((a) => a.id));
    setFresh((prev) => new Set([...prev, ...ids]));
    onUnlocked?.(data.achievements.filter((d) => ids.has(d.id)));
    // If this fails, the next poll simply announces them again.
    api.achievementsSeen().catch(() => undefined);
  }, [data, me, onUnlocked]);

  const view = useMemo(() => {
    if (!data) return null;
    const users = ordered(data.users, me);
    const user = users.find((u) => u.name === who) ?? users[0];
    if (!user) return null;
    const others = users.filter((u) => u.name !== user.name);
    const byId = (u: UserProgress) => new Map(u.achievements.map((a) => [a.id, a]));
    const mineMap = byId(user);
    const otherMaps = others.map((o) => ({ name: o.name, map: byId(o) }));
    const items = data.achievements.map((def) => {
      const st = mineMap.get(def.id) ?? { id: def.id, unlocked: false, progress: 0, target: def.target };
      const holders = otherMaps.filter((o) => o.map.get(def.id)?.unlocked).map((o) => o.name);
      return { def, st, look: lookOf(def, st, user.code_tracking), holders };
    });
    const unlocked = items.filter((i) => i.st.unlocked);
    const newest = [...unlocked].sort((a, b) => (b.st.day ?? "").localeCompare(a.st.day ?? ""))[0];
    return {
      users,
      user,
      items,
      unlocked: unlocked.length,
      xp: user.xp_achievements,
      shame: unlocked.filter((i) => i.def.category === "shame").length,
      newest,
    };
  }, [data, who, me]);

  if (!data || !view) return failed ? <div class="banner">{t.errNetwork}</div> : <div class="spinner" aria-label="loading" />;

  const shown = view.items.filter((i) => cat === "all" || i.def.category === cat);
  const name = (id: string) => t.ach[id]?.[0] ?? id;
  const desc = (id: string) => t.ach[id]?.[1] ?? "";
  const xpText = (xp: number) => `${xp > 0 ? "+" : ""}${xp} XP`;
  const opened = openId ? view.items.find((i) => i.def.id === openId) : undefined;
  const isMeView = view.user.name === me;
  const packedOf = (id: string, packed?: boolean) => isMeView && !!packed && !unpacked.has(id);
  const open = (id: string, category: string, packed: boolean, forced?: Effect) =>
    setOpenState({ id, effect: forced ?? (packed ? pickEffect(category) : null), real: packed && !forced, n: Date.now() });
  const onEffectDone = () => {
    if (!openState?.real) return;
    const id = openState.id;
    setUnpacked((prev) => new Set(prev).add(id));
    api.openAchievement(id).catch(() => undefined); // retried by the next unpacking if it fails
  };

  return (
    <div class="dash">
      {failed && <div class="banner">{t.offline}</div>}
      <header class="page-head ach-head">
        <div>
          <p class="eyebrow">{t.achEyebrow}</p>
          <h1>{t.achTitle}</h1>
        </div>
        <div class="stats">
          <div class="stat">
            <b>
              {view.unlocked} / {view.items.length}
            </b>
            <span>{t.unlocked}</span>
          </div>
          <div class="stat">
            <b>+{fmtXP(view.xp)}</b>
            <span>{t.xpFromAch}</span>
          </div>
          <div class="stat stat-shame">
            <b>{view.shame}</b>
            <span>{t.shameful}</span>
          </div>
        </div>
      </header>

      {view.users.length > 1 && (
        <div class="seg" role="group" aria-label={t.viewOf}>
          {view.users.map((u) => (
            <button
              key={u.name}
              class={u.name === view.user.name ? "primary small" : "ghost small"}
              aria-pressed={u.name === view.user.name}
              onClick={() => setWho(u.name)}
            >
              {u.name === me ? `${u.name} · ${t.you}` : u.name}
            </button>
          ))}
        </div>
      )}

      {view.newest && (
        <section class={`card ach-newest look-${view.newest.look}`}>
          {packedOf(view.newest.def.id, view.newest.st.packed) ? (
            <PackageIcon id={view.newest.def.id} width={56} />
          ) : (
            <BriefsIcon id={view.newest.def.id} look={view.newest.look} width={56} />
          )}
          <div class="ach-newest-main">
            <p class="eyebrow">
              {t.newest}
              {view.newest.st.day && ` · ${fmtDay(view.newest.st.day, lang)}`}
            </p>
            <h2>{name(view.newest.def.id)}</h2>
            <p class="muted small">{desc(view.newest.def.id)}</p>
          </div>
          <span class="ach-newest-xp">+{view.newest.def.xp} XP</span>
        </section>
      )}

      <div class="tabs" role="group" aria-label={t.achTitle}>
        {cats.map((c) => {
          const n = c === "all" ? view.items.length : view.items.filter((i) => i.def.category === c).length;
          return (
            <button key={c} class={c === cat ? "primary small" : "ghost small"} aria-pressed={c === cat} onClick={() => setCat(c)}>
              {t.cat[c]} · {n}
            </button>
          );
        })}
      </div>

      <ul class="ach-grid">
        {shown.map(({ def, st, look, holders }) => {
          const hidden = look === "secret";
          const isNew = isMeView && fresh.has(def.id);
          const packed = packedOf(def.id, st.packed);
          const owners = holders.length
            ? holders.map((h) => t.alsoPartner(h)).join(", ")
            : st.unlocked
              ? isMeView
                ? t.onlyYou
                : ""
              : t.nobodyYet;
          return (
            <li key={def.id} class={`ach look-${look}${isNew ? " is-new" : ""}${packed ? " is-packed" : ""}`}>
              <button
                class="ach-open"
                aria-label={`${hidden ? t.secretTitle : name(def.id)} — ${packed ? t.unpack : t.viewerOpen}`}
                onClick={() => open(def.id, def.category, packed)}
              />
              <div class="ach-top">
                {packed ? <PackageIcon id={def.id} /> : <BriefsIcon id={def.id} look={look} />}
                <span class="mono ach-xp">{hidden ? "?" : xpText(def.xp)}</span>
              </div>
              <div>
                <h3>{hidden ? t.secretTitle : name(def.id)}</h3>
                <p class="small muted">{hidden ? (t.ach[def.id]?.[2] ?? "") : desc(def.id)}</p>
              </div>
              <code class="ach-rule">{hidden ? t.secretRule : def.rule}</code>
              {look === "progress" && (
                <div class="ach-progress">
                  <div class="xp-track thin">
                    <div class="xp-fill" style={{ width: `${Math.round((st.progress / st.target) * 100)}%` }} />
                  </div>
                  <span class="mono small">
                    {st.progress} / {st.target}
                  </span>
                </div>
              )}
              <p class="ach-meta">
                {packed && <span class="tag tag-unpack">{t.unpack}</span>}
                {packed && " "}
                {isNew && !packed && <span class="tag tag-new">{t.newTag}</span>}
                {isNew && !packed && " "}
                {look === "soon" && <span class="tag">{t.soon}</span>}
                {st.unlocked && st.day && `${t.unlockedOn} ${fmtDay(st.day, lang)}`}
                {st.unlocked && st.day && owners && " · "}
                {owners}
              </p>
              {DEBUG_EFFECTS && st.unlocked && (
                <div class="fx-debug" aria-label="MVP">
                  {EFFECTS.map((fx) => (
                    <button key={fx} class="ghost small" onClick={() => open(def.id, def.category, false, fx)}>
                      ▶ {t.fxName[fx]}
                    </button>
                  ))}
                </div>
              )}
            </li>
          );
        })}
      </ul>

      {opened && (
        <AchViewer
          key={openState?.n}
          effect={openState?.effect}
          onEffectDone={onEffectDone}
          flexText={flexTextFor(opened.def.xp)}
          stampText={t.shameStamp}
          skipHint={t.skipHint}
          id={opened.def.id}
          look={opened.look}
          xp={opened.look === "secret" ? "?" : xpText(opened.def.xp)}
          closeLabel={t.close}
          hint={t.viewerHint}
          onClose={closeViewer}
        >
          <p class="eyebrow">
            {t.cat[opened.def.category]}
            {opened.look !== "secret" && t.briefs[opened.def.id] && ` · ${t.briefs[opened.def.id]}`}
          </p>
          <h2>{opened.look === "secret" ? t.secretTitle : name(opened.def.id)}</h2>
          <p class="muted">{opened.look === "secret" ? (t.ach[opened.def.id]?.[2] ?? "") : desc(opened.def.id)}</p>
          <code class="ach-rule">{opened.look === "secret" ? t.secretRule : opened.def.rule}</code>
          {!opened.st.unlocked && opened.st.target > 1 && (
            <div class="ach-progress">
              <div class="xp-track thin">
                <div class="xp-fill" style={{ width: `${Math.round((opened.st.progress / opened.st.target) * 100)}%` }} />
              </div>
              <span class="mono small">
                {opened.st.progress} / {opened.st.target}
              </span>
            </div>
          )}
          <p class="ach-meta">
            {opened.st.unlocked && opened.st.day ? `${t.unlockedOn} ${fmtDay(opened.st.day, lang)}` : opened.look === "soon" ? t.soon : t.notYet}
          </p>
        </AchViewer>
      )}
    </div>
  );
}
