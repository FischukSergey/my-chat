//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"my-chat/internal/store"
)

func TestUserRepository_SearchByUsernamePrefix(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	alice := insertUserWithUsername(t, ctx, s, "alice_"+uuid.NewString()[:8])
	bobName := "bob_" + uuid.NewString()[:8]
	bobbyName := "bobby_" + uuid.NewString()[:8]
	carolName := "carol_" + uuid.NewString()[:8]
	bob := insertUserWithUsername(t, ctx, s, bobName)
	_ = insertUserWithUsername(t, ctx, s, bobbyName)
	_ = insertUserWithUsername(t, ctx, s, carolName)

	blockedID := uuid.NewString()
	blockedName := "bobz_" + uuid.NewString()[:8]
	_, err := s.DB().Exec(ctx,
		"INSERT INTO users (id, username, status) VALUES ($1, $2, 'blocked')",
		blockedID, blockedName,
	)
	if err != nil {
		t.Fatalf("insert blocked: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.DB().Exec(ctx, "DELETE FROM users WHERE id = $1", blockedID)
	})

	repo := store.NewUserRepository(s)
	prefix := "bob"
	hits, err := repo.SearchByUsernamePrefix(ctx, prefix, alice, 50)
	if err != nil {
		t.Fatalf("SearchByUsernamePrefix: %v", err)
	}

	names := make([]string, 0, len(hits))
	for _, u := range hits {
		if u.ID == alice {
			t.Error("caller must be excluded")
		}
		if u.ID == blockedID {
			t.Error("blocked user must be excluded")
		}
		if u.Status != "active" {
			t.Errorf("unexpected status %q", u.Status)
		}
		names = append(names, u.Username)
	}

	foundBob, foundBobby := false, false
	for _, name := range names {
		switch name {
		case bobName:
			foundBob = true
		case bobbyName:
			foundBobby = true
		case carolName:
			t.Errorf("carol must not match prefix %q", prefix)
		}
	}
	if !foundBob || !foundBobby {
		t.Errorf("expected bob and bobby, got %v", names)
	}

	// Exclude bob himself when he searches.
	hits, err = repo.SearchByUsernamePrefix(ctx, prefix, bob, 50)
	if err != nil {
		t.Fatalf("SearchByUsernamePrefix as bob: %v", err)
	}
	for _, u := range hits {
		if u.ID == bob {
			t.Error("self must be excluded when searching as bob")
		}
	}
}
