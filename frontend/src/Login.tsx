import { useState } from "preact/hooks";
import { api, ApiError, type Me } from "./api";
import type { Dict } from "./i18n";
import { BgControls, DungeonBg, bgBoost, prefersReducedMotion, useBgPrefs } from "./DungeonBg";

export function Login({ t, onLogin }: { t: Dict; onLogin: (me: Me) => void }) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const { prefs: bg, update: updateBg } = useBgPrefs();
  // Form micro-animations: shake on a wrong token, "enter the portal" on success.
  const [fx, setFx] = useState<"" | "shake" | "opening">("");
  const animate = bg.formFx && !prefersReducedMotion();

  const submit = async (e: Event) => {
    e.preventDefault();
    if (!token.trim()) return;
    setBusy(true);
    setError("");
    bgBoost();
    try {
      const me = await api.login(token.trim());
      if (animate) {
        setFx("opening");
        bgBoost();
        setTimeout(() => onLogin(me), 650);
      } else onLogin(me);
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) setError(t.errLimited);
      else if (err instanceof ApiError && err.status === 401) setError(t.errInvalid);
      else setError(t.errNetwork);
      if (animate) {
        setFx("");
        requestAnimationFrame(() => setFx("shake"));
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
    <DungeonBg effect={bg.effect} strength={bg.strength} />
    <section
      class={"card login" + (bg.formFx ? " login-fx" : "") + (fx ? " login-" + fx : "")}
      onAnimationEnd={(e) => fx === "shake" && e.target === e.currentTarget && setFx("")}
    >
      <div class="tape" aria-hidden="true" />
      <p class="eyebrow">{t.tagline}</p>
      <h1>{t.loginTitle}</h1>
      <p class="muted">{t.loginText}</p>
      <form onSubmit={submit}>
        <label for="token">{t.tokenLabel}</label>
        <input
          id="token"
          type="password"
          autocomplete="off"
          spellcheck={false}
          placeholder="gs_…"
          value={token}
          onInput={(e) => setToken((e.target as HTMLInputElement).value)}
          disabled={busy}
          autofocus
        />
        {error && (
          <p class="error" role="alert">
            {error}
          </p>
        )}
        <button class="primary" type="submit" disabled={busy || !token.trim()}>
          {busy ? t.loggingIn : t.loginBtn}
        </button>
      </form>
    </section>
    <BgControls t={t} prefs={bg} update={updateBg} />
    </>
  );
}
