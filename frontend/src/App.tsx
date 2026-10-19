import { useCallback, useEffect, useState } from "preact/hooks";
import { api, type AchievementDef, type Me } from "./api";
import { useI18n } from "./i18n";
import { Login } from "./Login";
import { DungeonBg } from "./DungeonBg";
import { Dashboard } from "./Dashboard";
import { AchievementsPage, RanksPage } from "./Progress";

type Route = "dash" | "ranks" | "achievements";

// Hash routing: the app is served under /app/ by the Go server, so
// #/ranks needs no server-side fallback.
function routeFromHash(): Route {
  const h = location.hash.replace(/^#\/?/, "");
  return h === "ranks" || h === "achievements" ? h : "dash";
}

function useRoute() {
  const [route, setRoute] = useState<Route>(routeFromHash);
  useEffect(() => {
    const on = () => {
      setRoute(routeFromHash());
      window.scrollTo(0, 0);
    };
    window.addEventListener("hashchange", on);
    return () => window.removeEventListener("hashchange", on);
  }, []);
  return route;
}

type Auth = { kind: "loading" } | { kind: "guest" } | { kind: "user"; me: Me };

export function App() {
  const i18n = useI18n();
  const [auth, setAuth] = useState<Auth>({ kind: "loading" });
  const route = useRoute();

  useEffect(() => {
    api
      .me()
      .then((me) => setAuth({ kind: "user", me }))
      // 401 = not logged in; on a network error the login screen is the
      // safest place too — it shows a readable error on submit.
      .catch(() => setAuth({ kind: "guest" }));
  }, []);

  // Stable identity: pages use it as a useCallback dependency.
  const toGuest = useCallback(() => setAuth({ kind: "guest" }), []);

  // "Achievement unlocked" toast, shown on whatever page is open.
  const [unlocked, setUnlocked] = useState<AchievementDef[]>([]);
  const onUnlocked = useCallback((defs: AchievementDef[]) => setUnlocked(defs), []);
  useEffect(() => {
    if (!unlocked.length) return;
    const id = setTimeout(() => setUnlocked([]), 8000);
    return () => clearTimeout(id);
  }, [unlocked]);
  const t = i18n.t;
  const unlockedXP = unlocked.reduce((sum, d) => sum + d.xp, 0);

  const logout = async () => {
    await api.logout().catch(() => undefined);
    setAuth({ kind: "guest" });
  };

  return (
    <div class="shell">
      {/* Main pages: floating ♂ / Go glyphs at fixed strength, no pointer parallax. */}
      {auth.kind === "user" && <DungeonBg effect="glyphs" strength={100} parallax={false} />}
      <header class="top">
        <a class="brand" href="#/">
          <span class="mars" aria-hidden="true">♂</span> Gachi Streak
        </a>
        {auth.kind === "user" && (
          <nav class="nav" aria-label="Main">
            {(["dash", "ranks", "achievements"] as const).map((r) => (
              <a key={r} href={r === "dash" ? "#/" : `#/${r}`} aria-current={route === r ? "page" : undefined}>
                {i18n.t.nav[r]}
              </a>
            ))}
          </nav>
        )}
        <div class="top-actions">
          <button class="ghost small" onClick={i18n.toggle} aria-label="Switch language">
            {i18n.lang === "uk" ? "EN" : "UA"}
          </button>
          {auth.kind === "user" && (
            <button class="ghost small" onClick={logout}>
              {i18n.t.logout}
            </button>
          )}
        </div>
      </header>

      <main>
        {auth.kind === "loading" && <div class="spinner" aria-label="loading" />}
        {auth.kind === "guest" && <Login t={i18n.t} onLogin={(me) => setAuth({ kind: "user", me })} />}
        {/* All pages are mounted once after sign-in and only hidden/shown on
            navigation: no spinner, no refetch and no layout jump when switching. */}
        {auth.kind === "user" && (
          <>
            <div hidden={route !== "dash"}>
              <Dashboard t={i18n.t} lang={i18n.lang} me={auth.me} onUnauthorized={toGuest} />
            </div>
            <div hidden={route !== "ranks"}>
              <RanksPage t={i18n.t} me={auth.me.name} onUnauthorized={toGuest} />
            </div>
            <div hidden={route !== "achievements"}>
              <AchievementsPage
                t={i18n.t}
                lang={i18n.lang}
                me={auth.me.name}
                onUnauthorized={toGuest}
                onUnlocked={onUnlocked}
              />
            </div>
          </>
        )}
        {auth.kind === "user" && unlocked.length > 0 && (
          <div class="unlock-toast" role="status">
            <span class="unlock-cup" aria-hidden="true">🏆</span>
            <span>
              {unlocked.length === 1
                ? t.unlockToast(t.ach[unlocked[0].id]?.[0] ?? unlocked[0].id, unlockedXP)
                : t.unlockMany(unlocked.length, unlockedXP)}
            </span>
            {route !== "achievements" && (
              <a class="unlock-open" href="#/achievements" onClick={() => setUnlocked([])}>
                {t.unlockOpen}
              </a>
            )}
            <button class="ghost small" aria-label="Close" onClick={() => setUnlocked([])}>
              ×
            </button>
          </div>
        )}
      </main>

      <footer class="foot">{i18n.t.footer}</footer>
    </div>
  );
}
