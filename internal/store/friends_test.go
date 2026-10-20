package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFriendships(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	fr := s.Friends()
	now := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

	var ids []int64
	for _, name := range []string{"alex", "sasha", "olya"} {
		u, _, err := s.CreateUser(ctx, User{Name: name, Timezone: "UTC"}, now)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, u.ID)
	}
	alex, sasha, olya := ids[0], ids[1], ids[2]

	if u, err := s.UserByName(ctx, "sasha"); err != nil || u.ID != sasha {
		t.Fatalf("UserByName: %+v %v", u, err)
	}
	if _, err := s.UserByName(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UserByName missing: %v", err)
	}

	if _, err := fr.Request(ctx, alex, alex, now); err == nil {
		t.Fatal("self request must fail")
	}

	// alex → sasha: outgoing for alex, incoming for sasha; asking twice is a no-op.
	for range 2 {
		if rel, err := fr.Request(ctx, alex, sasha, now); err != nil || rel != RelOutgoing {
			t.Fatalf("request: %q %v", rel, err)
		}
	}
	rels, _ := fr.Relations(ctx, alex)
	if rels[sasha] != RelOutgoing || len(rels) != 1 {
		t.Fatalf("alex relations: %v", rels)
	}
	rels, _ = fr.Relations(ctx, sasha)
	if rels[alex] != RelIncoming {
		t.Fatalf("sasha relations: %v", rels)
	}
	in, out, err := fr.Requests(ctx, sasha)
	if err != nil || len(in) != 1 || in[0].Name != "alex" || len(out) != 0 || !in[0].CreatedAt.Equal(now) {
		t.Fatalf("sasha requests: %+v %+v %v", in, out, err)
	}

	// Only the addressee can accept; the requester gets ErrNotFound.
	if err := fr.Accept(ctx, alex, sasha, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("requester accept: %v", err)
	}
	if err := fr.Accept(ctx, sasha, alex, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if rel, _ := fr.Request(ctx, sasha, alex, now); rel != RelFriend {
		t.Fatalf("request to a friend: %q", rel)
	}
	for _, id := range []int64{alex, sasha} {
		rels, _ := fr.Relations(ctx, id)
		if len(rels) != 1 || (rels[alex] != RelFriend && rels[sasha] != RelFriend) {
			t.Fatalf("relations of %d: %v", id, rels)
		}
	}
	if in, out, _ := fr.Requests(ctx, sasha); len(in)+len(out) != 0 {
		t.Fatalf("accepted request still pending: %+v %+v", in, out)
	}

	// Either side can unfriend, once.
	if err := fr.Remove(ctx, sasha, alex); err != nil {
		t.Fatal(err)
	}
	if err := fr.Remove(ctx, alex, sasha); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second remove: %v", err)
	}

	// Two requests that meet become a friendship without an explicit accept.
	if _, err := fr.Request(ctx, olya, alex, now); err != nil {
		t.Fatal(err)
	}
	if rel, err := fr.Request(ctx, alex, olya, now); err != nil || rel != RelFriend {
		t.Fatalf("mutual request: %q %v", rel, err)
	}

	// Decline and cancel delete the request, so it can be sent again.
	if _, err := fr.Request(ctx, sasha, alex, now); err != nil {
		t.Fatal(err)
	}
	if err := fr.Cancel(ctx, alex, sasha); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancel someone else's request: %v", err)
	}
	if err := fr.Decline(ctx, alex, sasha); err != nil {
		t.Fatal(err)
	}
	if err := fr.Decline(ctx, alex, sasha); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second decline: %v", err)
	}
	if rel, _ := fr.Request(ctx, sasha, alex, now); rel != RelOutgoing {
		t.Fatalf("request after decline: %q", rel)
	}
	if err := fr.Cancel(ctx, sasha, alex); err != nil {
		t.Fatal(err)
	}
	rels, _ = fr.Relations(ctx, alex)
	if len(rels) != 1 || rels[olya] != RelFriend {
		t.Fatalf("alex final relations: %v", rels)
	}

	// Deleting a user drops their friendships (ON DELETE CASCADE).
	if _, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, olya); err != nil {
		t.Fatal(err)
	}
	if rels, _ := fr.Relations(ctx, alex); len(rels) != 0 {
		t.Fatalf("relations after delete: %v", rels)
	}
}
