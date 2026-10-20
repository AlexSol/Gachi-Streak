import { useEffect, useMemo, useState } from "preact/hooks";
import { api, ApiError, type ProfileComparison, type Relation, type UserProfile } from "./api";
import type { Dict, Lang } from "./i18n";
import { appColor, fmtDur } from "./Activity";
import { RankBadge } from "./Progress";

function localDate(day: string, lang: Lang, options: Intl.DateTimeFormatOptions = { day: "numeric", month: "short", year: "numeric" }) {
  return new Date(`${day}T12:00:00`).toLocaleDateString(lang === "uk" ? "uk-UA" : "en-GB", options);
}

function relationText(relation: Relation, t: Dict) {
  switch (relation) {
    case "friend": return t.profileFriends;
    case "incoming": return t.profileIncoming;
    case "outgoing": return t.profileOutgoing;
    default: return t.profileStranger;
  }
}

export function ProfilePage({
  name,
  me,
  t,
  lang,
  onUnauthorized,
}: {
  name: string;
  me: string;
  t: Dict;
  lang: Lang;
  onUnauthorized: () => void;
}) {
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState("");
  const [copied, setCopied] = useState(false);
  const [reload, setReload] = useState(0);
  const self = name === me;

  useEffect(() => {
    const controller = new AbortController();
    setProfile(null);
    setLoading(true);
    setFailed(false);
    setNotFound(false);
    api.userProfile(name, controller.signal)
      .then((data) => setProfile(data))
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (error instanceof ApiError && error.status === 401) onUnauthorized();
        else if (error instanceof ApiError && error.status === 404) setNotFound(true);
        else setFailed(true);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [name, reload, onUnauthorized]);

  const act = async () => {
    if (!profile || busy) return;
    if (profile.relation === "friend" && !window.confirm(t.friendRemoveConfirm(profile.name))) return;
    setBusy(true);
    setActionError("");
    try {
      if (profile.relation === "friend") await api.removeFriend(profile.name);
      else if (profile.relation === "outgoing") await api.cancelFriend(profile.name);
      else await api.addFriend(profile.name);
      setReload((value) => value + 1);
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) onUnauthorized();
      else setActionError(t.friendErr(error instanceof Error ? error.message : String(error)));
    } finally {
      setBusy(false);
    }
  };

  const copyLink = async () => {
    try {
      await navigator.clipboard.writeText(`${location.origin}${location.pathname}#/u/${encodeURIComponent(name)}`);
      setCopied(true);
      setTimeout(() => setCopied(false), 2500);
    } catch (error) {
      setActionError(t.profileCopyError(error instanceof Error ? error.message : String(error)));
    }
  };

  if (loading && !profile) return <div class="spinner" aria-label="loading" />;
  if (notFound) return <div class="banner" role="alert">{t.profileNotFound(name)}</div>;
  if (failed && !profile) return (
    <div class="banner profile-load-error" role="alert">
      <span>{t.profileLoadError}</span>
      <button class="ghost small" onClick={() => setReload((value) => value + 1)}>{t.profileRetry}</button>
    </div>
  );
  if (!profile) return <div class="banner" role="alert">{t.profileLoadError}</div>;

  return (
    <div class="profile">
      {(failed || actionError) && <div class="banner" role="alert">{actionError || t.offline}</div>}
      <header class="profile-hero">
        <a class="profile-back" href="#/friends">{t.profileBack}</a>
        <div class="profile-person">
          <div class="profile-avatar" aria-hidden="true">{profile.name.slice(0, 1).toLocaleUpperCase()}</div>
          <div class="profile-identity">
            <h1>{profile.name}{self && <span class="profile-you"> · {t.you}</span>}</h1>
            <div class="profile-rank">
              <RankBadge level={profile.rank} size={32} />
              <span>{profile.rank_name}</span>
              <span class="profile-dot" aria-hidden="true">·</span>
              <span>{t.profileSince(localDate(profile.created_day, lang))}</span>
              {!self && <span class="profile-dot" aria-hidden="true">·</span>}
              {!self && <span>{relationText(profile.relation, t)}</span>}
            </div>
          </div>
        </div>
        <div class="profile-actions">
          {self ? (
            <>
              <button class="ghost small" disabled title={t.profileEditSoon}>{t.profileEdit}</button>
              <button class="primary small" onClick={copyLink}>{copied ? t.profileCopied : t.profileCopyLink}</button>
            </>
          ) : (
            <>
              <button
                class={profile.relation === "friend" ? "ghost small" : "primary small"}
                disabled={busy}
                onClick={act}
              >
                {profile.relation === "friend"
                  ? t.profileRemoveFriend
                  : profile.relation === "outgoing"
                    ? t.profileCancelRequest
                    : profile.relation === "incoming"
                      ? t.friendAccept
                      : t.friendAdd}
              </button>
              <button class="ghost small" onClick={() => document.getElementById("profile-comparison")?.scrollIntoView({ block: "start" })}>
                {t.profileChallenge}
              </button>
            </>
          )}
        </div>
      </header>

      <section class="profile-stats" aria-label={t.profileStats}>
        <ProfileStat value={String(profile.streak)} label={t.streak} detail={t.profileCurrent} highlight />
        <ProfileStat value={String(profile.best)} label={t.best} />
        <ProfileStat value={String(profile.active_days)} label={t.profileCountedDays} />
        <ProfileStat value={profile.xp.toLocaleString(lang === "uk" ? "uk-UA" : "en-GB")} label="XP" />
        <ProfileStat value={String(profile.achievements)} label={t.profileAchievements} />
        <ProfileStat value={String(profile.failures)} label={t.profileMissed} danger={profile.failures > 0} />
      </section>

      <section class="profile-section" aria-labelledby="profile-heatmap-title">
        <div class="profile-section-head">
          <h2 id="profile-heatmap-title">{t.profileYear}</h2>
          <span class="muted">{t.profileYearHint}</span>
        </div>
        <Heatmap days={profile.days} lang={lang} t={t} />
      </section>

      <div class="profile-columns">
        <section class="profile-section" aria-labelledby="profile-apps-title">
          <div class="profile-section-head">
            <h2 id="profile-apps-title">{t.profileTopApps}</h2>
            <span class="muted">{t.profileLast30}</span>
          </div>
          {profile.top_apps.length ? (
            <AppList profile={profile} t={t} />
          ) : <p class="profile-empty">{t.profileNoApps}</p>}
        </section>
        <section class="profile-section" aria-labelledby="profile-achievements-title">
          <div class="profile-section-head">
            <h2 id="profile-achievements-title">{t.profileLatest}</h2>
            <a href="#/achievements">{t.achTitle}</a>
          </div>
          {profile.latest_achievements.length ? (
            <ul class="profile-achievements">
              {profile.latest_achievements.map((achievement) => {
                const title = achievement.secret ? t.secretTitle : (t.ach[achievement.id]?.[0] ?? achievement.id);
                return (
                  <li key={achievement.id}>
                    <span class={`profile-ach-mark${achievement.category === "shame" ? " is-shame" : ""}`} aria-hidden="true" />
                    <span class="profile-ach-title">{title}</span>
                    <span class="profile-ach-date">{localDate(achievement.day, lang, { day: "numeric", month: "short" })}</span>
                  </li>
                );
              })}
            </ul>
          ) : <p class="profile-empty">{t.profileNoAchievements}</p>}
        </section>
      </div>

      {self && (
        <section class="profile-section" aria-labelledby="profile-log-title">
          <div class="profile-section-head">
            <h2 id="profile-log-title">{t.profileLearningLog}</h2>
            <span class="muted">{t.profilePrivate}</span>
          </div>
          {profile.learning_log?.length ? (
            <ol class="profile-log">
              {profile.learning_log.map((entry, index) => (
                <li key={`${entry.day}-${index}`}>
                  <time dateTime={entry.day}>{localDate(entry.day, lang)}</time>
                  <p>{entry.note}</p>
                </li>
              ))}
            </ol>
          ) : <p class="profile-empty">{t.profileNoNotes}</p>}
        </section>
      )}

      {!self && profile.comparison && (
        <Comparison comparison={profile.comparison} me={me} user={profile.name} lang={lang} t={t} />
      )}

      {!self && profile.shared_friends.length > 0 && (
        <section class="profile-section" aria-labelledby="profile-mutual-title">
          <div class="profile-section-head">
            <h2 id="profile-mutual-title">{t.profileSharedFriends}</h2>
            <span class="muted">{profile.shared_friends.length}</span>
          </div>
          <ul class="profile-friends">
            {profile.shared_friends.map((friend) => (
              <li key={friend}><a href={`#/u/${encodeURIComponent(friend)}`}>{friend}</a></li>
            ))}
          </ul>
        </section>
      )}
      {self && <p class="profile-edit-note">{t.profileEditSoon}</p>}
    </div>
  );
}

function ProfileStat({ value, label, detail, highlight, danger }: { value: string; label: string; detail?: string; highlight?: boolean; danger?: boolean }) {
  return (
    <div class={`profile-stat${highlight ? " is-highlight" : ""}${danger ? " is-danger" : ""}`}>
      <span class="profile-stat-label">{label}</span>
      <strong>{value}</strong>
      {detail && <span class="profile-stat-detail">{detail}</span>}
    </div>
  );
}

function Heatmap({ days, lang, t }: { days: UserProfile["days"]; lang: Lang; t: Dict }) {
  const dateFormatter = useMemo(
    () => new Intl.DateTimeFormat(lang === "uk" ? "uk-UA" : "en-GB", { weekday: "long", day: "numeric", month: "long", year: "numeric" }),
    [lang],
  );
  const padded = useMemo(() => {
    const offset = days.length ? (new Date(`${days[0].day}T12:00:00`).getDay() + 6) % 7 : 0;
    return [...Array(offset).fill(null), ...days];
  }, [days]);
  return (
    <>
      <div class="profile-heatmap-scroll">
        <div class="profile-heatmap" role="grid" aria-label={t.profileYear}>
          {padded.map((day, index) => {
            if (!day) return <span key={`pad-${index}`} class="heat-cell is-empty" aria-hidden="true" />;
            const state = day.missed ? (day.counted ? "both" : "missed") : day.counted ? "counted" : "none";
            const date = new Date(`${day.day}T12:00:00`);
            const label = `${dateFormatter.format(date)} · ${day.missed ? t.profileDayMissed : ""}${day.missed && day.counted ? " · " : ""}${day.counted ? t.profileDayCounted : !day.missed ? t.profileDayEmpty : ""}`;
            return <span key={day.day} role="gridcell" aria-label={label} title={label} class={`heat-cell state-${state}`} />;
          })}
        </div>
      </div>
      <div class="profile-heat-legend" aria-label={t.profileLegend}>
        <span><i class="heat-cell state-none" />{t.profileDayEmpty}</span>
        <span><i class="heat-cell state-counted" />{t.profileDayCounted}</span>
        <span><i class="heat-cell state-missed" />{t.profileDayMissed}</span>
      </div>
    </>
  );
}

function AppList({ profile, t }: { profile: UserProfile; t: Dict }) {
  const max = Math.max(1, ...profile.top_apps.map((app) => app.seconds));
  return (
    <ul class="profile-apps">
      {profile.top_apps.map((app) => (
        <li key={app.app}>
          <div class="profile-app-head">
            <span>{app.app}</span><span class="mono">{fmtDur(app.seconds, t)}</span>
          </div>
          <div class="profile-app-track">
            <span style={{ width: `${(app.seconds / max) * 100}%`, background: appColor(app.app) }} />
          </div>
        </li>
      ))}
    </ul>
  );
}

function Comparison({ comparison, me, user, lang, t }: { comparison: ProfileComparison; me: string; user: string; lang: Lang; t: Dict }) {
  const max = Math.max(1, ...comparison.days.flatMap((day) => [day.me_seconds, day.user_seconds]));
  return (
    <section id="profile-comparison" class="profile-section profile-comparison" aria-labelledby="profile-compare-title">
      <div class="profile-section-head">
        <h2 id="profile-compare-title">{t.profileCompare(user)}</h2>
        <span class="muted">{t.profileLast30}</span>
      </div>
      <div class="profile-compare-totals">
        <div><span>{me}</span><strong>{t.profileCompareSummary(comparison.totals.me_counted, fmtDur(comparison.totals.me_seconds, t))}</strong></div>
        <div><span>{user}</span><strong>{t.profileCompareSummary(comparison.totals.user_counted, fmtDur(comparison.totals.user_seconds, t))}</strong></div>
      </div>
      <div class="profile-compare-legend">
        <span><i class="is-me" />{me}</span><span><i class="is-user" />{user}</span><span><i class="is-counted" />{t.profileDayCounted}</span>
      </div>
      <div class="profile-compare-scroll">
        <div class="profile-compare-chart" role="img" aria-label={t.profileCompareChart(me, user)}>
          {comparison.days.map((day) => (
            <div key={day.day} class="compare-day" title={`${localDate(day.day, lang)} · ${me}: ${fmtDur(day.me_seconds, t)} · ${user}: ${fmtDur(day.user_seconds, t)}`}>
              <span class={`compare-bar is-me${day.me_counted ? " is-counted" : ""}`} style={{ height: `${Math.max(day.me_counted ? 4 : 0, (day.me_seconds / max) * 100)}%` }} />
              <span class={`compare-bar is-user${day.user_counted ? " is-counted" : ""}`} style={{ height: `${Math.max(day.user_counted ? 4 : 0, (day.user_seconds / max) * 100)}%` }} />
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
