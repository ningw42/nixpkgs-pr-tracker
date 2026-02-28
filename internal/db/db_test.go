package db

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	// Use a unique file::memory: with shared cache so all connections from
	// the sql.DB pool see the same in-memory database.
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	d, err := New(dsn)
	if err != nil {
		t.Fatalf("opening in-memory DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestMigration(t *testing.T) {
	d := newTestDB(t)

	// Verify tables exist by running queries against them
	_, err := d.db.Exec("SELECT count(*) FROM tracked_prs")
	if err != nil {
		t.Fatalf("tracked_prs table not created: %v", err)
	}
	_, err = d.db.Exec("SELECT count(*) FROM branch_status")
	if err != nil {
		t.Fatalf("branch_status table not created: %v", err)
	}
}

func TestAddPR(t *testing.T) {
	d := newTestDB(t)

	if err := d.AddPR(123); err != nil {
		t.Fatalf("AddPR: %v", err)
	}

	pr, err := d.GetPR(123)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.PRNumber != 123 {
		t.Errorf("PRNumber = %d, want 123", pr.PRNumber)
	}
	if pr.Status != "open" {
		t.Errorf("Status = %q, want %q", pr.Status, "open")
	}
}

func TestAddPRDuplicate(t *testing.T) {
	d := newTestDB(t)

	if err := d.AddPR(100); err != nil {
		t.Fatalf("first AddPR: %v", err)
	}
	// INSERT OR IGNORE should not error
	if err := d.AddPR(100); err != nil {
		t.Fatalf("duplicate AddPR: %v", err)
	}

	prs, err := d.ListPRs()
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if len(prs) != 1 {
		t.Errorf("len(prs) = %d, want 1", len(prs))
	}
}

func TestGetPRNotFound(t *testing.T) {
	d := newTestDB(t)

	_, err := d.GetPR(999)
	if err == nil {
		t.Fatal("expected error for non-existent PR")
	}
}

func TestRemovePR(t *testing.T) {
	d := newTestDB(t)

	d.AddPR(1)
	if err := d.RemovePR(1); err != nil {
		t.Fatalf("RemovePR: %v", err)
	}

	_, err := d.GetPR(1)
	if err == nil {
		t.Fatal("expected error after removal")
	}
}

func TestRemovePRWithBranchStatus(t *testing.T) {
	d := newTestDB(t)

	d.AddPR(1)
	d.UpdateBranchLanded(1, "nixos-unstable")

	if err := d.RemovePR(1); err != nil {
		t.Fatalf("RemovePR with branch status: %v", err)
	}

	// Branch status should also be removed
	statuses, err := d.GetBranchStatus(1)
	if err != nil {
		t.Fatalf("GetBranchStatus: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("remaining branch statuses = %d, want 0", len(statuses))
	}
}

func TestListPRsOrdering(t *testing.T) {
	d := newTestDB(t)

	d.AddPR(10)
	d.AddPR(30)
	d.AddPR(20)

	prs, err := d.ListPRs()
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if len(prs) != 3 {
		t.Fatalf("len(prs) = %d, want 3", len(prs))
	}
	// ORDER BY pr_number DESC
	if prs[0].PRNumber != 30 || prs[1].PRNumber != 20 || prs[2].PRNumber != 10 {
		t.Errorf("ordering: got %d, %d, %d; want 30, 20, 10", prs[0].PRNumber, prs[1].PRNumber, prs[2].PRNumber)
	}
}

func TestUpdatePRStatus(t *testing.T) {
	d := newTestDB(t)

	d.AddPR(5)
	if err := d.UpdatePRStatus(5, "merged", "abc123", "My PR", "author1"); err != nil {
		t.Fatalf("UpdatePRStatus: %v", err)
	}

	pr, err := d.GetPR(5)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.Status != "merged" {
		t.Errorf("Status = %q, want %q", pr.Status, "merged")
	}
	if pr.MergeCommit != "abc123" {
		t.Errorf("MergeCommit = %q, want %q", pr.MergeCommit, "abc123")
	}
	if pr.Title != "My PR" {
		t.Errorf("Title = %q, want %q", pr.Title, "My PR")
	}
	if pr.Author != "author1" {
		t.Errorf("Author = %q, want %q", pr.Author, "author1")
	}
}

func TestUpdateBranchLanded(t *testing.T) {
	d := newTestDB(t)

	d.AddPR(7)
	if err := d.UpdateBranchLanded(7, "nixos-unstable"); err != nil {
		t.Fatalf("UpdateBranchLanded: %v", err)
	}

	statuses, err := d.GetBranchStatus(7)
	if err != nil {
		t.Fatalf("GetBranchStatus: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("len(statuses) = %d, want 1", len(statuses))
	}
	if !statuses[0].Landed {
		t.Error("expected Landed = true")
	}
	if statuses[0].Branch != "nixos-unstable" {
		t.Errorf("Branch = %q, want %q", statuses[0].Branch, "nixos-unstable")
	}
}

func TestUpdateBranchLandedIdempotent(t *testing.T) {
	d := newTestDB(t)

	d.AddPR(8)
	d.UpdateBranchLanded(8, "nixos-unstable")

	// Should not error on duplicate
	if err := d.UpdateBranchLanded(8, "nixos-unstable"); err != nil {
		t.Fatalf("idempotent UpdateBranchLanded: %v", err)
	}

	statuses, err := d.GetBranchStatus(8)
	if err != nil {
		t.Fatalf("GetBranchStatus: %v", err)
	}
	if len(statuses) != 1 {
		t.Errorf("len(statuses) = %d, want 1 (idempotent)", len(statuses))
	}
}

func TestMultipleBranches(t *testing.T) {
	d := newTestDB(t)

	d.AddPR(9)
	d.UpdateBranchLanded(9, "nixos-unstable")
	d.UpdateBranchLanded(9, "nixos-24.11")

	statuses, err := d.GetBranchStatus(9)
	if err != nil {
		t.Fatalf("GetBranchStatus: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("len(statuses) = %d, want 2", len(statuses))
	}
}

func TestListPRsIncludesBranches(t *testing.T) {
	d := newTestDB(t)

	d.AddPR(11)
	d.UpdateBranchLanded(11, "nixos-unstable")

	prs, err := d.ListPRs()
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("len(prs) = %d, want 1", len(prs))
	}
	if len(prs[0].Branches) != 1 {
		t.Fatalf("len(Branches) = %d, want 1", len(prs[0].Branches))
	}
	if !prs[0].Branches[0].Landed {
		t.Error("expected branch to be landed")
	}
}

func TestUpdateLastChecked(t *testing.T) {
	d := newTestDB(t)

	d.AddPR(42)

	// Before UpdateLastChecked, LastCheckedAt should be zero.
	pr, err := d.GetPR(42)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if !pr.LastCheckedAt.IsZero() {
		t.Errorf("LastCheckedAt before update = %v, want zero", pr.LastCheckedAt)
	}

	if err := d.UpdateLastChecked(42); err != nil {
		t.Fatalf("UpdateLastChecked: %v", err)
	}

	pr, err = d.GetPR(42)
	if err != nil {
		t.Fatalf("GetPR after update: %v", err)
	}
	if pr.LastCheckedAt.IsZero() {
		t.Error("LastCheckedAt after update should not be zero")
	}
}

func TestMigrationFromV1(t *testing.T) {
	// Simulate a v1 database (no last_checked_at column) and verify
	// that opening it with New() applies the v2 migration.
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("opening raw DB: %v", err)
	}
	// Keep sqlDB open so the shared in-memory database survives.
	defer sqlDB.Close()

	// Create v1 schema manually.
	if _, err := sqlDB.Exec(`
		CREATE TABLE tracked_prs (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			pr_number     INTEGER UNIQUE NOT NULL,
			title         TEXT NOT NULL DEFAULT '',
			author        TEXT NOT NULL DEFAULT '',
			status        TEXT NOT NULL DEFAULT 'open',
			merge_commit  TEXT NOT NULL DEFAULT '',
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE branch_status (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			pr_number   INTEGER NOT NULL,
			branch      TEXT NOT NULL,
			landed      BOOLEAN NOT NULL DEFAULT 0,
			landed_at   DATETIME,
			UNIQUE(pr_number, branch),
			FOREIGN KEY (pr_number) REFERENCES tracked_prs(pr_number)
		);
		PRAGMA user_version = 1;
	`); err != nil {
		t.Fatalf("creating v1 schema: %v", err)
	}

	// Insert a row before migration.
	if _, err := sqlDB.Exec(`INSERT INTO tracked_prs (pr_number) VALUES (99)`); err != nil {
		t.Fatalf("inserting v1 row: %v", err)
	}

	// Open via New() which should apply v2 migration.
	d, err := New(dsn)
	if err != nil {
		t.Fatalf("New on v1 DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	pr, err := d.GetPR(99)
	if err != nil {
		t.Fatalf("GetPR after migration: %v", err)
	}
	if !pr.LastCheckedAt.IsZero() {
		t.Errorf("LastCheckedAt for pre-existing row = %v, want zero", pr.LastCheckedAt)
	}

	// Verify user_version is now 2.
	var version int
	if err := d.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	if version != 2 {
		t.Errorf("user_version = %d, want 2", version)
	}
}
