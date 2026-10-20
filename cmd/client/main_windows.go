//go:build windows

// Command gachi-client is the Windows tray agent: it polls the server,
// shows both streaks and punishes its owner's desktop when they slack off.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"fyne.io/systray"

	"github.com/AlexSol/gachi-streak/assets"
	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/client"
	"github.com/AlexSol/gachi-streak/internal/winapi"
)

// version is the release version; set it at build time with
// -ldflags "-X main.version=1.2.3" (make client-win VERSION=1.2.3).
// Dev builds do not check for updates.
var version = "dev"

const (
	appName   = "GachiStreak"
	title     = "Gachi Streak"
	maxUsers  = 4
	snoozeFor = 15 * time.Minute
)

type app struct {
	cfg        client.Config
	cfgPath    string
	dataDir    string
	api        *client.APIClient
	engine     *client.Engine
	mu         sync.Mutex // serializes poll/ping/snooze
	refresh    chan struct{}
	userItems  []*systray.MenuItem
	statusItem *systray.MenuItem
	pingItem   *systray.MenuItem
	snoozeItem *systray.MenuItem
	autoItem   *systray.MenuItem
	gitItem    *systray.MenuItem
	demoItem   *systray.MenuItem
	statsItem  *systray.MenuItem
	statsRows  []*systray.MenuItem
	achItem    *systray.MenuItem
	netStats   *client.NetStats
	netItem    *systray.MenuItem
	netRows    []*systray.MenuItem // summary lines
	netEpRows  []*systray.MenuItem // per endpoint
	netLogItem *systray.MenuItem
	netCopy    *systray.MenuItem
	tracker    *client.Tracker
	widget     *Widget
	widgetItem *systray.MenuItem
	modeDesk   *systray.MenuItem
	modeOver   *systray.MenuItem
	lastStatus *api.StatusResponse
	localGit   client.LocalGit
	desk       *winapi.Desktop
	cfgErr     error     // non-nil → not configured, server is not polled
	cfgMod     time.Time // config.json mtime, for auto-reload
	achAt      time.Time // last GET /progress
	updateItem *systray.MenuItem
	update     *client.Update // newer release, nil = up to date
	updAt      time.Time      // last GET /client/version
	updToasted bool           // a required update was announced this run
	updCheck   *systray.MenuItem
}

// updateEvery limits GET /client/version: the server itself refreshes
// the release every 15 minutes.
const updateEvery = time.Hour

// achEvery limits GET /progress: unlocks are stored by the server worker
// once a minute anyway.
const achEvery = time.Minute

func main() {
	winapi.EnableDPIAwareness() // crisp widget on scaled displays
	if !winapi.SingleInstance(appName) {
		winapi.MessageBox(title, "Gachi Streak вже запущений (дивись у трей).", winapi.MBInfo)
		return
	}
	dataDir, err := winapi.DataDir()
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fatal(err)
	}
	setupLog(dataDir)
	log.Println("start", title, client.ReadBuildInfo(version))

	a := &app{
		cfgPath: filepath.Join(dataDir, "config.json"),
		dataDir: dataDir,
		refresh: make(chan struct{}, 1),
		// Counts every request since the agent started, across config reloads.
		netStats: client.NewNetStats(time.Now()),
	}
	created := a.loadConfig()
	switch {
	case created:
		_ = winapi.Open(a.cfgPath)
		go winapi.MessageBox(title, fmt.Sprintf(
			"Створено конфіг:\n%s\n\nВпиши server_url і token (видає `gachi-server user add`) і збережи файл — програма підхопить зміни сама, перезапуск не потрібен.\n\nПоки що працює демо-режим у меню трею.",
			a.cfgPath), winapi.MBInfo)
	case a.cfgErr != nil:
		go winapi.MessageBox(title, "Проблема з конфігом "+a.cfgPath+":\n\n"+a.cfgErr.Error()+
			"\n\nПрограма працює в треї. Виправ конфіг і збережи — зміни підхопляться автоматично.", winapi.MBWarning)
	}
	systray.Run(a.onReady, func() { log.Println("exit") })
}

func (a *app) onReady() {
	systray.SetIcon(client.StateIcon("offline"))
	systray.SetTitle(title)
	systray.SetTooltip(title + ": підключаюсь…")

	a.statusItem = systray.AddMenuItem("Підключаюсь до сервера…", "")
	a.statusItem.Disable()
	a.updateItem = systray.AddMenuItem("", "Відкрити сторінку релізу й завантажити нову версію")
	a.updateItem.Hide()
	for i := 0; i < maxUsers; i++ {
		it := systray.AddMenuItem("", "")
		it.Disable()
		it.Hide()
		a.userItems = append(a.userItems, it)
	}
	a.gitItem = systray.AddMenuItem("", "Локальні репозиторії з config.json → local_repos")
	a.gitItem.Disable()
	a.updateGitItem()
	a.statsItem = systray.AddMenuItem("📊 Сьогодні в програмах", "Час в активних вікнах (лише назви програм)")
	for i := 0; i < 8; i++ {
		it := a.statsItem.AddSubMenuItem("", "")
		it.Disable()
		it.Hide()
		a.statsRows = append(a.statsRows, it)
	}
	a.netItem = systray.AddMenuItem("📡 Мережа", "Запити до сервера, помилки й трафік від запуску")
	for i := 0; i < 8; i++ {
		it := a.netItem.AddSubMenuItem("", "")
		it.Disable()
		a.netRows = append(a.netRows, it)
	}
	a.netItem.AddSeparator()
	for i := 0; i < 8; i++ {
		it := a.netItem.AddSubMenuItem("", "Запитів · помилок · середня відповідь")
		it.Disable()
		it.Hide()
		a.netEpRows = append(a.netEpRows, it)
	}
	a.netItem.AddSeparator()
	a.netCopy = a.netItem.AddSubMenuItem(netCopyTitle, "Повний звіт у буфер обміну — встав у чат адміну")
	a.netLogItem = a.netItem.AddSubMenuItem("📝 Записати звіт у client.log", "Повний звіт, зручно переслати адміну")
	a.renderNet()
	a.achItem = systray.AddMenuItem("🏆 Ачівки", "Відкрити сторінку ачівок у браузері")
	systray.AddSeparator()
	a.pingItem = systray.AddMenuItem("✅ Я сьогодні вчив Go (відмітити)", "POST /ping")
	refreshItem := systray.AddMenuItem("🔄 Оновити", "")
	a.snoozeItem = systray.AddMenuItem("🙈 Пауза 15 хв (повернути шпалери)", "Наприклад, для демонстрації екрана")
	systray.AddSeparator()
	a.demoItem = systray.AddMenuItem("🧪 Демо", "Примусово показати покарання, не чекаючи дедлайну")
	demoWarn := a.demoItem.AddSubMenuItem("⚠️ Показати попередження", "Шпалери + звук попередження (ще раз — наступна картинка)")
	demoFail := a.demoItem.AddSubMenuItem("💀 Показати провал", "Шпалери + звук провалу (ще раз — наступна картинка)")
	a.demoItem.AddSeparator()
	demoSndWarn := a.demoItem.AddSubMenuItem("🔊 Звук попередження", "Тільки звук")
	demoSndFail := a.demoItem.AddSubMenuItem("🔊 Звук провалу", "Тільки звук")
	a.demoItem.AddSeparator()
	demoStop := a.demoItem.AddSubMenuItem("⏹ Вимкнути демо", "Повернути шпалери і реальний стан із сервера")
	systray.AddSeparator()
	a.widgetItem = systray.AddMenuItemCheckbox("🪟 Віджет", "Показати / сховати віджет", false)
	modeMenu := systray.AddMenuItem("Режим віджета", "")
	a.modeDesk = modeMenu.AddSubMenuItemCheckbox("🖥 На робочому столі (під вікнами)", "", false)
	a.modeOver = modeMenu.AddSubMenuItemCheckbox("🎮 Поверх усіх вікон (як Discord)", "", false)
	a.syncWidgetMenu()
	imagesItem := systray.AddMenuItem("📂 Папка з картинками", "")
	configItem := systray.AddMenuItem("⚙️ Відкрити конфіг", "Зміни підхоплюються автоматично після збереження")
	reloadItem := systray.AddMenuItem("♻️ Перечитати конфіг", "")
	a.autoItem = systray.AddMenuItemCheckbox("Запускати з Windows", "", winapi.AutostartEnabled(appName))
	systray.AddSeparator()
	bi := client.ReadBuildInfo(version)
	infoItem := systray.AddMenuItem("ℹ️ Про програму", "Версія, збірка й посилання")
	for _, line := range []string{
		title + " " + bi.String(),
		"Спільний щоденний стрік для двох, хто вчить Go.",
		"Пропустив день — шпалери-покарання і стрік до нуля ♂",
	} {
		infoItem.AddSubMenuItem(line, "").Disable()
	}
	if !bi.Time.IsZero() {
		infoItem.AddSubMenuItem("Коміт від "+bi.Time.Local().Format("2006-01-02 15:04")+" · "+bi.GoVersion, "").Disable()
	}
	infoItem.AddSeparator()
	siteItem := infoItem.AddSubMenuItem("🌐 Відкрити сайт", "Веб-дашборд на твоєму сервері")
	repoItem := infoItem.AddSubMenuItem("💻 Проєкт на GitHub", client.ProjectURL)
	a.updCheck = infoItem.AddSubMenuItem(updCheckTitle, "Запитати сервер про нову версію клієнта")
	quitItem := systray.AddMenuItem("Вихід", "Шпалери-покарання НЕ знімаються при виході 😈")

	systray.SetOnTapped(func() { a.kick() }) // left click = refresh

	a.widget = newWidget(a.dataDir, a.ping, a.openWeb, a.openUpdate, a.widgetClosed)
	a.widget.SetSnap(a.cfg.WidgetSnapPx)
	if a.cfg.ShowWidget {
		go a.widget.Show(winapi.WidgetMode(a.cfg.NormalizedWidgetMode()))
	}

	go a.loop()
	go a.trackLoop()
	go func() {
		for {
			select {
			case <-a.pingItem.ClickedCh:
				go a.ping()
			case <-refreshItem.ClickedCh:
				a.kick()
			case <-a.updateItem.ClickedCh:
				a.openUpdate()
			case <-siteItem.ClickedCh:
				a.openWeb()
			case <-a.updCheck.ClickedCh:
				go a.checkUpdateNow()
			case <-repoItem.ClickedCh:
				_ = winapi.Open(client.ProjectURL)
			case <-a.netCopy.ClickedCh:
				go a.copyNetReport()
			case <-a.netLogItem.ClickedCh:
				go a.logNetReport()
			case <-a.achItem.ClickedCh:
				a.openWebAt("#/achievements")
			case <-demoWarn.ClickedCh:
				go a.demo(client.ModeWarning)
			case <-demoFail.ClickedCh:
				go a.demo(client.ModeFailed)
			case <-demoSndWarn.ClickedCh:
				go a.locked(func() { a.engine.DemoSound(client.ModeWarning) })
			case <-demoSndFail.ClickedCh:
				go a.locked(func() { a.engine.DemoSound(client.ModeFailed) })
			case <-demoStop.ClickedCh:
				go a.demo(client.ModeNone)
			case <-a.snoozeItem.ClickedCh:
				go a.snooze()
			case <-imagesItem.ClickedCh:
				_ = winapi.Open(filepath.Dir(a.cfg.FailedImagesDir))
			case <-configItem.ClickedCh:
				_ = winapi.Open(a.cfgPath)
			case <-reloadItem.ClickedCh:
				go func() { a.reload(true); a.kick() }()
			case <-a.autoItem.ClickedCh:
				on := !a.autoItem.Checked()
				if err := winapi.SetAutostart(appName, on); err != nil {
					winapi.MessageBox(title, "Не вдалося змінити автозапуск: "+err.Error(), winapi.MBError)
					continue
				}
				if on {
					a.autoItem.Check()
				} else {
					a.autoItem.Uncheck()
				}
			case <-a.widgetItem.ClickedCh:
				go a.toggleWidget()
			case <-a.modeDesk.ClickedCh:
				go a.setWidgetMode(client.WidgetModeDesktop)
			case <-a.modeOver.ClickedCh:
				go a.setWidgetMode(client.WidgetModeOverlay)
			case <-quitItem.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func (a *app) kick() {
	select {
	case a.refresh <- struct{}{}:
	default:
	}
}

func (a *app) loop() {
	for {
		a.reload(false)
		a.poll()
		wait := a.cfg.PollInterval()
		if a.cfgErr != nil {
			wait = 5 * time.Second // react quickly once the config is fixed
		}
		select {
		case <-time.After(wait):
		case <-a.refresh:
		}
	}
}

// loadConfig (re)reads config.json. Never fatal: problems end up in cfgErr
// and are shown in the tray. Returns true if a template was just created.
func (a *app) loadConfig() (created bool) {
	cfg, err := client.LoadConfig(a.cfgPath, a.dataDir)
	created = errors.Is(err, client.ErrConfigCreated)
	if created {
		err = nil
	}
	if st, e := os.Stat(a.cfgPath); e == nil {
		a.cfgMod = st.ModTime()
	}
	if err != nil { // unreadable or broken JSON: keep the previous settings
		log.Println("config:", err)
		a.cfgErr = err
		if a.engine == nil {
			a.useConfig(client.DefaultConfig(a.dataDir))
		}
		return created
	}
	a.cfgErr = cfg.Validate()
	a.useConfig(cfg)
	return created
}

func (a *app) useConfig(cfg client.Config) {
	a.cfg = cfg
	a.api = client.NewAPIClient(cfg, a.netStats)
	a.localGit = client.LocalGit{Repos: cfg.LocalRepos, Authors: cfg.LocalAuthorEmails}
	if a.tracker == nil {
		a.tracker = client.NewTracker(filepath.Join(a.dataDir, "activity.json"), cfg.ActivityIgnore)
	} else {
		a.tracker.SetIgnore(cfg.ActivityIgnore)
	}
	if a.engine == nil {
		a.desk = &winapi.Desktop{DataDir: a.dataDir}
		a.engine = client.NewEngine(cfg, a.desk, a.dataDir)
	} else {
		a.engine.Cfg = cfg
	}
	a.desk.Managed = []string{cfg.WarningImagesDir, cfg.FailedImagesDir}
	installAssets(cfg, a.dataDir)
}

// reload re-reads the config if the file changed (or always, if force).
func (a *app) reload(force bool) {
	st, err := os.Stat(a.cfgPath)
	if !force && (err != nil || st.ModTime().Equal(a.cfgMod)) {
		return
	}
	a.mu.Lock()
	a.loadConfig()
	a.mu.Unlock()
	log.Println("config reloaded, error:", a.cfgErr)
	a.updateGitItem()
	if a.widget != nil { // apply widget settings edited by hand
		a.widget.SetSnap(a.cfg.WidgetSnapPx)
		mode := winapi.WidgetMode(a.cfg.NormalizedWidgetMode())
		switch open := a.widget.IsOpen(); {
		case a.cfg.ShowWidget && !open:
			a.widget.Show(mode)
			a.locked(func() { a.pushWidget("") })
		case !a.cfg.ShowWidget && open:
			a.widget.Close()
		case open:
			a.widget.SetMode(mode)
		}
		a.syncWidgetMenu()
	}
}

func (a *app) updateGitItem() {
	if a.gitItem == nil {
		return
	}
	if len(a.localGit.Repos) == 0 {
		a.gitItem.Hide()
		return
	}
	a.gitItem.SetTitle(fmt.Sprintf("📁 Відстежую локальних репо: %d", len(a.localGit.Repos)))
	a.gitItem.Show()
}

func (a *app) showConfigProblem() {
	if a.engine.Demo == "" {
		systray.SetIcon(client.StateIcon("offline"))
	}
	systray.SetTooltip(title + ": потрібно налаштувати конфіг")
	a.statusItem.SetTitle("⚙️ Конфіг: " + shorten(a.cfgErr.Error(), 70))
	for _, it := range a.userItems {
		it.Hide()
	}
}

func (a *app) poll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfgErr != nil {
		a.showConfigProblem()
		a.pushWidget("config")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, err := a.api.Status(ctx)
	if err != nil {
		log.Println("status:", err)
		systray.SetIcon(client.StateIcon("offline"))
		systray.SetTooltip(title + ": немає зв'язку з сервером")
		a.statusItem.SetTitle("❌ Немає зв'язку: " + shorten(err.Error(), 60))
		a.pushWidget("offline")
		return
	}
	me, ok := client.FindMe(st)
	if ok {
		if loc, err := time.LoadLocation(me.Timezone); err == nil {
			a.tracker.SetLocation(loc)
		}
	}
	if a.cfg.ActivityTracking && a.cfg.ShareActivity {
		if err := a.api.Presence(ctx, a.tracker.Report()); err != nil {
			log.Println("presence:", err)
		}
	}
	if ok && !me.DoneToday && len(a.localGit.Repos) > 0 {
		if upd, counted := a.checkLocalGit(ctx, me); counted {
			// re-read so both users' lines are fresh
			if st2, err := a.api.Status(ctx); err == nil {
				st = st2
				me, _ = client.FindMe(st)
			} else {
				me = upd
			}
		}
	}
	a.lastStatus = &st
	a.checkUpdate(ctx, false)
	a.render(st)
	a.pushWidget("")
	if !ok {
		return
	}
	if _, err := a.engine.Apply(me, time.Now()); err != nil {
		log.Println("apply:", err)
	}
	a.checkAchievements(ctx)
}

// checkAchievements refreshes the tray counter and announces new unlocks
// with a Windows notification. Call with a.mu held.
func (a *app) checkAchievements(ctx context.Context) {
	if time.Since(a.achAt) < achEvery {
		return
	}
	a.achAt = time.Now()
	p, err := a.api.Progress(ctx)
	if err != nil {
		log.Println("progress:", err)
		return
	}
	n, total := client.AchievementCount(p)
	a.achItem.SetTitle(fmt.Sprintf("🏆 Ачівки: %d / %d", n, total))

	fresh := client.NewUnlocks(p)
	if len(fresh) == 0 || !a.cfg.AchievementNotifications {
		return
	}
	title, body := client.UnlockMessage(fresh)
	log.Printf("achievements unlocked: %s — %s", title, body)
	// Mark seen first: a broken toast must not repeat every minute.
	if err := a.api.AchievementsSeen(ctx); err != nil {
		log.Println("achievements seen:", err)
		return
	}
	url := a.webURL("#/achievements")
	go func() {
		if err := winapi.Toast(title, body, url); err != nil {
			log.Println(err)
			winapi.MessageBox(title, body, winapi.MBInfo) // fallback
		}
	}()
}

// checkUpdate asks the server for the latest client release, shows it in
// the tray and announces each new version once with a Windows
// notification (a required update — on every start). manual (the tray
// item) skips the hourly limit and always notifies. Call with a.mu held.
func (a *app) checkUpdate(ctx context.Context, manual bool) (found bool, err error) {
	if !client.IsRelease(version) || !manual && time.Since(a.updAt) < updateEvery {
		return false, nil
	}
	a.updAt = time.Now()
	v, err := a.api.ClientVersion(ctx)
	if err != nil {
		log.Println("client version:", err)
		return false, err
	}
	u, ok := client.CheckUpdate(version, v)
	if !ok {
		a.update = nil
		a.updateItem.Hide()
		return false, nil
	}
	a.update = &u
	a.updateItem.SetTitle(client.UpdateMenuTitle(u, version))
	a.updateItem.Show()

	seen := filepath.Join(a.dataDir, "update_seen.txt")
	if !manual && client.UpdateSeen(seen) == u.Latest && (!u.Required || a.updToasted) {
		return true, nil
	}
	// Mark seen first: a broken toast must not repeat every hour.
	a.updToasted = true
	if err := client.SetUpdateSeen(seen, u.Latest); err != nil {
		log.Println("update seen:", err)
	}
	title, body := client.UpdateMessage(u, version)
	log.Printf("update available: %s (required %v)", u.Latest, u.Required)
	go func() {
		if err := winapi.Toast(title, body, u.URL); err != nil {
			log.Println(err)
			winapi.MessageBox(title, body+"\n\n"+u.URL, winapi.MBInfo) // fallback
		}
	}()
	return true, nil
}

const updCheckTitle = "🔍 Перевірити оновлення"

// checkUpdateNow runs a manual check from the tray and shows the result
// in the menu item for a few seconds.
func (a *app) checkUpdateNow() {
	result := func() string {
		if !client.IsRelease(version) {
			return "ℹ️ Збірка " + version + " — оновлення не перевіряються"
		}
		a.updCheck.SetTitle("⏳ Перевіряю…")
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.cfgErr != nil {
			return "⚙️ Спочатку налаштуй конфіг"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		found, err := a.checkUpdate(ctx, true)
		switch {
		case err != nil:
			return "❌ Не вдалося: " + shorten(err.Error(), 50)
		case found:
			a.pushWidget("")
			return "⬆️ Є нова версія — дивись перший рядок меню"
		default:
			return "✅ У тебе остання версія (v" + version + ")"
		}
	}()
	a.updCheck.SetTitle(result)
	time.Sleep(5 * time.Second)
	a.updCheck.SetTitle(updCheckTitle)
}

// openUpdate opens the release page of the available update.
func (a *app) openUpdate() {
	a.mu.Lock()
	u := a.update
	a.mu.Unlock()
	if u != nil {
		_ = winapi.Open(u.URL)
	}
}

// checkLocalGit pings the server if a local repo has a commit from today.
func (a *app) checkLocalGit(ctx context.Context, me api.UserStatus) (api.UserStatus, bool) {
	loc, err := time.LoadLocation(me.Timezone)
	if err != nil {
		loc = time.Local
	}
	c, errs := a.localGit.FindToday(ctx, time.Now(), loc)
	for _, e := range errs {
		log.Println("local git:", e)
	}
	if c == nil {
		title := fmt.Sprintf("📁 Локальні репо (%d): сьогодні комітів нема", len(a.localGit.Repos))
		if len(errs) > 0 {
			title = "📁 Локальні репо: помилка — " + shorten(errs[0].Error(), 50)
		}
		a.gitItem.SetTitle(title)
		return me, false
	}
	resp, err := a.api.Ping(ctx, "local_git", c.Note())
	if err != nil {
		log.Println("local git ping:", err)
		return me, false
	}
	log.Println("local git counted:", c.Note())
	a.gitItem.SetTitle("📁 Зараховано коміт: " + shorten(filepath.Base(c.Repo)+" "+c.Hash[:min(7, len(c.Hash))], 50))
	return resp.User, true
}

// trackLoop samples the foreground program every few seconds.
func (a *app) trackLoop() {
	const every = 5 * time.Second
	tick := time.NewTicker(every)
	defer tick.Stop()
	lastSave := time.Now()
	for now := range tick.C {
		a.renderNet()
		if !a.cfg.ActivityTracking {
			continue
		}
		exe, err := winapi.ForegroundExe()
		if err != nil {
			exe = "" // e.g. an elevated window we may not inspect
		}
		a.tracker.Sample(now, exe, winapi.IdleDuration() >= client.IdleAfter)
		if now.Sub(lastSave) >= time.Minute {
			if err := a.tracker.Save(); err != nil {
				log.Println("activity save:", err)
			}
			lastSave = now
		}
	}
}

// renderNet refreshes the "📡 Мережа" submenu from the counters.
func (a *app) renderNet() {
	if a.netItem == nil {
		return
	}
	snap := a.netStats.Snapshot(time.Now())
	a.netItem.SetTitle(snap.Title())
	lines := snap.Lines()
	for i, it := range a.netRows {
		if i < len(lines) {
			it.SetTitle(shorten(lines[i], 90))
			it.Show()
		} else {
			it.Hide()
		}
	}
	for i, it := range a.netEpRows {
		if i < len(snap.Endpoints) {
			it.SetTitle(snap.Endpoints[i].Line())
			it.Show()
		} else {
			it.Hide()
		}
	}
}

const netCopyTitle = "📋 Скопіювати звіт"

func (a *app) netReport() string {
	return a.netStats.Snapshot(time.Now()).Report(a.netStats.Started, a.cfg.ServerURL, client.ReadBuildInfo(version).String())
}

// copyNetReport puts the network report on the clipboard and confirms it
// in the menu item for a few seconds.
func (a *app) copyNetReport() {
	if err := winapi.SetClipboardText(a.netReport()); err != nil {
		log.Println("clipboard:", err)
		a.netCopy.SetTitle("❌ Не вдалося скопіювати")
	} else {
		a.netCopy.SetTitle("✅ Скопійовано")
	}
	time.Sleep(3 * time.Second)
	a.netCopy.SetTitle(netCopyTitle)
}

// logNetReport writes the network report to client.log and opens it.
func (a *app) logNetReport() {
	log.Print("network report\n" + strings.ReplaceAll(a.netReport(), "\r\n", "\n"))
	_ = winapi.Open(filepath.Join(a.dataDir, "client.log"))
}

func (a *app) renderStats() {
	if a.statsItem == nil {
		return
	}
	if !a.cfg.ActivityTracking {
		a.statsItem.SetTitle("📊 Статистика програм вимкнена")
		for _, it := range a.statsRows {
			it.Hide()
		}
		return
	}
	top := a.tracker.Top(len(a.statsRows))
	var total int64
	for _, x := range top {
		total += x.Seconds
	}
	a.statsItem.SetTitle("📊 Сьогодні в програмах: " + client.FormatDuration(time.Duration(total)*time.Second))
	for i, it := range a.statsRows {
		if i < len(top) {
			it.SetTitle(fmt.Sprintf("%s — %s", top[i].App, client.FormatDuration(time.Duration(top[i].Seconds)*time.Second)))
			it.Show()
		} else {
			it.Hide()
		}
	}
}

func (a *app) render(st api.StatusResponse) {
	a.renderStats()
	me, _ := client.FindMe(st)
	if a.engine.Demo != "" {
		systray.SetIcon(client.StateIcon(string(a.engine.Demo)))
	} else {
		systray.SetIcon(client.StateIcon(me.State))
	}
	tip := client.Tooltip(me)
	if a.update != nil {
		tip += "\n⬆️ Є нова версія клієнта"
	}
	systray.SetTooltip(tip)
	a.statusItem.SetTitle("Оновлено о " + time.Now().Format("15:04"))
	for i, it := range a.userItems {
		if i < len(st.Users) {
			u := st.Users[i]
			it.SetTitle(client.StatusLine(u, u.Name == st.Me))
			it.Show()
		} else {
			it.Hide()
		}
	}
	if me.DoneToday {
		a.pingItem.SetTitle("✅ На сьогодні зараховано (можна ще)")
	} else {
		a.pingItem.SetTitle("✅ Я сьогодні вчив Go (відмітити)")
	}
}

func (a *app) ping() {
	if a.cfgErr != nil {
		a.locked(func() { a.pushWidget("config") })
		winapi.MessageBox(title, "Спочатку налаштуй конфіг ("+a.cfgPath+"):\n\n"+a.cfgErr.Error(), winapi.MBWarning)
		return
	}
	a.mu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	resp, err := a.api.Ping(ctx, "manual", "tray")
	cancel()
	if err == nil {
		_, err = a.engine.Apply(resp.User, time.Now())
	}
	a.mu.Unlock()

	if err != nil {
		log.Println("ping:", err)
		a.locked(func() { a.pushWidget("") }) // un-stick the widget button
		winapi.MessageBox(title, "Не вдалося відмітити заняття:\n"+err.Error(), winapi.MBError)
		return
	}
	a.kick()
}

func (a *app) locked(f func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f()
}

// demo forces a punishment mode (or stops the demo with ModeNone).
func (a *app) demo(m client.Mode) {
	a.mu.Lock()
	var err error
	if m == client.ModeNone {
		err = a.engine.DemoStop()
	} else {
		err = a.engine.DemoShow(m, time.Now())
	}
	a.mu.Unlock()
	if err != nil {
		log.Println("demo:", err)
		winapi.MessageBox(title, "Демо: "+err.Error(), winapi.MBError)
		return
	}
	switch m {
	case client.ModeWarning:
		a.demoItem.SetTitle("🧪 Демо: ⚠️ попередження (активне)")
		systray.SetIcon(client.StateIcon("warning"))
	case client.ModeFailed:
		a.demoItem.SetTitle("🧪 Демо: 💀 провал (активне)")
		systray.SetIcon(client.StateIcon("failed"))
	default:
		a.demoItem.SetTitle("🧪 Демо")
		a.kick() // back to the real state
	}
	a.locked(func() { a.pushWidget("") })
}

// pushWidget sends the latest known state to the widget. Call with a.mu held.
func (a *app) pushWidget(errKind string) {
	if a.widget == nil {
		return
	}
	d := widgetData{Error: errKind, Demo: string(a.engine.Demo), Users: []widgetUser{}}
	if a.update != nil {
		d.Update = &widgetUpdate{Version: a.update.Latest, Required: a.update.Required}
	}
	if a.lastStatus != nil {
		for _, u := range a.lastStatus.Users {
			wu := widgetUser{
				Name: u.Name, Me: u.Name == a.lastStatus.Me, Streak: u.Streak, State: u.State,
				Deadline: u.Deadline.Format(time.RFC3339), DoneToday: u.DoneToday,
			}
			if u.Presence != nil {
				wu.Presence = u.Presence
			}
			d.Users = append(d.Users, wu)
		}
	}
	if a.cfg.ActivityTracking {
		top := a.tracker.Top(0)
		for _, x := range top {
			d.TodayTotal += x.Seconds
		}
		if len(top) > 0 {
			d.TodayTop = top[0].App
		}
	}
	a.widget.Update(d)
}

func (a *app) syncWidgetMenu() {
	if a.widgetItem == nil {
		return
	}
	if a.widget != nil && a.widget.IsOpen() || a.widget == nil && a.cfg.ShowWidget {
		a.widgetItem.Check()
	} else {
		a.widgetItem.Uncheck()
	}
	if a.cfg.NormalizedWidgetMode() == client.WidgetModeOverlay {
		a.modeOver.Check()
		a.modeDesk.Uncheck()
	} else {
		a.modeDesk.Check()
		a.modeOver.Uncheck()
	}
}

func (a *app) toggleWidget() {
	if a.widget.IsOpen() {
		a.widget.Close() // widgetClosed saves show_widget=false
		return
	}
	a.widget.Show(winapi.WidgetMode(a.cfg.NormalizedWidgetMode()))
	a.saveConfigFields(map[string]any{"show_widget": true})
	a.locked(func() { a.pushWidget("") })
	a.syncWidgetMenu()
}

func (a *app) setWidgetMode(mode string) {
	a.saveConfigFields(map[string]any{"widget_mode": mode})
	a.cfg.WidgetMode = mode
	if a.widget.IsOpen() {
		a.widget.SetMode(winapi.WidgetMode(mode))
	} else {
		a.widget.Show(winapi.WidgetMode(mode))
		a.saveConfigFields(map[string]any{"show_widget": true})
		a.locked(func() { a.pushWidget("") })
	}
	a.syncWidgetMenu()
}

// widgetClosed runs when the widget window goes away (✕ or tray toggle).
func (a *app) widgetClosed() {
	a.saveConfigFields(map[string]any{"show_widget": false})
	a.cfg.ShowWidget = false
	a.syncWidgetMenu()
}

func (a *app) saveConfigFields(fields map[string]any) {
	if err := client.SetConfigFields(a.cfgPath, fields); err != nil {
		log.Println("config save:", err)
		return
	}
	// our own write must not look like an external edit
	if st, err := os.Stat(a.cfgPath); err == nil {
		a.mu.Lock()
		a.cfgMod = st.ModTime()
		a.mu.Unlock()
	}
}

func (a *app) openWeb() { a.openWebAt("") }

// openWebAt opens the web app, optionally at a route like "#/achievements".
func (a *app) openWebAt(route string) {
	if u := a.webURL(route); u != "" {
		_ = winapi.Open(u)
	}
}

func (a *app) webURL(route string) string {
	base := strings.TrimRight(a.cfg.ServerURL, "/")
	if base == "" {
		return ""
	}
	return base + "/app/" + route
}

func (a *app) snooze() {
	a.mu.Lock()
	err := a.engine.Snooze(time.Now().Add(snoozeFor))
	a.mu.Unlock()
	if err != nil {
		winapi.MessageBox(title, "Не вдалося повернути шпалери:\n"+err.Error(), winapi.MBError)
	}
}

// installAssets unpacks the bundled wallpapers and sounds into empty folders.
func installAssets(cfg client.Config, dataDir string) {
	jobs := []struct {
		src, dst string
		exts     map[string]bool
	}{
		{"images/warning", cfg.WarningImagesDir, client.ImageExts()},
		{"images/failed", cfg.FailedImagesDir, client.ImageExts()},
		{"sounds/warning", filepath.Join(dataDir, "sounds", "warning"), client.SoundExts()},
		{"sounds/failed", filepath.Join(dataDir, "sounds", "failed"), client.SoundExts()},
	}
	for _, j := range jobs {
		if j.dst == "" {
			continue
		}
		if n, err := client.InstallDefaults(assets.FS, j.src, j.dst, j.exts); err != nil {
			log.Println("assets:", j.dst, err)
		} else if n > 0 {
			log.Printf("assets: installed %d files into %s", n, j.dst)
		}
	}
}

func setupLog(dataDir string) {
	f, err := os.OpenFile(filepath.Join(dataDir, "client.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		log.SetOutput(f)
	}
}

func fatal(err error) {
	winapi.MessageBox(title, err.Error(), winapi.MBError)
	os.Exit(1)
}

func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
