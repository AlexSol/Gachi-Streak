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
	"sync"
	"time"
	_ "time/tzdata"

	"fyne.io/systray"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/client"
	"github.com/AlexSol/gachi-streak/internal/winapi"
)

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
}

func main() {
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

	cfgPath := filepath.Join(dataDir, "config.json")
	cfg, err := client.LoadConfig(cfgPath, dataDir)
	if errors.Is(err, client.ErrConfigCreated) {
		winapi.MessageBox(title, fmt.Sprintf(
			"Створено конфіг:\n%s\n\nВпиши server_url і token (видає `gachi-server user add`), збережи і запусти програму ще раз.\n\nКартинки клади в папки images\\warning та images\\failed поруч із конфігом.",
			cfgPath), winapi.MBInfo)
		_ = winapi.Open(cfgPath)
		return
	}
	if err == nil {
		err = cfg.Validate()
	}
	if err != nil {
		winapi.MessageBox(title, "Проблема з конфігом "+cfgPath+":\n\n"+err.Error(), winapi.MBError)
		_ = winapi.Open(cfgPath)
		return
	}

	desk := &winapi.Desktop{DataDir: dataDir, Managed: []string{cfg.WarningImagesDir, cfg.FailedImagesDir}}
	a := &app{
		cfg:     cfg,
		cfgPath: cfgPath,
		dataDir: dataDir,
		api:     client.NewAPIClient(cfg),
		engine:  client.NewEngine(cfg, desk, dataDir),
		refresh: make(chan struct{}, 1),
	}
	systray.Run(a.onReady, func() { log.Println("exit") })
}

func (a *app) onReady() {
	systray.SetIcon(client.StateIcon("offline"))
	systray.SetTitle(title)
	systray.SetTooltip(title + ": підключаюсь…")

	a.statusItem = systray.AddMenuItem("Підключаюсь до сервера…", "")
	a.statusItem.Disable()
	for i := 0; i < maxUsers; i++ {
		it := systray.AddMenuItem("", "")
		it.Disable()
		it.Hide()
		a.userItems = append(a.userItems, it)
	}
	systray.AddSeparator()
	a.pingItem = systray.AddMenuItem("✅ Я сьогодні вчив Go (відмітити)", "POST /ping")
	refreshItem := systray.AddMenuItem("🔄 Оновити", "")
	a.snoozeItem = systray.AddMenuItem("🙈 Пауза 15 хв (повернути шпалери)", "Наприклад, для демонстрації екрана")
	systray.AddSeparator()
	imagesItem := systray.AddMenuItem("📂 Папка з картинками", "")
	configItem := systray.AddMenuItem("⚙️ Відкрити конфіг", "Після змін перезапусти програму")
	a.autoItem = systray.AddMenuItemCheckbox("Запускати з Windows", "", winapi.AutostartEnabled(appName))
	systray.AddSeparator()
	quitItem := systray.AddMenuItem("Вихід", "Шпалери-покарання НЕ знімаються при виході 😈")

	systray.SetOnTapped(func() { a.kick() }) // left click = refresh

	go a.loop()
	go func() {
		for {
			select {
			case <-a.pingItem.ClickedCh:
				go a.ping()
			case <-refreshItem.ClickedCh:
				a.kick()
			case <-a.snoozeItem.ClickedCh:
				go a.snooze()
			case <-imagesItem.ClickedCh:
				_ = winapi.Open(filepath.Dir(a.cfg.FailedImagesDir))
			case <-configItem.ClickedCh:
				_ = winapi.Open(a.cfgPath)
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
	t := time.NewTicker(a.cfg.PollInterval())
	defer t.Stop()
	for {
		a.poll()
		select {
		case <-t.C:
		case <-a.refresh:
		}
	}
}

func (a *app) poll() {
	a.mu.Lock()
	defer a.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, err := a.api.Status(ctx)
	if err != nil {
		log.Println("status:", err)
		systray.SetIcon(client.StateIcon("offline"))
		systray.SetTooltip(title + ": немає зв'язку з сервером")
		a.statusItem.SetTitle("❌ Немає зв'язку: " + shorten(err.Error(), 60))
		return
	}
	a.render(st)

	me, ok := client.FindMe(st)
	if !ok {
		return
	}
	if _, err := a.engine.Apply(me, time.Now()); err != nil {
		log.Println("apply:", err)
	}
}

func (a *app) render(st api.StatusResponse) {
	me, _ := client.FindMe(st)
	systray.SetIcon(client.StateIcon(me.State))
	systray.SetTooltip(client.Tooltip(me))
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
	a.mu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	resp, err := a.api.Ping(ctx, "tray")
	cancel()
	if err == nil {
		_, err = a.engine.Apply(resp.User, time.Now())
	}
	a.mu.Unlock()

	if err != nil {
		log.Println("ping:", err)
		winapi.MessageBox(title, "Не вдалося відмітити заняття:\n"+err.Error(), winapi.MBError)
		return
	}
	a.kick()
}

func (a *app) snooze() {
	a.mu.Lock()
	err := a.engine.Snooze(time.Now().Add(snoozeFor))
	a.mu.Unlock()
	if err != nil {
		winapi.MessageBox(title, "Не вдалося повернути шпалери:\n"+err.Error(), winapi.MBError)
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
