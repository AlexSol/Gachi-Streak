// Command gachi-server is the shared streak backend.
//
//	gachi-server serve  [-addr :8080] [-db gachi.db] [-warn 5h]
//	gachi-server user add    -name alex -tz Europe/Lisbon [-github AlexSol] [-repo AlexSol/go-diary]
//	gachi-server user list
//	gachi-server user token  -name alex          (issue a new token)
//	gachi-server user github -name alex -github AlexSol [-repo ...]
//	gachi-server achievements unlock|lock [-name alex]  (open / reset achievements)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
	_ "time/tzdata" // time zones work even on Windows / scratch containers

	"github.com/AlexSol/gachi-streak/internal/server"
	"github.com/AlexSol/gachi-streak/internal/store"
	"github.com/AlexSol/gachi-streak/internal/webui"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = cmdServe(os.Args[2:])
	case "user":
		err = cmdUser(os.Args[2:])
	case "achievements":
		err = cmdAchievements(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  gachi-server serve [-addr :8080] [-db gachi.db] [-warn 5h] [-github-interval 10m]
                     [-release-repo owner/name] [-client-min 0.2.0]
  gachi-server user add    -name NAME -tz Europe/Kyiv [-github LOGIN] [-repo owner/repo]
  gachi-server user list
  gachi-server user token  -name NAME
  gachi-server user github -name NAME -github LOGIN [-repo owner/repo]
  gachi-server achievements unlock|lock [-name NAME]   (no -name = every user)

env: GACHI_DB, GACHI_ADDR, GACHI_RELEASE_REPO, GACHI_CLIENT_MIN,
     GITHUB_TOKEN (optional, raises GitHub API limits)`)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", env("GACHI_ADDR", ":8080"), "listen address")
	dbPath := fs.String("db", env("GACHI_DB", "gachi.db"), "SQLite database file")
	warn := fs.Duration("warn", 5*time.Hour, "warning window before the deadline")
	tick := fs.Duration("tick", time.Minute, "deadline worker interval")
	ghEvery := fs.Duration("github-interval", 10*time.Minute, "GitHub auto-check interval (0 = off)")
	releaseRepo := fs.String("release-repo", env("GACHI_RELEASE_REPO", "AlexSol/Gachi-Streak"), "GitHub repo with client releases (\"\" = off)")
	clientMin := fs.String("client-min", env("GACHI_CLIENT_MIN", ""), "oldest supported client version, older ones must update")
	debug := fs.Bool("debug", false, "verbose logging")
	fs.Parse(args)

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	srv := server.New(st, *warn, log)
	srv.UI = webui.Handler("/app/")
	go srv.RunDeadlineWorker(ctx, *tick)
	gh := &server.GitHubChecker{S: srv, Token: os.Getenv("GITHUB_TOKEN")}
	if *ghEvery > 0 {
		go gh.Run(ctx, *ghEvery)
	}
	if *releaseRepo != "" {
		srv.Releases = &server.ReleaseChecker{GH: gh, Repo: *releaseRepo, Min: strings.TrimPrefix(*clientMin, "v")}
		go srv.Releases.Run(ctx, 15*time.Minute)
	}

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Info("gachi-server listening", "addr", *addr, "db", *dbPath, "warn", *warn)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("bye")
	return nil
}

func cmdUser(args []string) error {
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}
	fs := flag.NewFlagSet("user "+args[0], flag.ExitOnError)
	dbPath := fs.String("db", env("GACHI_DB", "gachi.db"), "SQLite database file")
	name := fs.String("name", "", "user name")
	tz := fs.String("tz", "Europe/Kyiv", "IANA time zone; the deadline is local midnight")
	gh := fs.String("github", "", "GitHub login for auto-check of pushes")
	repo := fs.String("repo", "", "only count pushes to this repo (owner/name)")
	fs.Parse(args[1:])

	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	switch args[0] {
	case "add":
		if *name == "" {
			return errors.New("-name is required")
		}
		u, token, err := st.CreateUser(ctx, store.User{
			Name: *name, Timezone: *tz, GitHubLogin: *gh, GitHubRepo: *repo,
		}, time.Now())
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return fmt.Errorf("user %q already exists — to get a new token run: gachi-server user token -name %s", *name, *name)
			}
			return err
		}
		fmt.Printf("user %q created (tz %s, accountable from %s)\n", u.Name, u.Timezone, u.CreatedDay)
		fmt.Printf("token (shown once, put it into the client config):\n%s\n", token)
	case "token":
		if *name == "" {
			return errors.New("-name is required")
		}
		token, err := st.RotateToken(ctx, *name)
		if err != nil {
			return err
		}
		fmt.Println(token)
	case "github":
		if *name == "" {
			return errors.New("-name is required")
		}
		return st.UpdateGitHub(ctx, *name, *gh, *repo)
	case "list":
		users, err := st.ListUsers(ctx)
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tTZ\tSTREAK\tBEST\tLAST ACTIVE\tGITHUB")
		for _, u := range users {
			s, _ := st.GetStreak(ctx, u.ID)
			ghs := u.GitHubLogin
			if u.GitHubRepo != "" {
				ghs += " (" + u.GitHubRepo + ")"
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%d\t%s\t%s\n", u.ID, u.Name, u.Timezone, s.Current, s.Best, s.LastActiveDay, ghs)
		}
		w.Flush()
	default:
		usage()
		os.Exit(2)
	}
	return nil
}
