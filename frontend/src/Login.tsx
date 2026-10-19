import { useState } from "preact/hooks";
import { api, ApiError, type Me } from "./api";
import type { Dict } from "./i18n";
import { BgControls, DungeonBg, useBgPrefs } from "./DungeonBg";

export function Login({ t, onLogin }: { t: Dict; onLogin: (me: Me) => void }) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const bg = useBgPrefs();

  const submit = async (e: Event) => {
    e.preventDefault();
    if (!token.trim()) return;
    setBusy(true);
    setError("");
    try {
      onLogin(await api.login(token.trim()));
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) setError(t.errLimited);
      else if (err instanceof ApiError && err.status === 401) setError(t.errInvalid);
      else setError(t.errNetwork);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
    <DungeonBg effect={bg.effect} strength={bg.strength} />
    <section class="card login">
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
    <BgControls t={t} effect={bg.effect} strength={bg.strength} update={bg.update} />
    </>
  );
}
