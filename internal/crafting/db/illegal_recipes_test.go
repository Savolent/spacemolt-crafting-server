package db

import (
	"context"
	"testing"
)

func TestIllegalRecipesStore_IsIllegal_NotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	// Apply migration 007 to create illegal_recipes table
	migration, err := GetMigration007()
	if err != nil {
		t.Fatalf("getting migration 007: %v", err)
	}
	if _, err = db.ExecContext(ctx, migration.UpSQL); err != nil {
		t.Fatalf("applying migration 007: %v", err)
	}

	store := NewIllegalRecipesStore(db)

	isIllegal, info, err := store.IsIllegal(ctx, "nonexistent_recipe")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if isIllegal {
		t.Fatal("expected recipe to not be illegal")
	}
	if info != nil {
		t.Fatal("expected nil info for non-illegal recipe")
	}
}

func TestIllegalRecipesStore_MarkIllegal(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	// Apply migration 007 to create illegal_recipes table
	migration, err := GetMigration007()
	if err != nil {
		t.Fatalf("getting migration 007: %v", err)
	}
	if _, err = db.ExecContext(ctx, migration.UpSQL); err != nil {
		t.Fatalf("applying migration 007: %v", err)
	}

	store := NewIllegalRecipesStore(db)

	err = store.MarkIllegal(ctx, "test_recipe", "test ban", "test location")
	if err != nil {
		t.Fatalf("failed to mark illegal: %v", err)
	}

	isIllegal, info, err := store.IsIllegal(ctx, "test_recipe")
	if err != nil {
		t.Fatalf("failed to check illegal: %v", err)
	}
	if !isIllegal {
		t.Fatal("expected recipe to be illegal")
	}
	if info == nil {
		t.Fatal("expected illegal info")
	}
	if info.BanReason != "test ban" {
		t.Errorf("expected ban reason 'test ban', got '%s'", info.BanReason)
	}
	if info.LegalLocation != "test location" {
		t.Errorf("expected location 'test location', got '%s'", info.LegalLocation)
	}
}

func TestIllegalRecipesStore_MarkIllegal_Update(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	// Apply migration 007 to create illegal_recipes table
	migration, err := GetMigration007()
	if err != nil {
		t.Fatalf("getting migration 007: %v", err)
	}
	if _, err = db.ExecContext(ctx, migration.UpSQL); err != nil {
		t.Fatalf("applying migration 007: %v", err)
	}

	store := NewIllegalRecipesStore(db)

	// First insert
	err = store.MarkIllegal(ctx, "update_recipe", "original reason", "original location")
	if err != nil {
		t.Fatalf("first insert failed: %v", err)
	}

	// Update with new info
	err = store.MarkIllegal(ctx, "update_recipe", "updated reason", "updated location")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Verify updated
	_, info, err := store.IsIllegal(ctx, "update_recipe")
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if info.BanReason != "updated reason" {
		t.Errorf("expected updated reason, got '%s'", info.BanReason)
	}
	if info.LegalLocation != "updated location" {
		t.Errorf("expected updated location, got '%s'", info.LegalLocation)
	}
}

func TestIllegalRecipesStore_MarkLegal(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	// Apply migration 007 to create illegal_recipes table
	migration, err := GetMigration007()
	if err != nil {
		t.Fatalf("getting migration 007: %v", err)
	}
	if _, err = db.ExecContext(ctx, migration.UpSQL); err != nil {
		t.Fatalf("applying migration 007: %v", err)
	}

	store := NewIllegalRecipesStore(db)

	// Mark as illegal first
	err = store.MarkIllegal(ctx, "legalize_recipe", "ban", "location")
	if err != nil {
		t.Fatalf("mark illegal failed: %v", err)
	}

	// Verify it's illegal
	isIllegal, _, err := store.IsIllegal(ctx, "legalize_recipe")
	if err != nil || !isIllegal {
		t.Fatal("recipe should be illegal before marking legal")
	}

	// Mark as legal
	err = store.MarkLegal(ctx, "legalize_recipe")
	if err != nil {
		t.Fatalf("mark legal failed: %v", err)
	}

	// Verify it's no longer illegal
	isLegal, info, err := store.IsIllegal(ctx, "legalize_recipe")
	if err != nil {
		t.Fatalf("check after legalize failed: %v", err)
	}
	if isLegal {
		t.Fatal("recipe should not be illegal after marking legal")
	}
	if info != nil {
		t.Fatal("expected nil info after marking legal")
	}
}
