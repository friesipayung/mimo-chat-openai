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
	Enabled        bool       `json:"enabled"`
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
	APIKeyName       string     `json:"api_key_name"`
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

type APIKey struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"`
	Enabled   bool       `json:"enabled"`
	LastUsed  *time.Time `json:"last_used"`
	ReqCount  int64      `json:"req_count"`
	CreatedAt time.Time  `json:"created_at"`
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
			enabled INTEGER DEFAULT 1,
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
			api_key_name TEXT DEFAULT '',
			model TEXT DEFAULT '',
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			status_code INTEGER DEFAULT 0,
			error TEXT DEFAULT '',
			duration_ms INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (cookie_id) REFERENCES cookies(id)
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			key TEXT NOT NULL UNIQUE,
			enabled INTEGER DEFAULT 1,
			last_used DATETIME,
			req_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return err
		}
	}

	// Add enabled column to existing cookies table if not exists
	db.conn.Exec("ALTER TABLE cookies ADD COLUMN enabled INTEGER DEFAULT 1")

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
	var enabled int
	err := db.conn.QueryRow(
		`SELECT id, alias, service_token, user_id, ph, full_cookie, enabled, status, last_check, last_used, total_requests, total_tokens, created_at
		 FROM cookies WHERE id = ?`, id,
	).Scan(&c.ID, &c.Alias, &c.ServiceToken, &c.UserID, &c.Ph, &c.FullCookie, &enabled, &c.Status, &c.LastCheck, &c.LastUsed, &c.TotalRequests, &c.TotalTokens, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	c.Enabled = enabled == 1
	return &c, nil
}

func (db *DB) GetRandomCookie() (*Cookie, error) {
	var c Cookie
	var enabled int
	err := db.conn.QueryRow(
		`SELECT id, alias, service_token, user_id, ph, full_cookie, enabled, status, total_requests, total_tokens, created_at
		 FROM cookies WHERE enabled = 1 AND status != 'expired' ORDER BY RANDOM() LIMIT 1`,
	).Scan(&c.ID, &c.Alias, &c.ServiceToken, &c.UserID, &c.Ph, &c.FullCookie, &enabled, &c.Status, &c.TotalRequests, &c.TotalTokens, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	c.Enabled = enabled == 1
	return &c, nil
}

func (db *DB) ListCookies() ([]Cookie, error) {
	rows, err := db.conn.Query(
		`SELECT id, alias, service_token, user_id, ph, full_cookie, enabled, status, last_check, last_used, total_requests, total_tokens, created_at
		 FROM cookies ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cookies []Cookie
	for rows.Next() {
		var c Cookie
		var enabled int
		if err := rows.Scan(&c.ID, &c.Alias, &c.ServiceToken, &c.UserID, &c.Ph, &c.FullCookie, &enabled, &c.Status, &c.LastCheck, &c.LastUsed, &c.TotalRequests, &c.TotalTokens, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Enabled = enabled == 1
		cookies = append(cookies, c)
	}
	return cookies, nil
}

func (db *DB) ToggleCookie(id int64, enabled bool) error {
	_, err := db.conn.Exec("UPDATE cookies SET enabled = ? WHERE id = ?", enabled, id)
	return err
}

func (db *DB) UpdateCookieAlias(id int64, alias string) error {
	_, err := db.conn.Exec("UPDATE cookies SET alias = ? WHERE id = ?", alias, id)
	return err
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
		`INSERT INTO request_logs (cookie_id, cookie_alias, api_key_name, model, prompt_tokens, completion_tokens, status_code, error, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.CookieID, log.CookieAlias, log.APIKeyName, log.Model, log.PromptTokens, log.CompletionTokens, log.StatusCode, log.Error, log.DurationMs,
	)
	return err
}

func (db *DB) GetLogs(limit int) ([]RequestLog, error) {
	rows, err := db.conn.Query(
		`SELECT id, cookie_id, cookie_alias, api_key_name, model, prompt_tokens, completion_tokens, status_code, error, duration_ms, created_at
		 FROM request_logs ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var l RequestLog
		if err := rows.Scan(&l.ID, &l.CookieID, &l.CookieAlias, &l.APIKeyName, &l.Model, &l.PromptTokens, &l.CompletionTokens, &l.StatusCode, &l.Error, &l.DurationMs, &l.CreatedAt); err != nil {
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

// API Key management

func (db *DB) AddAPIKey(name, key string) (int64, error) {
	result, err := db.conn.Exec(
		`INSERT INTO api_keys (name, key) VALUES (?, ?)`,
		name, key,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (db *DB) DeleteAPIKey(id int64) error {
	_, err := db.conn.Exec("DELETE FROM api_keys WHERE id = ?", id)
	return err
}

func (db *DB) GetAPIKeyByKey(key string) (*APIKey, error) {
	var k APIKey
	var enabled int
	err := db.conn.QueryRow(
		`SELECT id, name, key, enabled, last_used, req_count, created_at
		 FROM api_keys WHERE key = ? AND enabled = 1`, key,
	).Scan(&k.ID, &k.Name, &k.Key, &enabled, &k.LastUsed, &k.ReqCount, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	k.Enabled = enabled == 1
	return &k, nil
}

func (db *DB) ListAPIKeys() ([]APIKey, error) {
	rows, err := db.conn.Query(
		`SELECT id, name, key, enabled, last_used, req_count, created_at
		 FROM api_keys ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var enabled int
		if err := rows.Scan(&k.ID, &k.Name, &k.Key, &enabled, &k.LastUsed, &k.ReqCount, &k.CreatedAt); err != nil {
			return nil, err
		}
		k.Enabled = enabled == 1
		keys = append(keys, k)
	}
	return keys, nil
}

func (db *DB) ToggleAPIKey(id int64, enabled bool) error {
	_, err := db.conn.Exec("UPDATE api_keys SET enabled = ? WHERE id = ?", enabled, id)
	return err
}

func (db *DB) UpdateAPIKeyUsage(key string) error {
	now := time.Now()
	_, err := db.conn.Exec(
		"UPDATE api_keys SET req_count = req_count + 1, last_used = ? WHERE key = ?",
		now, key,
	)
	return err
}

// Password management

func (db *DB) GetPassword() (string, error) {
	val, err := db.GetConfig("admin_password")
	if err != nil {
		return "", err
	}
	if val == "" {
		return "12345678", nil
	}
	return val, nil
}

func (db *DB) SetPassword(password string) error {
	return db.SetConfig("admin_password", password)
}
