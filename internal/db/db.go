package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

type Cookie struct {
	ID             int64      `json:"id"`
	Alias          string     `json:"alias"`
	ServiceToken   string     `json:"service_token"`
	UserID         string     `json:"user_id"`
	Ph             string     `json:"ph"`
	FullCookie     string     `json:"full_cookie"`
	Status         string     `json:"status"`
	LastCheck      *time.Time `json:"last_check"`
	LastUsed       *time.Time `json:"last_used"`
	TotalRequests  int64      `json:"total_requests"`
	TotalTokens    int64      `json:"total_tokens"`
	CreatedAt      time.Time  `json:"created_at"`
}

type RequestLog struct {
	ID               int64      `json:"id"`
	CookieID         *int64     `json:"cookie_id"`
	CookieAlias      string     `json:"cookie_alias"`
	Model            string     `json:"model"`
	PromptTokens     int        `json:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens"`
	StatusCode       int        `json:"status_code"`
	Error            string     `json:"error"`
	DurationMs       int        `json:"duration_ms"`
	CreatedAt        time.Time  `json:"created_at"`
}

type Stats struct {
	TotalCookies  int     `json:"total_cookies"`
	ActiveCookies int     `json:"active_cookies"`
	ExpiredCookies int    `json:"expired_cookies"`
	TotalRequests int64   `json:"total_requests"`
	TodayRequests int64   `json:"today_requests"`
	TotalTokens   int64   `json:"total_tokens"`
	TodayTokens   int64   `json:"today_tokens"`
	SuccessRate   float64 `json:"success_rate"`
}

func Open(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS cookies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			alias TEXT NOT NULL UNIQUE,
			service_token TEXT NOT NULL,
			user_id TEXT NOT NULL,
			ph TEXT NOT NULL,
			full_cookie TEXT NOT NULL,
			status TEXT DEFAULT 'unknown',
			last_check DATETIME,
			last_used DATETIME,
			total_requests INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS request_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cookie_id INTEGER,
			cookie_alias TEXT DEFAULT '',
			model TEXT DEFAULT '',
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			status_code INTEGER DEFAULT 0,
			error TEXT DEFAULT '',
			duration_ms INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (cookie_id) REFERENCES cookies(id)
		)`,
	}

	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) AddCookie(alias, serviceToken, userId, ph, fullCookie string) (int64, error) {
	result, err := db.conn.Exec(
		`INSERT INTO cookies (alias, service_token, user_id, ph, full_cookie) VALUES (?, ?, ?, ?, ?)`,
		alias, serviceToken, userId, ph, fullCookie,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (db *DB) DeleteCookie(id int64) error {
	_, err := db.conn.Exec("DELETE FROM cookies WHERE id = ?", id)
	return err
}

func (db *DB) GetCookie(id int64) (*Cookie, error) {
	var c Cookie
	err := db.conn.QueryRow(
		`SELECT id, alias, service_token, user_id, ph, full_cookie, status, last_check, last_used, total_requests, total_tokens, created_at
		 FROM cookies WHERE id = ?`, id,
	).Scan(&c.ID, &c.Alias, &c.ServiceToken, &c.UserID, &c.Ph, &c.FullCookie, &c.Status, &c.LastCheck, &c.LastUsed, &c.TotalRequests, &c.TotalTokens, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (db *DB) GetRandomCookie() (*Cookie, error) {
	var c Cookie
	err := db.conn.QueryRow(
		`SELECT id, alias, service_token, user_id, ph, full_cookie, status, total_requests, total_tokens, created_at
		 FROM cookies WHERE status != 'expired' ORDER BY RANDOM() LIMIT 1`,
	).Scan(&c.ID, &c.Alias, &c.ServiceToken, &c.UserID, &c.Ph, &c.FullCookie, &c.Status, &c.TotalRequests, &c.TotalTokens, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (db *DB) ListCookies() ([]Cookie, error) {
	rows, err := db.conn.Query(
		`SELECT id, alias, service_token, user_id, ph, full_cookie, status, last_check, last_used, total_requests, total_tokens, created_at
		 FROM cookies ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cookies []Cookie
	for rows.Next() {
		var c Cookie
		if err := rows.Scan(&c.ID, &c.Alias, &c.ServiceToken, &c.UserID, &c.Ph, &c.FullCookie, &c.Status, &c.LastCheck, &c.LastUsed, &c.TotalRequests, &c.TotalTokens, &c.CreatedAt); err != nil {
			return nil, err
		}
		cookies = append(cookies, c)
	}
	return cookies, nil
}

func (db *DB) UpdateCookieStatus(id int64, status string) error {
	now := time.Now()
	_, err := db.conn.Exec("UPDATE cookies SET status = ?, last_check = ? WHERE id = ?", status, now, id)
	return err
}

func (db *DB) UpdateCookieUsage(id int64, tokens int) error {
	now := time.Now()
	_, err := db.conn.Exec(
		"UPDATE cookies SET total_requests = total_requests + 1, total_tokens = total_tokens + ?, last_used = ? WHERE id = ?",
		tokens, now, id,
	)
	return err
}

func (db *DB) AddLog(log *RequestLog) error {
	_, err := db.conn.Exec(
		`INSERT INTO request_logs (cookie_id, cookie_alias, model, prompt_tokens, completion_tokens, status_code, error, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		log.CookieID, log.CookieAlias, log.Model, log.PromptTokens, log.CompletionTokens, log.StatusCode, log.Error, log.DurationMs,
	)
	return err
}

func (db *DB) GetLogs(limit int) ([]RequestLog, error) {
	rows, err := db.conn.Query(
		`SELECT id, cookie_id, cookie_alias, model, prompt_tokens, completion_tokens, status_code, error, duration_ms, created_at
		 FROM request_logs ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var l RequestLog
		if err := rows.Scan(&l.ID, &l.CookieID, &l.CookieAlias, &l.Model, &l.PromptTokens, &l.CompletionTokens, &l.StatusCode, &l.Error, &l.DurationMs, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

func (db *DB) GetStats() (*Stats, error) {
	s := &Stats{}

	db.conn.QueryRow("SELECT COUNT(*) FROM cookies").Scan(&s.TotalCookies)
	db.conn.QueryRow("SELECT COUNT(*) FROM cookies WHERE status = 'valid'").Scan(&s.ActiveCookies)
	db.conn.QueryRow("SELECT COUNT(*) FROM cookies WHERE status = 'expired'").Scan(&s.ExpiredCookies)
	db.conn.QueryRow("SELECT COALESCE(SUM(total_requests), 0) FROM cookies").Scan(&s.TotalRequests)
	db.conn.QueryRow("SELECT COALESCE(SUM(total_tokens), 0) FROM cookies").Scan(&s.TotalTokens)

	today := time.Now().Format("2006-01-02")
	db.conn.QueryRow("SELECT COUNT(*) FROM request_logs WHERE created_at >= ?", today).Scan(&s.TodayRequests)
	db.conn.QueryRow("SELECT COALESCE(SUM(prompt_tokens + completion_tokens), 0) FROM request_logs WHERE created_at >= ?", today).Scan(&s.TodayTokens)

	var totalLogs, successLogs int64
	db.conn.QueryRow("SELECT COUNT(*) FROM request_logs").Scan(&totalLogs)
	db.conn.QueryRow("SELECT COUNT(*) FROM request_logs WHERE status_code = 200").Scan(&successLogs)
	if totalLogs > 0 {
		s.SuccessRate = float64(successLogs) / float64(totalLogs) * 100
	}

	return s, nil
}

func (db *DB) GetConfig(key string) (string, error) {
	var value string
	err := db.conn.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (db *DB) SetConfig(key, value string) error {
	_, err := db.conn.Exec(
		`INSERT INTO config (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = CURRENT_TIMESTAMP`,
		key, value, value,
	)
	return err
}

func (db *DB) GetAllConfig() (map[string]string, error) {
	rows, err := db.conn.Query("SELECT key, value FROM config")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	config := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		config[k] = v
	}
	return config, nil
}
