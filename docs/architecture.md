# Архітектура Gachi Streak v1

```
┌────────────────────┐   GET /status (кожні 60с)   ┌──────────────────────────┐
│ gachi-client.exe   │ ───────────────────────────▶│ gachi-server             │
│ (трей, Windows)    │   POST /ping (кнопка)        │                          │
│                    │ ◀─────────────────────────── │  HTTP API                │
│  Engine ──▶ Win32  │                              │  Deadline worker (1 хв)  │
│  SPI_SETDESKWALL…  │                              │  GitHub checker (10 хв)  │
│  PlaySoundW        │                              │  SQLite                  │
└────────────────────┘                              └─────────┬────────────────┘
                                                              │ GET /users/{login}/events
                                                              ▼
                                                        api.github.com
```

**Сервер — джерело правди.** Клієнт не рахує стрики й не довіряє локальному годиннику: стан (`safe/pending/warning/failed`) обчислює сервер, клієнт лише відображає його на робочому столі.

## Доменні правила (`internal/streak`)

- День користувача — календарна дата в його часовому поясі (`YYYY-MM-DD`; рядкове порівняння = хронологічне).
- Дедлайн — наступна місцева північ (рахується через календар, тож DST не ламає).
- Пінг у день `D`: якщо останній активний день `D-1` → стрик +1; якщо `D` → без змін; інакше → стрик = 1.
- Пропуск: якщо «вчора» ≥ дня реєстрації і останній активний день < «вчора» → подія `failed` за вчора, стрик = 0. Одна подія на кожен пропущений дедлайн (унікальний ключ), тож щоночі без активності — нове покарання.
- Пріоритет станів: `safe` → `failed` → `warning` (≤ 5 год) → `pending`.

## Схема БД

```sql
users(id PK, name UNIQUE, token_hash UNIQUE, timezone, github_login, github_repo,
      created_at, created_day)
streaks(user_id PK → users, current, best, last_active_day, updated_at)
activities(id PK, user_id → users, day, source['manual'|'github'], note, created_at)
      INDEX(user_id, day)
events(id PK, user_id → users, kind['warning'|'failed'], day, created_at)
      UNIQUE(user_id, kind, day)
```

- Токени зберігаються як SHA-256, сам токен показується один раз.
- Час — RFC3339 UTC текстом (однаково для будь-якого драйвера).
- SQL простий, перенесення на PostgreSQL: `AUTOINCREMENT` → `GENERATED ... AS IDENTITY`, `?` → `$n`.

## Клієнт (`internal/client` + `internal/winapi`)

`Engine.Apply(status)` — скінченний автомат `none → warning → failed → none`:

1. Вхід у покарання з `none` → бекап поточних шпалер: шлях, **копія файлу** (Windows часто віддає `TranscodedWallpaper`, який перезапишеться) і `WallpaperStyle/TileWallpaper` з реєстру.
2. Випадкова картинка з папки режиму (або згенерована заглушка), стиль «Fill», `SystemParametersInfoW(SPI_SETDESKWALLPAPER, …, SPIF_UPDATEINIFILE|SPIF_SENDWININICHANGE)`.
3. Звук: попередження — при вході у `warning`; провал — один раз на кожен новий `failed`-евент (його id зберігається в `state.json`).
4. Ротація картинки кожні `rotate_minutes`.
5. `safe/pending` → відновлення з бекапу.

Стан агента (`%APPDATA%\GachiStreak\state.json`) переживає перезапуск: не робить бекап покарання замість оригіналу й не повторює звук для старого провалу.

Інше: один екземпляр (іменований mutex), автозапуск через `HKCU\...\Run`, лог у `client.log`, іконка трею генерується в коді (колір = стан).

## Джерела активності

| source | хто фіксує | як |
|---|---|---|
| `manual` | клієнт | кнопка в треї |
| `local_git` | клієнт | `git log --all --since=…` у папках з `local_repos`; коміт з датою автора «сьогодні» (TZ користувача) від свого email |
| `github` | сервер | `GET /users/{login}/events`, `PushEvent` за сьогодні (з фільтром `github_repo`) |

`github` клієнт надіслати не може (сервер відповідає 400) — це джерело резервоване за серверним чекером.

## Відомі обмеження v1

- Без зв'язку з сервером клієнт тримає останній відомий стан.
- Якщо в Windows було слайд-шоу/Spotlight, відновиться лише поточна картинка.
- GitHub events API показує лише публічні події (або приватні власника токена).
