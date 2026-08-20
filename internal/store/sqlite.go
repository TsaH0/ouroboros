package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/oklog/ulid/v2"

	"github.com/TsaH0/ouroboros/internal/model"
	"github.com/TsaH0/ouroboros/internal/recon"
	"github.com/TsaH0/ouroboros/internal/scope"
)

// SQLiteStore implements Store backed by a SQLite database with WAL mode.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens or creates a SQLite database at path, runs migrations,
// and enables WAL mode.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- Migrations ---

type migration struct {
	Version int
	Up      string
}

var migrations = []migration{
	{Version: 1, Up: schemaV1},
	{Version: 2, Up: schemaV2},
}

const schemaV1 = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS flows (
    id TEXT PRIMARY KEY,
    start_time TEXT NOT NULL,
    duration_ns INTEGER NOT NULL DEFAULT 0,
    client_addr TEXT NOT NULL DEFAULT '',
    server_addr TEXT NOT NULL DEFAULT '',
    scheme TEXT NOT NULL DEFAULT '',
    host TEXT NOT NULL DEFAULT '',
    port INTEGER NOT NULL DEFAULT 0,
    state TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    scope_status TEXT NOT NULL DEFAULT 'unknown',
    tags TEXT NOT NULL DEFAULT '[]',
    notes TEXT NOT NULL DEFAULT '',
    req_method TEXT,
    req_url TEXT,
    req_version TEXT,
    req_headers TEXT,
    req_body BLOB,
    resp_status INTEGER,
    resp_version TEXT,
    resp_headers TEXT,
    resp_body BLOB
);
CREATE INDEX IF NOT EXISTS idx_flows_start ON flows(start_time);
CREATE INDEX IF NOT EXISTS idx_flows_host ON flows(host);

CREATE TABLE IF NOT EXISTS analyses (
    id TEXT PRIMARY KEY,
    flow_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    raw_json TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_analyses_flow ON analyses(flow_id);

CREATE TABLE IF NOT EXISTS recon_runs (
    id TEXT PRIMARY KEY,
    target TEXT NOT NULL,
    created_at TEXT NOT NULL,
    summary_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_recon_target ON recon_runs(target);

CREATE TABLE IF NOT EXISTS technologies (
    run_id TEXT NOT NULL,
    name TEXT NOT NULL,
    version TEXT NOT NULL DEFAULT '',
    confidence TEXT NOT NULL DEFAULT '',
    host TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS vulnerabilities (
    run_id TEXT NOT NULL,
    cve TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL DEFAULT '',
    exploit_ref TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS scope_rules (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    pattern TEXT NOT NULL,
    match_mode TEXT NOT NULL,
    action TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    priority INTEGER NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const schemaV2 = `
ALTER TABLE scope_rules ADD COLUMN preset_id TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS scope_presets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_scope_presets_name ON scope_presets(name);
CREATE INDEX IF NOT EXISTS idx_scope_rules_preset ON scope_rules(preset_id);
`

func (s *SQLiteStore) migrate() error {
	// Create schema_migrations if it doesn't exist.
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Get current version.
	var current int
	err = s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current)
	if err != nil {
		return fmt.Errorf("read current version: %w", err)
	}

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}
		if _, err := tx.Exec(m.Up); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", m.Version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, m.Version, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}
	}
	return nil
}

// --- Flows ---

func (s *SQLiteStore) SaveFlow(_ context.Context, f *model.Flow) error {
	reqJSON, reqBody := marshalMessage(f.Request)
	respJSON, respBody := marshalMessage(f.Response)
	tagsJSON, _ := json.Marshal(f.Tags)
	if len(tagsJSON) == 0 {
		tagsJSON = []byte("[]")
	}

	_, err := s.db.Exec(`INSERT OR REPLACE INTO flows (
		id, start_time, duration_ns, client_addr, server_addr,
		scheme, host, port, state, error, scope_status,
		tags, notes,
		req_method, req_url, req_version, req_headers, req_body,
		resp_status, resp_version, resp_headers, resp_body
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.ID,
		f.StartTime.UTC().Format(time.RFC3339Nano),
		f.Duration.Nanoseconds(),
		f.ClientAddr,
		f.ServerAddr,
		f.Scheme,
		f.Host,
		f.Port,
		string(f.State),
		f.Error,
		string(f.ScopeStatus),
		string(tagsJSON),
		f.Notes,
		reqMethod(f.Request), reqURL(f.Request), reqVersion(f.Request), reqJSON, reqBody,
		respStatus(f.Response), respVersion(f.Response), respJSON, respBody,
	)
	return err
}

func (s *SQLiteStore) GetFlow(_ context.Context, id string) (*model.Flow, error) {
	row := s.db.QueryRow(`SELECT
		id, start_time, duration_ns, client_addr, server_addr,
		scheme, host, port, state, error, scope_status,
		tags, notes,
		req_method, req_url, req_version, req_headers, req_body,
		resp_status, resp_version, resp_headers, resp_body
	FROM flows WHERE id = ?`, id)

	f, err := scanFlow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *SQLiteStore) DeleteFlow(_ context.Context, id string) error {
	_, err := s.db.Exec(`DELETE FROM flows WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) ClearFlows(_ context.Context) error {
	_, err := s.db.Exec(`DELETE FROM flows`)
	if err != nil {
		return err
	}
	// Also clean dependent analyses to avoid orphans.
	_, _ = s.db.Exec(`DELETE FROM analyses`)
	return nil
}

func (s *SQLiteStore) ListFlows(_ context.Context) ([]*model.Flow, error) {
	rows, err := s.db.Query(`SELECT
		id, start_time, duration_ns, client_addr, server_addr,
		scheme, host, port, state, error, scope_status,
		tags, notes,
		req_method, req_url, req_version, req_headers, req_body,
		resp_status, resp_version, resp_headers, resp_body
	FROM flows ORDER BY start_time`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flows []*model.Flow
	for rows.Next() {
		f, err := scanFlow(rows)
		if err != nil {
			return nil, err
		}
		flows = append(flows, f)
	}
	return flows, rows.Err()
}

// --- Scope rules ---

func (s *SQLiteStore) LoadScopeRules(_ context.Context) ([]scope.Rule, error) {
	rows, err := s.db.Query(`SELECT id, preset_id, kind, pattern, match_mode, action, enabled, priority, note, created_at, updated_at FROM scope_rules ORDER BY priority DESC, created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []scope.Rule
	for rows.Next() {
		var r scope.Rule
		var enabled int
		var createdStr, updatedStr string
		if err := rows.Scan(&r.ID, &r.PresetID, &r.Kind, &r.Pattern, &r.MatchMode, &r.Action, &enabled, &r.Priority, &r.Note, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *SQLiteStore) SaveScopeRule(_ context.Context, rule *scope.Rule) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO scope_rules (id, preset_id, kind, pattern, match_mode, action, enabled, priority, note, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		rule.ID, rule.PresetID, rule.Kind, rule.Pattern, rule.MatchMode, rule.Action, boolToInt(rule.Enabled), rule.Priority, rule.Note,
		rule.CreatedAt.UTC().Format(time.RFC3339Nano),
		rule.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) DeleteScopeRule(_ context.Context, id string) error {
	_, err := s.db.Exec(`DELETE FROM scope_rules WHERE id = ?`, id)
	return err
}

// --- Scope presets ---

func (s *SQLiteStore) ListScopePresets(_ context.Context) ([]scope.Preset, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at, updated_at FROM scope_presets ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var presets []scope.Preset
	for rows.Next() {
		var p scope.Preset
		var createdStr, updatedStr string
		if err := rows.Scan(&p.ID, &p.Name, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		presets = append(presets, p)
	}
	return presets, rows.Err()
}

func (s *SQLiteStore) SaveScopePreset(_ context.Context, p *scope.Preset) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO scope_presets (id, name, created_at, updated_at) VALUES (?,?,?,?)`,
		p.ID, p.Name,
		p.CreatedAt.UTC().Format(time.RFC3339Nano),
		p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) DeleteScopePreset(_ context.Context, id string) error {
	_, err := s.db.Exec(`DELETE FROM scope_presets WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) LoadScopeRulesForPreset(_ context.Context, presetID string) ([]scope.Rule, error) {
	rows, err := s.db.Query(
		`SELECT id, preset_id, kind, pattern, match_mode, action, enabled, priority, note, created_at, updated_at
		 FROM scope_rules WHERE preset_id = ? ORDER BY priority DESC, created_at`,
		presetID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []scope.Rule
	for rows.Next() {
		var r scope.Rule
		var enabled int
		var createdStr, updatedStr string
		if err := rows.Scan(&r.ID, &r.PresetID, &r.Kind, &r.Pattern, &r.MatchMode, &r.Action, &enabled, &r.Priority, &r.Note, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// --- Recon ---

func (s *SQLiteStore) SaveRecon(_ context.Context, summary *recon.ReconSummary) error {
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal recon summary: %w", err)
	}
	id := ulid.MustNewDefault(time.Now()).String()
	_, err = s.db.Exec(`INSERT INTO recon_runs (id, target, created_at, summary_json) VALUES (?,?,?,?)`,
		id, summary.Target, summary.CreatedAt.UTC().Format(time.RFC3339Nano), string(summaryJSON))
	return err
}

func (s *SQLiteStore) ListRecon(_ context.Context) ([]*recon.ReconSummary, error) {
	rows, err := s.db.Query(`SELECT summary_json FROM recon_runs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*recon.ReconSummary
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var summary recon.ReconSummary
		if err := json.Unmarshal([]byte(raw), &summary); err != nil {
			return nil, err
		}
		results = append(results, &summary)
	}
	return results, rows.Err()
}

func (s *SQLiteStore) GetRecon(_ context.Context, target string) (*recon.ReconSummary, error) {
	row := s.db.QueryRow(`SELECT summary_json FROM recon_runs WHERE target = ? ORDER BY created_at DESC LIMIT 1`, target)
	var raw string
	if err := row.Scan(&raw); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var summary recon.ReconSummary
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

// --- Technologies ---

func (s *SQLiteStore) SaveTechnology(_ context.Context, runID string, t recon.Technology) error {
	_, err := s.db.Exec(`INSERT INTO technologies (run_id, name, version, confidence, host, source) VALUES (?,?,?,?,?,?)`,
		runID, t.Name, t.Version, t.Confidence, t.Host, t.Source)
	return err
}

// --- Vulnerabilities ---

func (s *SQLiteStore) SaveVulnerability(_ context.Context, runID string, v recon.Vulnerability) error {
	_, err := s.db.Exec(`INSERT INTO vulnerabilities (run_id, cve, title, severity, exploit_ref, source) VALUES (?,?,?,?,?,?)`,
		runID, v.CVE, v.Title, v.Severity, v.ExploitRef, v.Source)
	return err
}

// --- Analyses ---

func (s *SQLiteStore) SaveAnalysis(_ context.Context, a *model.Analysis) error {
	_, err := s.db.Exec(`INSERT INTO analyses (id, flow_id, kind, provider, model, summary, raw_json, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		a.ID, a.FlowID, a.Kind, a.Provider, a.Model, a.Summary, a.RawJSON, a.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLiteStore) ListAnalyses(_ context.Context, flowID string) ([]*model.Analysis, error) {
	rows, err := s.db.Query(`SELECT id, flow_id, kind, provider, model, summary, raw_json, created_at FROM analyses WHERE flow_id = ? ORDER BY created_at`, flowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*model.Analysis
	for rows.Next() {
		var a model.Analysis
		var createdStr string
		if err := rows.Scan(&a.ID, &a.FlowID, &a.Kind, &a.Provider, &a.Model, &a.Summary, &a.RawJSON, &createdStr); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		results = append(results, &a)
	}
	return results, rows.Err()
}

// --- Settings ---

func (s *SQLiteStore) SaveSetting(_ context.Context, key, value string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES (?,?)`, key, value)
	return err
}

func (s *SQLiteStore) GetSetting(_ context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// --- Helpers ---

type scannable interface {
	Scan(dest ...any) error
}

func scanFlow(row scannable) (*model.Flow, error) {
	var (
		f            model.Flow
		startStr     string
		durNS        int64
		stateStr     string
		scopeStr     string
		tagsJSON     string
		reqMethod    sql.NullString
		reqURL       sql.NullString
		reqVersion   sql.NullString
		reqHeaders   sql.NullString
		reqBody      []byte
		respStatus   sql.NullInt64
		respVersion  sql.NullString
		respHeaders  sql.NullString
		respBody     []byte
	)
	if err := row.Scan(
		&f.ID, &startStr, &durNS, &f.ClientAddr, &f.ServerAddr,
		&f.Scheme, &f.Host, &f.Port, &stateStr, &f.Error, &scopeStr,
		&tagsJSON, &f.Notes,
		&reqMethod, &reqURL, &reqVersion, &reqHeaders, &reqBody,
		&respStatus, &respVersion, &respHeaders, &respBody,
	); err != nil {
		return nil, err
	}
	f.StartTime, _ = time.Parse(time.RFC3339Nano, startStr)
	f.Duration = time.Duration(durNS)
	f.State = model.FlowState(stateStr)
	f.ScopeStatus = model.ScopeStatus(scopeStr)
	json.Unmarshal([]byte(tagsJSON), &f.Tags)

	if reqMethod.Valid {
		f.Request = &model.Message{
			Method:      reqMethod.String,
			URL:         reqURL.String,
			HTTPVersion: reqVersion.String,
		}
		if reqHeaders.Valid {
			json.Unmarshal([]byte(reqHeaders.String), &f.Request.Headers)
		}
		f.Request.Body = reqBody
	}
	if respStatus.Valid {
		f.Response = &model.Message{
			StatusCode:  int(respStatus.Int64),
			HTTPVersion: respVersion.String,
		}
		if respHeaders.Valid {
			json.Unmarshal([]byte(respHeaders.String), &f.Response.Headers)
		}
		f.Response.Body = respBody
	}
	return &f, nil
}

func marshalMessage(m *model.Message) (headersJSON string, body []byte) {
	if m == nil {
		return "", nil
	}
	h, _ := json.Marshal(m.Headers)
	return string(h), m.Body
}

func reqMethod(m *model.Message) string {
	if m == nil {
		return ""
	}
	return m.Method
}

func reqURL(m *model.Message) string {
	if m == nil {
		return ""
	}
	return m.URL
}

func reqVersion(m *model.Message) string {
	if m == nil {
		return ""
	}
	return m.HTTPVersion
}

func respStatus(m *model.Message) int {
	if m == nil {
		return 0
	}
	return m.StatusCode
}

func respVersion(m *model.Message) string {
	if m == nil {
		return ""
	}
	return m.HTTPVersion
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
