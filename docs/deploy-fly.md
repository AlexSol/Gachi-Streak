# Деплой сервера на Fly.io

Вартість: машина `shared-cpu-1x` / 256 MB, яка працює постійно, + volume 1 GB — приблизно $2–3 на місяць.

## 1. Встановити flyctl і залогінитись (PowerShell)

```powershell
iwr https://fly.io/install.ps1 -useb | iex
fly auth login
```

## 2. Створити застосунок

Ім'я застосунку має бути унікальним у всьому Fly. Відкрий `fly.toml` і заміни `app = "gachi-streak"` на своє, наприклад `gachi-streak-alexsol`. Потім:

```powershell
fly apps create gachi-streak-alexsol
```

## 3. Створити диск для бази

Регіон має збігатися з `primary_region` у `fly.toml`.

```powershell
fly volumes create gachi_data --size 1 --region ams
```

Fly попередить, що один volume — це одна копія без реплікації. Для двох людей це нормально, щоденні снапшоти Fly робить сам (`fly volumes snapshots list gachi_data`).

## 4. (Необов'язково) Токен GitHub для автоперевірки пушів

```powershell
fly secrets set GITHUB_TOKEN=ghp_xxx
```

Без нього теж працює: 60 запитів на годину до GitHub API вистачає на двох.

## 5. Задеплоїти

```powershell
fly deploy
fly status
curl https://gachi-streak-alexsol.fly.dev/healthz   # → ok
```

## 6. Створити користувачів

Команди виконуються всередині машини, база одразу на volume:

```powershell
fly ssh console -C "/gachi-server user add -name alex -tz Europe/Lisbon -github AlexSol"
fly ssh console -C "/gachi-server user add -name buddy -tz Europe/Kyiv"
fly ssh console -C "/gachi-server user list"
```

Збережи токени, вони показуються один раз. Загубив — `user token -name alex` видасть новий.

## 7. Налаштувати клієнти

У `%APPDATA%\GachiStreak\config.json` в обох:

```json
"server_url": "https://gachi-streak-alexsol.fly.dev",
"token": "gs_…"
```

Клієнт підхопить зміни сам.

## Корисне

```powershell
fly logs                 # логи сервера: пінги, провали, GitHub-перевірки
fly deploy               # оновити після git pull
fly ssh console -C "/gachi-server user list"
```

Порада: закоміть `go.sum`, який з'явився у тебе після `go mod tidy`, щоб збірки були відтворюваними.
