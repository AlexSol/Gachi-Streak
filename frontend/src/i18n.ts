import { useEffect, useState } from "preact/hooks";

export type Lang = "uk" | "en";

const dict = {
  uk: {
    tagline: "Dungeon стриків вивчення Go",
    loginTitle: "Вхід у dungeon",
    loginText: "Доступ лише за інвайт-токеном. Токен видає адмін — він починається з gs_.",
    tokenLabel: "Інвайт-токен",
    loginBtn: "Увійти",
    loggingIn: "Перевіряю…",
    errInvalid: "Невірний токен. Dungeon master тебе не впізнав.",
    errLimited: "Забагато спроб. Почекай 15 хвилин.",
    errNetwork: "Сервер недоступний. Спробуй пізніше.",
    logout: "Вийти",
    you: "ти",
    streak: "стрик",
    best: "рекорд",
    days: (n: number) => (n % 10 === 1 && n % 100 !== 11 ? "день" : [2, 3, 4].includes(n % 10) && ![12, 13, 14].includes(n % 100) ? "дні" : "днів"),
    left: "до дедлайну",
    state: {
      safe: "Сьогодні зроблено",
      pending: "Чекає на заняття",
      warning: "ГОРИТЬ! Менше 5 годин",
      failed: "Стрик злитий",
    },
    lastActive: "Остання активність",
    never: "ще не було",
    lastFail: "Останній провал",
    pingBtn: "✅ Я сьогодні вчив Go",
    pingDone: "✅ На сьогодні зараховано",
    pinging: "Відмічаю…",
    pingCounted: "Зараховано! Стрик живе ♂",
    pingAgain: "Сьогодні вже зараховано — але молодець.",
    notePlaceholder: "Що вчив? (необов'язково)",
    updated: "Оновлено",
    offline: "Немає зв'язку з сервером, показую останні дані",
    footer: "♂ boy next door, commit today ♂",
    now: "Зараз",
    nowIn: "у",
    afk: "AFK — відійшов",
    offlineUser: "Клієнт не в мережі",
    noClient: "Desktop-клієнт ще не підключений",
    todayApps: "Сьогодні в програмах",
    noApps: "Сьогодні ще нічого не відстежено",
    weekTitle: "Статистика програм · 7 днів",
    total: "Всього",
    h: "г",
    m: "хв",
  },
  en: {
    tagline: "The Go learning streak dungeon",
    loginTitle: "Enter the dungeon",
    loginText: "Invite-only. The admin hands out tokens — they start with gs_.",
    tokenLabel: "Invite token",
    loginBtn: "Sign in",
    loggingIn: "Checking…",
    errInvalid: "Wrong token. The dungeon master doesn't know you.",
    errLimited: "Too many attempts. Wait 15 minutes.",
    errNetwork: "Server is unreachable. Try again later.",
    logout: "Sign out",
    you: "you",
    streak: "streak",
    best: "best",
    days: (n: number) => (n === 1 ? "day" : "days"),
    left: "until deadline",
    state: {
      safe: "Done for today",
      pending: "Waiting for practice",
      warning: "ON FIRE! Under 5 hours",
      failed: "Streak lost",
    },
    lastActive: "Last activity",
    never: "never",
    lastFail: "Last failure",
    pingBtn: "✅ I studied Go today",
    pingDone: "✅ Counted for today",
    pinging: "Saving…",
    pingCounted: "Counted! The streak lives ♂",
    pingAgain: "Already counted today — still, well done.",
    notePlaceholder: "What did you learn? (optional)",
    updated: "Updated",
    offline: "No connection to the server, showing last known data",
    footer: "♂ boy next door, commit today ♂",
    now: "Now",
    nowIn: "in",
    afk: "AFK — away",
    offlineUser: "Client is offline",
    noClient: "Desktop client not connected yet",
    todayApps: "Today in programs",
    noApps: "Nothing tracked today yet",
    weekTitle: "Program stats · 7 days",
    total: "Total",
    h: "h",
    m: "m",
  },
};

export type Dict = (typeof dict)["uk"];

const KEY = "gs_lang";

// Browser / OS language decides; a manual switch is remembered per browser.
export function detectLang(): Lang {
  try {
    const saved = localStorage.getItem(KEY);
    if (saved === "uk" || saved === "en") return saved;
  } catch {
    /* storage may be blocked */
  }
  const prefs = navigator.languages?.length ? navigator.languages : [navigator.language ?? ""];
  for (const p of prefs) {
    const primary = String(p).toLowerCase().split("-")[0];
    if (primary === "uk" || primary === "en") return primary;
  }
  return "en";
}

export function useI18n() {
  const [lang, setLang] = useState<Lang>(detectLang);
  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);
  const toggle = () => {
    const next: Lang = lang === "uk" ? "en" : "uk";
    try {
      localStorage.setItem(KEY, next);
    } catch {
      /* ignore */
    }
    setLang(next);
  };
  return { lang, t: dict[lang] as Dict, toggle };
}
