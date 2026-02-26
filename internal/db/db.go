package db

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type TrackedPR struct {
	ID          int
	PRNumber    int
	Title       string
	Author      string
	Status      string
	MergeCommit string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Branches    []BranchStatus
}

type BranchStatus struct {
	Branch   string
	Landed   bool
	LandedAt *time.Time
}

type DB struct {
	db *sql.DB
}

func New(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	d := &DB{db: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) migrate() error {
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS tracked_prs (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			pr_number     INTEGER UNIQUE NOT NULL,
			title         TEXT NOT NULL DEFAULT '',
			author        TEXT NOT NULL DEFAULT '',
			status        TEXT NOT NULL DEFAULT 'open',
			merge_commit  TEXT NOT NULL DEFAULT '',
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS branch_status (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			pr_number   INTEGER NOT NULL,
			branch      TEXT NOT NULL,
			landed      BOOLEAN NOT NULL DEFAULT 0,
			landed_at   DATETIME,
			UNIQUE(pr_number, branch),
			FOREIGN KEY (pr_number) REFERENCES tracked_prs(pr_number)
		);
	`)
	return err
}

func (d *DB) AddPR(prNumber int) error {
	_, err := d.db.Exec(
		`INSERT OR IGNORE INTO tracked_prs (pr_number) VALUES (?)`,
		prNumber,
	)
	return err
}

func (d *DB) RemovePR(prNumber int) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM branch_status WHERE pr_number = ?`, prNumber); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tracked_prs WHERE pr_number = ?`, prNumber); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) ListPRs() ([]TrackedPR, error) {
	rows, err := d.db.Query(`SELECT id, pr_number, title, author, status, merge_commit, created_at, updated_at FROM tracked_prs ORDER BY pr_number DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prs []TrackedPR
	for rows.Next() {
		var pr TrackedPR
		if err := rows.Scan(&pr.ID, &pr.PRNumber, &pr.Title, &pr.Author, &pr.Status, &pr.MergeCommit, &pr.CreatedAt, &pr.UpdatedAt); err != nil {
			return nil, err
		}
		branches, err := d.GetBranchStatus(pr.PRNumber)
		if err != nil {
			return nil, err
		}
		pr.Branches = branches
		prs = append(prs, pr)
	}
	return prs, rows.Err()
}

func (d *DB) GetPR(prNumber int) (*TrackedPR, error) {
	var pr TrackedPR
	err := d.db.QueryRow(
		`SELECT id, pr_number, title, author, status, merge_commit, created_at, updated_at FROM tracked_prs WHERE pr_number = ?`,
		prNumber,
	).Scan(&pr.ID, &pr.PRNumber, &pr.Title, &pr.Author, &pr.Status, &pr.MergeCommit, &pr.CreatedAt, &pr.UpdatedAt)
	if err != nil {
		return nil, err
	}
	branches, err := d.GetBranchStatus(pr.PRNumber)
	if err != nil {
		return nil, err
	}
	pr.Branches = branches
	return &pr, nil
}

func (d *DB) UpdatePRStatus(prNumber int, status string, mergeCommit string, title string, author string) error {
	_, err := d.db.Exec(
		`UPDATE tracked_prs SET status = ?, merge_commit = ?, title = ?, author = ?, updated_at = CURRENT_TIMESTAMP WHERE pr_number = ?`,
		status, mergeCommit, title, author, prNumber,
	)
	return err
}

func (d *DB) UpdateBranchLanded(prNumber int, branch string) error {
	_, err := d.db.Exec(
		`INSERT INTO branch_status (pr_number, branch, landed, landed_at) VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		 ON CONFLICT(pr_number, branch) DO UPDATE SET landed = 1, landed_at = CURRENT_TIMESTAMP`,
		prNumber, branch,
	)
	return err
}

func (d *DB) GetBranchStatus(prNumber int) ([]BranchStatus, error) {
	rows, err := d.db.Query(`SELECT branch, landed, landed_at FROM branch_status WHERE pr_number = ?`, prNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var statuses []BranchStatus
	for rows.Next() {
		var bs BranchStatus
		if err := rows.Scan(&bs.Branch, &bs.Landed, &bs.LandedAt); err != nil {
			return nil, err
		}
		statuses = append(statuses, bs)
	}
	return statuses, rows.Err()
}
