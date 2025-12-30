package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

type Star struct {
	ID        int64
	Type      string // "project", "session", "message"
	TargetID  string // Project encoded name, session ID, or message UUID
	ProjectID string // For context
	Note      string
	CreatedAt time.Time
}

type Tag struct {
	ID   int64
	Name string
}

type ItemTag struct {
	ItemType string // "project", "session", "message"
	ItemID   string
	TagID    int64
}

func Init(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}

	dbPath := filepath.Join(dataDir, "ccx.db")
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	return migrate()
}

func migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS stars (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		target_id TEXT NOT NULL,
		project_id TEXT,
		note TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(type, target_id)
	);

	CREATE TABLE IF NOT EXISTS tags (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL
	);

	CREATE TABLE IF NOT EXISTS item_tags (
		item_type TEXT NOT NULL,
		item_id TEXT NOT NULL,
		tag_id INTEGER NOT NULL,
		FOREIGN KEY (tag_id) REFERENCES tags(id),
		PRIMARY KEY (item_type, item_id, tag_id)
	);

	CREATE INDEX IF NOT EXISTS idx_stars_type_target ON stars(type, target_id);
	CREATE INDEX IF NOT EXISTS idx_stars_project ON stars(project_id);
	CREATE INDEX IF NOT EXISTS idx_item_tags_item ON item_tags(item_type, item_id);
	`

	_, err := db.Exec(schema)
	return err
}

func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

func AddStar(itemType, targetID, projectID, note string) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO stars (type, target_id, project_id, note) VALUES (?, ?, ?, ?)`,
		itemType, targetID, projectID, note,
	)
	return err
}

func RemoveStar(itemType, targetID string) error {
	_, err := db.Exec(
		`DELETE FROM stars WHERE type = ? AND target_id = ?`,
		itemType, targetID,
	)
	return err
}

func IsStarred(itemType, targetID string) bool {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM stars WHERE type = ? AND target_id = ?`,
		itemType, targetID,
	).Scan(&count)
	return err == nil && count > 0
}

func GetStars(itemType string) ([]Star, error) {
	rows, err := db.Query(
		`SELECT id, type, target_id, project_id, note, created_at FROM stars WHERE type = ? ORDER BY created_at DESC`,
		itemType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stars []Star
	for rows.Next() {
		var s Star
		var projectID sql.NullString
		var note sql.NullString
		if err := rows.Scan(&s.ID, &s.Type, &s.TargetID, &projectID, &note, &s.CreatedAt); err != nil {
			continue
		}
		s.ProjectID = projectID.String
		s.Note = note.String
		stars = append(stars, s)
	}
	return stars, nil
}

func GetAllStars() ([]Star, error) {
	rows, err := db.Query(
		`SELECT id, type, target_id, project_id, note, created_at FROM stars ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stars []Star
	for rows.Next() {
		var s Star
		var projectID sql.NullString
		var note sql.NullString
		if err := rows.Scan(&s.ID, &s.Type, &s.TargetID, &projectID, &note, &s.CreatedAt); err != nil {
			continue
		}
		s.ProjectID = projectID.String
		s.Note = note.String
		stars = append(stars, s)
	}
	return stars, nil
}

func AddTag(name string) (int64, error) {
	result, err := db.Exec(`INSERT OR IGNORE INTO tags (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil || id == 0 {
		var tagID int64
		err = db.QueryRow(`SELECT id FROM tags WHERE name = ?`, name).Scan(&tagID)
		return tagID, err
	}
	return id, nil
}

func TagItem(itemType, itemID string, tagID int64) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO item_tags (item_type, item_id, tag_id) VALUES (?, ?, ?)`,
		itemType, itemID, tagID,
	)
	return err
}

func UntagItem(itemType, itemID string, tagID int64) error {
	_, err := db.Exec(
		`DELETE FROM item_tags WHERE item_type = ? AND item_id = ? AND tag_id = ?`,
		itemType, itemID, tagID,
	)
	return err
}

func GetItemTags(itemType, itemID string) ([]Tag, error) {
	rows, err := db.Query(
		`SELECT t.id, t.name FROM tags t
		 JOIN item_tags it ON t.id = it.tag_id
		 WHERE it.item_type = ? AND it.item_id = ?`,
		itemType, itemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			continue
		}
		tags = append(tags, t)
	}
	return tags, nil
}

func GetAllTags() ([]Tag, error) {
	rows, err := db.Query(`SELECT id, name FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			continue
		}
		tags = append(tags, t)
	}
	return tags, nil
}
