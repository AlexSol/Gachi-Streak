// Admin commands for achievements: open or reset them for testing and demos.
// They work on the database file directly (safe while the server runs: WAL
// + busy timeout), so only someone with shell access to the server, e.g.
// `fly ssh console`, can use them; there is no HTTP endpoint.
//
//	gachi-server achievements unlock [-name NAME] [-db gachi.db]
//	gachi-server achievements lock   [-name NAME] [-db gachi.db]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/AlexSol/gachi-streak/internal/progress"
	"github.com/AlexSol/gachi-streak/internal/store"
	"github.com/AlexSol/gachi-streak/internal/streak"
)

func cmdAchievements(args []string) error {
	if len(args) < 1 || (args[0] != "unlock" && args[0] != "lock") {
		return errors.New("usage: gachi-server achievements unlock|lock [-name NAME] [-db gachi.db]")
	}
	fs := flag.NewFlagSet("achievements "+args[0], flag.ExitOnError)
	dbPath := fs.String("db", env("GACHI_DB", "gachi.db"), "SQLite database file")
	name := fs.String("name", "", "user name (empty = every user)")
	fs.Parse(args[1:])

	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	users, err := st.ListUsers(ctx)
	if err != nil {
		return err
	}
	repo := st.Achievements()
	now := time.Now()
	matched := false
	for _, u := range users {
		if *name != "" && u.Name != *name {
			continue
		}
		matched = true
		if args[0] == "lock" {
			n, err := repo.DeleteByUser(ctx, u.ID)
			if err != nil {
				return err
			}
			fmt.Printf("%s: %d achievements locked (the ones you still qualify for come back on the next pass)\n", u.Name, n)
			continue
		}
		today := streak.DayOf(now, u.Location())
		n := 0
		for _, def := range progress.Achievements {
			ok, err := repo.Unlock(ctx, u.ID, def.ID, today, now)
			if err != nil {
				return err
			}
			if ok {
				n++
			}
		}
		fmt.Printf("%s: %d achievements unlocked (%d already had)\n", u.Name, n, len(progress.Achievements)-n)
	}
	if !matched {
		if *name != "" {
			return fmt.Errorf("user %q not found", *name)
		}
		return errors.New("no users in " + *dbPath)
	}
	return nil
}
