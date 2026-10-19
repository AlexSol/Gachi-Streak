# Gachi Streak 🔥

Спільний трекер щоденного стрику для двох людей, які вчать Go, з жорсткою системою покарань за прокрастинацію.

- **Сервер** (Go + SQLite) рахує стрики, дедлайни й наказання.
- **Клієнт для Windows** живе в треї, показує обидва стрики й міняє шпалери робочого столу, коли ти халтуриш.

> v1 — лише Windows-клієнт. Сервер збирається під будь-яку ОС.

## Як це працює

| Стан | Коли | Що робить клієнт |
|---|---|---|
| ✅ `safe` | сьогодні вже є активність | повертає твої шпалери |
| ⏳ `pending` | активності ще нема, до дедлайну > 5 год | нічого |
| ⚠️ `warning` | активності нема, до дедлайну ≤ 5 год | шпалери з `images\warning`, звук попередження, зміна картинки кожні 30 хв |
| 💀 `failed` | дедлайн пропущено | стрик → 0, шпалери з `images\failed`, звук провалу |

- Дедлайн — **місцева північ** у часовому поясі користувача (кожен має свій).
- Активність зараховується кнопкою в треї (`POST /ping`) або **автоматично**, якщо сервер бачить сьогоднішній push на GitHub (можна обмежити одним репозиторієм).
- Вийти з програми, щоб зняти покарання, не вийде: шпалери лишаються. Знімає їх тільки зарахована активність. Для демонстрації екрана є «Пауза 15 хв».

## Швидкий старт

### 1. Сервер

Linux / macOS:

```bash
go mod tidy            # перший раз: підтягне залежності й створить go.sum
go build -o bin/gachi-server ./cmd/server

# додати учасників (токен показується один раз)
./bin/gachi-server user add -name alex  -tz Europe/Lisbon -github AlexSol -repo AlexSol/go-diary
./bin/gachi-server user add -name buddy -tz Europe/Kyiv   -github buddy-login

./bin/gachi-server serve -addr :8080
```

Windows (PowerShell) — обов'язково з `.exe`, інакше Windows не знає, чим запускати файл, і питає «Select an app to open»:

```powershell
go mod tidy
go build -o bin\gachi-server.exe .\cmd\server

.\bin\gachi-server.exe user add -name alex -tz Europe/Lisbon -github AlexSol
.\bin\gachi-server.exe serve -addr :8080
```

Або в Docker:

```bash
docker build -f deploy/Dockerfile -t gachi-server .
docker run -d -p 8080:8080 -v gachi-data:/data --name gachi gachi-server
docker exec gachi /gachi-server user add -name alex -tz Europe/Lisbon
```

Параметри `serve`: `-db gachi.db`, `-warn 5h`, `-tick 1m`, `-github-interval 10m` (0 — вимкнути), `-debug`.
Змінні середовища: `GACHI_DB`, `GACHI_ADDR`, `GITHUB_TOKEN` (необов'язково, піднімає ліміт GitHub API).

Інші команди: `user list`, `user token -name alex` (перевипустити токен), `user github -name alex -github LOGIN -repo owner/repo`.

### 2. Клієнт (Windows)

```powershell
powershell -ExecutionPolicy Bypass -File build.ps1
# або з Linux/macOS: make client-win
```

1. Запусти `bin\gachi-client.exe` — він створить `%APPDATA%\GachiStreak\config.json` і відкриє його.
2. Впиши `server_url` і `token`, збережи, запусти ще раз.
3. Поклади картинки (`.jpg/.png/.bmp`) у `%APPDATA%\GachiStreak\images\warning` і `...\images\failed`. Поки папки порожні, ставиться згенерована смугаста заглушка.
4. У меню трею увімкни «Запускати з Windows».

Приклад конфігу — [`examples/client-config.json`](examples/client-config.json). Звук — шлях до `.wav` або системний аліас (`SystemHand`, `SystemExclamation`, `SystemAsterisk`).

## API

Усі запити, окрім `/healthz`, з заголовком `Authorization: Bearer <token>`.

```bash
curl -X POST -H "Authorization: Bearer $T" -d '{"note":"channels"}' http://localhost:8080/ping
curl -H "Authorization: Bearer $T" http://localhost:8080/status
```

`GET /status` повертає обох учасників: стрик, рекорд, стан, дедлайн, секунди до нього, останній провал.

## Структура

```
cmd/server        — HTTP API + воркери + CLI для користувачів
cmd/client        — трей-агент для Windows
internal/streak   — чиста доменна логіка (дні, дедлайни, стани)
internal/store    — SQLite (modernc, без CGO; тег cgosqlite → mattn/go-sqlite3)
internal/server   — хендлери, воркер дедлайнів, GitHub-перевірка
internal/client   — конфіг, API-клієнт, рушій покарань, іконки, заглушки
internal/winapi   — Win32: шпалери, звук, автозапуск, MessageBox
```

Детальніше — [docs/architecture.md](docs/architecture.md).

## Тести

```bash
go test ./...
```

## Далі

- Linux (`gsettings`) і macOS (`osascript`) клієнти
- WebSocket замість опитування
- PostgreSQL як альтернативне сховище
- «Заморозка» стрику (вихідний за домовленістю)
