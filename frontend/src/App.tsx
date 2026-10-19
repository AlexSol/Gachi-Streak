import { useEffect, useState } from "preact/hooks";
import { api, type Me } from "./api";
import { useI18n } from "./i18n";
import { Login } from "./Login";
import { Dashboard } from "./Dashboard";

type Auth = { kind: "loading" } | { kind: "guest" } | { kind: "user"; me: Me };

export function App() {
  const i18n = useI18n();
  const [auth, setAuth] = useState<Auth>({ kind: "loading" });

  useEffect(() => {
    api
      .me()
      .then((me) => setAuth({ kind: "user", me }))
      // 401 = not logged in; on a network error the login screen is the
      // safest place too — it shows a readable error on submit.
      .catch(() => setAuth({ kind: "guest" }));
  }, []);

  const logout = async () => {
    await api.logout().catch(() => undefined);
    setAuth({ kind: "guest" });
  };

  return (
    <div class="shell">
      <header class="top">
        <a class="brand" href="/">
          <span class="mars" aria-hidden="true">♂</span> Gachi Streak
        </a>
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
        {auth.kind === "user" && (
          <Dashboard t={i18n.t} lang={i18n.lang} me={auth.me} onUnauthorized={() => setAuth({ kind: "guest" })} />
        )}
      </main>

      <footer class="foot">{i18n.t.footer}</footer>
    </div>
  );
}
