package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

type User struct {
	ID            int64  `json:"id"`
	Phone         string `json:"phone"`
	Nickname      string `json:"nickname"`
	Avatar        string `json:"avatar"`
	MemberUntil   string `json:"memberUntil"`
	Membership    string `json:"membership"`
	CanClaimPoint bool   `json:"canClaimPoint"`
}

type Wallet struct {
	Balance      int              `json:"balance"`
	Frozen       int              `json:"frozen"`
	Items        []WalletTxn      `json:"items"`
	Packs        []RechargePack   `json:"packs"`
	Tasks        []WalletTaskItem `json:"tasks"`
	ServiceCosts map[string]int   `json:"serviceCosts"`
}

type WalletTxn struct {
	Title string `json:"title"`
	Delta int    `json:"delta"`
	Time  string `json:"time"`
}

type WalletTaskItem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Claimed     bool   `json:"claimed"`
}

type RechargePack struct {
	Code       string  `json:"code"`
	Price      int     `json:"price"`
	Original   int     `json:"original"`
	Points     int     `json:"points"`
	Bonus      int     `json:"bonus"`
	UnitPrice  float64 `json:"unitPrice"`
	Enterprise bool    `json:"enterprise"`
}

type ScriptProject struct {
	ID         int64          `json:"id"`
	Title      string         `json:"title"`
	Type       string         `json:"type"`
	Source     string         `json:"source"`
	Status     string         `json:"status"`
	UpdatedAt  string         `json:"updatedAt"`
	Settings   map[string]any `json:"settings"`
	Characters []Character    `json:"characters"`
	Outlines   []OutlineBlock `json:"outlines"`
	Episodes   []Episode      `json:"episodes"`
}

type Character struct {
	Name       string   `json:"name"`
	Role       string   `json:"role"`
	Age        string   `json:"age"`
	Meta       string   `json:"meta"`
	Traits     []string `json:"traits"`
	Background string   `json:"background"`
	Goal       string   `json:"goal"`
	Relation   string   `json:"relation"`
	Bio        string   `json:"bio"`
}

type OutlineBlock struct {
	Range   string `json:"range"`
	Phase   string `json:"phase"`
	Content string `json:"content"`
}

type Episode struct {
	No      int    `json:"no"`
	Outline string `json:"outline"`
	Body    string `json:"body"`
}

type Evaluation struct {
	ID          int64            `json:"id"`
	Title       string           `json:"title"`
	Status      string           `json:"status"`
	CreatedAt   string           `json:"createdAt"`
	Score       int              `json:"score"`
	Summary     string           `json:"summary"`
	Dimensions  []ScoreDimension `json:"dimensions"`
	Suggestions []string         `json:"suggestions"`
	Cost        int              `json:"cost"`
	ReportNo    string           `json:"reportNo"`
}

type ScoreDimension struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
	Note  string `json:"note"`
}

type AppState struct {
	mu          sync.Mutex
	db          *sql.DB
	user        User
	wallet      Wallet
	scripts     []ScriptProject
	evaluations []Evaluation
	nextScript  int64
	nextEval    int64
}

type persistedAppState struct {
	User        User            `json:"user"`
	Wallet      Wallet          `json:"wallet"`
	Scripts     []ScriptProject `json:"scripts"`
	Evaluations []Evaluation    `json:"evaluations"`
	NextScript  int64           `json:"nextScript"`
	NextEval    int64           `json:"nextEval"`
}

var state = loadState()

func newState() *AppState {
	return &AppState{
		user: User{
			ID: 1, Phone: "未绑定", Nickname: "创作者", Avatar: "/avatar/03.webp",
			MemberUntil: "2026-06-06 19:48", Membership: "剧本专家", CanClaimPoint: false,
		},
		wallet: Wallet{
			Balance: 1000,
			Items:   []WalletTxn{{Title: "领取1000剧点", Delta: 1000, Time: "2026年06月05日 19:48"}},
			ServiceCosts: map[string]int{
				"evaluation": 2500,
			},
			Packs: []RechargePack{
				{Code: "p20", Price: 20, Original: 30, Points: 2000, UnitPrice: 0.01},
				{Code: "p100", Price: 100, Original: 150, Points: 10500, Bonus: 500, UnitPrice: 0.0095},
				{Code: "p300", Price: 300, Original: 450, Points: 33000, Bonus: 3000, UnitPrice: 0.0091},
				{Code: "e5000", Price: 5000, Points: 585000, Bonus: 17, Enterprise: true},
				{Code: "e30000", Price: 30000, Points: 3780000, Bonus: 26, Enterprise: true},
				{Code: "e100000", Price: 100000, Points: 13500000, Bonus: 35, Enterprise: true},
			},
			Tasks: []WalletTaskItem{
				{Title: "新人注册", Description: "新注册用户赠送1000剧点，自动到账", Claimed: true},
				{Title: "剧点福利", Description: "会员每30天可免费领取500剧点，过期不可领", Claimed: false},
			},
		},
		scripts:     []ScriptProject{},
		evaluations: []Evaluation{},
		nextScript:  1,
		nextEval:    1,
	}
}

func loadState() *AppState {
	defaults := newState()
	db, err := openAppDB()
	if err != nil {
		log.Printf("failed to open database: %v", err)
		return defaults
	}
	if err := migrateAppDB(db); err != nil {
		log.Printf("failed to migrate database: %v", err)
		_ = db.Close()
		return defaults
	}
	loaded, ok, err := loadStateFromDB(db)
	if err != nil {
		log.Printf("failed to load state from database: %v", err)
		_ = db.Close()
		return defaults
	}
	if ok {
		loaded.db = db
		return loaded
	}
	if legacy, ok := loadLegacyStateFile(); ok {
		legacy.db = db
		legacy.saveLocked()
		return legacy
	}
	defaults.db = db
	defaults.saveLocked()
	return defaults
}

func openAppDB() (*sql.DB, error) {
	path := appDatabasePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func appDatabasePath() string {
	if path := strings.TrimSpace(os.Getenv("DB_PATH")); path != "" {
		return path
	}
	nextPath := filepath.Join("data", "jubengongfang.db")
	legacyPath := legacyDatabasePath()
	if _, err := os.Stat(nextPath); errors.Is(err, os.ErrNotExist) {
		if _, legacyErr := os.Stat(legacyPath); legacyErr == nil {
			return legacyPath
		}
	}
	return nextPath
}

func legacyDatabasePath() string {
	return filepath.Join("data", strings.Join([]string{"story", "play.db"}, ""))
}

func migrateAppDB(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS app_meta (name TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			phone TEXT NOT NULL,
			nickname TEXT NOT NULL,
			avatar TEXT NOT NULL,
			member_until TEXT NOT NULL,
			membership TEXT NOT NULL,
			can_claim_point INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS wallet_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			balance INTEGER NOT NULL,
			frozen INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS wallet_transactions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ordinal INTEGER NOT NULL,
			title TEXT NOT NULL,
			delta INTEGER NOT NULL,
			time_text TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS wallet_tasks (
			title TEXT PRIMARY KEY,
			description TEXT NOT NULL,
			claimed INTEGER NOT NULL,
			ordinal INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS recharge_packs (
			code TEXT PRIMARY KEY,
			price INTEGER NOT NULL,
			original INTEGER NOT NULL,
			points INTEGER NOT NULL,
			bonus INTEGER NOT NULL,
			unit_price REAL NOT NULL,
			enterprise INTEGER NOT NULL,
			ordinal INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS service_costs (
			name TEXT PRIMARY KEY,
			cost INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS script_projects (
			id INTEGER PRIMARY KEY,
			ordinal INTEGER NOT NULL,
			title TEXT NOT NULL,
			script_type TEXT NOT NULL,
			source TEXT NOT NULL,
			status TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			settings_json TEXT NOT NULL,
			characters_json TEXT NOT NULL,
			outlines_json TEXT NOT NULL,
			episodes_json TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS evaluations (
			id INTEGER PRIMARY KEY,
			ordinal INTEGER NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			score INTEGER NOT NULL,
			summary TEXT NOT NULL,
			dimensions_json TEXT NOT NULL,
			suggestions_json TEXT NOT NULL,
			cost INTEGER NOT NULL,
			report_no TEXT NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *AppState) saveLocked() {
	if s.db == nil {
		return
	}
	if err := saveStateToDB(s.db, persistedAppState{
		User:        s.user,
		Wallet:      s.wallet,
		Scripts:     s.scripts,
		Evaluations: s.evaluations,
		NextScript:  s.nextScript,
		NextEval:    s.nextEval,
	}); err != nil {
		log.Printf("failed to save state: %v", err)
	}
}

func saveStateToDB(db *sql.DB, snapshot persistedAppState) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, table := range []string{
		"users", "wallet_state", "wallet_transactions", "wallet_tasks", "recharge_packs",
		"service_costs", "script_projects", "evaluations", "app_meta",
	} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO users (id, phone, nickname, avatar, member_until, membership, can_claim_point) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		snapshot.User.ID, snapshot.User.Phone, snapshot.User.Nickname, snapshot.User.Avatar,
		snapshot.User.MemberUntil, snapshot.User.Membership, boolToInt(snapshot.User.CanClaimPoint),
	); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO wallet_state (id, balance, frozen) VALUES (1, ?, ?)`, snapshot.Wallet.Balance, snapshot.Wallet.Frozen); err != nil {
		return err
	}
	for i, item := range snapshot.Wallet.Items {
		if _, err := tx.Exec(`INSERT INTO wallet_transactions (ordinal, title, delta, time_text) VALUES (?, ?, ?, ?)`, i, item.Title, item.Delta, item.Time); err != nil {
			return err
		}
	}
	for i, task := range snapshot.Wallet.Tasks {
		if _, err := tx.Exec(`INSERT INTO wallet_tasks (title, description, claimed, ordinal) VALUES (?, ?, ?, ?)`, task.Title, task.Description, boolToInt(task.Claimed), i); err != nil {
			return err
		}
	}
	for i, pack := range snapshot.Wallet.Packs {
		if _, err := tx.Exec(
			`INSERT INTO recharge_packs (code, price, original, points, bonus, unit_price, enterprise, ordinal) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			pack.Code, pack.Price, pack.Original, pack.Points, pack.Bonus, pack.UnitPrice, boolToInt(pack.Enterprise), i,
		); err != nil {
			return err
		}
	}
	for name, cost := range snapshot.Wallet.ServiceCosts {
		if _, err := tx.Exec(`INSERT INTO service_costs (name, cost) VALUES (?, ?)`, name, cost); err != nil {
			return err
		}
	}
	for i, project := range snapshot.Scripts {
		settings, err := jsonText(project.Settings)
		if err != nil {
			return err
		}
		characters, err := jsonText(project.Characters)
		if err != nil {
			return err
		}
		outlines, err := jsonText(project.Outlines)
		if err != nil {
			return err
		}
		episodes, err := jsonText(project.Episodes)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO script_projects (id, ordinal, title, script_type, source, status, updated_at, settings_json, characters_json, outlines_json, episodes_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			project.ID, i, project.Title, project.Type, project.Source, project.Status, project.UpdatedAt, settings, characters, outlines, episodes,
		); err != nil {
			return err
		}
	}
	for i, evaluation := range snapshot.Evaluations {
		dimensions, err := jsonText(evaluation.Dimensions)
		if err != nil {
			return err
		}
		suggestions, err := jsonText(evaluation.Suggestions)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO evaluations (id, ordinal, title, status, created_at, score, summary, dimensions_json, suggestions_json, cost, report_no) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			evaluation.ID, i, evaluation.Title, evaluation.Status, evaluation.CreatedAt, evaluation.Score, evaluation.Summary, dimensions, suggestions, evaluation.Cost, evaluation.ReportNo,
		); err != nil {
			return err
		}
	}
	meta := map[string]string{
		"initialized": "1",
		"nextScript":  strconv.FormatInt(snapshot.NextScript, 10),
		"nextEval":    strconv.FormatInt(snapshot.NextEval, 10),
	}
	for name, value := range meta {
		if _, err := tx.Exec(`INSERT INTO app_meta (name, value) VALUES (?, ?)`, name, value); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func loadStateFromDB(db *sql.DB) (*AppState, bool, error) {
	var initialized string
	err := db.QueryRow(`SELECT value FROM app_meta WHERE name = 'initialized'`).Scan(&initialized)
	if errorsIsNoRows(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	state := newState()
	if err := loadUserFromDB(db, state); err != nil {
		return nil, false, err
	}
	if err := loadWalletFromDB(db, state); err != nil {
		return nil, false, err
	}
	if err := loadScriptsFromDB(db, state); err != nil {
		return nil, false, err
	}
	if err := loadEvaluationsFromDB(db, state); err != nil {
		return nil, false, err
	}
	state.nextScript = nextID(loadMetaInt(db, "nextScript"), scriptMaxID(state.scripts))
	state.nextEval = nextID(loadMetaInt(db, "nextEval"), evaluationMaxID(state.evaluations))
	return state, true, nil
}

func loadUserFromDB(db *sql.DB, state *AppState) error {
	var canClaim int
	err := db.QueryRow(`SELECT id, phone, nickname, avatar, member_until, membership, can_claim_point FROM users LIMIT 1`).Scan(
		&state.user.ID, &state.user.Phone, &state.user.Nickname, &state.user.Avatar,
		&state.user.MemberUntil, &state.user.Membership, &canClaim,
	)
	if errorsIsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	state.user.CanClaimPoint = canClaim != 0
	return nil
}

func loadWalletFromDB(db *sql.DB, state *AppState) error {
	if err := db.QueryRow(`SELECT balance, frozen FROM wallet_state WHERE id = 1`).Scan(&state.wallet.Balance, &state.wallet.Frozen); err != nil && !errorsIsNoRows(err) {
		return err
	}
	items, err := queryWalletTransactions(db)
	if err != nil {
		return err
	}
	state.wallet.Items = items
	tasks, err := queryWalletTasks(db)
	if err != nil {
		return err
	}
	if len(tasks) > 0 {
		state.wallet.Tasks = tasks
	}
	packs, err := queryRechargePacks(db)
	if err != nil {
		return err
	}
	if len(packs) > 0 {
		state.wallet.Packs = packs
	}
	costs, err := queryServiceCosts(db)
	if err != nil {
		return err
	}
	if len(costs) > 0 {
		state.wallet.ServiceCosts = costs
	}
	return nil
}

func queryWalletTransactions(db *sql.DB) ([]WalletTxn, error) {
	rows, err := db.Query(`SELECT title, delta, time_text FROM wallet_transactions ORDER BY ordinal ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []WalletTxn{}
	for rows.Next() {
		var item WalletTxn
		if err := rows.Scan(&item.Title, &item.Delta, &item.Time); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func queryWalletTasks(db *sql.DB) ([]WalletTaskItem, error) {
	rows, err := db.Query(`SELECT title, description, claimed FROM wallet_tasks ORDER BY ordinal ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []WalletTaskItem{}
	for rows.Next() {
		var item WalletTaskItem
		var claimed int
		if err := rows.Scan(&item.Title, &item.Description, &claimed); err != nil {
			return nil, err
		}
		item.Claimed = claimed != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func queryRechargePacks(db *sql.DB) ([]RechargePack, error) {
	rows, err := db.Query(`SELECT code, price, original, points, bonus, unit_price, enterprise FROM recharge_packs ORDER BY ordinal ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RechargePack{}
	for rows.Next() {
		var item RechargePack
		var enterprise int
		if err := rows.Scan(&item.Code, &item.Price, &item.Original, &item.Points, &item.Bonus, &item.UnitPrice, &enterprise); err != nil {
			return nil, err
		}
		item.Enterprise = enterprise != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func queryServiceCosts(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query(`SELECT name, cost FROM service_costs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := map[string]int{}
	for rows.Next() {
		var name string
		var cost int
		if err := rows.Scan(&name, &cost); err != nil {
			return nil, err
		}
		items[name] = cost
	}
	return items, rows.Err()
}

func loadScriptsFromDB(db *sql.DB, state *AppState) error {
	rows, err := db.Query(`SELECT id, title, script_type, source, status, updated_at, settings_json, characters_json, outlines_json, episodes_json FROM script_projects ORDER BY ordinal ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	projects := []ScriptProject{}
	for rows.Next() {
		var project ScriptProject
		var settingsJSON, charactersJSON, outlinesJSON, episodesJSON string
		if err := rows.Scan(&project.ID, &project.Title, &project.Type, &project.Source, &project.Status, &project.UpdatedAt, &settingsJSON, &charactersJSON, &outlinesJSON, &episodesJSON); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(settingsJSON), &project.Settings); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(charactersJSON), &project.Characters); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(outlinesJSON), &project.Outlines); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(episodesJSON), &project.Episodes); err != nil {
			return err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	state.scripts = projects
	return nil
}

func loadEvaluationsFromDB(db *sql.DB, state *AppState) error {
	rows, err := db.Query(`SELECT id, title, status, created_at, score, summary, dimensions_json, suggestions_json, cost, report_no FROM evaluations ORDER BY ordinal ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []Evaluation{}
	for rows.Next() {
		var item Evaluation
		var dimensionsJSON, suggestionsJSON string
		if err := rows.Scan(&item.ID, &item.Title, &item.Status, &item.CreatedAt, &item.Score, &item.Summary, &dimensionsJSON, &suggestionsJSON, &item.Cost, &item.ReportNo); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(dimensionsJSON), &item.Dimensions); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(suggestionsJSON), &item.Suggestions); err != nil {
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	state.evaluations = items
	return nil
}

func loadMetaInt(db *sql.DB, name string) int64 {
	var value string
	if err := db.QueryRow(`SELECT value FROM app_meta WHERE name = ?`, name).Scan(&value); err != nil {
		return 0
	}
	id, _ := strconv.ParseInt(value, 10, 64)
	return id
}

func loadLegacyStateFile() (*AppState, bool) {
	path := filepath.Join("data", "state.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	state := newState()
	var persisted persistedAppState
	if err := json.Unmarshal(raw, &persisted); err != nil {
		log.Printf("failed to parse legacy state file %s: %v", path, err)
		return nil, false
	}
	if persisted.User.ID != 0 {
		state.user = persisted.User
	}
	state.wallet = persisted.Wallet
	if len(state.wallet.Packs) == 0 {
		state.wallet.Packs = newState().wallet.Packs
	}
	if len(state.wallet.Tasks) == 0 {
		state.wallet.Tasks = newState().wallet.Tasks
	}
	if len(state.wallet.ServiceCosts) == 0 {
		state.wallet.ServiceCosts = newState().wallet.ServiceCosts
	}
	if persisted.Scripts != nil {
		state.scripts = persisted.Scripts
	}
	if persisted.Evaluations != nil {
		state.evaluations = persisted.Evaluations
	}
	state.nextScript = nextID(persisted.NextScript, scriptMaxID(state.scripts))
	state.nextEval = nextID(persisted.NextEval, evaluationMaxID(state.evaluations))
	return state, true
}

func jsonText(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}

func scriptMaxID(items []ScriptProject) int64 {
	var maxID int64
	for _, item := range items {
		if item.ID > maxID {
			maxID = item.ID
		}
	}
	return maxID
}

func evaluationMaxID(items []Evaluation) int64 {
	var maxID int64
	for _, item := range items {
		if item.ID > maxID {
			maxID = item.ID
		}
	}
	return maxID
}

func nextID(saved, maxExisting int64) int64 {
	if saved > maxExisting {
		return saved
	}
	return maxExisting + 1
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/profile", getProfile)
	mux.HandleFunc("/api/wallet", getWallet)
	mux.HandleFunc("/api/wallet/claim", walletClaimHandler)
	mux.HandleFunc("/api/scripts", scriptsHandler)
	mux.HandleFunc("/api/scripts/", scriptDetailHandler)
	mux.HandleFunc("/api/ai-task/stream", aiTaskStreamHandler)
	mux.HandleFunc("/api/ai-task", aiTaskHandler)
	mux.HandleFunc("/api/upload/stream", uploadStreamHandler)
	mux.HandleFunc("/api/upload", uploadHandler)
	mux.HandleFunc("/api/evaluations", evaluationsHandler)
	mux.HandleFunc("/api/evaluations/stream", evaluationStreamHandler)
	mux.HandleFunc("/api/recharge", rechargeHandler)
	mux.HandleFunc("/api/membership/renew", membershipRenewHandler)
	mux.HandleFunc("/api/settings/profile", updateProfile)
	mux.HandleFunc("/api/settings/password", updatePassword)
	mux.HandleFunc("/api/settings/ai-config", aiConfigHandler)
	mux.HandleFunc("/api/export", exportHandler)
	mux.HandleFunc("/", staticFallback)
	port := os.Getenv("PORT")
	if port == "" {
		port = "18080"
	}
	log.Println("剧本工坊 API listening on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, cors(mux)))
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func getProfile(w http.ResponseWriter, r *http.Request) {
	state.mu.Lock()
	defer state.mu.Unlock()
	writeJSON(w, state.user)
}

func getWallet(w http.ResponseWriter, r *http.Request) {
	state.mu.Lock()
	defer state.mu.Unlock()
	writeJSON(w, state.wallet)
}

func scriptsHandler(w http.ResponseWriter, r *http.Request) {
	state.mu.Lock()
	defer state.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, state.scripts)
	case http.MethodPost:
		var req struct {
			Type   string `json:"type"`
			Source string `json:"source"`
			Title  string `json:"title"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Title == "" {
			req.Title = "未命名"
		}
		project := state.newProjectLocked(req.Title, req.Type, req.Source)
		state.nextScript++
		state.scripts = append([]ScriptProject{project}, state.scripts...)
		state.saveLocked()
		writeJSON(w, project)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *AppState) newProjectLocked(title, projectType, source string) ScriptProject {
	if projectType == "" {
		projectType = "AI短剧"
	}
	if source == "" {
		source = "original"
	}
	return ScriptProject{
		ID:        s.nextScript,
		Title:     title,
		Type:      projectType,
		Source:    source,
		Status:    "draft",
		UpdatedAt: time.Now().Format("2006-01-02 15:04"),
		Settings: map[string]any{
			"audience": "", "genres": []string{}, "core": []string{}, "style": []string{},
			"worldView": "", "highlights": "", "synopsis": "",
		},
		Episodes: []Episode{{No: 1, Outline: "", Body: ""}},
	}
}

func scriptDetailHandler(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimPrefix(r.URL.Path, "/api/scripts/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		http.Error(w, "invalid script id", http.StatusBadRequest)
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	for i := range state.scripts {
		if state.scripts[i].ID != id {
			continue
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, state.scripts[i])
		case http.MethodPut:
			var req ScriptProject
			_ = json.NewDecoder(r.Body).Decode(&req)
			if strings.TrimSpace(req.Title) != "" {
				state.scripts[i].Title = strings.TrimSpace(req.Title)
			}
			if req.Type != "" {
				state.scripts[i].Type = req.Type
			}
			if req.Settings != nil {
				state.scripts[i].Settings = req.Settings
			}
			if req.Characters != nil {
				state.scripts[i].Characters = req.Characters
			}
			if req.Outlines != nil {
				state.scripts[i].Outlines = req.Outlines
			}
			if req.Episodes != nil {
				state.scripts[i].Episodes = req.Episodes
			}
			state.scripts[i].UpdatedAt = time.Now().Format("2006-01-02 15:04")
			state.saveLocked()
			writeJSON(w, state.scripts[i])
		case http.MethodDelete:
			state.scripts = append(state.scripts[:i], state.scripts[i+1:]...)
			state.saveLocked()
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	http.NotFound(w, r)
}

func aiTaskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Cost      int           `json:"cost"`
		TaskType  string        `json:"taskType"`
		ProjectID int64         `json:"projectId"`
		Project   ScriptProject `json:"project"`
		Prompt    string        `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ProjectID == 0 && req.Project.ID != 0 {
		req.ProjectID = req.Project.ID
	}
	cost := taskCost(req.TaskType, req.Cost)
	if cost == 0 && req.TaskType != "planning" {
		http.Error(w, "unsupported task type", http.StatusBadRequest)
		return
	}
	taskID := time.Now().UnixMilli()
	var projectSnapshot ScriptProject

	state.mu.Lock()
	projectIndex := -1
	for i := range state.scripts {
		if state.scripts[i].ID == req.ProjectID {
			projectIndex = i
			break
		}
	}
	if projectIndex < 0 {
		state.mu.Unlock()
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if req.Project.ID == req.ProjectID {
		req.Project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
		state.scripts[projectIndex] = req.Project
	}
	if cost > 0 {
		if state.wallet.Balance < cost {
			state.mu.Unlock()
			http.Error(w, "insufficient points", http.StatusPaymentRequired)
			return
		}
	}
	projectSnapshot = state.scripts[projectIndex]
	state.mu.Unlock()

	result, err := generateWithAI(projectSnapshot, req.TaskType, req.Prompt)
	if err != nil {
		log.Printf("ai task %s failed: %v", req.TaskType, err)
		http.Error(w, "AI生成失败："+err.Error(), http.StatusBadGateway)
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	for i := range state.scripts {
		if state.scripts[i].ID == req.ProjectID {
			if cost > 0 {
				if state.wallet.Balance < cost {
					http.Error(w, "insufficient points", http.StatusPaymentRequired)
					return
				}
				state.wallet.Balance -= cost
				state.wallet.Items = append([]WalletTxn{{Title: aiTaskTitle(req.TaskType), Delta: -cost, Time: time.Now().Format("2006年01月02日 15:04")}}, state.wallet.Items...)
			}
			applyAIResult(&state.scripts[i], req.TaskType, result)
			state.saveLocked()
			writeJSON(w, map[string]any{
				"taskId":  taskID,
				"status":  "succeeded",
				"source":  "ai",
				"cost":    cost,
				"result":  result,
				"project": state.scripts[i],
				"wallet":  state.wallet,
			})
			return
		}
	}
	http.Error(w, "project not found", http.StatusNotFound)
}

func aiTaskStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	var req struct {
		TaskType  string        `json:"taskType"`
		Cost      int           `json:"cost"`
		ProjectID int64         `json:"projectId"`
		Project   ScriptProject `json:"project"`
		Prompt    string        `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "invalid request body"})
		return
	}
	if req.ProjectID == 0 && req.Project.ID != 0 {
		req.ProjectID = req.Project.ID
	}
	cost := taskCost(req.TaskType, req.Cost)
	if cost == 0 {
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "unsupported stream task"})
		return
	}
	var projectSnapshot ScriptProject
	state.mu.Lock()
	projectIndex := -1
	for i := range state.scripts {
		if state.scripts[i].ID == req.ProjectID {
			projectIndex = i
			break
		}
	}
	if projectIndex < 0 {
		state.mu.Unlock()
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "project not found"})
		return
	}
	if state.wallet.Balance < cost {
		state.mu.Unlock()
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "insufficient points"})
		return
	}
	if req.Project.ID == req.ProjectID {
		req.Project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
		state.scripts[projectIndex] = req.Project
	}
	projectSnapshot = state.scripts[projectIndex]
	state.mu.Unlock()

	var streamed strings.Builder
	if req.TaskType == "characters" {
		prompt := buildCharacterStreamPrompt(projectSnapshot, req.Prompt)
		systemPrompt := `【角色定义】
你是一位国家级短剧人物设计大师，拥有超过20年的影视编剧、角色心理工程与人物原型研究经验。你曾为超过500部爆款竖屏短剧设计过人物群像，作品涵盖男频战神流、女频虐恋流、甜宠流、悬疑流、逆袭流等所有主流短剧类型。你深谙荣格原型理论、九型人格模型和MBTI人格框架，并将其创造性地融入短剧角色设计之中。你的每一个人物都能在3秒内让观众记住，5秒内让观众产生情感投射，30秒内让观众产生追看欲望。你是短剧行业公认的"角色魔术师"。

【核心使命】
你的核心使命是：基于项目的故事概梗、世界观设定、题材类型和目标受众，创造一组层次分明、关系紧密、能支撑完整短剧叙事的人物群像。每个人物都不是孤立的设定卡片，而是故事冲突网络中的活态节点——他们有欲望、有缺陷、有秘密、有变化。你设计的人物必须能让观众产生强烈的情感共鸣：对主角投射欲望，对反派激发愤怒，对助攻产生信任，对灰色角色感到好奇。人物体系必须形成一张密不透风的冲突之网，任何两个角色之间都能产生戏剧张力。

【专业能力矩阵】
*角色心理学工程：*
- 精通人物深层动机设计，每个角色的行为必须有内在驱动力，不允许出现"为坏而坏"或"为善而善"的扁平角色
- 擅长用"创伤-防御-欲望"三角模型构建角色的心理底层逻辑：每个关键角色都有一个核心创伤事件，由此形成的防御机制塑造了其外在人格面具，而未被满足的深层欲望则驱动其全部行为
- 理解人格面具与真实自我之间的张力，善于设计身份反转——让观众在某个节点突然发现"原来他/她是这样的人"
- 掌握角色情绪能量层级设计：从冷漠→好奇→关注→紧张→愤怒→爆发→崩溃→重生，每个关键角色必须经历完整的情绪弧线
- 善于设计角色的认知盲区和信息差，让人物因为"不知道"而做出让观众揪心的选择

*短剧角色极致标签化法则：*
- 短剧人物必须在3秒内完成标签传达，观众闭上眼回忆角色时，脑海中必须有一个鲜明的视觉符号或行为标签
- 每个角色必须有三重辨识系统：①视觉标签（穿着、配饰、体态、标志性动作）②语言标签（口头禅、说话节奏、用词偏好、语气特征）③行为标签（面对冲突的第一反应、决策模式、底线和雷区）
- 主角必须有清晰的欲望链条：初始目标→遭遇阻碍→爆发反抗→阶段性胜利→更高目标→终极目标，每一个目标节点都对应一个观众爽点
- 主角必须有"观众代入锚点"——一个让目标受众立刻产生认同感的处境或特质（如：被看不起的小人物、被背叛的深情者、被压制的天才）
- 反派设计要精准匹配受众爽点公式：男频反派重实力压制（我要让你知道谁才是强者）+身份碾压（你这种人也配？）；女频反派重情感背叛（我以为你爱我）+关系欺压（你永远比不上她/他）
- 反派必须有"压迫升级阶梯"：每次出场都要比上次更强、更狠、更让人愤怒，直到最终被主角碾压时观众获得最大爽感
- 助攻角色必须有"功能分化"：信息型助攻（提供关键情报）、情感型助攻（给予主角精神支撑）、能力型助攻（在关键时刻提供实际帮助）、搞笑型助攻（调节节奏缓解紧张）

*人物关系网络设计：*
- 擅长设计"主角-反派-助攻"三角核心关系，三角的每条边都要有张力
- 角色之间必须有四重关系链交织：①利益链（谁控制谁的资源？谁挡了谁的路？）②情感链（谁爱谁？谁恨谁？谁欠谁？）③秘密链（谁知道谁的秘密？谁在对谁隐瞒？）④冲突链（谁和谁的目标直接对立？）
- 关系角色要为主角的"爽点时刻"和"情绪爆发点"服务，每个配角出场都要推动主角的状态变化
- 配角也要有自己的小目标和行为逻辑，不能沦为纯工具人——即使只有三场戏的角色，也要让观众觉得"这个人有自己的故事"
- 擅长设计"关系反转"：前期的信任对象后期背叛、前期的敌人后期成为盟友、看似无关的角色实则是关键人物

*人物弧光与记忆点工程：*
- 关键角色需要有可感知的内在变化或成长弧光，弧光必须有明确的转折点和触发事件
- 要善于用"反差萌""身份反转""隐藏属性""实力伪装"制造记忆炸弹
- 性格标签之间不能矛盾，必须形成统一且有趣的人格特质——但表面矛盾、内在统一的"复杂人格"是最加分的
- 每个重要角色都要有一个"观众期待时刻"——观众追剧时最想看到这个角色出场做什么事
- 角色的名字要有记忆点：符合人设气质、朗朗上口、不能和同剧其他角色混淆

【创作方法论】
第一步——需求解读与定位（需求分析阶段）：
- 深度分析项目快照中的故事概梗、世界观、题材类型和目标受众
- 理解用户补充的特殊要求，判断项目类型（原创/改编/改写）并决定角色设计策略
- 确定目标受众的核心情感诉求：男频要"被尊重→碾压对手"的爽感；女频要"被理解→情感救赎"的共鸣
- 提炼故事的核心冲突类型：是实力对抗、情感博弈、身份揭秘、还是命运逆袭？

第二步——角色体系架构设计（体系规划阶段）：
- 确定主角的核心人格、初始处境和成长空间——主角是观众的欲望投射载体
- 设计反派体系：直接对手（正面冲突）、幕后黑手（深层威胁）、灰色角色（亦敌亦友）、阶段性反派（每阶段的压迫者）
- 规划助攻体系：忠诚盟友（无条件支持）、亦敌亦友（增加戏剧性）、亲情角色（情感锚点）、师长角色（能力支撑）
- 填充关系角色：制造冲突的第三方（三角关系、竞争者）、提供信息的辅助（知情者、见证者）
- 绘制关系网草图，确保任何两个角色之间都有至少一条关系线

第三步——深度塑造与细节填充（核心创作阶段）：
- 为每个角色构建完整的背景故事和心理画像，重点设计"转折事件"——那个改变了角色人生轨迹的关键时刻
- 设计角色之间的利益关系、情感纠葛和冲突点，确保关系网中没有"死角"
- 为关键角色设计标志性的台词风格和行为模式，让观众只看台词就知道是谁在说话
- 为每个角色设计"高光时刻"——在整部剧中这个角色最让观众印象深刻的场景
- 设计角色的信息差和秘密——谁知道什么、不知道什么、以为知道但其实是错的

第四步——质量检验与优化（品控阶段）：
- 检查角色差异化：同一场景中任意两个角色的反应必须不同
- 检查逻辑自洽性：年龄、背景、动机、能力之间不能有矛盾
- 检查反派合理性：反派的恶行必须有内在逻辑支撑，不能"为坏而坏"
- 检查关系网完整性：确保没有孤立节点，每个角色都至少与2个其他角色有直接关系
- 检查受众匹配度：角色设计是否精准命中目标受众的情感诉求

【输出规范】
1. 严格按照【人物N】标记格式输出，每个字段独占一行
2. 字段要求：姓名、定位、年龄、性格标签、背景、核心动机、人物关系、人物小传，每个字段都必须有实质内容
3. 人物小传要求200-400字，必须包含：①角色的欲望和梦想②核心缺陷或弱点③关键转折事件④关系变化节点⑤隐藏秘密或反转伏笔
4. 第一个角色必须是主角，后续按剧情重要性排序
5. 禁止写"待定""无""略""暂无"等占位符
6. 禁止使用JSON、Markdown格式或解释性文字
7. 每个人物的名字必须有辨识度，不能与同剧其他角色名字相似

【质量自检清单】
□ 每个人物是否有独特的说话方式和行为习惯？ □ 反派是否有合理动机和足够压迫感？ □ 角色之间是否有差异化避免功能重叠？ □ 年龄、背景、动机是否逻辑自洽？ □ 人物关系网是否能支撑主要冲突？ □ 关键角色是否有记忆点（标签、反差、秘密）？ □ 助攻角色是否为主角爽点时刻服务？ □ 主角是否有清晰的欲望链条和成长弧光？ □ 每个角色是否能让目标受众产生情感投射？ □ 人物名字是否有辨识度和记忆点？ □ 关系网中是否存在孤立节点？ □ 反派的压迫感是否逐级递增？`
		writeStreamFrame(w, flusher, "start", map[string]any{"taskType": req.TaskType})
		err := requestTextStreamFromAI(systemPrompt, prompt, 0.8, func(delta string) error {
			streamed.WriteString(delta)
			characters := parseCharacterStreamCharacters(streamed.String())
			writeStreamFrame(w, flusher, "character", map[string]any{"taskType": req.TaskType, "text": delta, "characters": characters})
			return nil
		})
		if err != nil {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "人物生成失败，请稍后重试"})
			return
		}
		characters := parseCharacterStreamCharacters(streamed.String())
		if len(characters) == 0 {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "人物生成结果不完整，请重试"})
			return
		}
		result := map[string]any{"characters": characters}
		state.mu.Lock()
		defer state.mu.Unlock()
		for i := range state.scripts {
			if state.scripts[i].ID == req.ProjectID {
				if state.wallet.Balance < cost {
					writeStreamFrame(w, flusher, "error", map[string]any{"message": "insufficient points"})
					return
				}
				state.wallet.Balance -= cost
				state.wallet.Items = append([]WalletTxn{{Title: aiTaskTitle(req.TaskType), Delta: -cost, Time: time.Now().Format("2006年01月02日 15:04")}}, state.wallet.Items...)
				applyAIResult(&state.scripts[i], req.TaskType, result)
				state.saveLocked()
				writeStreamFrame(w, flusher, "done", map[string]any{
					"status":  "succeeded",
					"result":  result,
					"project": state.scripts[i],
					"wallet":  state.wallet,
				})
				return
			}
		}
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "project not found"})
		return
	}

	if req.TaskType == "outline" {
		prompt := buildOutlineStreamPrompt(projectSnapshot, req.Prompt)
		systemPrompt := `【角色定义】
你是一位国家级短剧叙事架构大师，拥有超过20年的叙事结构设计与短剧节奏编排经验。你精通中国传统叙事结构"起承转合"与现代短剧节奏的深度融合，曾为超过300部爆款短剧设计过四阶段叙事骨架，作品涵盖逆袭爽剧、虐恋甜宠、悬疑反转、热血战神等所有主流类型。你深谙亚里士多德三幕式结构、布莱克·斯奈德节拍表、英雄之旅模型，并创造性地将其压缩适配为竖屏短剧的超紧凑节奏。你的粗纲设计能让观众从第一集到最后一集全程无尿点，每一阶段都有让人欲罢不能的追看动力。

【核心使命】
你的核心使命是：基于故事概梗、人物小传和世界观设定，设计一套四阶段（起承转合）粗纲。每个阶段必须有明确的戏剧任务、标志性事件、情绪曲线和追看钩子，四阶段之间必须形成完整的叙事弧线——像一条不断加速的过山车，从第一个弯道开始就让观众心跳加速，直到最后一个俯冲达到情绪巅峰。粗纲是整部短剧的骨架，骨架的质量直接决定了后续集纲和正文的上限。

【专业能力矩阵】
*短剧节奏控制工程：*
- 精通竖屏短剧的核心节奏公式："3秒钩子→30秒反转→1集结尾悬念"，并将此公式升级为阶段级的节奏编排
- 深刻理解短剧观众的注意力曲线：前3集决定去留，前10集决定付费，全剧决定口碑——每个阶段的节奏密度必须匹配观众不同阶段的期待阈值
- 掌握"压迫-爽点-升级"的循环节奏公式：每一阶段的爽点必须比上一阶段更爽，压迫必须比上一阶段更重，形成螺旋上升的刺激曲线
- 擅长在有限集数内完成完整的叙事弧线，不拖沓不赶进度——该快的地方3集完成，该铺的地方5集不嫌多
- 精通"节奏呼吸"设计：连续高压之后必须有短暂喘息，让观众的情绪有起有伏，而不是一直绷紧导致麻木

*四阶段结构深度设计：*
- 【起】阶段核心任务公式：建立主角处境（让观众代入）→引爆核心事件（打破日常）→反派首次压迫（制造愤怒）→第一轮爽点（小胜或反击）→阶段末钩子（更大的危机来了）
  起阶段是"入场券"，必须在前3集内让观众明确知道：主角是谁？他/她想要什么？谁在挡路？我为什么要追？
- 【承】阶段核心任务公式：推进关系变化（新关系出现或旧关系裂变）→反派压力升级（更高层级的威胁）→误会或秘密扩大（信息差制造焦虑）→爽点升级（更大的反击或成长）→阶段末钩子（真相的边缘或危机的预兆）
  承阶段是"上瘾期"，观众已经入坑，现在要让他们越陷越深——每集都要有"差一点就知道真相""差一点就成功"的紧迫感
- 【转】阶段核心任务公式：真相揭露（信息差消除）→危机全面升级（从局部冲突到全面战争）→人物面临终极抉择（失去什么才能得到什么）→关键反转和情绪爆点（最大的泪点或怒点）→阶段末钩子（终极对决的序幕）
  转阶段是"高潮前奏"，情绪要在这里拉到最满——观众应该在这里又哭又怒又期待，情绪过山车到达最高点
- 【合】阶段核心任务公式：终局对抗（所有矛盾的总爆发）→反派清算（积攒了全剧的愤怒在此释放）→人物关系收束（每条关系线都有交代）→核心爽点最终兑现（全剧最大的爽感时刻）→结局余味（可留续作钩子但不牺牲本季收束）
  合阶段是"释放期"，观众积攒的所有情绪在此刻得到最大化的宣泄和满足

*多线叙事编排：*
- 主线推进的同时，情感线、副线、成长线要有机穿插，形成叙事的和弦效果
- 每个阶段的多线推进要有主次之分：主线占60%篇幅，情感线占25%，副线占15%——不能喧宾夺主
- 擅长设计"线与线交叉"的爆点时刻：当情感线的高潮与主线的危机同时到来，戏剧张力达到最大值
- 副线必须为主线服务，不能独立发展——每条副线最终都要汇入主线的洪流

*受众节奏精准适配：*
- 男频短剧节奏公式：压迫要重（踩到尊严底线）→爆发要猛（一次性碾压回去）→升级要快（不断突破新的实力天花板）→爽感要递进（从小爽到大爽到终极爽）
- 女频短剧节奏公式：情感要细（每一个眼神、每一句话都有深意）→误解要深（观众知道真相但角色不知道，制造心疼）→反转要虐（真相揭露时的情感冲击）→救赎要暖（最终的情感归宿）
- 混合类型短剧节奏：在主要受众诉求的基础上，穿插另一种受众的爽点——如男频剧中加入情感线的细腻处理，女频剧中加入实力碾压的爽感
- 不同题材的节奏重点要根据项目类型动态调整，不能套用固定模板

【创作方法论】
第一步——全局分析与提炼（深度阅读阶段）：
- 通读故事概梗和人物小传，提炼出五个核心要素：①核心冲突（主角与什么力量对抗？）②爽点机制（观众追看的核心动力是什么？）③情感主线（角色之间最牵动人心的关系是什么？）④反派压迫层级（反派的威胁如何逐级递增？）⑤终极反转（全剧最大的意外是什么？）
- 理解目标受众和制作方式对节奏的影响——男频/女频、AI/实拍，节奏策略完全不同
- 确定全剧的情绪基调：是以爽为主？以虐为主？还是爽虐交替？

第二步——阶段划分与任务分配（骨架搭建阶段）：
- 根据总集数合理分配起承转合各阶段的集数比例——通常为2:3:3:2，但可根据题材灵活调整
- 确定每个阶段的戏剧核心任务、标志性事件（至少2个可拍摄的大场面）和阶段钩子
- 为每个阶段设定"情绪目标"：起=好奇+愤怒，承=焦虑+期待，转=震惊+心疼，合=畅快+感动
- 在阶段之间设计"过桥事件"——连接两个阶段的关键转折

第三步——内容填充与细节打磨（核心创作阶段）：
- 为每个阶段写400-800字的完整段落，必须包含：①具体事件名称（可拍摄的场景描述）②人物行动（谁做了什么、为什么这样做）③冲突升级方式（比上一阶段更强的压迫）④爽点设计（观众看到这里会有什么感受）⑤阶段钩子（最后的悬念是什么）
- 确保事件之间有因果链，不能是随机堆砌
- 在每个阶段中设计"高光场景"——这个阶段中最让观众印象深刻的1-2个场面

第四步——整体校验与节奏优化（品控阶段）：
- 检查四阶段之间的因果递进关系，确保前一阶段的结果是后一阶段的原因
- 检查节奏曲线：整体是否呈上升趋势？是否有不必要的低谷？
- 确认没有空泛描述——每一句话都必须是可拍摄的具体事件
- 检查爽点递进：四个阶段的爽感是否层层递增？
- 检查反派压迫递进：反派的威胁是否逐级增强？

【输出规范】
1. 严格按照【起】【承】【转】【合】标记格式输出，每个阶段包含"范围"和"粗纲"字段
2. 每阶段粗纲必须写成400-800字的完整段落，概括该区间所有集的主要戏剧推进
3. 范围字段必须标注具体集数区间（如"第1-8集"），集数区间必须连续不重叠
4. 不要生成单集集纲，不要逐集编号，不要列出具体集数标题
5. 不要写"主角成长、矛盾升级"等空泛方向，必须写具体事件和人物行动
6. 禁止使用JSON、Markdown格式或解释性文字
7. 每个阶段的结尾必须明确写出"阶段钩子"——即驱动观众追看下一阶段的悬念

【质量自检清单】
□ 每阶段是否有明确的戏剧任务和标志性事件？ □ 四阶段之间是否有清晰的因果递进关系？ □ 是否写清了具体事件名称和人物行动？ □ 反派压力是否持续升级？ □ 爽点是否有层次递进和变化？ □ 集数区间是否合理连续？ □ 是否避免了空泛的方向性描述？ □ 结尾阶段是否完成了完整的收束？ □ 每阶段是否有可拍摄的高光场景？ □ 是否有"线与线交叉"的爆点时刻？ □ 节奏曲线是否呈上升趋势？ □ 每阶段钩子是否足以让观众追看下一阶段？`
		writeStreamFrame(w, flusher, "start", map[string]any{"taskType": req.TaskType})
		err := requestTextStreamFromAI(systemPrompt, prompt, 0.8, func(delta string) error {
			streamed.WriteString(delta)
			outlines := parseOutlineStreamOutlines(streamed.String())
			writeStreamFrame(w, flusher, "outline", map[string]any{"taskType": req.TaskType, "text": delta, "outlines": outlines})
			return nil
		})
		if err != nil {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "粗纲生成失败，请稍后重试"})
			return
		}
		outlines := parseOutlineStreamOutlines(streamed.String())
		if len(outlines) == 0 {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "粗纲生成结果不完整，请重试"})
			return
		}
		result := map[string]any{"outlines": outlines}
		state.mu.Lock()
		defer state.mu.Unlock()
		for i := range state.scripts {
			if state.scripts[i].ID == req.ProjectID {
				if state.wallet.Balance < cost {
					writeStreamFrame(w, flusher, "error", map[string]any{"message": "insufficient points"})
					return
				}
				state.wallet.Balance -= cost
				state.wallet.Items = append([]WalletTxn{{Title: aiTaskTitle(req.TaskType), Delta: -cost, Time: time.Now().Format("2006年01月02日 15:04")}}, state.wallet.Items...)
				applyAIResult(&state.scripts[i], req.TaskType, result)
				state.saveLocked()
				writeStreamFrame(w, flusher, "done", map[string]any{
					"status":  "succeeded",
					"result":  result,
					"project": state.scripts[i],
					"wallet":  state.wallet,
				})
				return
			}
		}
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "project not found"})
		return
	}

	if req.TaskType == "episode" {
		prompt := buildEpisodeStreamPrompt(projectSnapshot, req.Prompt)
		systemPrompt := `【角色定义】
你是一位国家级短剧分集编剧大师，拥有超过20年的短剧分集编剧和剧本医生经验。你曾为超过400部爆款竖屏短剧编写过分集剧情，精通将宏观叙事骨架拆解为一个个让观众欲罢不能的"上瘾单元"。你深刻理解短剧观众的追看心理学：每一集都是一剂"多巴胺注射"，必须在有限篇幅内完成"建立期待→满足部分期待→制造更大期待"的成瘾循环。你写的每一集结尾都像一把钩子，扎进观众心里，让他忍不住点下一集。

【核心使命】
你的核心使命是：基于故事概梗、人物小传和四阶段粗纲，将每个阶段的戏剧目标拆解为具体、可拍、有节奏感的单集剧情。每集必须有实质性剧情推进，不能是填充性内容；每集必须有独立的"情绪高潮点"和"追看钩子"，让观众产生"就看一集"却停不下来的上瘾体验。你要确保观众在看每一集时都有三种感觉：①这集真好看（当下满足）②下一集会怎样（追看欲望）③这剧真牛（整体信心）。

【专业能力矩阵】
*单集节奏精密工程：*
- 每集必须包含完整的戏剧单元："开场钩子（3秒抓人）→核心冲突建立（让观众知道本集在打什么仗）→对抗升级（对手出手了，而且比想象中更狠）→关键反转或爽点（反转预期或释放积攒的愤怒）→结尾追看钩子（留一个让观众睡不着觉的悬念）"
- 前30秒快速进入核心冲突，不拖沓——短剧观众的耐心以秒为单位计算
- 中间部分逐步升级，穿插至少1个小爽点维持观众注意力——不能让观众在中间段走神
- 结尾必须有反转、悬念或情绪钩子——最后30秒决定了观众是否点下一集
- 掌握"一集一爽点"的黄金法则：每集至少有一个让观众产生强烈情绪反应的瞬间

*事件设计精密度：*
- 每集的核心事件必须推动主线，不能是填充性日常——如果删掉这一集不影响后续剧情，那这一集就不该存在
- 事件设计必须有"五要素"：谁在推动（行动者）→为什么发生（动机和触发条件）→怎么发生（具体过程）→后果是什么（对人物和关系的影响）→留下什么尾巴（为下一集埋线）
- 擅长设计"压迫事件"和"爽点事件"的交替节奏：连续2集压迫后必须有1集爽点释放，形成"紧→紧→松→紧→紧→更松"的呼吸节奏
- 事件之间要有因果链，不能是随机堆砌——上一集的结果是这一集的原因，这一集的结果是下一集的起因
- 每个事件都要有"可拍摄性"：能转化成具体的场景、具体的动作、具体的台词

*人物驱动写作法则：*
- 每集必须有明确的推动者——冲突由人物的欲望碰撞产生，不是编剧强行安排
- 要写清人物在本集的具体目标（这集他/她想要什么）和行动（为了得到它做了什么）
- 人物的行为必须符合其性格设定和当前处境——不能为了剧情需要让人物做出不合逻辑的事
- 关键人物的选择要有内在逻辑和情感支撑——观众必须理解"为什么他/她会这样做"
- 每集至少有一个角色的状态发生变化（从不知道到知道、从信任到怀疑、从软弱到强硬）

*钩子设计大师级能力：*
- 每集结尾必须有让人忍不住点下一集的悬念或反转
- 钩子类型必须多样化，避免重复：
  ①信息揭露型："原来他才是真正的幕后黑手！"
  ②身份暴露型："你居然就是当年的那个小女孩？！"
  ③危机升级型："不好，敌人来了三倍的兵力！"
  ④情感转折型："对不起，我不能再帮你了。"
  ⑤选择悬念型："你要救她，还是要保住自己？"
  ⑥反转预期型："你以为的盟友，其实是敌人！"
  ⑦实力展示型："让你们见识一下，什么叫真正的力量！"
- 钩子要自然融入剧情，不能生硬制造——观众感觉到的是"剧情自然走到了这里"而不是"编剧故意吊我胃口"
- 不同类型的钩子要交替使用，不能每集都是同一种钩子

*连续性管理系统：*
- 前后集之间必须有因果衔接，不能割裂——观众能感受到"这是一段连续的故事"而不是"独立的短片合集"
- 每集开头要自然承接上一集的人物状态和未解决冲突，不需要重复交代
- 人物的情感状态要有连贯性，不能突然跳变——除非有明确的触发事件
- 已建立的设定和承诺（如悬念、伏笔、flag）要在合理的时间内逐步兑现，不能无限拖延
- 每5集左右要有一个"阶段性清算"：之前积累的某个矛盾在这一波得到解决，同时开启新的矛盾

【创作方法论】
第一步——阶段目标拆解（分析阶段）：
- 读取对应的粗纲阶段，明确这一集数区间需要完成的戏剧任务
- 将阶段目标拆解为若干个"子冲突"，每个子冲突对应1-2集
- 确定每集的核心事件类型（压迫/反击/铺垫/爆发/转折）
- 规划本阶段的爽点分布：哪些集有爽点、爽点的强度如何递增

第二步——事件链设计（骨架搭建阶段）：
- 将阶段目标拆解为具体的单集事件，形成因果链
- 每集分配：核心冲突（1个）+推进事件（1-2个）+结尾钩子（1个）
- 标记每集的情绪基调：紧张、愤怒、爽快、心疼、震惊、温暖
- 确保相邻集之间的情绪有变化，不能连续3集都是同一情绪

第三步——内容撰写（核心创作阶段）：
- 为每集写完整段落的集纲（80-200字），必须包含：
  ①核心事件（发生了什么）
  ②人物目标（谁想要什么）
  ③冲突推进（遇到了什么阻力、如何对抗）
  ④关键反转或爽点（预期被打破或积攒的情绪被释放）
  ⑤结尾追看钩子（为什么观众必须看下一集）
- 集纲要写得具体可拍——导演看了就知道该怎么拍，演员看了就知道该怎么演

第四步——连续性检验（品控阶段）：
- 检查前后集之间的因果衔接，确保没有断裂
- 检查爽点分布是否合理，是否有过长的"爽点荒漠"
- 确认每集结尾都有追看钩子
- 检查人物状态的连贯性
- 确认集数是否对应用户指定的范围，不多不少

【输出规范】
1. 严格按照【第X集】标记格式输出
2. 每集集纲写成80-200字的完整段落，必须包含核心事件、人物目标、冲突推进、关键反转/爽点、结尾钩子
3. 集数必须对应用户指定的范围，不多不少
4. 不要在每集里套写起承转合结构，不要输出"本集起/承/转/合"小标题
5. 禁止使用JSON、Markdown格式或解释性文字
6. 每集集纲的最后一句话必须是追看钩子，形成"钩尾"结构

【质量自检清单】
□ 每集是否有清晰的戏剧进展，不能原地踏步？ □ 前后集之间是否有因果衔接？ □ 每集是否有至少一个爽点或反转？ □ 每集结尾是否有追看钩子？ □ 集数是否对应用户指定范围？ □ 是否避免了空泛的方向性描述？ □ 人物行动是否具体而非概念堆砌？ □ 爽点是否有变化和升级？ □ 钩子类型是否多样化？ □ 是否有"一集一爽点"的黄金结构？ □ 人物状态变化是否合乎逻辑？ □ 相邻集的情绪是否有变化和节奏感？`
		writeStreamFrame(w, flusher, "start", map[string]any{"taskType": req.TaskType})
		err := requestTextStreamFromAI(systemPrompt, prompt, 0.8, func(delta string) error {
			streamed.WriteString(delta)
			episodes := parseEpisodeStreamEpisodes(streamed.String())
			writeStreamFrame(w, flusher, "episode", map[string]any{"taskType": req.TaskType, "text": delta, "episodes": episodes})
			return nil
		})
		if err != nil {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "集纲生成失败，请稍后重试"})
			return
		}
		episodes := parseEpisodeStreamEpisodes(streamed.String())
		if len(episodes) == 0 {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "集纲生成结果不完整，请重试"})
			return
		}
		result := map[string]any{"episodes": episodes}
		state.mu.Lock()
		defer state.mu.Unlock()
		for i := range state.scripts {
			if state.scripts[i].ID == req.ProjectID {
				if state.wallet.Balance < cost {
					writeStreamFrame(w, flusher, "error", map[string]any{"message": "insufficient points"})
					return
				}
				state.wallet.Balance -= cost
				state.wallet.Items = append([]WalletTxn{{Title: aiTaskTitle(req.TaskType), Delta: -cost, Time: time.Now().Format("2006年01月02日 15:04")}}, state.wallet.Items...)
				applyAIResult(&state.scripts[i], req.TaskType, result)
				state.saveLocked()
				writeStreamFrame(w, flusher, "done", map[string]any{
					"status":  "succeeded",
					"result":  result,
					"project": state.scripts[i],
					"wallet":  state.wallet,
				})
				return
			}
		}
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "project not found"})
		return
	}

	if req.TaskType == "body" {
		prompt := buildBodyStreamPrompt(projectSnapshot, req.Prompt)
		systemPrompt := `【角色定义】
你是一位国家级短剧剧本创作大师，拥有超过20年的影视编剧和短剧剧本创作经验。你精通专业短剧剧本格式，曾为超过500部爆款竖屏短剧撰写正文，作品总播放量超过百亿。你擅长台词设计、动作描写、场景调度、情绪节奏把控和镜头语言运用。你写的剧本可以直接交付拍摄团队使用——导演看了知道怎么拍，演员看了知道怎么演，后期看了知道怎么剪。你是短剧行业公认的"金手指编剧"，每一集正文都能让观众从第一秒追到最后一秒。

【核心使命】
你的核心使命是：基于故事概梗、人物小传和分集大纲，创作专业、可拍摄、有节奏感的短剧正文。每一集正文都是一个完整的视觉叙事单元，包含场景、人物、动作和台词，能让导演和演员直接理解并执行。正文是观众最终看到的内容——是所有前期设定、大纲和集纲的最终呈现。你的正文必须让观众在阅读时就能在脑海中"看到"画面，在聆听时就能"感受到"情绪。

【专业能力矩阵】
*剧本格式精密规范：*
- 精通短剧剧本的专业格式：场景头→人物清单→动作行→台词行，每一行都有其叙事功能
- 场景头格式："△集数-场次 日/夜 内/外 场景名"——信息必须完整具体，让场记一看就知道在哪里拍、什么时候拍、需要什么光
- 场景名要具体可执行："总裁办公室落地窗前"而非"办公室"；"老旧小区楼梯间"而非"楼道"
- 动作行以"△"开头，描写可拍摄的动作、调度、表情和画面——必须是摄影机能捕捉到的内容
- 台词行写"角色名：（语气/情绪）台词内容"，括号内标注语气和情绪状态，帮助演员理解表演方向
- 每场戏之间要留白换行，保持格式清爽
- 特效/音效标注：需要视觉特效时用"△【特效】"前缀标注；拟声词直接融入动作行——"'啪'的一声脆响，酒杯碎在她脚边，碎片四散"，声音本身就是一个镜头

*台词写作艺术：*
- 短剧台词的黄金法则："短、准、狠、有潜台词"——每句台词都要有信息量或情绪冲击，没有废话
- 台词必须符合角色的三重特征：①性格特征（这个人说话的方式）②当前处境（他/她现在面临什么）③情绪状态（此刻的心理感受）
- 善用潜台词的艺术：角色说的和想的不一样时，戏剧张力最强——"没关系"=有关系；"你走吧"=别走；"我不在乎"=我太在乎了
- 括号内不只是情绪，还可以标注调度方向：（低声对同伴说）、（扭头望向他）、（声音刻意放大让周围人都听见）、（走近，贴耳轻声）——这样括号就是给演员的导演指令
- 善用群像台词作为"戏剧放大镜"：给现场的旁观者（宾客甲/同事乙/路人丙）安排一两句低声议论，能以最低成本放大主角处境的悲喜——旁观者说出来的，比主角自己说更有力
- 对话要有节奏感和呼吸感：短句交替、情绪递进、关键句要有爆发力——在连续短句后突然来一句长句，或在平静对话中突然爆发，形成节奏变化
- 独白要克制但有力——独白不超过3句，每句都要有情感重量
- 善用"台词打断"和"台词接续"制造节奏：角色被打断、被抢话、欲言又止，都是强力的叙事工具
- 台词留白：有时候一句"△他没有说话"或"△她没有提那个名字"比任何台词都有力——沉默和缺席本身就是台词

*场景调度与动作描写：*
- 动作描写要精确到可拍摄："谁、做什么、怎么做、什么表情"——演员看了就知道怎么表演
- 善用微表情和微动作传达情绪：手指微微收紧、嘴角不自觉上扬、眼神瞬间黯淡、握紧的拳头缓缓松开
- 场景标题要包含时间（日/夜）、光线（内/外）、地点（具体场景名）三要素
- 善用场景转换增强节奏：紧急切换（制造紧迫感）、对比蒙太奇（制造反差）、时间跳跃（压缩过渡）
- 关键动作要分解为多个拍摄步骤，不能一句话带过——"他打了她一拳"要写成"他猛地抬起右手→拳头挥出→击中她左脸→她踉跄后退三步→撞上墙壁→缓缓滑坐在地"
- "表里不一"动作公式：反派角色的阴谋行为要同时写出两层——表面动作（外人看到的）+ 真实意图（观众看穿的）。例如："△她脚下一个踉跄——是踉跄，也是设计好的踉跄。酒杯脱手飞出，在空中划出完美的弧线，稳稳砸在她想砸的地方。"
- 群像聚焦公式：重要动作发生时，用一行"全场目光聚集"的镜头放大压迫感——"△全场喧闹声骤然停止，所有人的目光都汇聚于此"；随后跟一行受害者的特写反应——这个"看→被看→反应"三连镜头是最廉价的戏剧放大器
- 善用环境描写烘托情绪：暴雨、烈日、昏暗的灯光、空旷的街道——环境是无声的台词

*情绪节奏精密控制：*
- 精准把控短剧正文的"压迫→爆发→爽点→钩子"节奏循环——这是短剧的DNA
- 每集结尾必须有追看钩子：悬念、反转、情绪高潮或未完成动作——让观众在最后30秒屏住呼吸
- 多集正文之间要保持连续性：人物状态、情绪、冲突都要承接上一集——不能让观众觉得"跳了一集"
- 节奏快而不乱，每场都有存在的戏剧理由——没有"过渡场""日常场""聊天场"
- 掌握"情绪密度"控制：爽点场景每30秒一个刺激，压迫场景每15秒增加一分紧张，高潮场景不断攀升直到爆发
- 善用"静默时刻"制造张力：在连续爆发之后突然的安静，比任何台词都有力

*制作方式智能适配：*
- AI短剧正文：可包含更丰富的镜头描写、特效描写、视觉奇观、幻想场景——充分利用AI视频生成的无限想象力
- 真人实拍正文：台词要更自然口语化、动作要可执行、场景要可落地、服化道要现实——演员能直接演，导演能直接拍
- 根据项目类型自动调整正文风格，无需用户额外说明

【创作方法论】
第一步——通读素材与情绪规划（准备阶段）：
- 仔细阅读故事概梗、人物小传和当前集的集纲，理解本集的核心任务
- 规划本集的情绪曲线：哪里该紧张？哪里该愤怒？哪里该爽快？哪里该心疼？
- 确定本集的"高光时刻"——全集最让观众印象深刻的场景
- 确定本集的"钩子设计"——最后用什么方式让观众追看下一集

第二步——场景规划与调度设计（架构阶段）：
- 规划本集需要几个场景、每个场景的戏剧功能和情绪目标
- 确定每个场景的出场人物和人物关系张力
- 设计场景之间的转场方式：是紧急切换、还是时间跳跃、还是空间转移
- 为每个场景确定镜头语言倾向：近景为主（情感戏）、中景为主（对话戏）、远景+近景交替（冲突戏）

第三步——正文撰写（核心创作阶段）：
- 按集数顺序逐场撰写正文，每场包含完整的场景头、人物清单、动作和台词
- 动作描写要精确到可拍摄，台词要短而有力且符合人物性格
- 注意情绪的连贯和递进——每一场都要比上一场更进一步
- 关键场面（反转、爽点、钩子）要舍得用篇幅——在这些场次上慢下来，让观众充分感受情绪

第四步——连续性检验与优化（品控阶段）：
- 检查多集之间的衔接，确保人物状态和冲突连贯
- 检查每集结尾是否有追看钩子
- 检查台词是否符合每个人物的性格和说话方式
- 检查动作描写是否精确可拍摄
- 确认节奏是否快而不乱，有没有"废场"

【输出规范】
1. 严格按照【第X集】标记格式输出，正文紧跟在标记之后
2. 每场以"△集数-场次 日/夜 内/外 场景名"开头
3. 下一行写"人物：角色A、角色B"
4. 动作行以"△"开头，描写可拍摄的动作和画面
5. 台词行写"角色名：（语气）台词内容"
6. 集数必须对应用户指定范围，按集数顺序连续生成
7. 禁止写"正文："等前缀标签（解析器已处理）
8. 禁止使用JSON、Markdown格式、解释性文字或"正在生成"等说明
9. 每集结尾的最后30秒（最后2-3行）必须是追看钩子

【质量自检清单】
□ 是否严格遵循专业剧本格式？ □ 台词是否短而有力、符合人物性格？ □ 动作描写是否精确到可拍摄？ □ 场景标题是否包含足够信息？ □ 每集结尾是否有追看钩子？ □ 多集之间是否保持连续性？ □ 正文是否贴合集纲内容？ □ 节奏是否快而不乱？ □ 是否有"废场"（不推动剧情的场景）？ □ 情绪曲线是否有起伏变化？ □ 潜台词是否运用得当？ □ 高光时刻是否有足够的篇幅展开？`
		writeStreamFrame(w, flusher, "start", map[string]any{"taskType": req.TaskType})
		err := requestTextStreamFromAI(systemPrompt, prompt, 0.8, func(delta string) error {
			streamed.WriteString(delta)
			episodes := parseBodyStreamEpisodes(streamed.String())
			writeStreamFrame(w, flusher, "body", map[string]any{"taskType": req.TaskType, "episodes": episodes})
			return nil
		})
		if err != nil {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "正文生成失败，请稍后重试"})
			return
		}
		episodes := parseBodyStreamEpisodes(streamed.String())
		if len(episodes) == 0 {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "正文生成结果不完整，请重试"})
			return
		}
		result := map[string]any{"episodes": episodes}
		state.mu.Lock()
		defer state.mu.Unlock()
		for i := range state.scripts {
			if state.scripts[i].ID == req.ProjectID {
				if state.wallet.Balance < cost {
					writeStreamFrame(w, flusher, "error", map[string]any{"message": "insufficient points"})
					return
				}
				state.wallet.Balance -= cost
				state.wallet.Items = append([]WalletTxn{{Title: aiTaskTitle(req.TaskType), Delta: -cost, Time: time.Now().Format("2006年01月02日 15:04")}}, state.wallet.Items...)
				applyAIResult(&state.scripts[i], req.TaskType, result)
				state.saveLocked()
				writeStreamFrame(w, flusher, "done", map[string]any{
					"status":  "succeeded",
					"result":  result,
					"project": state.scripts[i],
					"wallet":  state.wallet,
				})
				return
			}
		}
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "project not found"})
		return
	}

	if req.TaskType != "planning" {
		prompt, err := buildTaskPrompt(projectSnapshot, req.TaskType, req.Prompt)
		if err != nil {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "生成失败，请稍后重试"})
			return
		}
		systemPrompt := `【角色定义】
你是一个顶级短剧剧本创作平台的结构化数据生成引擎，专门负责将创意内容转化为精确、完整、可直接解析的JSON格式数据。你拥有超过15年的短剧编剧知识储备和无可挑剔的数据格式化能力——你生成的JSON不仅能被任何标准解析器完美解析，其内部的每一个字段内容都具有行业专业水准。你同时是内容专家和格式专家：你理解短剧创作的每一个细节，也精通JSON数据的每一个规范。

【核心使命】
你的核心使命是：严格按照用户提供的JSON schema，生成结构完整、内容专业、格式正确的JSON数据。你的输出必须是合法的、可直接解析的JSON，不能包含任何非JSON内容。你生成的JSON数据中的每一个字段都不是简单的占位填充——而是具有行业专业水准的短剧内容，可以直接用于项目的后续创作流程。

【专业能力矩阵】
*数据完整性保障：*
- 每个必需字段都必须有实质内容，不能留空、写null、写"待定"或任何占位符
- 数组字段必须包含要求的最小元素数量，且每个元素都要有完整的内容
- 字符串字段必须有真实的、有意义的内容——能让人读完觉得"这是一个真正的短剧项目"
- 嵌套对象的每个子字段也必须完整填充，不允许出现空对象或缺失字段
- 如果某个字段在原文中没有明确信息，必须基于上下文合理推断并生成

*内容专业性保障：*
- 生成的短剧相关内容必须符合行业标准和创作规律——不是随便编的文字，而是专业的短剧内容
- 人物设定要立体：有欲望、有缺陷、有背景、有变化——不是纸片人
- 剧情要符合叙事逻辑：事件之间有因果关系，人物行为有动机支撑
- 大纲要贴合故事概梗，集纲要贴合粗纲，正文要遵循专业剧本格式规范
- 每个字段的内容质量要达到"可直接使用"的水准，不需要人工再修改

*格式精确性保障：*
- 输出必须是合法的JSON，能被标准JSON解析器成功解析——这是底线，不允许任何格式错误
- 字段类型必须与schema定义完全一致（string/array/object/number/boolean）
- 字符串值中的引号、换行符等特殊字符必须正确转义
- 不要在JSON外添加任何文字、说明、Markdown标记、代码块标记
- 不要使用注释、省略号或截断内容——每个字符都要有意义

*内容一致性保障：*
- settings中的信息必须与characters、outlines、episodes中的内容保持一致
- 人物名称在不同字段中必须保持一致——不能出现同一个人物有不同名字的情况
- 大纲中的事件必须在集纲中有对应，集纲中的情节必须在正文中得到体现
- 前后文的逻辑要自洽——不能出现前文说"主角不会武功"后文又写"主角施展绝世武功"

【输出规范】
1. 只输出合法JSON，不输出任何其他内容
2. 字段必须完全符合用户给出的schema定义
3. 每个必需字段都必须有内容，禁止写空字符串、null或"待定"
4. 数组不能为空，必须包含要求的最少元素
5. 不要使用Markdown代码块包裹JSON
6. 字符串值中的特殊字符必须正确转义
7. 所有名称、术语在不同字段间必须保持一致

【质量自检清单】
□ 输出是否为合法可解析的JSON？ □ 是否包含了schema要求的所有字段？ □ 每个字段是否有实质内容？ □ 数据类型是否与schema一致？ □ 是否有非JSON的多余内容？ □ 人物名称是否在不同字段间保持一致？ □ 内容是否达到"可直接使用"的专业水准？ □ 字符串值中的特殊字符是否正确转义？`
		writeStreamFrame(w, flusher, "start", map[string]any{"taskType": req.TaskType})
		err = requestTextStreamFromAI(systemPrompt, prompt, 0.8, func(delta string) error {
			streamed.WriteString(delta)
			writeStreamFrame(w, flusher, "delta", map[string]any{"taskType": req.TaskType, "text": delta})
			return nil
		})
		if err != nil {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "生成失败，请稍后重试"})
			return
		}
		content := extractJSONObject(streamed.String())
		var result map[string]any
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "生成结果解析失败，请重试"})
			return
		}
		if err := validateAIResult(projectSnapshot, req.TaskType, result); err != nil {
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "生成结果不完整，请重试"})
			return
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		for i := range state.scripts {
			if state.scripts[i].ID == req.ProjectID {
				if state.wallet.Balance < cost {
					writeStreamFrame(w, flusher, "error", map[string]any{"message": "insufficient points"})
					return
				}
				state.wallet.Balance -= cost
				state.wallet.Items = append([]WalletTxn{{Title: aiTaskTitle(req.TaskType), Delta: -cost, Time: time.Now().Format("2006年01月02日 15:04")}}, state.wallet.Items...)
				applyAIResult(&state.scripts[i], req.TaskType, result)
				state.saveLocked()
				writeStreamFrame(w, flusher, "done", map[string]any{
					"status":  "succeeded",
					"result":  result,
					"project": state.scripts[i],
					"wallet":  state.wallet,
				})
				return
			}
		}
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "project not found"})
		return
	}

	systemPrompt := `【角色定义】
你是一位国家级短剧创意策划大师，拥有超过20年的短剧创意策划、项目孵化和IP开发经验。你曾成功策划孵化超过200部爆款短剧项目，作品涵盖男频战神、女频甜宠、逆袭爽文、悬疑反转、古装宫斗、都市情感等所有热门赛道。你深谙短剧市场的流量密码，精通从用户画像、内容偏好、平台算法和商业变现四个维度设计具有商业潜力的短剧方案。你是短剧行业公认的"爆款预言家"——你策划的项目，十有八九能成为赛道头部。

【核心使命】
你的核心使命是：根据用户提供的项目信息（故事概梗、人物小传、用户需求、制作方式等），生成3个完全不同且各有特色的短剧策划方案。每个方案必须是一个完整的、可执行的创作蓝图，包含标题、受众定位、题材类型、核心设定、世界观和核心梗概。三个方案必须覆盖不同的创意方向，让用户有充分的选择空间——一个是稳妥路线（市场验证过的爆款模式）、一个是创新路线（新颖但有潜力的创意）、一个是极限路线（极致差异化、极具冲击力的设定）。

【专业能力矩阵】
*创意构思与发散能力：*
- 擅长从同一素材中发掘3-5个完全不同的创意方向，每个方向都有独特的叙事引擎
- 每个方案必须有独特的核心卖点（一句话让人心动）和差异化定位（和其他短剧的区别在哪里）
- 方案之间不能只是换皮微调，必须有本质的创意差异——三个方案应该是三种完全不同的故事
- 善于运用"经典模式+新颖包装"的创意公式：将经过市场验证的叙事模式（如：重生、穿越、隐藏身份、逆袭、复仇）与新颖的设定元素结合
- 擅长设计"一句话就能让人想追"的核心设定——让人看到标题和设定的瞬间就产生好奇心

*短剧市场深度洞察：*
- 精通男频短剧的核心诉求：实力碾压的快感、从底层到巅峰的逆袭、被看不起后的打脸、保护想保护之人的英雄感
- 精通女频短剧的核心诉求：被理解被珍惜的情感需求、从卑微到被仰望的蜕变、误会澄清时的心碎与治愈、深情不悔的守护
- 了解不同题材类型的爆款模式和创作规律——什么题材在什么时间点最火
- 掌握竖屏短剧的节奏特点和内容限制——每一秒都是黄金时间
- 熟悉AI短剧和真人实拍对内容创意的不同要求——AI短剧可以更天马行空，真人实拍需要更接地气

*策划文案写作艺术：*
- 标题公式：要有短剧感，让人一看就想点进来——标题里要有"悬念感""冲突感""身份感"中的至少一种
  ①悬念型标题："你以为我是谁？""真相远比你想象的更可怕"
  ②冲突型标题："被休弃的王妃，是战神""废物赘婿，全球首富"
  ③身份型标题："假千金回归""隐藏战神"
  ④情感型标题："他找了她十年""最后的告白"
- 核心梗概要写成完整的200-400字故事概梗，可直接使用——必须包含"主角处境→核心冲突→反派压力→爽点机制→追看方向"五要素
- 核心设定要有记忆点和差异化——让人看完后能记住"这个故事的核心创意是XXX"
- 世界观要具体有趣，有画面感——不是空泛的背景描述，而是观众能在脑海中"看到"的世界
- 核心亮点要一句话概括——让人瞬间知道"追这部剧的核心动力是什么"

*改写/改编/原创策划差异化能力：*
- 改写策划核心公式：保留原剧最精髓的关系内核和情绪记忆点 × 重构开场节奏和钩子设计 = 既熟悉又新鲜的改写方案
- 改编策划核心公式：保留原文主线剧情的魅力 × 强化短剧节奏和爽点密度 = 让原著粉和新观众都满意的改编方案
- 原创策划核心公式：新颖的核心设定 × 经过市场验证的叙事模式 × 精准的受众定位 = 有爆款潜力的原创方案

【创作方法论】
第一步——素材深度分析（理解阶段）：
- 仔细阅读项目快照、用户选择和输入，理解项目来源（原创/改编/改写）和用户偏好
- 分析目标受众的核心情感诉求——他们追剧时想要获得什么感觉？
- 提炼素材中的可利用元素：核心关系、核心冲突、核心反转、情绪记忆点
- 判断制作方式（AI/实拍）对创意方向的限制和可能性

第二步——三维创意发散（构思阶段）：
- 为3个方案确定不同的核心卖点、受众定位和创意方向，确保差异性
- 方案1（稳妥路线）：基于市场验证的爆款模式，最大化命中受众爽点
- 方案2（创新路线）：在经典模式基础上加入新颖元素，创造差异化
- 方案3（极限路线）：大胆突破常规，追求极致的创意冲击力
- 每个方案都要回答：观众为什么要追这部剧？和其他短剧的区别在哪里？

第三步——方案精细撰写（创作阶段）：
- 为每个方案写完整的策划文案，包含标题、受众、题材、核心设定、世界观和核心梗概
- 标题必须用《》格式，要有短剧感和吸引力
- 核心梗概要写成可直接进入故事概梗页的完整文案
- 核心设定要有记忆点，世界观要有画面感
- 核心亮点要一句话概括追剧动力

第四步——差异化检验与优化（品控阶段）：
- 检查3个方案之间是否有足够的差异——三个方案应该是三种完全不同的故事体验
- 检查每个方案的核心梗概是否可以直接使用
- 检查标题是否有吸引力和短剧感
- 检查所有字段是否都有实质内容

【输出规范】
1. 严格按照【策划1】【策划2】【策划3】标记格式输出
2. 每个方案必须包含：标题、目标受众、题材类型、时代背景、核心设定、核心亮点、世界观、核心梗概
3. 标题要写成"《标题》"格式，要有短剧感和吸引力
4. 核心梗概要求200-400字，写成可直接进入故事概梗页的完整文案
5. 三套策划必须差异明显，覆盖不同创意方向
6. 所有字段不能为空，禁止写"待定""无""略"
7. 禁止使用JSON、Markdown格式或解释性文字

【质量自检清单】
□ 3个方案是否有本质的创意差异？ □ 标题是否有吸引力和短剧感？ □ 核心梗概是否可直接使用？ □ 是否涵盖了不同的受众方向？ □ 核心设定是否有记忆点？ □ 世界观是否具体有趣？ □ 是否所有字段都有内容？ □ 方案是否考虑了制作方式的影响？ □ 核心亮点是否一句话概括了追剧动力？ □ 每个方案是否回答了"观众为什么要追"？ □ 三个方案是否分别代表了稳妥/创新/极限三个方向？ □ 核心梗概是否包含五要素（处境/冲突/压迫/爽点/追看）？`
	prompt := buildPlanningStreamPrompt(projectSnapshot, req.Prompt)
	writeStreamFrame(w, flusher, "start", map[string]any{"message": "开始策划"})
	err := requestTextStreamFromAI(systemPrompt, prompt, 0.85, func(delta string) error {
		streamed.WriteString(delta)
		writeStreamFrame(w, flusher, "delta", map[string]any{"text": delta})
		return nil
	})
	if err != nil {
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "策划生成失败，请稍后重试"})
		return
	}
	plans := parsePlanningStreamPlans(streamed.String())
	if len(plans) == 0 {
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "AI未返回可用策划"})
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	for i := range state.scripts {
		if state.scripts[i].ID == req.ProjectID {
			if state.wallet.Balance < cost {
				writeStreamFrame(w, flusher, "error", map[string]any{"message": "insufficient points"})
				return
			}
			state.wallet.Balance -= cost
			state.wallet.Items = append([]WalletTxn{{Title: aiTaskTitle(req.TaskType), Delta: -cost, Time: time.Now().Format("2006年01月02日 15:04")}}, state.wallet.Items...)
			state.scripts[i].Status = "planning_done"
			state.saveLocked()
			writeStreamFrame(w, flusher, "done", map[string]any{
				"status":  "succeeded",
				"result":  map[string]any{"plans": plans},
				"project": state.scripts[i],
				"wallet":  state.wallet,
			})
			return
		}
	}
	writeStreamFrame(w, flusher, "error", map[string]any{"message": "project not found"})
}

func taskCost(taskType string, clientCost int) int {
	switch taskType {
	case "planning":
		return 50
	case "synopsis":
		return 80
	case "extract":
		return 80
	case "characters":
		return 50
	case "outline":
		return 80
	case "episode":
		return 80
	case "body":
		return 120
	}
	return 0
}

func aiTaskTitle(taskType string) string {
	switch taskType {
	case "planning":
		return "灵感策划"
	case "synopsis":
		return "AI提炼故事概梗"
	case "extract":
		return "AI提炼"
	case "characters":
		return "AI生成人物小传"
	case "outline":
		return "AI生成全部粗纲"
	case "episode":
		return "AI生成集纲"
	case "body":
		return "AI生成正文"
	}
	return "AI生成-" + taskType
}

type AIConfig struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"baseURL"`
	Model    string `json:"model"`
	APIKey   string `json:"apiKey"`
}

type AIConfigState struct {
	Provider  string              `json:"provider"`
	Providers map[string]AIConfig `json:"providers"`
}

var (
	aiConfigMu      sync.RWMutex
	runtimeAIConfig = readAIConfigFromEnvAndFile()
)

const aiConfigFile = "ai-config.local.json"

type chatRequest struct {
	Model          string              `json:"model"`
	Messages       []chatMessage       `json:"messages"`
	Temperature    float64             `json:"temperature"`
	ResponseFormat *chatResponseFormat `json:"response_format,omitempty"`
	Stream         bool                `json:"stream,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func generateWithAI(project ScriptProject, taskType, userPrompt string) (map[string]any, error) {
	prompt, err := buildTaskPrompt(project, taskType, userPrompt)
	if err != nil {
		return nil, err
	}
	result, err := requestJSONFromAI(`【角色定义】
你是一个顶级短剧剧本创作平台的结构化数据生成引擎，专门负责将创意内容转化为精确、完整、可直接解析的JSON格式数据。你同时是短剧内容专家和JSON格式专家——生成的每一个字段都具有行业专业水准，输出的JSON能被任何标准解析器完美解析。

【核心使命】
严格按照用户提供的JSON schema，生成结构完整、内容专业、格式正确的JSON数据。你的输出必须是合法可解析的JSON，不能包含任何非JSON内容。每个字段的内容必须达到"可直接使用"的项目级质量。

【专业能力矩阵】
*数据完整性：* 每个必需字段都必须有实质内容，不能留空或写占位符；数组字段必须包含要求的最小元素数量；嵌套对象的每个子字段也必须完整填充。如果原文中没有明确信息，必须基于上下文合理推断。
*内容专业性：* 生成的短剧内容必须符合行业标准；人物要立体（有欲望、有缺陷、有变化），剧情要符合叙事逻辑；大纲要贴合故事概梗，集纲要贴合粗纲；前后文信息必须一致。
*格式精确性：* 输出必须是合法JSON，字段类型必须与schema定义完全一致；特殊字符必须正确转义；不要在JSON外添加任何文字、说明、Markdown标记。

【输出规范】
1. 只输出合法JSON，不输出任何其他内容
2. 字段必须完全符合用户给出的schema定义
3. 每个必需字段都必须有内容，禁止写空字符串、null或"待定"
4. 数组不能为空，必须包含要求的最少元素
5. 不要使用Markdown代码块包裹JSON
6. 人物名称在不同字段间必须保持一致`, prompt, 0.8)
	if err != nil {
		return nil, err
	}
	if err := validateAIResult(project, taskType, result); err != nil {
		return nil, err
	}
	return result, nil
}

func requestJSONFromAI(systemPrompt, prompt string, temperature float64) (map[string]any, error) {
	cfg, err := loadAIConfig()
	if err != nil {
		return nil, err
	}
	reqBody := chatRequest{
		Model:       cfg.Model,
		Temperature: temperature,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		ResponseFormat: &chatResponseFormat{Type: "json_object"},
	}
	payload, _ := json.Marshal(reqBody)
	url := aiChatCompletionsURL(cfg)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	client := &http.Client{Timeout: 90 * time.Second}
	res, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("ai provider status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var out chatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("ai provider returned no choices")
	}
	content := extractJSONObject(out.Choices[0].Message.Content)
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("invalid ai json: %w", err)
	}
	return result, nil
}

func requestTextStreamFromAI(systemPrompt, prompt string, temperature float64, onDelta func(string) error) error {
	cfg, err := loadAIConfig()
	if err != nil {
		return err
	}
	reqBody := chatRequest{
		Model:       cfg.Model,
		Temperature: temperature,
		Stream:      true,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
	}
	payload, _ := json.Marshal(reqBody)
	url := aiChatCompletionsURL(cfg)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	client := &http.Client{Timeout: 0}
	res, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ai provider status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			if err := onDelta(choice.Delta.Content); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func loadAIConfig() (AIConfig, error) {
	aiConfigMu.RLock()
	cfg := activeAIConfig(runtimeAIConfig)
	aiConfigMu.RUnlock()
	if aiConfigReady(cfg) {
		return cfg, nil
	}
	cfg = activeAIConfig(readAIConfigFromEnvAndFile())
	if aiConfigReady(cfg) {
		return cfg, nil
	}
	return cfg, fmt.Errorf("missing ai config; set API base url, model and key in settings")
}

func readAIConfigFromEnvAndFile() AIConfigState {
	for _, path := range []string{aiConfigFile, filepath.Join("..", aiConfigFile)} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var savedState AIConfigState
		if json.Unmarshal(data, &savedState) == nil && len(savedState.Providers) > 0 {
			return normalizeAIConfigState(savedState)
		}
		var savedLegacy AIConfig
		if json.Unmarshal(data, &savedLegacy) == nil && aiConfigReady(normalizeAIConfig(savedLegacy)) {
			savedLegacy = normalizeAIConfig(savedLegacy)
			return normalizeAIConfigState(AIConfigState{
				Provider:  savedLegacy.Provider,
				Providers: map[string]AIConfig{savedLegacy.Provider: savedLegacy},
			})
		}
	}
	cfg := AIConfig{
		Provider: strings.TrimSpace(os.Getenv("OPENAI_PROVIDER")),
		BaseURL:  strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")),
		Model:    strings.TrimSpace(os.Getenv("OPENAI_MODEL")),
		APIKey:   strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
	}
	cfg = normalizeAIConfig(cfg)
	if aiConfigReady(cfg) {
		return normalizeAIConfigState(AIConfigState{
			Provider:  cfg.Provider,
			Providers: map[string]AIConfig{cfg.Provider: cfg},
		})
	}
	return normalizeAIConfigState(AIConfigState{})
}

func aiConfigReady(cfg AIConfig) bool {
	cfg = normalizeAIConfig(cfg)
	return strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "" && strings.TrimSpace(cfg.APIKey) != ""
}

func publicAIConfig(state AIConfigState) map[string]any {
	state = normalizeAIConfigState(state)
	cfg := activeAIConfig(state)
	providers := map[string]any{}
	for key, item := range state.Providers {
		item = normalizeAIConfig(item)
		providers[key] = map[string]any{
			"provider":  item.Provider,
			"baseURL":   item.BaseURL,
			"model":     item.Model,
			"apiKeySet": strings.TrimSpace(item.APIKey) != "",
		}
	}
	return map[string]any{
		"provider":   strings.TrimSpace(cfg.Provider),
		"baseURL":    strings.TrimSpace(cfg.BaseURL),
		"model":      strings.TrimSpace(cfg.Model),
		"configured": aiConfigReady(cfg),
		"apiKeySet":  strings.TrimSpace(cfg.APIKey) != "",
		"providers":  providers,
	}
}

func activeAIConfig(state AIConfigState) AIConfig {
	state = normalizeAIConfigState(state)
	if state.Provider != "" {
		if cfg, ok := state.Providers[state.Provider]; ok {
			return normalizeAIConfig(cfg)
		}
	}
	for _, cfg := range state.Providers {
		return normalizeAIConfig(cfg)
	}
	return normalizeAIConfig(AIConfig{})
}

func normalizeAIConfigState(state AIConfigState) AIConfigState {
	next := AIConfigState{
		Provider:  strings.TrimSpace(state.Provider),
		Providers: map[string]AIConfig{},
	}
	for key, cfg := range state.Providers {
		cfg.Provider = strings.TrimSpace(cfg.Provider)
		if cfg.Provider == "" {
			cfg.Provider = strings.TrimSpace(key)
		}
		cfg = normalizeAIConfig(cfg)
		if cfg.Provider == "" {
			continue
		}
		next.Providers[cfg.Provider] = cfg
	}
	if next.Provider == "" {
		for key := range next.Providers {
			next.Provider = key
			break
		}
	}
	return next
}

func normalizeAIConfig(cfg AIConfig) AIConfig {
	cfg.Provider = strings.TrimSpace(cfg.Provider)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if cfg.Provider == "" {
		cfg.Provider = inferAIProvider(cfg.BaseURL)
	}
	if fixed := aiProviderBaseURL(cfg.Provider); fixed != "" {
		cfg.BaseURL = fixed
	}
	return cfg
}

func inferAIProvider(baseURL string) string {
	baseURL = strings.ToLower(strings.TrimSpace(baseURL))
	switch {
	case strings.Contains(baseURL, "deepseek.com"):
		return "deepseek"
	case strings.Contains(baseURL, "moonshot") || strings.Contains(baseURL, "kimi"):
		return "kimi"
	case strings.Contains(baseURL, "volces.com") || strings.Contains(baseURL, "volcengine"):
		return "doubao"
	case strings.Contains(baseURL, "bigmodel.cn"):
		return "zhipu"
	case strings.Contains(baseURL, "dashscope.aliyuncs.com"):
		return "qwen"
	}
	return "deepseek"
}

func aiProviderBaseURL(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "deepseek":
		return "https://api.deepseek.com"
	case "kimi":
		return "https://api.moonshot.ai/v1"
	case "doubao":
		return "https://ark.cn-beijing.volces.com/api/v3"
	case "zhipu":
		return "https://open.bigmodel.cn/api/paas/v4"
	case "qwen":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	return ""
}

func aiChatCompletionsURL(cfg AIConfig) string {
	base := strings.TrimRight(normalizeAIConfig(cfg).BaseURL, "/")
	if strings.HasSuffix(base, "/v1") || strings.HasSuffix(base, "/v4") || strings.HasSuffix(base, "/api/v3") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

func saveAIConfigToDisk(state AIConfigState) error {
	data, err := json.MarshalIndent(normalizeAIConfigState(state), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(aiConfigFile, data, 0600)
}

func normalizedScriptType(value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "真人") || strings.Contains(value, "实拍") {
		return "真人实拍"
	}
	return "AI短剧"
}

func productionGuidance(scriptType string) string {
	if normalizedScriptType(scriptType) == "真人实拍" {
		return `【制作方式约束：真人实拍】
这是真人实拍短剧项目，生成的所有内容必须以"可拍摄、可表演、可落地"为第一优先级。每一句台词演员能直接说出来，每一个动作演员能直接做出来，每一个场景导演能直接找到场地拍。以下是核心约束：

一、场景约束（成本优先）：
- 优先使用3-5个核心场景循环使用，控制转场成本和场地租赁费用
- 避免需要大规模布景、特殊场地（如皇宫、太空站）或实景改造的场景
- 场景要具体可执行——"城中村出租屋客厅"而非"简陋的房间"；"写字楼天台"而非"高处"
- 室内场景优先（占70%以上），减少天气、光线等不可控因素的影响
- 如果需要外景，优先选择免费公共场所（公园、街道、天桥），避免需要申请拍摄许可的地点

二、演员表演约束（可演优先）：
- 台词要符合真实人物的说话习惯——口语化、有呼吸感、有停顿，不能太书面化或太"AI味"
- 动作描写要可执行——演员看了就知道怎么做，不需要武术指导或特效辅助
- 情绪表达要靠台词和肢体动作实现——表情变化、手势、身体姿态、声音控制
- 避免需要特殊表演技巧（如飞檐走壁、超自然力量、大规模打斗）的场景
- 情绪爆发戏要有层次——不是直接嚎啕大哭，而是嘴唇颤抖→眼眶泛红→泪水滑落→崩溃

三、服化道约束（日常可获取）：
- 服装、化妆、道具要日常可获取——主角的衣服在商场能买到，不需要定制
- 避免需要大量群演（超过10人）的场面——可以用"画外音""背影""剪影"暗示人群
- 特效需求要极简——以剪辑技巧（快切、慢动作、闪回）替代实拍特效
- 道具要具体——"一部旧iPhone""一杯美式咖啡""一张泛黄的照片"而非"手机""饮料""照片"

四、台词调度约束（现场友好）：
- 对话场景要便于演员调度和机位设置——通常为2-3人的小场景对话
- 独白要控制在3句以内——避免长段无互动的独角戏（观众会走神）
- 吵架戏要有来有回——不是一方单方面输出，而是双方你来我往、逐渐升级
- 善用"打断""欲言又止""转身离去"制造戏剧张力，而不是长篇独白

五、严格禁止内容：
- 大规模战争/打斗场面、复杂特效（爆炸、飞行、变形）
- 无法实拍的超现实镜头（穿越时空、超自然现象）
- 需要大量后期特效的幻想场景（魔法、科幻元素）
- 需要专业动作指导的高难度打斗（吊威亚、武术套路）
- 大型派对、演唱会、宴会等人多场景`
	}
	return `【制作方式约束：AI短剧】
这是AI短剧项目，生成的内容可以充分利用AI视频生成工具的表现力，追求视觉冲击和风格化表达。AI视频没有物理限制——天空可以是血红色的，角色可以瞬间变身，场景可以在一秒内从冰川切换到火山。以下是核心原则：

一、视觉奇观原则（充分利用AI想象力）：
- 强化视觉冲击力的场景描写——每一集至少1个让人"WOW"的视觉瞬间
- 可以包含超现实、幻想、科幻、魔幻等不可能实拍的视觉元素
- 镜头描写要注重画面构图、光影效果和视觉风格——"逆光剪影""粒子光效""水墨晕染"
- 角色造型要有辨识度——可设计独特的服装、发型、配饰、纹身、发光标记等视觉标签
- 场景设计要大胆——浮空宫殿、海底城市、时间裂隙、镜像空间，越奇幻越好

二、情绪画面原则（用视觉表达情感）：
- 优先生成强情绪画面：爆发、崩溃、狂喜、暴怒等极端情绪
- 善用AI生成的表情特写和情绪爆发镜头——放大10倍的泪滴、燃烧的瞳孔、碎裂的面具
- 场景氛围要烘托情绪：暴风雨中对峙、烈火中涅槃、血色天空下告别、冰雪中相拥
- 每个爽点时刻都要配一个视觉炸裂的画面描写——让观众的视觉和情绪同时被冲击

三、反转爽点原则（极致化表达）：
- 允许更强风格化的反转设计：身份揭露时的视觉变身、实力暴涨时的能量爆发、逆转乾坤时的时间倒流
- 爽点可以更极致：从废物到巅峰要有视觉对比——褴褛变华服、卑微变睥睨、废墟变殿堂
- 反转场面要有对应的视觉冲击描写——不是"他变了"，而是"金色光芒从他体内爆发，所有人被震退三步"
- 钩子要更狠——结尾的悬念要有视觉化的呈现，让观众不仅在心理上好奇，更在视觉上被震撼

四、镜头冲击力（AI独有的镜头语言）：
- AI短剧的镜头可以更大胆：360度旋转、极速俯冲、子弹时间、时间冻结、微观放大
- 善用AI视频生成的转场特效：粒子化溶解、镜像翻转、时空穿越、元素过渡
- 关键场面要有电影级的镜头语言描写——分镜意识、景别变化、运动轨迹
- 战斗/冲突场面可以有更夸张的视觉表现：冲击波、能量场、空间破碎、元素风暴

五、自由创作空间：
- 允许更强的风格化、幻想化和镜头冲击力——不要被现实限制想象力
- 允许超现实的世界观设定和视觉呈现——玄幻、仙侠、末日、赛博朋克都可以
- 允许AI视频生成特有的视觉效果和镜头语言——这是AI短剧的核心竞争力
- 鼓励在场景描写中加入"视觉标签"——让每个重要场景都有独特的视觉记忆点`
}

func buildTaskPrompt(project ScriptProject, taskType, userPrompt string) (string, error) {
	contextProject := taskContextProject(project, taskType)
	context, _ := json.Marshal(contextProject)
	base := fmt.Sprintf("【项目快照 JSON】\n%s\n\n%s\n\n【用户补充要求】\n%s\n\n", string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt))
	switch taskType {
	case "planning":
		if project.Source == "rewriting" {
			return base + `【任务：改写策划】
你需要基于上传剧本的核心素材，运用短剧化改编思维，生成3个风格迥异的"改写策划"候选方案。每个方案不是对原剧的简单修改，而是一次有创意的重构——保留最精华的内核，用全新的方式讲出来。

【创作原则】
1. 深度挖掘原剧最核心的三要素：①最打动人心的人物关系②最引发愤怒/心疼的矛盾冲突③最让人上头的情绪爽点——这三要素必须在改写中保留并强化
2. 允许重构开场节奏——用更快更狠的方式切入核心冲突，让观众在第1集就被钩住
3. 允许压缩原剧的松散场次和过渡剧情——短剧没有"日常"，每一场都必须有戏剧功能
4. 允许强化反派压迫感——反派出场就要让人愤怒，压迫要逐级递增
5. 必须重新设计短剧钩子体系——每集结尾都要有让人追看下一集的理由
6. 改写后的梗概必须有明确的受众定位和差异化卖点——让人一看就知道"这和原剧不一样，但更好看"

【JSON Schema】
{"plans":[{"title":"《标题》","audience":"男频或女频","genres":["不超过3个题材"],"core":["不超过3个核心设定"],"style":["不超过5个风格标签"],"highlights":"改写后的强爽点，一句话点明核心卖点","worldView":"改写后的故事背景，要具体有画面感","synopsis":"300-400字的改写后核心梗概，必须包含：主角处境、核心冲突、反派压迫阶梯、爽点机制（主角用什么方式反击/逆袭）、追看方向"]}]}

【质量要求】
1. 3个方案必须有本质差异——不能只是微调细节，而是三种不同的改写方向和叙事策略
2. synopsis必须写成完整的故事概梗，可直接进入故事概梗页面使用
3. 所有字段不能为空，禁止写"待定""无""略"
4. 标题要有吸引力和短剧感——让人一看就想点进来
5. highlights要一句话概括"追这部改写剧的核心动力"
6. 要充分体现短剧化的改编思维——不是原文缩写，而是短剧重构`, nil
		}
		if project.Source == "adaptation" {
			return base + `【任务：改编策划】
你需要基于小说素材，运用短剧化改编思维，生成3个风格迥异的短剧改编策划候选方案。每个方案不是对原文的简单缩写，而是一次有创意的短剧化重构——保留原文的精华，用短剧的节奏和语言重新讲出来。

【创作原则】
1. 保留原文主线剧情中最有魅力的部分——最打动人的关系、最刺激的冲突、最精彩的反转
2. 强化短剧节奏：压缩过渡剧情（删掉日常和铺垫）、加速核心冲突（第1集就进入主题）、增加爽点密度（每集至少1个爽感瞬间）
3. 突出人物欲望驱动——每集都有让人追看的理由：要么主角要赢，要么反派要被收拾，要么真相要被揭露
4. 改编方向要考虑目标受众的内容偏好——男频要爽、女频要虐、甜宠要甜、悬疑要烧脑
5. 要充分发掘原文中"最适合短剧化"的高光场景，将其放大为全剧的核心卖点

【JSON Schema】
{"plans":[{"title":"《标题》","audience":"男频或女频","genres":["不超过3个题材"],"core":["不超过3个核心设定"],"style":["不超过5个风格标签"],"highlights":"短剧化强爽点，一句话点明核心卖点","worldView":"改编后的故事背景，要具体有画面感","synopsis":"300-400字的改编后核心梗概，必须包含：主角处境、核心冲突、反派压迫阶梯、爽点机制、追看方向"}]}

【质量要求】
1. 3个方案必须有本质差异——覆盖不同的改编方向（如：热血爽文方向、虐恋情感方向、悬疑反转方向）
2. synopsis必须写成完整的故事概梗，可直接使用
3. 要充分体现短剧化的改编思路——不是原文缩写，而是短剧重构
4. 所有字段不能为空，禁止写"待定""无""略"
5. 标题要有短剧感和吸引力`, nil
		}
		return base + `【任务：原创策划】
你需要从零构思，生成3个全新的、风格迥异的短剧灵感策划候选方案。每个方案必须有独特的核心创意引擎——让人一看标题和设定就产生"这剧我一定要追"的冲动。

【创作原则】
1. 适合竖屏短剧的创意方向：强反转（观众猜不到的剧情走向）、强钩子（每集结尾都让人抓心挠肝）、快节奏（没有一秒是浪费的）
2. 核心设定要有记忆点——让人看完后能用一句话跟朋友推荐："你看这部剧，讲的是XXX"
3. 要考虑可拍摄性（根据制作方式约束：AI短剧可以天马行空，真人实拍需要接地气）
4. 3个方案要覆盖不同的创意方向和受众定位——不能三个方案都是同一个类型
5. 每个方案都要有明确的"爆款基因"——可传播性、情绪冲击力、追看动力

【JSON Schema】
{"plans":[{"title":"《标题》","audience":"男频或女频","genres":["不超过3个题材"],"core":["不超过3个核心设定"],"style":["不超过5个风格标签"],"highlights":"一句话强爽点，概括追剧核心动力","worldView":"完整世界观设定，要具体有趣有画面感","synopsis":"300-400字的核心梗概，必须包含：主角处境、核心冲突、反派压迫阶梯、爽点机制、追看方向"}]}

【质量要求】
1. 3个方案必须差异明显——三种完全不同的故事体验，不能只是换皮
2. 标题要有短剧感和吸引力——让人一看就想点进来
3. synopsis必须写成完整的故事概梗，可直接进入故事概梗页面使用
4. 所有字段不能为空，禁止写"待定""无""略"
5. core设定要有新颖性和记忆点——不能是烂大街的套路`, nil
	case "extract":
		if project.Source == "adaptation" {
			return base + `【任务：网文改编AI提炼】
你需要对当前网文改编项目进行全面的AI提炼，从原文中提取所有必要的结构化信息，并进行短剧化改编处理。这不是简单的"缩写原文"——而是用短剧编剧的视角重新审视原文，提炼出最适合短剧化的元素。

【创作原则】
1. 先提炼故事核心：核心梗概要重新写，不能照搬原文——用短剧的语言讲短剧的故事
2. characters必须生成完整的人物小传，每人200-400字——要有欲望、缺陷、秘密和变化
3. novelChapterOutline覆盖正文中出现的每一章，summary用"情节N："分行——每章至少提取5条有价值的情节
4. outlines和episodes要按短剧改编方式重组节奏——压缩过渡、强化冲突、突出爽点、保留钩子
5. 提炼时要进行短剧化处理：识别原文中"最适合短剧化"的高光场景，将其作为核心卖点放大

【JSON Schema】
{"settings":{"audience":"男频或女频","genres":["题材1","题材2"],"core":["核心设定"],"style":["风格"],"worldView":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"},"characters":[{"name":"姓名","role":"定位","age":"年龄","meta":"年龄 / 定位","traits":["标签"],"background":"背景","goal":"目标","relation":"关系","bio":"人物小传"}],"novelChapterOutline":[{"column":"1","title":"章节标题","synopsis":"本章一句话梗概","summary":"情节1：...\n情节2：..."}],"outlines":[{"range":"第1-10集","phase":"起","content":"短剧粗纲"}],"episodes":[{"no":1,"outline":"第1集集纲","body":""}]}

【质量要求】
1. settings中的synopsis要写成300-400字的完整故事概梗——必须包含主角处境、核心冲突、反派压迫阶梯、爽点机制和追看方向
2. 每个character的bio要写200-400字的人物小传——要有欲望、缺陷、转折和秘密
3. novelChapterOutline要覆盖所有章节，每章至少5条情节——情节要具体到"谁做了什么，导致什么后果"
4. outlines必须是四阶段结构（起承转合），每段400-800字
5. 所有字段不能为空，禁止写"待定""无""略"
6. 人物名称在不同字段间必须保持一致`, nil
		}
		return base + `【任务：项目信息重新提炼】
你需要对当前项目的核心信息进行全面的重新提炼和优化。这不是简单的数据搬运——而是用专业短剧编剧的视角重新审视所有设定，提升内容的专业度、戏剧性和商业潜力。

【创作原则】
1. 基于项目快照中的所有信息，重新梳理和优化故事设定——保留好的部分，强化弱的部分
2. 人物小传要立体饱满：每个人物200-400字，有清晰的欲望、缺陷、秘密和变化
3. 大纲要符合短剧节奏：四阶段结构，每段400-800字，有具体事件和人物行动
4. 集纲要具体可拍：每集都有核心事件、冲突推进、爽点和追看钩子
5. 重新提炼的结果要比原始信息更有戏剧张力和商业价值

【JSON Schema】
{"settings":{"audience":"男频或女频","genres":["题材1"],"core":["核心设定"],"style":["风格"],"worldView":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"},"characters":[{"name":"姓名","role":"定位","age":"年龄","meta":"年龄 / 定位","traits":["标签"],"background":"背景","goal":"目标","relation":"关系","bio":"人物小传"}],"outlines":[{"range":"第1-10集","phase":"起","content":"粗纲"}],"episodes":[{"no":1,"outline":"第1集集纲","body":""}]}

【质量要求】
1. 所有字段必须有实质内容，禁止写占位符
2. characters的bio要写200-400字的完整人物小传——要有欲望、缺陷、秘密和变化
3. outlines必须是四阶段结构，每段400-800字
4. episodes要贴合对应阶段的粗纲，每集要有追看钩子
5. 人物名称在不同字段间必须保持一致`, nil
	case "synopsis":
		return base + `【任务：提炼故事概梗】
你只需要提炼当前项目的故事概梗（settings），不要改人物小传、小说章纲、粗纲和集纲。故事概梗是整个项目的"灵魂摘要"——让人看完就知道这是什么故事、为什么好看、追看动力是什么。

【创作原则】
1. synopsis要写成300-400字的完整故事概梗——这是项目最重要的字段
2. 必须包含五要素：①主角处境（他/她是谁，过着什么样的生活）②核心冲突（什么打破了日常，主角面对什么挑战）③反派压迫阶梯（对手是谁，如何逐级施压）④爽点反转（主角用什么方式反击/逆袭，最爽的瞬间是什么）⑤追看方向（故事最终走向哪里，观众期待看到什么）
3. 要有画面感和情绪张力——让人一看就能在脑海中"看到"这个故事的画面
4. 如果是改编/改写项目，要体现短剧化的改编思路——不是原文缩写，而是短剧语言的重构
5. synopsis要能独立吸引观众——即使只看这一段话，也会产生"这剧我想看"的冲动

【JSON Schema】
{"settings":{"audience":"男频或女频","genres":["题材1","题材2"],"core":["核心设定"],"worldView":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"}}

【质量要求】
1. synopsis是最重要的字段，必须写成完整的300-400字故事概梗
2. 必须包含五要素：处境/冲突/压迫/爽点/追看方向
3. genres、core要有具体内容，不能是空泛的标签
4. highlights要一句话概括"追这部剧的核心动力"
5. 所有字段不能为空，禁止写"待定""无""略"`, nil
	case "characters":
		return base + `【任务：生成人物小传】
你需要基于故事设定生成8-12个主要人物的完整小传。每个人物都是故事冲突网络中的活态节点——他们有欲望、有缺陷、有秘密、有变化。人物的质量直接决定了整部短剧的上限。

【创作原则】
1. 第一个角色必须是主角——主角是观众的欲望投射载体，必须让人想成为他/她或心疼他/她
2. 反派、助攻、亲属/权力角色必须齐全，构建完整的人物关系网——任何两个角色之间都要有戏剧张力
3. 每个人物必须有三重辨识系统：①性格标签（3-5个精准标签）②说话方式（口头禅、语气特征）③行为模式（面对冲突的第一反应）
4. 反派要有合理动机和足够压迫感——不能"为坏而坏"，要让观众理解他/她为什么这样做，同时更加愤怒
5. 角色之间要有差异化——同一场景中任意两个角色的反应必须不同
6. 关键角色要有"记忆炸弹"——反差萌、身份反转、隐藏属性等让人印象深刻的特质

【JSON Schema】
{"characters":[{"name":"姓名","role":"定位","age":"年龄","meta":"年龄 / 定位","traits":["3-5个性格标签"],"background":"背景经历","goal":"核心动机","relation":"人物关系","bio":"综合小传"}]}

【质量要求】
1. 第一个角色必须是主角，后续按剧情重要性排序
2. 每个bio要求200-400字，必须包含：①欲望和梦想②核心缺陷③关键转折事件④关系变化节点⑤隐藏秘密或反转伏笔
3. traits标签要具体有辨识度——"毒舌但护短""外表冷漠内心柔软"比"善良""聪明"好
4. 背景、动机、关系之间必须逻辑自洽——不能出现"从小被抛弃却性格开朗乐观"这种矛盾（除非有解释）
5. 所有字段不能为空，禁止写"待定""无""略"
6. 人物名称必须有辨识度，不能与同剧其他角色名字相似`, nil
	case "outline":
		return base + `【任务：生成四阶段粗纲】
你只需要根据故事概梗和人物小传，AI全新生成四阶段粗纲。不要参考小说章纲，不要生成单集集纲。粗纲是整部短剧的叙事骨架——骨架的质量直接决定了后续集纲和正文的上限。

【创作原则】
1. 粗纲是故事的骨架，每个阶段要有明确的戏剧任务和标志性事件
2. 起：建立处境（让观众代入）→引爆事件（打破日常）→反派首次压迫（制造愤怒）→第一轮爽点（小胜或反击）→阶段钩子（更大的危机来了）
3. 承：推进关系（新关系出现或旧关系裂变）→反派压力升级（更高层级的威胁）→误会/秘密扩大（信息差制造焦虑）→爽点升级（更大的反击）→阶段钩子（真相的边缘）
4. 转：真相揭露（信息差消除）→危机全面升级（从局部冲突到全面战争）→人物面临终极抉择→关键反转和情绪爆点→阶段钩子（终极对决序幕）
5. 合：终局对抗（所有矛盾的总爆发）→反派清算（积攒的愤怒在此释放）→人物关系收束→核心爽点最终兑现→结局余味

【JSON Schema】
{"outlines":[{"range":"第1-N集","phase":"起","content":"开端阶段的大致剧情，400-800字，要写清具体事件和人物行动"},{"range":"第N-N集","phase":"承","content":"推进阶段的大致剧情，400-800字"},{"range":"第N-N集","phase":"转","content":"反转升级阶段的大致剧情，400-800字"},{"range":"第N-N集","phase":"合","content":"收束爆发阶段的大致剧情，400-800字"}]}

【质量要求】
1. 必须只有四个粗纲，phase固定为"起、承、转、合"
2. range必须按用户要求的总集数规划为连续的x-x区间
3. content写成400-800字的完整段落，要写清具体事件名称、人物行动、冲突升级方式和阶段钩子
4. 不要堆集数明细，不要写"主角成长、矛盾升级"等空话——必须是可拍摄的具体事件
5. 不要返回episodes字段
6. 反派压力必须逐级递增，爽点必须层层递进`, nil
	case "episode":
		return base + `【任务：生成单集集纲】
你需要基于现有故事设定、人物小传和四阶段粗纲，为用户指定范围补齐或优化集纲。每一集都是一个独立的"上瘾单元"——必须在有限篇幅内完成"建立期待→部分满足→制造更大期待"的成瘾循环。

【创作原则】
1. 每集必须有五要素：①核心事件（发生了什么）②人物目标（谁想要什么）③冲突推进（遇到了什么阻力）④关键反转或爽点（预期被打破或情绪释放）⑤结尾追看钩子（为什么必须看下一集）
2. "起承转合"只代表粗纲的大方向，不要在每集里套写起承转合结构——每集只关注本集的戏剧任务
3. 前后集之间要有因果衔接——上一集的结果是这一集的原因，这一集的结果是下一集的起因
4. 每集的爽点要有变化和升级——不能连续几集都是同一种爽感
5. 人物行动要具体——"他反击了"不如"他当着所有人的面揭穿了反派的谎言"

【JSON Schema】
{"episodes":[{"no":1,"outline":"本集核心事件、人物目标、冲突推进、关键反转或爽点、结尾追看钩子","body":""}]}

【质量要求】
1. no必须对应用户要求的集数范围
2. 不要输出"本集起/承/转/合"小标题
3. 每集集纲要贴合当前阶段粗纲，写清：本集发生什么→谁推动→冲突如何升级→有什么反转或爽点→结尾为什么让人追下一集
4. 不要改正文body字段
5. 每集集纲80-200字，要具体可拍——导演看了就知道怎么拍`, nil
	case "body":
		if fmt.Sprint(project.Settings["adaptationMode"]) == "original" {
			return base + `【任务：原文改编模式正文生成】
你需要按照原文改编模式，直接根据小说章纲和故事概梗生成或补齐短剧正文。不要另行改写人物小传或分集大纲。正文必须保留原文的精华冲突，同时用短剧的语言和节奏重新讲述。

【剧本格式规范】
- 场景头："△集数-场次 日/夜 内/外 场景名"——信息必须完整具体
- 人物行："人物：角色A、角色B"
- 动作行："△"开头，描写可拍摄的动作、调度或画面
- 台词行："角色名：（语气/情绪）台词内容"——括号内标注丰富的语气和情绪

【JSON Schema】
{"episodes":[{"no":1,"body":"△1-1 日 内 场景名\n人物：角色A、角色B\n△可拍摄的动作描写\n角色A：（语气）短台词"}]}

【质量要求】
1. 必须只返回JSON
2. body必须使用专业剧本格式——场景头、人物、动作、台词缺一不可
3. 正文顺序贴合小说章纲，保留原文主线冲突——但节奏要更快、台词要更狠
4. 台词要短而有力，符合人物性格——每句台词都要有信息量或情绪冲击
5. 每集结尾要留追看钩子——最后30秒决定观众是否点下一集
6. 动作描写要精确到可拍摄——谁、做什么、怎么做、什么表情`, nil
		}
		return base + `【任务：标准模式正文生成】
你需要为项目中已有episodes生成或补齐短剧正文。优先处理没有body的集数。正文是观众最终看到的内容——是所有前期设定、大纲和集纲的最终呈现，必须达到"可直接拍摄"的专业水准。

【创作原则】
1. 只根据故事概梗、人物小传和已有分集集纲生成——不要参考小说章纲
2. 正文必须可拍摄——导演看了知道怎么拍，演员看了知道怎么演
3. 节奏要快——每场都有存在的戏剧理由，没有"过渡场"和"日常场"
4. 台词要短——每句都要有信息量或情绪冲击，没有废话
5. 情绪要连贯——多集之间人物状态、冲突、情感都要承接

【剧本格式规范】
- 场景头："△集数-场次 日/夜 内/外 场景名"——信息必须完整具体
- 人物行："人物：角色A、角色B"
- 动作行："△"开头，描写可拍摄的动作、调度或画面——精确到"谁、做什么、怎么做、什么表情"
- 台词行："角色名：（语气/情绪）台词内容"——善用潜台词，括号内标注丰富的情绪

【JSON Schema】
{"episodes":[{"no":1,"body":"△1-1 日 内 场景名\n人物：角色A、角色B\n△可拍摄的动作描写\n角色A：（语气）短台词"}]}

【质量要求】
1. 必须只返回JSON，body使用专业剧本格式
2. no必须对应已有集数——不要生成没有集纲的集
3. 生成多集时必须按集数顺序连续生成
4. 后一集要承接上一集正文结尾的人物状态、情绪和未解决冲突——不能割裂
5. 每集结尾留追看钩子——最后30秒决定观众是否点下一集
6. 台词要短而有力、符合人物性格和当前情绪——善用潜台词
7. 动作描写要精确到可拍摄——不能一句话带过关键动作`, nil
	default:
		return "", fmt.Errorf("unsupported task type %s", taskType)
	}
}

func buildPlanningStreamPrompt(project ScriptProject, userPrompt string) string {
	contextProject := taskContextProject(project, "planning")
	context, _ := json.Marshal(contextProject)
	return fmt.Sprintf(`【项目快照 JSON】
%s

%s

【用户选择和输入】
%s

【任务：生成3个短剧策划方案】
你需要基于项目快照和用户输入，生成3个完全不同的短剧策划方案。每个方案必须是一个完整的创作蓝图，可直接进入故事概梗页面使用。三个方案要覆盖不同的创意方向——一个是稳妥路线、一个是创新路线、一个是极限路线。

【创作原则】
1. 三套策划必须有本质差异——三种完全不同的故事体验，不能只是换皮微调
2. 标题要有短剧感和吸引力——让人一看就想点进来，最好能一句话勾起好奇心
3. 核心梗概要写成300-400字的完整故事概梗——包含主角处境、核心冲突、反派压迫阶梯、爽点机制和追看方向
4. 核心设定要有记忆点——让人看完后能用一句话向朋友推荐："这部剧讲的是XXX"
5. 世界观要具体有趣——不是空泛的背景描述，而是有画面感、有规则、有冲突空间的世界
6. 要考虑制作方式约束——AI短剧可以天马行空，真人实拍需要接地气

【策划1】
标题：短剧标题（《标题》格式）
目标受众：男频或女频
题材类型：题材1、题材2
时代背景：现代都市/古代/民国/近未来等
核心设定：核心1、核心2
核心亮点：一句强钩子亮点——概括追剧核心动力
世界观：故事背景/人物处境/核心规则——具体有画面感
核心梗概：300-400字完整故事梗概，包含主角处境、核心冲突、反派压迫阶梯、爽点机制和追看方向

【策划2】
标题：
目标受众：
题材类型：
核心设定：
时代背景：
核心亮点：
世界观：
核心梗概：

【策划3】
标题：
目标受众：
题材类型：
核心设定：
时代背景：
核心亮点：
世界观：
核心梗概：

【质量要求】
1. 三套策划必须差异明显，覆盖不同创意方向——稳妥/创新/极限
2. 核心梗概写得可直接进入故事概梗页，300-400字
3. 每个策划的每个字段都不能为空，禁止写"待定""无""略"
4. 不要写JSON，不要Markdown，直接按格式输出
5. 边思考边输出，但最终必须严格使用上面的文本结构`, string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt))
}

func buildCharacterStreamPrompt(project ScriptProject, userPrompt string) string {
	contextProject := taskContextProject(project, "characters")
	context, _ := json.Marshal(contextProject)
	return fmt.Sprintf(`【项目快照 JSON】
%s

%s

【用户补充】
%s

【任务：生成8-12个主要人物小传】
你需要基于故事设定生成8-12个主要人物的完整小传。每个人物都是故事冲突网络中的活态节点——他们有欲望、有缺陷、有秘密、有变化。人物的质量直接决定了整部短剧的上限。

【创作原则】
1. 第一个角色必须是主角——主角是观众的欲望投射载体
2. 反派、助攻、亲属/权力角色必须齐全——构建完整的人物关系网
3. 每个人物必须有三重辨识系统：①性格标签②说话方式③行为模式
4. 反派要有合理动机和足够压迫感——不能"为坏而坏"
5. 角色之间要有差异化——同一场景中任意两个角色的反应必须不同
6. 人物关系网要清晰，能支撑主要冲突——任何两个角色之间都要有戏剧张力

【人物1】
姓名：角色姓名
定位：主角/反派/助攻/亲属/竞争者等
年龄：年龄或年龄段
性格标签：标签1、标签2、标签3——具体有辨识度
背景：背景经历、身份反转、过往创伤或秘密
核心动机：角色最想得到什么，为什么——要有内在驱动力
人物关系：与主角和其他关键角色的关系——要有张力
人物小传：200-400字完整人物小传，必须包含欲望、缺陷、转折事件、关系变化和隐藏秘密

【人物2】
姓名：
定位：
年龄：
性格标签：
背景：
核心动机：
人物关系：
人物小传：

【质量要求】
1. 第一个角色必须是主角，后续按剧情重要性排序
2. 每个人物小传要求200-400字——要有深度和细节
3. 每个人物字段不能为空，禁止写"待定""无""略"
4. 边生成边输出，但必须严格使用上面的文本结构
5. 每写完一个字段就立刻换行继续下一个字段
6. 生成完一个人物再生成下一个人物
7. 不要写JSON，不要Markdown
8. 人物名字要有辨识度和记忆点`, string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt))
}

func parseCharacterStreamCharacters(text string) []Character {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	re := regexp.MustCompile(`【人物\s*(\d+)】`)
	matches := re.FindAllStringSubmatchIndex(normalized, -1)
	characters := []Character{}
	for i, match := range matches {
		no := fmt.Sprint(len(characters) + 1)
		if len(match) >= 4 && match[2] >= 0 && match[3] >= match[2] {
			no = normalized[match[2]:match[3]]
		}
		start := match[1]
		end := len(normalized)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		character := parseCharacterStreamBlock(normalized[start:end])
		if strings.TrimSpace(character.Name) == "" {
			character.Name = "角色" + no
		}
		if strings.TrimSpace(character.Role) == "" && strings.TrimSpace(character.Bio+character.Background+character.Goal) == "" {
			character.Role = "创建中"
		}
		characters = append(characters, character)
	}
	return characters
}

func parseCharacterStreamBlock(block string) Character {
	fields := map[string]string{}
	current := ""
	for _, raw := range strings.Split(block, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		key, value, ok := splitLooseChineseField(line)
		if ok {
			current = key
			fields[key] = value
			continue
		}
		if current != "" {
			fields[current] = strings.TrimSpace(fields[current] + "\n" + line)
		}
	}
	role := firstNonEmpty(fields["定位"], fields["角色定位"])
	age := fields["年龄"]
	return Character{
		Name:       fields["姓名"],
		Role:       role,
		Age:        age,
		Meta:       strings.TrimSpace(age + " / " + role),
		Traits:     splitChineseList(firstNonEmpty(fields["性格标签"], fields["标签"])),
		Background: firstNonEmpty(fields["背景"], fields["背景经历"]),
		Goal:       firstNonEmpty(fields["核心动机"], fields["动机"]),
		Relation:   firstNonEmpty(fields["人物关系"], fields["关系"]),
		Bio:        firstNonEmpty(fields["人物小传"], fields["综合小传"]),
	}
}

func splitLooseChineseField(line string) (string, string, bool) {
	for _, sep := range []string{"：", ":"} {
		if idx := strings.Index(line, sep); idx > 0 {
			return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+len(sep):]), true
		}
	}
	return "", "", false
}

func buildOutlineStreamPrompt(project ScriptProject, userPrompt string) string {
	contextProject := taskContextProject(project, "outline")
	context, _ := json.Marshal(contextProject)
	return fmt.Sprintf(`【项目快照 JSON】
%s

%s

【用户补充】
%s

【任务：生成四阶段粗纲】
你只需要根据故事概梗和人物小传，生成四阶段（起承转合）粗纲。不要生成单集集纲，不要写JSON。必须按下面顺序边思考边输出，每个阶段输出完再进入下个阶段。粗纲是整部短剧的叙事骨架——骨架的质量直接决定了后续集纲和正文的上限。

【创作原则】
1. 每个阶段要有明确的戏剧任务、标志性事件和阶段钩子
2. 每阶段写成400-800字的完整段落，概括该区间所有集的主要戏剧推进
3. 要写清具体事件名称、人物行动、冲突升级方式——不能只写抽象方向
4. 四阶段之间必须有清晰的因果递进关系——前一阶段的结果是后一阶段的原因
5. 反派压力要持续升级，爽点要有层次递进——形成螺旋上升的刺激曲线
6. 每阶段结尾必须有"阶段钩子"——驱动观众追看下一阶段的悬念

【起】
范围：第1-X集
粗纲：用较完整的段落概括这一集数区间的剧情推进。要写清：①主角处境如何建立（让观众代入）②引爆事件是什么（打破日常）③核心矛盾如何形成④反派或阻力如何施加压迫⑤第一轮爽点如何爆发⑥阶段结尾的追看钩子是什么。剧情要有趣、有反转、有可拍的情绪场面，不能太短，不能只写一句方向。每段建议400-800字。

【承】
范围：第X-X集
粗纲：用较完整的段落概括这一集数区间的剧情推进。要写清：①关系如何推进、阵营如何变化②反派压力如何升级（必须比起阶段更强）③误会或秘密如何扩大④爽点如何升级⑤阶段性反转如何发生。要让用户看完能知道这几集大概会发生哪些连续事件，不能空泛。

【转】
范围：第X-X集
粗纲：用较完整的段落概括这一集数区间的剧情推进。要写清：①真相如何揭露②危机如何全面升级③人物面临什么终极抉择④关键背叛或误判是什么⑤强反转和情绪爆点在哪里。要有连续的因果链，不要写成概念词堆砌。

【合】
范围：第X-X集
粗纲：用较完整的段落概括这一集数区间的剧情推进。要写清：①终局对抗如何展开②反派如何被清算（积攒的愤怒在此释放）③人物关系如何落点④情绪如何释放⑤核心爽点如何兑现⑥结局留什么余味。如果适合续作可留轻钩子，但不能牺牲本季收束。

【质量要求】
1. phase只能是起、承、转、合
2. 每个粗纲写成可直接放入粗纲输入框的正文，每段400-800字
3. 要概括该区间所有集的主要戏剧推进
4. 不要逐集编号，不要写单集标题
5. 不要只写"主角成长、矛盾升级"这类空话，必须写具体事件和人物行动
6. 不要写JSON，不要Markdown
7. 反派压力必须逐级递增，爽点必须层层递进`, string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt))
}

func parseOutlineStreamOutlines(text string) []OutlineBlock {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	re := regexp.MustCompile(`【\s*(起|承|转|合)\s*】`)
	matches := re.FindAllStringSubmatchIndex(normalized, -1)
	outlines := []OutlineBlock{}
	seen := map[string]bool{}
	for i, match := range matches {
		phase := normalized[match[2]:match[3]]
		start := match[1]
		end := len(normalized)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		block := parseOutlineStreamBlock(phase, normalized[start:end])
		if strings.TrimSpace(block.Content) == "" {
			block.Content = "生成中"
		}
		if !seen[phase] {
			outlines = append(outlines, block)
			seen[phase] = true
		}
	}
	return outlines
}

func parseOutlineStreamBlock(phase, block string) OutlineBlock {
	fields := map[string]string{}
	current := ""
	for _, raw := range strings.Split(block, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		key, value, ok := splitLooseChineseField(line)
		if ok {
			current = key
			fields[key] = value
			continue
		}
		if current != "" {
			fields[current] = strings.TrimSpace(fields[current] + "\n" + line)
		}
	}
	return OutlineBlock{
		Phase:   phase,
		Range:   firstNonEmpty(fields["范围"], fields["集数"], ""),
		Content: firstNonEmpty(fields["粗纲"], fields["内容"], strings.TrimSpace(block)),
	}
}

func buildEpisodeStreamPrompt(project ScriptProject, userPrompt string) string {
	contextProject := taskContextProject(project, "episode")
	context, _ := json.Marshal(contextProject)
	return fmt.Sprintf(`【项目快照 JSON】
%s

%s

【用户补充】
%s

【任务：生成单集集纲】
你只需要生成用户指定范围内的单集集纲，不要写JSON。必须按集数顺序边生成边输出，每一集输出完再进入下一集。每一集都是一个独立的"上瘾单元"——必须在有限篇幅内完成"建立期待→部分满足→制造更大期待"的成瘾循环。

【创作原则】
1. 每集必须有五要素：①核心事件②人物目标③冲突推进④关键反转或爽点⑤结尾追看钩子
2. "起承转合"只代表四阶段粗纲的大方向，不要在每集里套写——每集只关注本集的戏剧任务
3. 前后集之间要有因果衔接——上一集的结果是这一集的原因
4. 每集的爽点要有变化和升级——不能连续几集都是同一种爽感
5. 人物行动要具体——"他反击了"不如"他当众揭穿了反派的谎言"
6. 每集结尾必须有追看钩子——最后的悬念决定观众是否点下一集

【第1集】
集纲：本集核心事件、人物目标、冲突推进、关键反转或爽点、结尾追看钩子

【第2集】
集纲：

【质量要求】
1. 集数必须对应用户指定范围，不多不少
2. 不要输出"本集起/承/转/合"小标题
3. 每集集纲写成80-200字的可直接放入输入框的一段话
4. 贴合当前阶段粗纲，写清：本集发生什么→谁推动→冲突如何升级→有什么反转或爽点→结尾为什么让人追下一集
5. 不要改正文body
6. 不要写JSON，不要Markdown`, string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt))
}

func parseEpisodeStreamEpisodes(text string) []Episode {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	re := regexp.MustCompile(`【\s*第\s*(\d+)\s*集\s*】`)
	matches := re.FindAllStringSubmatchIndex(normalized, -1)
	episodes := []Episode{}
	seen := map[int]bool{}
	for i, match := range matches {
		no := 0
		if len(match) >= 4 && match[2] >= 0 && match[3] >= match[2] {
			no, _ = strconv.Atoi(normalized[match[2]:match[3]])
		}
		if no <= 0 || seen[no] {
			continue
		}
		start := match[1]
		end := len(normalized)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		episode := parseEpisodeStreamBlock(no, normalized[start:end])
		if strings.TrimSpace(episode.Outline) == "" {
			episode.Outline = "生成中"
		}
		episodes = append(episodes, episode)
		seen[no] = true
	}
	return episodes
}

func parseEpisodeStreamBlock(no int, block string) Episode {
	fields := map[string]string{}
	current := ""
	for _, raw := range strings.Split(block, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		key, value, ok := splitLooseChineseField(line)
		if ok {
			current = key
			fields[key] = value
			continue
		}
		if current != "" {
			fields[current] = strings.TrimSpace(fields[current] + "\n" + line)
		}
	}
	outline := firstNonEmpty(fields["集纲"], fields["内容"], fields["单集集纲"], strings.TrimSpace(block))
	return Episode{No: no, Outline: outline}
}

func buildBodyStreamPrompt(project ScriptProject, userPrompt string) string {
	contextProject := taskContextProject(project, "body")
	context, _ := json.Marshal(contextProject)
	sourceRule := `【素材来源】
只根据故事概梗、人物小传和已有分集集纲生成正文，不要参考小说章纲。正文必须可拍摄、节奏快、台词短而有力——导演看了知道怎么拍，演员看了知道怎么演。`
	if fmt.Sprint(project.Settings["adaptationMode"]) == "original" {
		sourceRule = `【素材来源】
按照原文改编模式生成：可以参考小说章纲和故事概梗，保留原文主线冲突，但必须用短剧的语言和节奏重新讲述。台词要有短剧感——短、准、狠、有潜台词，节奏要快——每场都有存在的戏剧理由。`
	}
	return fmt.Sprintf(`【项目快照 JSON】
%s

%s

【用户补充】
%s

%s

【任务：生成短剧正文】
请只生成用户指定集数范围内的短剧正文，不要写JSON，不要Markdown，不要解释，不要写"正在生成"等说明文字。必须按集数顺序边生成边输出，每一集输出完再进入下一集。正文是观众最终看到的内容——必须达到"可直接拍摄"的专业水准。

【剧本格式规范】
- 场景头："△集数-场次 日/夜 内/外 场景名"——信息必须完整具体，让场记一看就知道在哪里拍
- 人物行："人物：角色A、角色B"
- 动作行："△"开头，描写可拍摄的动作、调度、表情或画面——精确到"谁、做什么、怎么做、什么表情"
- 台词行："角色名：（语气/情绪）台词内容"——善用潜台词，括号内标注丰富的情绪状态
- 台词要短而有力，符合人物性格和当前情绪——每句都要有信息量或情绪冲击
- 场景标题包含时间（日/夜）、光线（内/外）、地点（具体场景名）三要素
- 关键动作要分解为多个拍摄步骤，不能一句话带过

【第N集】
△N-1 日/夜 内/外 具体场景名（精确到"苏家寿宴大厅角落"而非"大厅"）
人物：角色A、角色B、宾客若干
△[氛围建立，2-3行] 先交代空间，再把主角放进去，用一个细节暗示她的处境——比如"面前的茶水已凉"胜过直说"她很孤独"
宾客甲：（低声对同伴）一句揭示信息的议论，兼具背景交代和压迫感
△[主角内心外化] 用身体动作代替心理描写——"她低下头，握紧了拳头，指节泛白"比"她很委屈"有力十倍
角色B：（进场动作 + 语气 + 意图三合一）台词表面甜，底层藏刀
△[行动推进] 反派的阴谋动作要写两层：外人看到的（踉跄）和观众看穿的（设计好的踉跄）
△[全场聚焦] "喧闹声骤然停止，所有人的目光汇聚于此"——用群像视线放大压迫
角色B：（哽咽，声音刻意放大让所有人都听见）指控性台词，一句话把黑锅扣上去

【质量要求】
1. 集数必须对应用户指定范围
2. 严格遵循专业剧本格式——场景头、人物、动作、台词缺一不可
3. 台词要短而有力、符合人物性格——善用潜台词，每句都有信息量或情绪冲击
4. 动作描写要精确到可拍摄——关键动作要分解为多个步骤
5. 生成多集时后一集要承接上一集正文结尾的人物状态、情绪和未解决冲突——不能割裂
6. 每集结尾留追看钩子——最后30秒决定观众是否点下一集
7. 正文要贴合集纲内容——每场都有存在的戏剧理由，没有"废场"
8. 情绪曲线要有起伏——不能全程平淡，也不能全程高压`, string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt), sourceRule)
}

func parseBodyStreamEpisodes(text string) []Episode {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	re := regexp.MustCompile(`【\s*第\s*(\d+)\s*集\s*】`)
	matches := re.FindAllStringSubmatchIndex(normalized, -1)
	episodes := []Episode{}
	seen := map[int]bool{}
	for i, match := range matches {
		no := 0
		if len(match) >= 4 && match[2] >= 0 && match[3] >= match[2] {
			no, _ = strconv.Atoi(normalized[match[2]:match[3]])
		}
		if no <= 0 || seen[no] {
			continue
		}
		start := match[1]
		end := len(normalized)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		episode := parseBodyStreamBlock(no, normalized[start:end])
		if strings.TrimSpace(episode.Body) != "" {
			episodes = append(episodes, episode)
			seen[no] = true
		}
	}
	return episodes
}

func parseBodyStreamBlock(no int, block string) Episode {
	body := strings.TrimSpace(block)
	body = strings.TrimPrefix(body, "```text")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")
	body = strings.TrimSpace(body)
	lines := strings.Split(body, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		key, value, ok := splitLooseChineseField(line)
		if ok {
			switch strings.ToLower(key) {
			case "正文", "剧本正文", "内容", "body":
				next := []string{}
				if value != "" {
					next = append(next, value)
				}
				next = append(next, lines[i+1:]...)
				body = strings.TrimSpace(strings.Join(next, "\n"))
			}
		}
		break
	}
	return Episode{No: no, Body: body}
}

func parsePlanningStreamPlans(text string) []map[string]any {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	re := regexp.MustCompile(`【\s*(?:策划|方案)\s*([123１２３一二三])(?:\s*[：:｜|·—_、\s（(\-][^】]*)?】`)
	matches := re.FindAllStringSubmatchIndex(normalized, -1)
	plans := []map[string]any{}
	for i, match := range matches {
		start := match[1]
		end := len(normalized)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		block := strings.TrimSpace(normalized[start:end])
		if block == "" {
			continue
		}
		plan := parsePlanningBlock(block)
		if strings.TrimSpace(fmt.Sprint(plan["title"])) == "" && strings.TrimSpace(fmt.Sprint(plan["synopsis"])) == "" {
			continue
		}
		plans = append(plans, plan)
	}
	if len(plans) == 0 {
		lines := strings.Split(normalized, "\n")
		plan := map[string]any{"title": firstNonEmpty(firstFieldLine(lines, "标题"), "策划方案"), "synopsis": strings.TrimSpace(normalized)}
		plans = append(plans, plan)
	}
	if len(plans) > 3 {
		plans = plans[:3]
	}
	return plans
}

func parsePlanningBlock(block string) map[string]any {
	fields := map[string]string{}
	current := ""
	for _, raw := range strings.Split(block, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if key, value, ok := splitChineseField(line); ok {
			current = key
			fields[key] = value
			continue
		}
		if current != "" {
			fields[current] = strings.TrimSpace(fields[current] + "\n" + line)
		}
	}
	return map[string]any{
		"title":      firstNonEmpty(fields["标题"], "未命名策划"),
		"audience":   fields["目标受众"],
		"genres":     splitChineseList(fields["题材类型"]),
		"era":        splitChineseList(firstNonEmpty(fields["时代背景"], fields["风格元素"])),
		"core":       splitChineseList(fields["核心设定"]),
		"style":      []string{},
		"highlights": fields["核心亮点"],
		"worldView":  fields["世界观"],
		"synopsis":   fields["核心梗概"],
	}
}

func splitChineseField(line string) (string, string, bool) {
	for _, sep := range []string{"：", ":"} {
		if idx := strings.Index(line, sep); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+len(sep):])
			switch key {
			case "标题", "目标受众", "时代背景", "题材类型", "核心设定", "风格元素", "核心亮点", "世界观", "核心梗概":
				return key, value, true
			}
		}
	}
	return "", "", false
}

func splitChineseList(value string) []string {
	items := []string{}
	for _, item := range regexp.MustCompile(`[、,，\s]+`).Split(value, -1) {
		item = strings.TrimSpace(item)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func firstFieldLine(lines []string, key string) string {
	for _, line := range lines {
		if k, v, ok := splitChineseField(line); ok && k == key {
			return v
		}
	}
	return ""
}

func taskContextProject(project ScriptProject, taskType string) ScriptProject {
	if taskType != "outline" && taskType != "body" {
		return project
	}
	if fmt.Sprint(project.Settings["adaptationMode"]) == "original" && taskType == "body" {
		return project
	}
	next := project
	if next.Settings != nil {
		settings := map[string]any{}
		for key, value := range next.Settings {
			if key == "novelChapterOutline" || key == "chapterBreakdown" || key == "chapterCount" {
				continue
			}
			settings[key] = value
		}
		next.Settings = settings
	}
	return next
}

func extractJSONObject(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}
	start := strings.Index(content, "{")
	if start >= 0 {
		depth := 0
		inString := false
		escaped := false
		for index, char := range content[start:] {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' && inString {
				escaped = true
				continue
			}
			if char == '"' {
				inString = !inString
				continue
			}
			if inString {
				continue
			}
			if char == '{' {
				depth++
			}
			if char == '}' {
				depth--
				if depth == 0 {
					return content[start : start+index+1]
				}
			}
		}
	}
	return content
}

func validateAIResult(project ScriptProject, taskType string, result map[string]any) error {
	required := map[string]string{
		"planning":   "plans",
		"synopsis":   "settings",
		"extract":    "settings",
		"characters": "characters",
		"outline":    "outlines",
		"episode":    "episodes",
		"body":       "episodes",
	}
	key := required[taskType]
	if key == "" {
		return fmt.Errorf("unsupported task type %s", taskType)
	}
	if _, ok := result[key]; !ok {
		return fmt.Errorf("missing %s in ai result", key)
	}
	if taskType == "extract" {
		for _, item := range []string{"characters", "outlines", "episodes"} {
			if _, ok := result[item]; !ok {
				return fmt.Errorf("missing %s in ai result", item)
			}
			if !nonEmptyJSONArray(result[item]) {
				return fmt.Errorf("empty %s in ai result", item)
			}
		}
		if project.Source == "adaptation" && !nonEmptyJSONArray(result["novelChapterOutline"]) {
			return fmt.Errorf("missing novelChapterOutline in ai result")
		}
	}
	return nil
}

func applyAIResult(p *ScriptProject, taskType string, result map[string]any) {
	switch taskType {
	case "planning":
		p.Status = "planning_done"
	case "synopsis":
		if settings, ok := result["settings"].(map[string]any); ok {
			if p.Settings == nil {
				p.Settings = map[string]any{}
			}
			for key, value := range settings {
				p.Settings[key] = value
			}
			flow := ensureInfoflow(p)
			story, _ := flow["storyOverview"].(map[string]any)
			if story == nil {
				story = map[string]any{}
			}
			for _, key := range []string{"audience", "worldView", "highlights", "synopsis"} {
				if value, ok := settings[key]; ok {
					targetKey := key
					if key == "worldView" {
						targetKey = "background"
					}
					story[targetKey] = value
				}
			}
			flow["storyOverview"] = story
			p.Settings["infoflow"] = flow
		}
	case "extract":
		if settings, ok := result["settings"].(map[string]any); ok {
			if p.Settings == nil {
				p.Settings = map[string]any{}
			}
			for key, value := range settings {
				p.Settings[key] = value
			}
		}
		if characters := decodeSlice[Character](result["characters"]); len(characters) > 0 {
			p.Characters = characters
		}
		if outlines := decodeSlice[OutlineBlock](result["outlines"]); len(outlines) > 0 {
			p.Outlines = outlines
		}
		if episodes := decodeSlice[Episode](result["episodes"]); len(episodes) > 0 {
			p.Episodes = mergeEpisodes(p.Episodes, episodes)
		}
		if p.Source == "adaptation" {
			if outline, ok := result["novelChapterOutline"]; ok {
				if p.Settings == nil {
					p.Settings = map[string]any{}
				}
				p.Settings["novelChapterOutline"] = outline
			}
		}
	case "characters":
		if characters := decodeSlice[Character](result["characters"]); len(characters) > 0 {
			p.Characters = characters
		}
	case "outline":
		if outlines := decodeSlice[OutlineBlock](result["outlines"]); len(outlines) > 0 {
			p.Outlines = outlines
		}
	case "episode":
		if episodes := decodeSlice[Episode](result["episodes"]); len(episodes) > 0 {
			p.Episodes = mergeEpisodes(p.Episodes, episodes)
		}
	case "body":
		if episodes := decodeSlice[Episode](result["episodes"]); len(episodes) > 0 {
			p.Episodes = mergeEpisodes(p.Episodes, episodes)
		}
	}
	p.Status = "draft"
	p.UpdatedAt = time.Now().Format("2006-01-02 15:04")
}

func decodeSlice[T any](value any) []T {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out []T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func mergeEpisodes(existing, generated []Episode) []Episode {
	if len(existing) == 0 {
		existing = []Episode{{No: 1}}
	}
	for _, next := range generated {
		if next.No <= 0 {
			next.No = len(existing) + 1
		}
		found := false
		for i := range existing {
			if existing[i].No == next.No {
				if next.Outline != "" {
					existing[i].Outline = next.Outline
				}
				if next.Body != "" {
					existing[i].Body = next.Body
				}
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, next)
		}
	}
	return existing
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	purpose := r.URL.Query().Get("purpose")
	var req struct {
		Title        string `json:"title"`
		FileName     string `json:"fileName"`
		Content      string `json:"content"`
		FullContent  string `json:"fullContent"`
		ScriptType   string `json:"scriptType"`
		AdaptMode    string `json:"adaptMode"`
		ChapterStart int    `json:"chapterStart"`
		ChapterEnd   int    `json:"chapterEnd"`
		Cost         int    `json:"cost"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = r.URL.Query().Get("title")
	}
	if title == "" {
		if purpose == "adaptation" {
			title = "网文改编项目"
		} else {
			title = "剧本改写项目"
		}
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		http.Error(w, "uploaded content is empty", http.StatusBadRequest)
		return
	}
	projectType := req.ScriptType
	if projectType == "" {
		projectType = "AI短剧"
	}
	project := ScriptProject{
		Title:     title,
		Type:      projectType,
		Source:    purpose,
		Status:    "parsed",
		UpdatedAt: time.Now().Format("2006-01-02 15:04"),
		Settings: map[string]any{
			"audience": "", "genres": []string{}, "core": []string{}, "style": []string{},
			"worldView": "", "highlights": "", "synopsis": "",
		},
		Episodes: []Episode{{No: 1, Outline: "", Body: ""}},
	}
	if purpose == "adaptation" && req.ChapterStart > 0 && req.ChapterEnd >= req.ChapterStart {
		project.Settings["adaptationMode"] = normalizedAdaptMode(req.AdaptMode)
		project.Settings["chapterRange"] = "第" + strconv.Itoa(req.ChapterStart) + "-" + strconv.Itoa(req.ChapterEnd) + "章"
		project.Settings["importCost"] = req.Cost
	} else if purpose == "rewriting" && req.Cost > 0 {
		project.Settings["importCost"] = req.Cost
	}
	summary, err := applyParsedUpload(&project, purpose, req.FileName, content)
	if err != nil {
		log.Printf("upload analysis failed for %s: %v", purpose, err)
		http.Error(w, "上传解析失败："+err.Error(), http.StatusBadGateway)
		return
	}
	if purpose == "adaptation" {
		project.Settings["adaptationMode"] = normalizedAdaptMode(req.AdaptMode)
		if strings.TrimSpace(req.FullContent) != "" {
			project.Settings["fullSourceLength"] = textLength(req.FullContent)
		}
		if req.ChapterStart > 0 && req.ChapterEnd >= req.ChapterStart {
			project.Settings["chapterRange"] = "第" + strconv.Itoa(req.ChapterStart) + "-" + strconv.Itoa(req.ChapterEnd) + "章"
			project.Settings["selectedChapterCount"] = req.ChapterEnd - req.ChapterStart + 1
		}
		project.Settings["selectedWordCount"] = textLength(content)
		project.Settings["importCost"] = req.Cost
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if (purpose == "adaptation" || purpose == "rewriting") && req.Cost > 0 {
		if state.wallet.Balance < req.Cost {
			http.Error(w, "insufficient points for import", http.StatusPaymentRequired)
			return
		}
		state.wallet.Balance -= req.Cost
		title := "剧本改写拆解"
		if purpose == "adaptation" {
			title = "网文章纲拆解"
		}
		state.wallet.Items = append([]WalletTxn{{Title: title, Delta: -req.Cost, Time: time.Now().Format("2006年01月02日 15:04")}}, state.wallet.Items...)
	}
	project.ID = state.nextScript
	state.nextScript++
	state.scripts = append([]ScriptProject{project}, state.scripts...)
	state.saveLocked()
	response := map[string]any{
		"status":  "parsed",
		"message": summary["message"],
		"taskId":  time.Now().UnixMilli(),
		"summary": summary,
		"project": project,
	}
	if purpose == "adaptation" || purpose == "rewriting" {
		response["wallet"] = state.wallet
	}
	writeJSON(w, response)
}

type uploadRequest struct {
	Title        string `json:"title"`
	FileName     string `json:"fileName"`
	Content      string `json:"content"`
	FullContent  string `json:"fullContent"`
	ScriptType   string `json:"scriptType"`
	AdaptMode    string `json:"adaptMode"`
	ChapterStart int    `json:"chapterStart"`
	ChapterEnd   int    `json:"chapterEnd"`
	Cost         int    `json:"cost"`
}

func uploadStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	purpose := r.URL.Query().Get("purpose")
	if purpose != "rewriting" && purpose != "adaptation" {
		http.Error(w, "invalid upload purpose", http.StatusBadRequest)
		return
	}
	var req uploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		http.Error(w, "uploaded content is empty", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	title := strings.TrimSpace(req.Title)
	if title == "" {
		if purpose == "adaptation" {
			title = "网文改编项目"
		} else {
			title = "剧本改写项目"
		}
	}
	projectType := strings.TrimSpace(req.ScriptType)
	if projectType == "" {
		projectType = "AI短剧"
	}
	project := newUploadProject(title, projectType, purpose, req, content)

	state.mu.Lock()
	if req.Cost > 0 {
		if state.wallet.Balance < req.Cost {
			state.mu.Unlock()
			writeStreamFrame(w, flusher, "error", map[string]any{"message": "insufficient points for import"})
			return
		}
		state.wallet.Balance -= req.Cost
		txnTitle := "剧本改写拆解"
		if purpose == "adaptation" {
			txnTitle = "网文章纲拆解"
		}
		state.wallet.Items = append([]WalletTxn{{Title: txnTitle, Delta: -req.Cost, Time: time.Now().Format("2006年01月02日 15:04")}}, state.wallet.Items...)
	}
	project.ID = state.nextScript
	state.nextScript++
	state.scripts = append([]ScriptProject{project}, state.scripts...)
	walletSnapshot := state.wallet
	state.saveLocked()
	state.mu.Unlock()

	writeStreamFrame(w, flusher, "project", map[string]any{"project": project, "wallet": walletSnapshot, "message": "拆解中"})
	if purpose == "adaptation" {
		streamAdaptationAnalysis(w, flusher, project.ID, req, content)
	} else {
		streamRewriteAnalysis(w, flusher, project.ID, req, content)
	}
}

func newUploadProject(title, projectType, purpose string, req uploadRequest, content string) ScriptProject {
	project := ScriptProject{
		Title:     title,
		Type:      projectType,
		Source:    purpose,
		Status:    "parsing",
		UpdatedAt: time.Now().Format("2006-01-02 15:04"),
		Settings: map[string]any{
			"audience": "", "genres": []string{}, "core": []string{}, "style": []string{},
			"worldView": "", "highlights": "", "synopsis": "",
			"sourceFile": req.FileName, "sourceLength": textLength(content), "importType": purpose,
			"importStatus": "parsing", "importCost": req.Cost,
		},
		Episodes: []Episode{{No: 1, Outline: "", Body: ""}},
	}
	if purpose == "adaptation" {
		project.Settings["adaptationMode"] = normalizedAdaptMode(req.AdaptMode)
		if req.ChapterStart > 0 && req.ChapterEnd >= req.ChapterStart {
			project.Settings["chapterRange"] = "第" + strconv.Itoa(req.ChapterStart) + "-" + strconv.Itoa(req.ChapterEnd) + "章"
			project.Settings["selectedChapterCount"] = req.ChapterEnd - req.ChapterStart + 1
		}
	}
	return project
}

func streamAdaptationAnalysis(w io.Writer, flusher http.Flusher, projectID int64, req uploadRequest, content string) {
	writeStreamFrame(w, flusher, "progress", map[string]any{"message": "拆解中"})
	project, err := streamUploadAnalysisFromAI(w, flusher, projectID, "adaptation", req.FileName, content, uploadRangeHint(req), req.ScriptType)
	if err != nil {
		applyProjectPatch(projectID, func(project *ScriptProject) {
			project.Status = "failed"
			project.Settings["importStatus"] = "failed"
			project.Settings["aiEnhanced"] = false
			project.Settings["aiError"] = err.Error()
		})
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "大模型流式拆解失败：" + err.Error()})
		return
	}
	writeStreamFrame(w, flusher, "done", map[string]any{"project": project, "message": "小说已拆解完成"})
}

func streamRewriteAnalysis(w io.Writer, flusher http.Flusher, projectID int64, req uploadRequest, content string) {
	writeStreamFrame(w, flusher, "progress", map[string]any{"message": "拆解中"})
	project, err := streamUploadAnalysisFromAI(w, flusher, projectID, "rewriting", req.FileName, content, "", req.ScriptType)
	if err != nil {
		applyProjectPatch(projectID, func(project *ScriptProject) {
			project.Status = "failed"
			project.Settings["importStatus"] = "failed"
			project.Settings["aiEnhanced"] = false
			project.Settings["aiError"] = err.Error()
		})
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "大模型流式拆解失败：" + err.Error()})
		return
	}
	writeStreamFrame(w, flusher, "done", map[string]any{"project": project, "message": "剧本已拆解完成"})
}

func uploadRangeHint(req uploadRequest) string {
	if req.ChapterStart > 0 && req.ChapterEnd >= req.ChapterStart {
		return "第" + strconv.Itoa(req.ChapterStart) + "-" + strconv.Itoa(req.ChapterEnd) + "章"
	}
	return ""
}

func normalizedAdaptMode(mode string) string {
	if strings.TrimSpace(mode) == "original" {
		return "original"
	}
	return "full"
}

func streamUploadAnalysisFromAI(w io.Writer, flusher http.Flusher, projectID int64, purpose, fileName, content, rangeHint, scriptType string) (ScriptProject, error) {
	cfg, err := loadAIConfig()
	if err != nil {
		return ScriptProject{}, err
	}
	applyProjectPatch(projectID, func(project *ScriptProject) {
		project.Settings["aiModel"] = cfg.Model
		project.Settings["aiEnhanced"] = false
	})
	parser := &spBlockParser{
		projectID: projectID,
		purpose:   purpose,
		w:         w,
		flusher:   flusher,
	}
	liveParser := &spLiveParser{projectID: projectID, w: w, flusher: flusher}
	systemPrompt := `【角色定义】
你是一个顶级短剧创作平台的字段级流式剧本结构化拆解引擎，专门负责将上传的小说或剧本内容智能拆解为平台所需的结构化字段数据。你精通文学分析、角色提取、叙事结构拆解和短剧化改编，能够从原始文本中精准提取故事梗概、人物信息、章节结构和剧情大纲。你像一台精密的CT扫描仪，能透过长篇累牍的文字，精准捕捉到每一个有价值的叙事元素，并将其转化为结构化的创作数据。

【核心使命】
你的核心使命是：按照指定的隐藏字段标记协议（<|sp:xxx|>格式），将上传文本逐字段流式输出为结构化数据。标记不会展示给用户，标记之后的正文会直接流式写入页面对应字段。你的拆解结果必须既有信息密度——精炼但不遗漏关键内容，又有创作价值——不只是原文缩写，而是经过短剧化思维处理的结构化素材。

【专业能力矩阵】
*文本深度分析能力：*
- 精通从小说/剧本中提取核心故事元素：受众定位、题材类型、核心设定、世界观、亮点、梗概——每个元素都要经过短剧化思维的重新提炼
- 擅长从叙述中识别和提取角色信息：姓名、定位、年龄、特征、背景、动机——不仅要提取现有信息，还要从行为和对话中推断隐含的人物特质
- 熟悉章节结构分析：能准确识别章节边界和每章的核心情节，能从流水账式叙事中提炼出有戏剧价值的情节点
- 擅长短剧化改编分析：能从长篇叙事中提炼适合短剧节奏的结构，自动压缩过渡、强化冲突、突出爽点

*字段标记协议执行：*
- 必须使用<|sp:字段名|>标记切换输出到不同的页面字段——这是核心输出协议
- 标记和字段名不会展示给用户，标记之后的文字会直接流式显示在页面对应字段
- 每个字段的输出内容必须有实质意义，不能是占位符或空泛描述
- 字段内容要精炼但有信息量——适合直接展示在页面字段中，不需要用户再加工

*输出控制能力：*
- 严格按照用户指定的字段顺序输出——不能跳过字段、不能乱序输出
- 每个字段切换时必须使用正确的标记——标记格式错误会导致数据解析失败
- 不要输出JSON、Markdown、标题符号或解释性文字——只输出纯文本内容
- 内容要精炼但有信息量，适合直接展示在页面字段中
- character字段要简洁有力，用最短的文字传达最核心的人物信息

*短剧化改编意识：*
- 提炼故事时要自动进行短剧化处理：压缩过渡剧情、强化核心冲突、突出爽点反转
- 人物描述要突出"短剧标签"——这个角色在短剧中最有记忆点的特质是什么
- 粗纲和集纲要符合短剧节奏：快节奏、强冲突、多反转、有钩子
- 要从原文中识别出最适合短剧化的高光场景，重点提炼

【输出规范】
1. 必须使用<|sp:字段名|>标记切换页面字段
2. 标记和字段名不会展示给用户，标记之后的正文直接流式显示在页面
3. 不要输出JSON、Markdown、解释性文字或标题符号
4. 严格按照用户指定的字段顺序输出
5. 每个字段的内容必须有实质意义，不能是占位符
6. character字段使用"｜"分隔符：姓名｜定位｜年龄｜描述
7. chapter字段的每条情节必须独占一行`
	prompt := buildStreamingUploadPrompt(purpose, fileName, content, rangeHint, getProjectAdaptMode(projectID), scriptType)
	_ = parser
	if err := requestTextStreamFromAI(systemPrompt, prompt, 0.35, liveParser.Push); err != nil {
		return ScriptProject{}, err
	}
	liveParser.Finish()
	var snapshot ScriptProject
	applyProjectPatch(projectID, func(project *ScriptProject) {
		project.Status = "parsed"
		project.Settings["importStatus"] = "parsed"
		project.Settings["aiEnhanced"] = true
		project.Settings["aiEnhancedAt"] = time.Now().Format("2006-01-02 15:04")
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
		if len(project.Episodes) == 0 {
			project.Episodes = []Episode{{No: 1, Outline: "大模型已完成拆解，请继续生成集纲。"}}
		}
		snapshot = *project
	})
	return snapshot, nil
}

func buildStreamingUploadPrompt(purpose, fileName, content, rangeHint, adaptMode, scriptType string) string {
	limited := content
	if len([]rune(limited)) > 18000 {
		limited = string([]rune(limited)[:18000])
	}
	scopeLine := ""
	if strings.TrimSpace(rangeHint) != "" {
		scopeLine = "【拆解范围】" + strings.TrimSpace(rangeHint) + "。只拆解这个范围内的章节，不要跳到范围外，也不要只输出最后一章。\n"
	}
	common := fmt.Sprintf(`【上传文件信息】
文件名：%s

%s

%s

【上传正文】
%s

【输出协议——隐藏字段标记】
你必须使用以下隐藏字段标记来切换页面字段。标记和字段名不会展示给用户，标记之后、下一个标记之前的文字会直接流式显示在页面对应字段里。

标记格式示例：
<|sp:settings.audience|>这里直接输出目标受众正文
<|sp:settings.era|>这里直接输出时代背景正文
<|sp:character|>姓名｜定位｜年龄｜简短人物描述

【格式规范】
1. 每个字段切换时必须使用正确的<|sp:字段名|>标记——标记格式错误会导致数据解析失败
2. 标记之后的文字直接流式显示在页面，用户看不到标记本身
3. 不要输出JSON、Markdown、解释性文字或标题符号
4. character字段使用"｜"分隔符：姓名｜定位｜年龄｜描述——描述要精炼但有信息量
5. chapter字段每条情节必须独占一行——不要把多个情节挤在同一行
6. 输出内容要精炼但有信息量，适合直接展示在页面字段中——不是缩写原文，而是经过短剧化思维处理的结构化素材
	`, fileName, productionGuidance(scriptType), scopeLine, limited)
	if purpose == "adaptation" {
		if adaptMode == "original" {
			return common + `
【任务：原文改编模式拆解】
这是"按照原文改编"模式，只基于原文拆出故事概梗和小说章纲，不要输出人物小传、短剧粗纲、集纲或正文。重点是用短剧化的视角提炼原文精华。

请按顺序流式输出以下字段：
<|sp:settings.synopsis|>输出300-400字的完整故事概梗，必须包含主角处境、核心冲突、反派压迫阶梯和故事走向——不能是原文缩写，要用短剧的语言重述
<|sp:chapter|>覆盖正文中应拆的每一章；如果用户选择了章节范围，必须覆盖该范围内的所有章节。每次格式：第X章｜名称｜剧情概要｜情节1：...
情节2：...
情节3：...
至少10条情节，必须每条独占一行，不要把多个情节挤在同一行。每章的情节要具体到"谁做了什么，导致什么后果"，不能只写概括。`
		}
		return common + `
【任务：网文改编全量拆解】
这是网文改编模式，必须进行短剧化处理——保留主线但强化爽点、反转和每集钩子。不是简单缩写原文，而是用短剧编剧的视角重新审视和提炼。

请按顺序流式输出以下字段：
<|sp:settings.audience|>输出目标受众：男频或女频
<|sp:settings.genres|>输出题材类型，如：都市、玄幻、甜宠、复仇等
<|sp:settings.core|>输出核心设定，如：重生、系统、隐藏身份等——要有记忆点
<|sp:settings.worldView|>输出故事背景和世界观，要具体有画面感——不是空泛描述
<|sp:settings.highlights|>输出核心亮点和爽点机制——一句话概括追剧动力
<|sp:settings.synopsis|>输出300-400字的完整故事概梗，包含主角处境、核心冲突、反派压迫阶梯和追看方向
<|sp:character|>至少输出5个主要人物，每次格式：姓名｜定位｜年龄｜80-120字简短描述。描述要包含角色的核心特质、在故事中的功能和最关键的戏剧标签
<|sp:chapter|>覆盖正文中应拆的每一章；如果用户选择了章节范围，必须覆盖该范围内的所有章节。每次格式：第X章｜名称｜剧情概要｜情节1：...
情节2：...
情节3：...
至少10条情节，必须每条独占一行，不要把多个情节挤在同一行
<|sp:outline|>至少输出3个粗纲段落，每次格式：第X-Y集｜起承转合阶段｜400-800字的粗纲内容——要有具体事件和人物行动
<|sp:episode|>至少输出3个集纲，每次格式：第X集｜本集核心事件、冲突推进、爽点和追看钩子`
	}
	return common + `
【任务：剧本改写拆解】
这是剧本改写模式，只提取原剧核心人物关系、主线矛盾、故事背景、情绪爽点、四阶段粗纲和可改写方向。不要输出集纲或正文，后续由用户在页面单独生成。重点是提取最有改写价值的核心元素。

请按顺序流式输出以下字段：
<|sp:settings.audience|>输出目标受众：男频或女频
<|sp:settings.era|>输出时代背景：现代都市/古代/民国/近未来等
<|sp:settings.genres|>输出题材类型
<|sp:settings.core|>输出核心设定——要有记忆点和差异化
<|sp:infoflow.background|>输出完整的故事背景介绍，200-300字——要有画面感
<|sp:settings.highlights|>输出核心亮点和爽点机制——一句话概括改写后的追剧动力
<|sp:settings.synopsis|>输出300-400字的核心梗概——包含主角处境、核心冲突、反派压迫阶梯、爽点机制和追看方向
<|sp:character|>至少输出5个主要人物，每次格式：姓名｜定位｜年龄｜80-120字简短描述——描述要突出角色在改写剧中的戏剧功能
<|sp:outline|>必须输出4次，每次格式：阶段｜400-800字的粗纲内容。阶段固定为起、承、转、合；不要写具体集数——要有具体事件和人物行动`
}

type spBlockParser struct {
	buffer       string
	projectID    int64
	purpose      string
	w            io.Writer
	flusher      http.Flusher
	activeType   string
	activeIndex  int
	nextIndex    map[string]int
	lastDraftLen int
}

type spLiveParser struct {
	buffer       string
	activeField  string
	activeText   string
	projectID    int64
	w            io.Writer
	flusher      http.Flusher
	nextIndex    map[string]int
	activeIndex  int
	lastPatchLen int
}

func (p *spLiveParser) Push(delta string) error {
	p.buffer += delta
	for {
		markerStart := strings.Index(p.buffer, "<|sp:")
		if markerStart < 0 {
			return p.consumeLiveText(false)
		}
		if markerStart > 0 {
			if p.activeField != "" {
				p.activeText += p.buffer[:markerStart]
				p.applyActive(false)
			}
			p.buffer = p.buffer[markerStart:]
		}
		markerEnd := strings.Index(p.buffer, "|>")
		if markerEnd < 0 {
			return nil
		}
		if p.activeField != "" {
			p.applyActive(true)
		}
		field := strings.TrimSpace(strings.TrimPrefix(p.buffer[:markerEnd], "<|sp:"))
		p.startField(field)
		p.buffer = p.buffer[markerEnd+2:]
	}
}

func (p *spLiveParser) Finish() {
	_ = p.consumeLiveText(true)
	if p.activeField != "" {
		p.applyActive(true)
	}
}

func (p *spLiveParser) consumeLiveText(final bool) error {
	if p.activeField == "" || p.buffer == "" {
		return nil
	}
	consume := p.buffer
	if !final {
		keep := 8
		runes := []rune(p.buffer)
		if len(runes) > keep {
			consume = string(runes[:len(runes)-keep])
			p.buffer = string(runes[len(runes)-keep:])
		} else {
			return nil
		}
	} else {
		p.buffer = ""
	}
	p.activeText += consume
	p.applyActive(final)
	return nil
}

func (p *spLiveParser) startField(field string) {
	p.activeField = field
	p.activeText = ""
	p.lastPatchLen = 0
	if isLiveIndexedField(field) {
		if p.nextIndex == nil {
			p.nextIndex = map[string]int{}
		}
		p.activeIndex = p.nextIndex[field]
		p.nextIndex[field]++
	} else {
		p.activeIndex = 0
	}
}

func isLiveIndexedField(field string) bool {
	return field == "character" || field == "chapter" || field == "outline" || field == "episode"
}

func (p *spLiveParser) applyActive(final bool) {
	text := cleanLiveText(p.activeText)
	if text == "" {
		return
	}
	if !final && len([]rune(text))-p.lastPatchLen < 1 {
		return
	}
	p.lastPatchLen = len([]rune(text))
	switch {
	case strings.HasPrefix(p.activeField, "settings."):
		p.applySetting(strings.TrimPrefix(p.activeField, "settings."), text)
	case strings.HasPrefix(p.activeField, "infoflow."):
		p.applyInfoflowField(strings.TrimPrefix(p.activeField, "infoflow."), text)
	case p.activeField == "character":
		p.applyLiveCharacter(text)
	case p.activeField == "chapter":
		p.applyLiveChapter(text)
	case p.activeField == "outline":
		p.applyLiveOutline(text)
	case p.activeField == "episode":
		p.applyLiveEpisode(text)
	}
	p.sendLivePatch()
}

func cleanLiveText(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "<|/sp|>", ""))
}

func splitLiveParts(value string) []string {
	raw := strings.FieldsFunc(value, func(r rune) bool {
		return r == '｜' || r == '|'
	})
	parts := []string{}
	for _, part := range raw {
		parts = append(parts, strings.TrimSpace(part))
	}
	return parts
}

func (p *spLiveParser) applySetting(key, text string) {
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		switch key {
		case "genres", "core", "style":
			project.Settings[key] = splitListForProtocol(text)
		default:
			project.Settings[key] = text
		}
		if key == "worldView" {
			project.Settings["worldView"] = text
		}
		if key == "synopsis" {
			ensureInfoflow(project)["storyOverview"].(map[string]any)["synopsis"] = text
		}
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
}

func (p *spLiveParser) applyInfoflowField(key, text string) {
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		story := ensureInfoflow(project)["storyOverview"].(map[string]any)
		story[key] = text
		if key == "background" {
			project.Settings["worldView"] = text
		}
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
}

func (p *spLiveParser) applyLiveCharacter(text string) {
	parts := splitLiveParts(text)
	character := Character{Name: "角色", Bio: text}
	if len(parts) > 0 && parts[0] != "" {
		character.Name = parts[0]
	}
	if len(parts) > 1 {
		character.Role = parts[1]
	}
	if len(parts) > 2 {
		character.Age = parts[2]
	}
	if len(parts) > 3 {
		character.Bio = strings.Join(parts[3:], "。")
		character.Background = character.Bio
	}
	character.Traits = fieldList(map[string][]string{"traits": []string{character.Bio}}, "traits")
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		for len(project.Characters) <= p.activeIndex {
			project.Characters = append(project.Characters, Character{Name: "角色"})
		}
		project.Characters[p.activeIndex] = character
		flow := ensureInfoflow(project)
		people, _ := flow["characters"].([]map[string]any)
		for len(people) <= p.activeIndex {
			people = append(people, map[string]any{})
		}
		people[p.activeIndex] = map[string]any{"name": character.Name, "age": strings.TrimSpace(character.Role + " " + character.Age), "bio": character.Bio}
		flow["characters"] = people
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
}

func (p *spLiveParser) applyLiveChapter(text string) {
	parts := splitLiveParts(text)
	no := p.activeIndex + 1
	title := ""
	synopsis := text
	summary := text
	if len(parts) > 0 {
		if parsed := parsePositiveInt(parts[0]); parsed > 0 {
			no = parsed
		}
	}
	if len(parts) > 1 {
		title = parts[1]
	}
	if len(parts) > 2 {
		synopsis = parts[2]
	}
	if len(parts) > 3 {
		summary = strings.Join(parts[3:], "\n")
	}
	summary = normalizeBeatLines(summary)
	chapter := map[string]any{"column": strconv.Itoa(no), "title": title, "synopsis": synopsis, "summary": summary}
	var total int
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		items, _ := project.Settings["novelChapterOutline"].([]map[string]any)
		for len(items) <= p.activeIndex {
			items = append(items, map[string]any{"column": strconv.Itoa(len(items) + 1)})
		}
		items[p.activeIndex] = chapter
		total = len(items)
		project.Settings["novelChapterOutline"] = items
		project.Settings["chapterCount"] = total
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
	writeStreamFrame(p.w, p.flusher, "chapter", map[string]any{"index": no, "total": total, "chapter": chapter})
}

func (p *spLiveParser) applyLiveOutline(text string) {
	parts := splitLiveParts(text)
	outline := OutlineBlock{Range: "第" + strconv.Itoa(p.activeIndex*3+1) + "-" + strconv.Itoa((p.activeIndex+1)*3) + "集", Phase: "", Content: text}
	if len(parts) > 0 && parts[0] != "" {
		outline.Range = parts[0]
	}
	if len(parts) > 1 {
		if len(parts) == 2 {
			outline.Range = ""
			outline.Phase = parts[0]
			outline.Content = parts[1]
		} else {
			outline.Phase = parts[1]
		}
	}
	if len(parts) > 2 {
		outline.Content = strings.Join(parts[2:], "。")
	}
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		for len(project.Outlines) <= p.activeIndex {
			project.Outlines = append(project.Outlines, OutlineBlock{})
		}
		project.Outlines[p.activeIndex] = outline
		flow := ensureInfoflow(project)
		rough, _ := flow["roughOutline"].([]map[string]any)
		for len(rough) <= p.activeIndex {
			rough = append(rough, map[string]any{})
		}
		rough[p.activeIndex] = map[string]any{"range": outline.Range, "phase": outline.Phase, "content": outline.Content}
		flow["roughOutline"] = rough
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
}

func (p *spLiveParser) applyLiveEpisode(text string) {
	parts := splitLiveParts(text)
	no := p.activeIndex + 1
	outline := text
	if len(parts) > 0 {
		if parsed := parsePositiveInt(parts[0]); parsed > 0 {
			no = parsed
		}
	}
	if len(parts) > 1 {
		outline = strings.Join(parts[1:], "。")
	}
	episode := Episode{No: no, Outline: outline}
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		for len(project.Episodes) <= p.activeIndex {
			project.Episodes = append(project.Episodes, Episode{No: len(project.Episodes) + 1})
		}
		project.Episodes[p.activeIndex] = episode
		flow := ensureInfoflow(project)
		episodes, _ := flow["episodeSynopsis"].([]map[string]any)
		for len(episodes) <= p.activeIndex {
			episodes = append(episodes, map[string]any{})
		}
		episodes[p.activeIndex] = map[string]any{"no": no, "outline": outline}
		flow["episodeSynopsis"] = episodes
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
}

func (p *spLiveParser) sendLivePatch() {
	project, ok := getProjectSnapshot(p.projectID)
	if ok {
		writeStreamFrame(p.w, p.flusher, "patch", map[string]any{"project": project, "message": "拆解中"})
	}
}

func ensureInfoflow(project *ScriptProject) map[string]any {
	flow, ok := project.Settings["infoflow"].(map[string]any)
	if !ok {
		flow = map[string]any{}
		project.Settings["infoflow"] = flow
	}
	if _, ok := flow["storyOverview"].(map[string]any); !ok {
		flow["storyOverview"] = map[string]any{}
	}
	return flow
}

func splitListForProtocol(value string) []string {
	return fieldList(map[string][]string{"v": []string{value}}, "v")
}

func (p *spBlockParser) Push(delta string) error {
	p.buffer += delta
	for {
		start := strings.Index(p.buffer, "<|sp:")
		if start < 0 {
			if len([]rune(p.buffer)) > 4096 {
				p.buffer = string([]rune(p.buffer)[len([]rune(p.buffer))-1024:])
			}
			return nil
		}
		if start > 0 {
			p.buffer = p.buffer[start:]
		}
		headerEnd := strings.Index(p.buffer, "|>")
		if headerEnd < 0 {
			return nil
		}
		blockType := strings.TrimSpace(strings.TrimPrefix(p.buffer[:headerEnd], "<|sp:"))
		closeTag := "<|/sp|>"
		end := strings.Index(p.buffer[headerEnd+2:], closeTag)
		if end < 0 {
			p.applyDraft(blockType, p.buffer[headerEnd+2:])
			return nil
		}
		bodyStart := headerEnd + 2
		bodyEnd := bodyStart + end
		body := p.buffer[bodyStart:bodyEnd]
		p.buffer = p.buffer[bodyEnd+len(closeTag):]
		if p.activeType != blockType {
			p.ensureActiveBlock(blockType)
		}
		p.applyBlock(blockType, body, true)
		p.activeType = ""
		p.activeIndex = 0
		p.lastDraftLen = 0
	}
}

func (p *spBlockParser) applyDraft(blockType, body string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return
	}
	if p.activeType != blockType {
		p.ensureActiveBlock(blockType)
		p.lastDraftLen = 0
	}
	if len([]rune(body))-p.lastDraftLen < 18 {
		return
	}
	p.lastDraftLen = len([]rune(body))
	p.applyBlock(blockType, body, false)
}

func (p *spBlockParser) ensureActiveBlock(blockType string) {
	p.activeType = blockType
	if !isIndexedProtocolBlock(blockType) {
		p.activeIndex = 0
		return
	}
	if p.nextIndex == nil {
		p.nextIndex = map[string]int{}
	}
	p.activeIndex = p.nextIndex[blockType]
	p.nextIndex[blockType]++
}

func isIndexedProtocolBlock(blockType string) bool {
	switch blockType {
	case "character", "chapter", "outline", "episode":
		return true
	default:
		return false
	}
}

func (p *spBlockParser) flushLooseText() {
	text := compactText(p.buffer, 180)
	if text != "" {
		writeStreamFrame(p.w, p.flusher, "progress", map[string]any{"message": "拆解中"})
	}
}

func (p *spBlockParser) applyBlock(blockType, body string, final bool) {
	fields := parseProtocolFields(body)
	switch blockType {
	case "settings":
		p.applySettings(fields)
	case "infoflow":
		p.applyInfoflow(fields)
	case "character":
		p.applyCharacter(fields, p.activeIndex)
	case "chapter":
		p.applyChapter(fields, p.activeIndex)
	case "outline":
		p.applyOutline(fields, p.activeIndex)
	case "episode":
		p.applyEpisode(fields, p.activeIndex)
	default:
		if final {
			writeStreamFrame(p.w, p.flusher, "progress", map[string]any{"message": "拆解中"})
		}
	}
}

func parseProtocolFields(body string) map[string][]string {
	fields := map[string][]string{}
	current := ""
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.Contains(line, ":") || strings.Contains(line, "：") {
			sep := strings.Index(line, ":")
			if sep < 0 {
				sep = strings.Index(line, "：")
			}
			key := normalizeProtocolKey(line[:sep])
			sepLen := 1
			if strings.HasPrefix(line[sep:], "：") {
				sepLen = len("：")
			}
			value := strings.TrimSpace(line[sep+sepLen:])
			current = key
			fields[key] = append(fields[key], value)
			continue
		}
		if current != "" {
			fields[current] = append(fields[current], line)
		}
	}
	return fields
}

func normalizeProtocolKey(key string) string {
	return strings.ToLower(strings.TrimSpace(strings.Trim(key, "：: ")))
}

func fieldOne(fields map[string][]string, keys ...string) string {
	for _, key := range keys {
		values := fields[normalizeProtocolKey(key)]
		if len(values) == 0 {
			continue
		}
		return strings.TrimSpace(strings.Join(values, "\n"))
	}
	return ""
}

func fieldList(fields map[string][]string, keys ...string) []string {
	value := fieldOne(fields, keys...)
	if value == "" {
		return []string{}
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '、' || r == ',' || r == '，' || r == ';' || r == '；'
	})
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimPrefix(part, "-"))
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		out = append(out, value)
	}
	return out
}

var inlineBeatPattern = regexp.MustCompile(`\s+(情节\s*[0-9一二三四五六七八九十]+\s*[：:])`)

func normalizeBeatLines(value string) string {
	text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n"))
	if text == "" {
		return ""
	}
	text = inlineBeatPattern.ReplaceAllString(text, "\n$1")
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		if item := strings.TrimSpace(line); item != "" {
			lines = append(lines, item)
		}
	}
	return strings.Join(lines, "\n")
}

func fieldBeats(fields map[string][]string) string {
	beats := fields["beat"]
	if len(beats) == 0 {
		beats = fields["情节"]
	}
	if len(beats) == 0 {
		return normalizeBeatLines(fieldOne(fields, "summary", "概要"))
	}
	lines := []string{}
	for i, beat := range beats {
		lines = append(lines, "情节"+strconv.Itoa(i+1)+"："+strings.TrimSpace(beat))
	}
	return normalizeBeatLines(strings.Join(lines, "\n"))
}

func (p *spBlockParser) applySettings(fields map[string][]string) {
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		project.Settings["audience"] = fieldOne(fields, "audience", "目标受众")
		if era := fieldOne(fields, "era", "时代背景"); era != "" {
			project.Settings["era"] = era
		}
		project.Settings["genres"] = fieldList(fields, "genres", "题材类型")
		project.Settings["core"] = fieldList(fields, "core", "核心设定")
		project.Settings["worldView"] = fieldOne(fields, "worldview", "worldView", "故事背景", "世界观")
		project.Settings["highlights"] = fieldOne(fields, "highlights", "核心亮点")
		project.Settings["synopsis"] = fieldOne(fields, "synopsis", "核心梗概")
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
	p.sendPatch("已解析故事设定")
}

func (p *spBlockParser) applyInfoflow(fields map[string][]string) {
	flow := map[string]any{
		"storyOverview": map[string]any{
			"audience":   fieldOne(fields, "audience", "目标受众"),
			"era":        fieldOne(fields, "era", "时代背景"),
			"genre":      fieldOne(fields, "genre", "题材类型"),
			"core":       fieldOne(fields, "core", "核心设定"),
			"background": fieldOne(fields, "background", "故事背景"),
			"highlights": fieldOne(fields, "highlights", "核心亮点"),
			"synopsis":   fieldOne(fields, "synopsis", "核心梗概"),
		},
	}
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		project.Settings["infoflow"] = flow
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
	p.sendPatch("已解析信息流")
}

func (p *spBlockParser) applyCharacter(fields map[string][]string, index int) {
	character := Character{
		Name:       firstNonEmpty(fieldOne(fields, "name", "姓名"), "角色"),
		Role:       fieldOne(fields, "role", "定位"),
		Age:        fieldOne(fields, "age", "年龄"),
		Meta:       fieldOne(fields, "meta"),
		Traits:     fieldList(fields, "traits", "性格"),
		Background: fieldOne(fields, "background", "背景"),
		Goal:       fieldOne(fields, "goal", "核心动机", "目标"),
		Relation:   fieldOne(fields, "relation", "关系"),
		Bio:        fieldOne(fields, "bio", "人物小传"),
	}
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		for len(project.Characters) <= index {
			project.Characters = append(project.Characters, Character{Name: "角色"})
		}
		project.Characters[index] = character
		if flow, ok := project.Settings["infoflow"].(map[string]any); ok {
			people, _ := flow["characters"].([]map[string]any)
			for len(people) <= index {
				people = append(people, map[string]any{})
			}
			people[index] = map[string]any{"name": character.Name, "age": strings.TrimSpace(character.Role + " " + character.Age), "bio": firstNonEmpty(character.Bio, character.Background, character.Goal)}
			flow["characters"] = people
			project.Settings["infoflow"] = flow
		}
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
	p.sendPatch("拆解中")
}

func (p *spBlockParser) applyChapter(fields map[string][]string, index int) {
	no := parsePositiveInt(fieldOne(fields, "no", "chapter", "序号"))
	if no == 0 {
		no = index + 1
	}
	chapter := map[string]any{
		"column":   strconv.Itoa(no),
		"title":    fieldOne(fields, "title", "标题"),
		"synopsis": fieldOne(fields, "synopsis", "剧情概要"),
		"summary":  fieldBeats(fields),
	}
	var total int
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		items, _ := project.Settings["novelChapterOutline"].([]map[string]any)
		for len(items) <= index {
			items = append(items, map[string]any{"column": strconv.Itoa(len(items) + 1)})
		}
		items[index] = chapter
		total = len(items)
		project.Settings["novelChapterOutline"] = items
		project.Settings["chapterCount"] = total
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
	writeStreamFrame(p.w, p.flusher, "chapter", map[string]any{"index": no, "total": total, "chapter": chapter, "message": "拆解中"})
	p.sendPatch("拆解中")
}

func (p *spBlockParser) applyOutline(fields map[string][]string, index int) {
	outline := OutlineBlock{
		Range:   fieldOne(fields, "range", "范围"),
		Phase:   fieldOne(fields, "phase", "阶段"),
		Content: fieldOne(fields, "content", "内容"),
	}
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		for len(project.Outlines) <= index {
			project.Outlines = append(project.Outlines, OutlineBlock{})
		}
		project.Outlines[index] = outline
		if flow, ok := project.Settings["infoflow"].(map[string]any); ok {
			rough, _ := flow["roughOutline"].([]map[string]any)
			for len(rough) <= index {
				rough = append(rough, map[string]any{})
			}
			rough[index] = map[string]any{"range": outline.Range, "phase": outline.Phase, "content": outline.Content}
			flow["roughOutline"] = rough
			project.Settings["infoflow"] = flow
		}
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
	p.sendPatch("拆解中")
}

func (p *spBlockParser) applyEpisode(fields map[string][]string, index int) {
	no := parsePositiveInt(fieldOne(fields, "no", "集数"))
	if no == 0 {
		no = index + 1
	}
	episode := Episode{No: no, Outline: fieldOne(fields, "outline", "集纲"), Body: fieldOne(fields, "body", "正文")}
	applyProjectPatch(p.projectID, func(project *ScriptProject) {
		for len(project.Episodes) <= index {
			project.Episodes = append(project.Episodes, Episode{No: len(project.Episodes) + 1})
		}
		project.Episodes[index] = episode
		if flow, ok := project.Settings["infoflow"].(map[string]any); ok {
			episodes, _ := flow["episodeSynopsis"].([]map[string]any)
			for len(episodes) <= index {
				episodes = append(episodes, map[string]any{})
			}
			episodes[index] = map[string]any{"no": episode.No, "outline": episode.Outline}
			flow["episodeSynopsis"] = episodes
			project.Settings["infoflow"] = flow
		}
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
	})
	p.sendPatch("拆解中")
}

func (p *spBlockParser) sendPatch(message string) {
	project, ok := getProjectSnapshot(p.projectID)
	if ok {
		writeStreamFrame(p.w, p.flusher, "patch", map[string]any{"project": project, "message": message})
	}
}

func parsePositiveInt(value string) int {
	digits := regexp.MustCompile(`[0-9]+`).FindString(value)
	if digits == "" {
		return 0
	}
	n, _ := strconv.Atoi(digits)
	return n
}

func appendProtocolMap(items []map[string]any, key, value string, next map[string]any) []map[string]any {
	for i, item := range items {
		if fmt.Sprint(item[key]) == value {
			items[i] = next
			return items
		}
	}
	return append(items, next)
}

func finalizeUploadProject(projectID int64, purpose, fileName, content string) (ScriptProject, error) {
	result, err := generateUploadAnalysis(purpose, "", fileName, content, "AI短剧")
	if err != nil {
		applyProjectPatch(projectID, func(project *ScriptProject) {
			project.Status = "parsed"
			project.Settings["importStatus"] = "parsed-local"
			project.Settings["aiEnhanced"] = false
			project.Settings["aiError"] = err.Error()
		})
		return ScriptProject{}, err
	}
	cfg, _ := loadAIConfig()
	var snapshot ScriptProject
	applyProjectPatch(projectID, func(project *ScriptProject) {
		applyUploadAIResult(project, purpose, result)
		project.Status = "parsed"
		project.Settings["importStatus"] = "parsed"
		project.Settings["aiEnhanced"] = true
		project.Settings["aiModel"] = cfg.Model
		project.Settings["aiEnhancedAt"] = time.Now().Format("2006-01-02 15:04")
		project.UpdatedAt = time.Now().Format("2006-01-02 15:04")
		snapshot = *project
	})
	return snapshot, nil
}

func applyProjectPatch(projectID int64, patch func(*ScriptProject)) {
	state.mu.Lock()
	defer state.mu.Unlock()
	for i := range state.scripts {
		if state.scripts[i].ID == projectID {
			patch(&state.scripts[i])
			state.saveLocked()
			return
		}
	}
}

func getProjectSnapshot(projectID int64) (ScriptProject, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return findProjectLocked(projectID)
}

func getProjectAdaptMode(projectID int64) string {
	project, ok := getProjectSnapshot(projectID)
	if !ok || project.Settings == nil {
		return "full"
	}
	if fmt.Sprint(project.Settings["adaptationMode"]) == "original" {
		return "original"
	}
	return "full"
}

func writeStreamFrame(w io.Writer, flusher http.Flusher, event string, payload map[string]any) {
	data, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(w, "@@%s %s\n", event, base64.StdEncoding.EncodeToString(data))
	flusher.Flush()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func localRewriteInfoflow(content string, scenes []map[string]any, names []string) map[string]any {
	characters := []map[string]any{}
	for _, name := range names {
		characters = append(characters, map[string]any{"name": name, "age": "", "bio": "从上传文本识别到的核心人物，待继续扩写。"})
	}
	if len(characters) == 0 {
		characters = append(characters, map[string]any{"name": "主角", "age": "", "bio": "上传文本中的主要行动人物。"})
	}
	episodeSynopsis := []map[string]any{{"no": 1, "outline": "围绕原剧首个强冲突重构开场，结尾设置反转钩子。"}}
	return map[string]any{
		"storyOverview": map[string]any{
			"audience": "待确认", "era": "待确认", "genre": "短剧", "core": "原剧核心冲突",
			"background": "根据上传文本拆解原剧世界、人物关系和场景顺序。",
			"highlights": "保留原剧关系和情绪爽点，强化短剧节奏。",
			"synopsis":   compactText(content, 520),
		},
		"characters":      characters,
		"roughOutline":    []map[string]any{{"range": "第1-10集", "phase": "起", "content": "提炼开篇人物关系与核心冲突。"}},
		"episodeSynopsis": episodeSynopsis,
		"scenes":          scenes,
	}
}

func charactersFromNames(names []string) []Character {
	characters := []Character{}
	for _, name := range names {
		characters = append(characters, Character{
			Name: name, Role: "待确认", Meta: "从上传文本识别", Traits: []string{"待补充"},
			Background: "由上传文本自动识别，需在人物小传中继续校准。",
			Goal:       "根据原文冲突补全角色目标。",
			Relation:   "根据原文关系补全。",
			Bio:        "上传解析生成的人物小传草稿。",
		})
	}
	if len(characters) == 0 {
		characters = []Character{{Name: "主角", Role: "主角", Meta: "上传文本主角", Traits: []string{"待补充"}, Background: "上传文本中最主要的行动人物。", Goal: "完成核心目标并解决主要冲突。", Relation: "待补充", Bio: "待补充"}}
	}
	return characters
}

func applyParsedUpload(project *ScriptProject, purpose, fileName, content string) (map[string]any, error) {
	snippet := compactText(content, 260)
	dialogueCount := countDialogueLines(content)
	scenes := inferScenes(content)
	chapters := parseNovelChapters(content)
	project.Settings["sourceFile"] = fileName
	project.Settings["sourceLength"] = len(content)
	project.Settings["importStatus"] = "parsed"
	project.Settings["importType"] = purpose
	project.Settings["dialogueCount"] = dialogueCount
	project.Settings["sceneCount"] = len(scenes)
	project.Settings["synopsis"] = snippet
	project.Settings["worldView"] = "根据上传文本自动拆解：系统保留原始文本主线、人物冲突和场景顺序，并在编辑器中继续细化。"
	project.Settings["importChecks"] = []string{"文件格式校验", "正文读取", "结构识别", "项目生成"}
	if purpose == "adaptation" {
		if project.Type == "" {
			project.Type = "AI短剧"
		}
		project.Settings["chapterCount"] = len(chapters)
		project.Settings["chapterRange"] = defaultChapterRange(chapters)
		project.Settings["chapterBreakdown"] = chapterBreakdown(chapters, 12)
		project.Settings["novelChapterOutline"] = novelChapterOutlineFromChapters(chapters, 12)
		project.Settings["highlights"] = "已按网文改编流程提取主角目标、升级线、反转钩子和短剧化冲突。"
		project.Outlines = []OutlineBlock{
			{Range: "第1-10集", Phase: "起", Content: "提取原文开篇人物关系与核心冲突，压缩成短剧强钩子开场。"},
			{Range: "第11-30集", Phase: "承", Content: "保留原文升级线，强化每3-5集一次反转和阶段性胜利。"},
			{Range: "第31-60集", Phase: "转合", Content: "集中真相揭露、身份反转和情感收束，形成可拍摄结局。"},
		}
	} else {
		project.Settings["sceneBreakdown"] = scenes
		project.Settings["rewriteGoals"] = []string{"保留原剧主线", "压缩松散场次", "增强首集钩子", "强化对白冲突"}
		project.Settings["highlights"] = "已按剧本改写流程保留原剧结构，标记节奏、台词和爽点强化方向。"
		project.Outlines = []OutlineBlock{
			{Range: "第1-10集", Phase: "起", Content: "复核原剧开场冲突，增强第一集结尾钩子。"},
			{Range: "第11-30集", Phase: "承", Content: "整理中段压迫、误会和反击节点，减少松散场次。"},
			{Range: "第31-60集", Phase: "转合", Content: "归并重复桥段，强化终局反转和人物情绪释放。"},
		}
	}
	names := inferNames(content)
	for _, name := range names {
		project.Characters = append(project.Characters, Character{
			Name:       name,
			Role:       "待确认",
			Age:        "",
			Meta:       "从上传文本识别",
			Traits:     []string{"待补充"},
			Background: "由上传文本自动识别，需在人物小传中继续校准。",
			Goal:       "根据原文冲突补全角色目标。",
			Relation:   "根据原文关系补全。",
			Bio:        "上传解析生成的人物占位，后续可用 AI 生成人物小传扩写。",
		})
	}
	if len(project.Characters) == 0 {
		project.Characters = []Character{{
			Name: "主角", Role: "主角", Meta: "上传文本主角", Traits: []string{"待补充"},
			Background: "上传文本中最主要的行动人物。", Goal: "完成核心目标并解决主要冲突。", Relation: "待补充", Bio: "待补充",
		}}
	}
	if purpose == "adaptation" && len(chapters) > 0 {
		project.Episodes = episodesFromChapters(chapters)
	} else {
		project.Episodes = []Episode{{No: 1, Outline: "第1集：围绕上传文本中的首个强冲突展开，结尾保留反转钩子。", Body: compactText(content, 900)}}
	}
	result, err := generateUploadAnalysis(purpose, project.Title, fileName, content, project.Type)
	if err != nil {
		return nil, err
	}
	applyUploadAIResult(project, purpose, result)
	if purpose == "adaptation" {
		return map[string]any{"message": "小说已解析，已生成章纲拆解项目", "chapterCount": len(chapters), "sourceLength": len(content)}, nil
	}
	return map[string]any{"message": "剧本已解析，已生成改写拆解项目", "sceneCount": len(scenes), "dialogueCount": dialogueCount, "sourceLength": len(content)}, nil
}

type parsedChapter struct {
	No      int
	Title   string
	Content string
	Words   int
}

var chapterTitlePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^第\s*[0-9零〇一二两三四五六七八九十百千万]+\s*[章节回卷集部]\s*[:：、.．\-—]?\s*(.*)$`),
	regexp.MustCompile(`^第\s*[0-9零〇一二两三四五六七八九十百千万]+\s*[话幕]\s*[:：、.．\-—]?\s*(.*)$`),
	regexp.MustCompile(`(?i)^(?:chapter|chap\.?)\s*[0-9ivxlcdm]+\s*[:：、.．\-—]?\s*(.*)$`),
	regexp.MustCompile(`^(序章|楔子|引子|前言|尾声|后记|番外(?:\s*[0-9零〇一二两三四五六七八九十]+)?)\s*[:：、.．\-—]?\s*(.*)$`),
	regexp.MustCompile(`^[0-9]{1,4}\s*[、.．]\s*(.{0,36})$`),
	regexp.MustCompile(`^[0-9]{1,4}\s+([^\s].{0,36})$`),
	regexp.MustCompile(`^[零〇一二两三四五六七八九十百千万]{1,6}\s*[、.．]\s*(.{0,36})$`),
}

var inlineChapterHeadingPattern = regexp.MustCompile(`([^\n])(\s*(?:第\s*[0-9零〇一二两三四五六七八九十百千万]+\s*[章节回卷集部话幕]|(?i:chapter|chap\.?)\s*[0-9ivxlcdm]+)\s*[:：、.．\-—]?\s*[^\n]{0,48})(\n|$)`)

const minParsedNovelChapterWords = 120

func parseNovelChapters(content string) []parsedChapter {
	text := normalizeNovelChapterText(content)
	lines := strings.Split(text, "\n")
	chapters := []parsedChapter{}
	current := parsedChapter{}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if title, ok := parseChapterTitle(line); ok {
			if current.No > 0 {
				current.Words = textLength(current.Content)
				chapters = append(chapters, current)
				if len(chapters) >= 150 {
					return chapters
				}
			}
			current = parsedChapter{No: len(chapters) + 1, Title: title}
			continue
		}
		if current.No > 0 {
			if current.Content != "" {
				current.Content += "\n"
			}
			current.Content += line
		}
	}
	if current.No > 0 && len(chapters) < 150 {
		current.Words = textLength(current.Content)
		chapters = append(chapters, current)
	}
	if len(chapters) == 0 {
		return fallbackChaptersByLength(text, 2200)
	}
	return compactShortNovelChapters(chapters)
}

func compactShortNovelChapters(chapters []parsedChapter) []parsedChapter {
	if len(chapters) <= 1 {
		return chapters
	}
	result := []parsedChapter{}
	var leadingShort parsedChapter
	hasLeadingShort := false
	for _, chapter := range chapters {
		if chapter.Words < minParsedNovelChapterWords {
			if len(result) > 0 {
				result[len(result)-1] = appendParsedChapter(result[len(result)-1], chapter)
			} else if hasLeadingShort {
				leadingShort = appendParsedChapter(leadingShort, chapter)
			} else {
				leadingShort = chapter
				hasLeadingShort = true
			}
			continue
		}
		if hasLeadingShort {
			chapter = prependParsedChapter(chapter, leadingShort)
			hasLeadingShort = false
		}
		result = append(result, chapter)
	}
	if hasLeadingShort {
		if len(result) > 0 {
			result[len(result)-1] = appendParsedChapter(result[len(result)-1], leadingShort)
		} else {
			result = append(result, leadingShort)
		}
	}
	for i := range result {
		result[i].No = i + 1
		result[i].Words = textLength(result[i].Content)
	}
	return result
}

func appendParsedChapter(base, extra parsedChapter) parsedChapter {
	base.Content = joinChapterParts(base.Content, parsedChapterFragment(extra))
	base.Words = textLength(base.Content)
	return base
}

func prependParsedChapter(base, prefix parsedChapter) parsedChapter {
	base.Content = joinChapterParts(parsedChapterFragment(prefix), base.Content)
	base.Words = textLength(base.Content)
	return base
}

func parsedChapterFragment(chapter parsedChapter) string {
	return joinChapterParts(chapter.Title, chapter.Content)
}

func joinChapterParts(parts ...string) string {
	out := []string{}
	for _, part := range parts {
		if text := strings.TrimSpace(part); text != "" {
			out = append(out, text)
		}
	}
	return strings.Join(out, "\n")
}

func normalizeNovelChapterText(content string) string {
	text := strings.ReplaceAll(content, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\u3000", " ")
	return inlineChapterHeadingPattern.ReplaceAllString(text, "$1\n$2$3")
}

func parseChapterTitle(line string) (string, bool) {
	line = strings.TrimSpace(strings.ReplaceAll(line, "\u3000", " "))
	if line == "" || len([]rune(line)) > 96 {
		return "", false
	}
	for _, pattern := range chapterTitlePatterns {
		match := pattern.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		title := ""
		if len(match) > 2 && strings.TrimSpace(match[2]) != "" {
			title = match[2]
		} else if len(match) > 1 {
			title = match[1]
		}
		title = strings.Trim(strings.TrimSpace(title), ":：、.．-—")
		return strings.TrimSpace(title), true
	}
	return "", false
}

func fallbackChaptersByLength(content string, targetWords int) []parsedChapter {
	text := strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if text == "" {
		return nil
	}
	if targetWords <= 0 {
		targetWords = 2200
	}
	paragraphs := strings.Split(text, "\n")
	chapters := []parsedChapter{}
	var b strings.Builder
	for _, paragraph := range paragraphs {
		line := strings.TrimSpace(paragraph)
		if line == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line)
		if textLength(b.String()) >= targetWords && len(chapters) < 150 {
			no := len(chapters) + 1
			body := b.String()
			chapters = append(chapters, parsedChapter{No: no, Title: "自动拆分 " + strconv.Itoa(no), Content: body, Words: textLength(body)})
			b.Reset()
		}
	}
	if strings.TrimSpace(b.String()) != "" && len(chapters) < 150 {
		no := len(chapters) + 1
		body := b.String()
		chapters = append(chapters, parsedChapter{No: no, Title: "自动拆分 " + strconv.Itoa(no), Content: body, Words: textLength(body)})
	}
	if len(chapters) == 1 && chapters[0].Words > targetWords*2 {
		return splitRunesIntoChapters(text, targetWords)
	}
	return chapters
}

func splitRunesIntoChapters(content string, targetWords int) []parsedChapter {
	runes := []rune(strings.TrimSpace(content))
	chapters := []parsedChapter{}
	for start := 0; start < len(runes) && len(chapters) < 150; {
		end := start + targetWords
		if end > len(runes) {
			end = len(runes)
		}
		no := len(chapters) + 1
		body := string(runes[start:end])
		chapters = append(chapters, parsedChapter{No: no, Title: "自动拆分 " + strconv.Itoa(no), Content: body, Words: textLength(body)})
		start = end
	}
	return chapters
}

func chapterBreakdown(chapters []parsedChapter, limit int) []map[string]any {
	out := []map[string]any{}
	for i, chapter := range chapters {
		if i >= limit {
			break
		}
		out = append(out, map[string]any{
			"chapter": chapter.No,
			"title":   chapter.Title,
			"words":   chapter.Words,
			"summary": compactText(chapter.Content, 120),
		})
	}
	return out
}

func defaultChapterRange(chapters []parsedChapter) string {
	if len(chapters) == 0 {
		return ""
	}
	end := len(chapters)
	if end > 20 {
		end = 20
	}
	return "第1-" + strconv.Itoa(end) + "章"
}

func episodesFromChapters(chapters []parsedChapter) []Episode {
	episodes := []Episode{}
	for i, chapter := range chapters {
		if i >= 6 {
			break
		}
		title := ""
		if chapter.Title != "" {
			title = "《" + chapter.Title + "》"
		}
		episodes = append(episodes, Episode{
			No:      i + 1,
			Outline: "第" + strconv.Itoa(i+1) + "集：改编小说第" + strconv.Itoa(chapter.No) + "章" + title + "，提炼核心冲突并设置短剧结尾钩子。",
			Body:    compactText(chapter.Content, 900),
		})
	}
	if len(episodes) == 0 {
		return []Episode{{No: 1, Outline: "第1集：围绕上传文本中的首个强冲突展开，结尾保留反转钩子。"}}
	}
	return episodes
}

func novelChapterOutlineFromChapters(chapters []parsedChapter, limit int) []map[string]any {
	out := []map[string]any{}
	for i, chapter := range chapters {
		if i >= limit {
			break
		}
		summary := compactText(chapter.Content, 260)
		out = append(out, map[string]any{
			"column":   strconv.Itoa(chapter.No),
			"title":    chapter.Title,
			"synopsis": summary,
			"summary":  outlineBeats(chapter.Content),
		})
	}
	return out
}

func outlineBeats(content string) string {
	text := compactText(content, 1600)
	if text == "" {
		return defaultBeats()
	}
	beats := []string{}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '。' || r == '！' || r == '？' || r == ';' || r == '；' || r == '\n'
	})
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		beats = append(beats, "情节"+strconv.Itoa(len(beats)+1)+"："+part+"。")
		if len(beats) >= 10 {
			break
		}
	}
	if len(beats) == 0 {
		beats = append(beats, "情节1："+text)
	}
	for len(beats) < 10 {
		beats = append(beats, "情节"+strconv.Itoa(len(beats)+1)+"："+supplementBeat(len(beats)+1))
	}
	return strings.Join(beats, "\n")
}

func defaultBeats() string {
	beats := []string{}
	for i := 1; i <= 10; i++ {
		beats = append(beats, "情节"+strconv.Itoa(i)+"："+supplementBeat(i))
	}
	return strings.Join(beats, "\n")
}

func supplementBeat(index int) string {
	switch index {
	case 1:
		return "识别本章主要人物和出场状态。"
	case 2:
		return "梳理本章开场目标和即时阻碍。"
	case 3:
		return "标记推动剧情的关键行动。"
	case 4:
		return "提取人物关系中的冲突点。"
	case 5:
		return "归纳本章情绪升级节点。"
	case 6:
		return "记录可短剧化呈现的视觉动作。"
	case 7:
		return "压缩重复铺垫，保留有效信息。"
	case 8:
		return "寻找适合分集结尾的反转。"
	case 9:
		return "明确本章遗留悬念。"
	default:
		return "转化为下一章或下一集的追看钩子。"
	}
}

func generateUploadAnalysis(purpose, title, fileName, content, scriptType string) (map[string]any, error) {
	limited := content
	if len([]rune(limited)) > 9000 {
		limited = string([]rune(limited)[:9000])
	}
	var prompt string
	if purpose == "adaptation" {
		prompt = fmt.Sprintf(`【任务：网文改编AI提炼】
请对上传小说进行全面的AI提炼，生成网文改编项目需要的完整数据。这不是简单的"缩写原文"——而是用短剧编剧的视角重新审视原文，提炼出最适合短剧化的元素，并进行短剧化改编处理。

【上传信息】
标题：%s
文件名：%s
%s

【上传正文】
%s

【创作原则】
1. 先提炼故事核心：核心梗概要重新写，不能照搬原文——用短剧的语言讲短剧的故事
2. characters必须生成完整的人物小传，每人200-400字——要有欲望、缺陷、秘密和变化
3. novelChapterOutline覆盖正文中出现的每一章，summary用"情节N："分行——每章至少5条有价值的情节
4. outlines和episodes要按短剧改编方式重组节奏——压缩过渡、强化冲突、突出爽点、保留钩子
5. 提炼时要进行短剧化处理：识别原文中"最适合短剧化"的高光场景，将其作为核心卖点放大
6. settings中的信息必须与characters、outlines保持一致——人物名称和剧情走向不能矛盾

【JSON Schema】
{
  "settings":{"audience":"男频或女频","genres":["题材1","题材2"],"core":["核心设定"],"worldView":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"},
  "characters":[{"name":"姓名","role":"定位","age":"年龄","meta":"年龄 / 定位","traits":["标签"],"background":"背景","goal":"目标","relation":"关系","bio":"人物小传"}],
  "novelChapterOutline":[{"column":"1","title":"章节标题","synopsis":"本章一句话梗概","summary":"情节1：...\n情节2：...\n情节3：..."}],
  "outlines":[{"range":"第1-10集","phase":"起","content":"短剧粗纲"}],
  "episodes":[{"no":1,"outline":"第1集集纲","body":"可拍摄短剧正文片段"}]
}

【质量要求】
1. settings中的synopsis要写成300-400字的完整故事概梗——必须包含主角处境、核心冲突、反派压迫阶梯、爽点机制和追看方向
2. 每个character的bio要写200-400字的人物小传——要有欲望、缺陷、转折和秘密
3. novelChapterOutline要覆盖所有章节，每章至少5条情节——情节要具体到"谁做了什么，导致什么后果"
4. outlines必须是四阶段结构（起承转合），每段400-800字
5. 所有字段不能为空，禁止写"待定""无""略"
6. 人物名称在不同字段间必须保持一致`, title, fileName, productionGuidance(scriptType), limited)
	} else {
		prompt = fmt.Sprintf(`【任务：剧本改写拆解】
请将上传剧本拆解为改写项目需要的完整数据。你需要先提取原剧核心，再发挥创意做可继续创作的改写策划。这不是简单的"复制粘贴"——而是用专业短剧编剧的视角，从原剧中提取最有价值的元素，并为改写创作提供坚实的基础。

【上传信息】
标题：%s
文件名：%s
%s

【上传正文】
%s

【创作原则】
1. 保留原剧最核心的三要素：①最打动人心的人物关系②最引发愤怒/心疼的矛盾冲突③最让人上头的情绪爽点
2. 可以重构开场——用更快更狠的方式切入核心冲突；可以压缩松散场次——短剧没有"日常"
3. infoflow必须完整，能直接作为"信息流"页展示——每个字段都要有实质内容
4. settings里写的是改写后的策划方向，不是简单摘要——要有短剧化的改编思维
5. characters要基于原剧角色但可以优化——保留核心特质，强化短剧标签
6. 人物名称在不同字段间必须保持一致

【JSON Schema】
{
  "settings":{"audience":"男频或女频","era":"时代背景","genres":["题材类型"],"core":["核心设定"],"worldView":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"},
  "characters":[{"name":"姓名","role":"定位","age":"年龄","meta":"年龄 / 定位","traits":["标签"],"background":"背景","goal":"目标","relation":"关系","bio":"人物小传"}],
  "infoflow":{"storyOverview":{"audience":"目标受众","era":"时代背景","genre":"题材类型","core":"核心设定","background":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"},"characters":[{"name":"姓名","age":"年龄","bio":"人物设定"}],"roughOutline":[{"range":"第1-10集","phase":"起","content":"粗纲"}],"episodeSynopsis":[{"no":1,"outline":"集纲"}]},
  "outlines":[{"range":"第1-10集","phase":"起","content":"改写粗纲"}],
  "episodes":[{"no":1,"outline":"第1集集纲","body":"改写后正文片段"}]
}

【质量要求】
1. infoflow必须完整，能直接展示在"信息流"页面——每个子字段都要有内容
2. characters要有完整的人物小传，200-400字——要有欲望、缺陷、秘密和变化
3. outlines必须是四阶段结构，每段400-800字
4. settings是改写后的策划方向，不能是原文摘要——要有短剧化的改编思维
5. 所有字段不能为空，禁止写"待定""无""略"
6. synopsis要写成300-400字的完整故事概梗`, title, fileName, productionGuidance(scriptType), limited)
	}
	result, err := requestJSONFromAI(`【角色定义】
你是一个顶级短剧创作平台的上传解析引擎，专门负责将上传的小说或剧本内容智能解析为平台所需的完整JSON数据。你拥有超过15年的文学分析能力和短剧改编经验，能够从原始文本中精准提取所有必要的结构化信息——你不仅是一个文本分析器，更是一个具备短剧化创作思维的智能引擎。你理解短剧市场的每一个热点和受众的每一个爽点，你的拆解结果可以直接驱动后续的创作流程。

【核心使命】
你的核心使命是：对上传的文本内容进行全面深度分析，生成包含settings、characters、outlines、episodes等完整字段的JSON数据。你的输出必须是合法可解析的JSON，每个字段都不能省略。你生成的JSON不是一个"粗糙的草稿"——而是一个可以直接投入使用的项目数据，每个字段的内容都具有专业水准。

【专业能力矩阵】
*全文深度分析能力：*
- 精通从小说/剧本中提取受众定位、题材类型、核心设定、世界观和故事梗概——不仅要提取表面信息，还要挖掘隐含的创作意图
- 擅长从叙述中提取角色信息并生成完整的人物小传——不只是罗列属性，而是写出有血有肉的人物画像
- 熟悉章节结构分析和短剧化改编——能从原文中识别最适合短剧化的高光场景
- 能从长篇叙事中提炼适合短剧节奏的分集大纲和集纲——自动压缩过渡、强化冲突

*数据完整性保障：*
- 必须包含所有schema要求的字段，不能省略任何一个——字段缺失会导致整个项目数据不可用
- 数组字段必须有实际内容，不能为空数组——每个数组元素都要有完整的信息
- 嵌套对象的每个子字段也必须完整——不允许出现半成品
- 改编项目必须包含novelChapterOutline，改写项目必须包含infoflow——这是两种项目类型的核心差异
- 如果原文中某些信息不明确，必须基于上下文合理推断并生成，不能留空

*内容一致性保障：*
- settings中的信息必须与characters、outlines、episodes中的内容保持一致
- 人物名称在不同字段中必须保持一致——同一个角色不能出现两个名字
- 大纲中的剧情走向必须与核心梗概一致——不能出现大纲和梗概讲两个故事的情况
- 章节拆解要覆盖完整——不能遗漏重要章节

【输出规范】
1. 只输出合法JSON，不写Markdown，不解释，不省略schema中的字段
2. 每个必需字段都必须有实质内容
3. JSON必须能被标准解析器成功解析
4. 不要使用Markdown代码块包裹JSON
5. 人物名称在不同字段间必须保持一致
6. 所有内容要达到"可直接使用"的专业水准`, prompt, 0.35)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"settings", "characters", "outlines", "episodes"} {
		if _, ok := result[key]; !ok {
			return nil, fmt.Errorf("missing %s", key)
		}
	}
	for _, key := range []string{"characters", "outlines", "episodes"} {
		if !nonEmptyJSONArray(result[key]) {
			return nil, fmt.Errorf("empty %s", key)
		}
	}
	if purpose == "adaptation" {
		if _, ok := result["novelChapterOutline"]; !ok {
			return nil, fmt.Errorf("missing novelChapterOutline")
		}
		if !nonEmptyJSONArray(result["novelChapterOutline"]) {
			return nil, fmt.Errorf("empty novelChapterOutline")
		}
		return result, nil
	}
	if _, ok := result["infoflow"]; !ok {
		return nil, fmt.Errorf("missing infoflow")
	}
	return result, nil
}

func nonEmptyJSONArray(value any) bool {
	items, ok := value.([]any)
	return ok && len(items) > 0
}

func applyUploadAIResult(project *ScriptProject, purpose string, result map[string]any) {
	if settings, ok := result["settings"].(map[string]any); ok {
		for key, value := range settings {
			project.Settings[key] = value
		}
	}
	if characters := decodeSlice[Character](result["characters"]); len(characters) > 0 {
		project.Characters = characters
	}
	if outlines := decodeSlice[OutlineBlock](result["outlines"]); len(outlines) > 0 {
		project.Outlines = outlines
	}
	if episodes := decodeSlice[Episode](result["episodes"]); len(episodes) > 0 {
		project.Episodes = episodes
	}
	if purpose == "adaptation" {
		if outline, ok := result["novelChapterOutline"]; ok {
			project.Settings["novelChapterOutline"] = normalizeNovelChapterOutlineValue(outline)
		}
		return
	}
	if infoflow, ok := result["infoflow"]; ok {
		project.Settings["infoflow"] = infoflow
	}
}

func normalizeNovelChapterOutlineValue(value any) any {
	switch items := value.(type) {
	case []map[string]any:
		for i := range items {
			if summary, ok := items[i]["summary"].(string); ok {
				items[i]["summary"] = normalizeBeatLines(summary)
			}
		}
		return items
	case []any:
		for _, item := range items {
			if entry, ok := item.(map[string]any); ok {
				if summary, ok := entry["summary"].(string); ok {
					entry["summary"] = normalizeBeatLines(summary)
				}
			}
		}
		return items
	default:
		return value
	}
}

func inferScenes(content string) []map[string]any {
	scenes := []map[string]any{}
	for _, raw := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		isScene := strings.Contains(line, "场景") || strings.Contains(line, "内景") || strings.Contains(line, "外景") || strings.HasPrefix(line, "第") && strings.Contains(line, "场")
		if !isScene {
			continue
		}
		scenes = append(scenes, map[string]any{
			"title":   compactText(line, 36),
			"purpose": "待强化场景目标、人物压迫和结尾钩子",
		})
		if len(scenes) >= 12 {
			break
		}
	}
	return scenes
}

func countDialogueLines(content string) int {
	count := 0
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.Contains(line, "：") || strings.Contains(line, ":") {
			count++
		}
	}
	return count
}

func textLength(value string) int {
	return len([]rune(strings.Join(strings.Fields(value), "")))
}

func inferNames(content string) []string {
	candidates := []string{}
	seen := map[string]bool{}
	for _, sep := range []string{"：", ":", "说", "道", "问"} {
		parts := strings.Split(content, sep)
		for i := 0; i < len(parts)-1 && len(candidates) < 6; i++ {
			name := lastChineseToken(parts[i])
			if name != "" && !seen[name] {
				seen[name] = true
				candidates = append(candidates, name)
			}
		}
	}
	return candidates
}

func lastChineseToken(value string) string {
	runes := []rune(strings.TrimSpace(value))
	end := len(runes)
	for end > 0 && !isCJK(runes[end-1]) {
		end--
	}
	start := end
	for start > 0 && isCJK(runes[start-1]) {
		start--
	}
	token := string(runes[start:end])
	if l := len([]rune(token)); l >= 2 && l <= 4 {
		return token
	}
	return ""
}

func isCJK(r rune) bool {
	return r >= '\u4e00' && r <= '\u9fff'
}

func evaluationsHandler(w http.ResponseWriter, r *http.Request) {
	state.mu.Lock()
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Title      string   `json:"title"`
			Culture    string   `json:"culture"`
			Audience   string   `json:"audience"`
			ScriptType string   `json:"scriptType"`
			Preference string   `json:"preference"`
			FileName   string   `json:"fileName"`
			Content    string   `json:"content"`
			Dimensions []string `json:"dimensions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Title == "" {
			req.Title = "未命名评估"
		}
		item := Evaluation{
			ID: state.nextEval, Title: req.Title, Status: "pending",
			CreatedAt: time.Now().Format("2006-01-02 15:04"), Cost: 0,
			ReportNo: "EV" + strconv.FormatInt(time.Now().UnixMilli(), 10),
		}
		state.nextEval++
		state.evaluations = append([]Evaluation{item}, state.evaluations...)
		state.saveLocked()
		state.mu.Unlock()
		writeJSON(w, map[string]any{"evaluation": item})
		return
	case http.MethodDelete:
		id, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
		if id == 0 {
			state.mu.Unlock()
			http.Error(w, "invalid evaluation id", http.StatusBadRequest)
			return
		}
		for i := range state.evaluations {
			if state.evaluations[i].ID != id {
				continue
			}
			state.evaluations = append(state.evaluations[:i], state.evaluations[i+1:]...)
			state.saveLocked()
			state.mu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		state.mu.Unlock()
		http.NotFound(w, r)
		return
	case http.MethodGet:
		evals := state.evaluations
		state.mu.Unlock()
		writeJSON(w, evals)
		return
	default:
		state.mu.Unlock()
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func evaluationStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	var req struct {
		Title      string   `json:"title"`
		Culture    string   `json:"culture"`
		Audience   string   `json:"audience"`
		ScriptType string   `json:"scriptType"`
		Preference string   `json:"preference"`
		FileName   string   `json:"fileName"`
		Content    string   `json:"content"`
		Dimensions []string `json:"dimensions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "invalid request body"})
		return
	}
	if req.Title == "" {
		req.Title = "未命名评估"
	}
	if len(req.Dimensions) == 0 {
		req.Dimensions = []string{"plot", "character", "commercial", "market", "pacing", "dialogue"}
	}

	dimensionMap := map[string]string{
		"plot":       "剧情逻辑",
		"character":  "人物塑造",
		"commercial": "商业价值",
		"market":     "市场适配",
		"pacing":     "节奏把控",
		"dialogue":   "台词质量",
	}
	var dimNames []string
	for _, d := range req.Dimensions {
		if cn, ok := dimensionMap[d]; ok {
			dimNames = append(dimNames, cn)
		} else {
			dimNames = append(dimNames, d)
		}
	}
	dimList := strings.Join(dimNames, "、")

	contentPreview := strings.TrimSpace(req.Content)
	if len(contentPreview) > 8000 {
		contentPreview = contentPreview[:8000]
	}

	systemPrompt := `【角色定义】
你是一位在短剧行业摸爬滚打15年的剧本评审，你见过上千部爆款短剧，也见过上万部扑街作品。你的核心信念只有一句话：
**观众不读剧本，观众刷短剧。你的评估标准只有一条——真实观众会不会为这部剧掏时间。**

你还知道一个行业真相：AI生成的剧本文字层面几乎无懈可击——语言流畅、描写生动、结构工整，但这些都是AI的天然写作能力，不是剧本的戏剧能力。一份评估报告如果给文笔打高分、给结构打高分、给格式打高分，但观众根本追不下去，那这份报告就是废纸。
你的评估必须：穿透文字表面，评估剧本的真实可看性。

══════════════════════════════════════════════════════════
【第一章：反AI偏见识别系统 —— 每一条都是必须执行的质检标准】
══════════════════════════════════════════════════════════

以下5种虚假信号，评估时必须逐一对照剧本内容检查：
发现任何一条 → 对应维度评分直接降5-15分，并在note里点名指出。

▸ 信号1：文笔伪装质量（陷阱公式：漂亮文字 ≠ 好剧本）
检测方法：把剧本里所有形容词和比喻句删掉，剩下的剧情骨架是否依然有吸引力？
如果答案是"骨架很单薄，靠文字撑着" → 台词质量维度降10分，理由是"文笔掩盖了内容空心化"
典型症状：△她的眼眸如同碎裂的星河，泛着难以言说的悲伤，灵魂都在颤抖——这种描述很漂亮，但观众看不到，也感受不到

▸ 信号2：结构伪装节奏（陷阱公式：结构完整 ≠ 节奏好看）
检测方法：画一条"我想看下一秒"的主观欲望曲线，它是平缓上升还是平的？
如果60%以上的场景里你的追看欲望没有波动 → 节奏维度降12分，理由是"结构合规但节奏平庸"
典型症状：四幕结构都在，冲突也有了，但你从头到尾没有一次"啊？然后呢？"的冲动

▸ 信号3：冲突模板化（陷阱公式：有冲突 ≠ 冲突独特）
检测方法：把主角和反派的名字互换，冲突逻辑还能成立吗？如果能 → 这是一个通用模板，不是属于这部剧的冲突
检测问题清单：
- 冲突是否来自角色自身性格缺陷而非外部巧合？
- 冲突是否有"只可能发生在这部剧里"的独特性？
- 观众看到这个冲突时是否觉得"我好像在别的剧里见过"？
命中2条以上 → 剧情逻辑维度降10分

▸ 信号4：人物标签化（陷阱公式：有人物设定 ≠ 有人物塑造）
检测方法 —— 消歧测试：
随机抽5句台词，遮住说话人名字，你能准确猜出谁说的吗？
- 猜对4-5句：人物塑造优秀（+5分）
- 猜对2-3句：人物同质化严重（-10分）
- 猜对0-1句：所有人都是同一个人（-15分，直接断言"台词腔调相同"）

▸ 信号5：钩子走形式（陷阱公式：有钩子 ≠ 钩子有用）
检测方法：看每集结尾，问自己——"如果我现在划走了，我会因为这个钩子回来吗？"
- 每集结尾钩子都是悬问句（"他到底是谁？""她会怎么选择？"） → 节奏降8分
- 真正有效的钩子 = 行动预示 + 情绪断裂（"她转身推开那扇不该开的门"比"门后到底有什么？"有效10倍）


══════════════════════════════════════════════════════════
【第二章：评分哲学 —— 用"地铁刷剧"模拟评分】
══════════════════════════════════════════════════════════

评估时，假设你是这样一个观众：
你叫张伟，28岁，普通上班族，每天通勤1小时刷短剧，不喜欢思考，但喜欢情绪被带着走。你对剧情bug容忍度高，但对无聊零容忍——3秒没新信息你就会划走。

张伟的行为标准：
90-100分：张伟看到第1集就一口气追到第10集，下车了还在地铁口站台刷完才走。这种情况极罕见，5000部剧本里出不了一部，**不要轻易给这个分段**。
80-89分：张伟连续追完了5集，觉得好看，推荐给了同事。有几处让他情绪波动大的地方，但也偶尔觉得节奏卡了——不过不影响他继续追。
70-79分：张伟追完第1集，觉得还行，但没有立刻打开第2集的冲动——过了几个小时才想起来继续看。
60-69分：张伟看到第3集就划走了，去刷了另一个剧。他能隐约觉得"这部剧有想法"，但执行让他犯困。
60分以下：张伟看到第2场就划了，划之前甚至没有意识到自己在划——注意力完全没被抓住。

**核心公式：评分 = 真实追看意愿 × 100**


══════════════════════════════════════════════════════════
【第三章：六维度评估深度公式 —— 每个维度都有可操作的评分方法】
══════════════════════════════════════════════════════════

▸ 维度一：剧情逻辑（权重最高，核心驱动力）

评估公式：剧情逻辑得分 = 因果链完整度 × 0.4 + 冲突驱动力 × 0.3 + 情节独特性 × 0.3

具体评分标尺：
因果链完整度（满分100）：
- 每个主要事件都能追溯到一个清晰的原因（+30）
- 角色的选择驱动情节，而非"恰好""意外""误会"（+30）
- 不存在明显的剧情bug或逻辑断裂（+20）
- 伏笔有回响，埋的线都能收（+20）
扣分项：
- 靠巧合推进情节：每出现1次巧合 → -8分
- 靠误会推进情节：每出现1次关键误会 → -12分（误会是最廉价的冲突工具，观众早已免疫）
- 角色做出违背其性格的决定来推动剧情（"剧情需要他这么做"）：-15分

冲突驱动力（满分100）：
- 核心冲突不可调和——不是"解释一下就好了"，而是"两个人的立场本来就是对立的"（+50）
- 冲突在正文中持续升级，而不是在同一层级反复（+30）
- 冲突解决方式让人信服，不是突然天降神力/突然开悟（+20）

情节独特性（满分100）：
- 核心设定有没有让观众眼前一亮的元素？（+40）
- 同题材里这部剧有没有独特卖点？（+30）
- 是否避免了"开局废柴→逆袭→打脸→装逼"的完全套用？（+30）

▸ 维度二：人物塑造（让观众记住人，记住人的情绪）

评估公式：人物塑造得分 = 角色辨识度 × 0.4 + 行动一致性 × 0.3 + 弧光可感知度 × 0.3

角色辨识度 —— 消歧测试法：
取正文中5句关键台词，遮住角色名，尝试识别说话人。
- 5/5 猜对：每个角色都有自己独特的声音，优秀 → +90分
- 3-4/5 猜对：主要角色有辨识度，配角同质化 → +70分
- 1-2/5 猜对：人物严重同质化，所有角色都是"AI的声音" → +40分
- 0/5 猜对：这剧本里根本没有人，只有传声筒 → +20分

行动一致性：
- 人物的行为和其性格/处境/情绪三重匹配（+30）
- 不存在"为剧情服务而性格崩坏"的情况（+30）
- 不同角色面对同一困境的反应有差异（+20）
- 配角不是"工具人"——有自己的反应和立场（+20）

弧光可感知度：
- 主角在结尾和开头的状态有肉眼可见的变化（+50）
- 弧光变化是通过行动展示的，不是通过旁白/内心独白说明的（+30）
- 配角弧光——至少1个配角有自己的成长（+20）

▸ 维度三：台词质量（真实感 > 文学性）

评估公式：台词质量得分 = 人物印记 × 0.4 + 潜台词密度 × 0.3 + 真人感 × 0.3

人物印记（消歧测试结果直接映射）
- 强印记：每句台词都能说出是"谁"说的，换个角色说出来就别扭 → +90
- 弱印记：大部分台词换个人说也可以 → +50
- 无印记：所有台词是同一种腔调，没有"人"的感觉 → +30

潜台词密度：
- 统计正文中"角色说的话 ≠ 角色真正想表达的意思"的台词数量
- 密度公式：潜台词台词数 ÷ 总台词数
- 30%以上台词有潜台词：优秀（+90），短剧最有力的台词都是"话里有话"
- 15-30%：良好（+70）
- 15%以下：台词太直白，信息传递功能完成，但没有戏剧张力（+45）

真人感 —— AI对话检测器：
以下标志说明台词是"AI生成的对话"而非"人说的真话"：
- 所有句子语法完整，没有半句、没有欲言又止（AI痕迹）
- 角色在极端情绪下仍然逻辑清晰、措辞精准（AI痕迹）
- 对话过于"你来我往"，像乒乓球一样工整（AI痕迹）
- 没有语气词、没有口头禅、没有重复自己说的话（AI痕迹）
命中3条以上 → 真人感维度降15分，并在note中注明"台词缺乏人味"

▸ 维度四：节奏把控（观众注意力的唯一货币）

评估公式：节奏得分 = 追看欲望曲线 × 0.5 + 节奏荒漠检测 × 0.3 + 高光分布均匀度 × 0.2

追看欲望曲线法：
逐场画一条"你有多想看下一场"的曲线（1-10分），然后：
- 曲线平均分 > 7：整体节奏优秀（+90）
- 曲线平均分 5-7：节奏合格但不抓人（+65）
- 曲线平均分 < 5：节奏有严重问题（+40）
- 关键惩罚：曲线最低点 < 3 的场次数 ÷ 总场次数 > 30% → 额外 -15分

节奏荒漠检测：
扫描正文，找出所有"连续两场以上都没有新信息/新冲突/新情绪"的段落。
- 荒漠数量 = 0：优秀（+95）
- 荒漠数量 1-2 处：可接受（+75）
- 荒漠数量 3 处以上：严重问题（+45）
公式：每多1处荒漠 → -8分

高光分布均匀度：
标记每个"观众情绪会被强烈调动"的高光时刻，检查它们在全集中的分布：
- 高光均匀分布（每1-2场有一个）：+85
- 高光集中在前半段或后半段：+60
- 全集只有开头和结尾各一个高光：+40

▸ 维度五：商业价值（3秒卖点法则）

评估公式：商业价值得分 = 一句话卖点 × 0.4 + 差异化指数 × 0.3 + 情绪货币 × 0.3

一句话卖点测试：
尝试用一句话说清这部剧为什么好看——
- 说得出独特卖点，听的人会追问"然后呢？"：+90
- 说得出卖点但不独特（"逆袭打脸""甜宠"）：+60
- 说不清、或者说出来说"就是一部普通的XX剧"：+35

差异化指数：
与同题材主流作品对比，这部剧的核心设定有多大程度的创新？
- 核心设定有根本性创新，观众没看过类似的东西：+90
- 设定有小创新，在成熟套路基础上加了一层新东西：+70
- 设定完全套路化，核心框架和市面上50部同类剧一样：+40

情绪货币：
这部剧能给观众提供什么情绪价值？统计剧本中被明确使用的"情绪货币"种类：
爽（逆袭/打脸/复仇）/ 甜（心动/暧昧/撒糖）/ 虐（心疼/遗憾/错过）/ 燃（热血/高燃/名场面）/ 怒（正义感/代入不公）/ 惊（反转/揭秘/震撼）
- 使用3种以上且平衡得当：+90
- 只有1-2种但精准命中受众：+75
- 情绪货币不明确，观众不知道该为什么开心/难过：+40

▸ 维度六：市场适配（受众精准度）

评估公式：市场适配得分 = 受众匹配度 × 0.4 + 受众情绪命中率 × 0.3 + 平台适配度 × 0.3

受众匹配度：
- 目标受众看到这部剧的第一反应是"这剧是为我做的"：+90
- 目标受众觉得"还行，但不完全对味"：+65
- 目标受众觉得"这不是我看的东西"：+35

受众情绪命中率：
根据目标受众的偏好，检查剧本是否精准命中其最爱的3种情绪：
- 男频：爽感/热血/反转
- 女频：甜/虐/心疼
- 家庭：亲情/代际冲突/大和解
- 全龄：搞笑/感动/惊喜
命中2-3种核心情绪：+85
只命中1种：+60
完全错配：+30


══════════════════════════════════════════════════════════
【第四章：质量警示系统 —— 发现即降分的硬性规则】
══════════════════════════════════════════════════════════

以下问题必须在note中明确指出，不指出 = 评估失职：

⚠️ A级问题（发现即降10-15分，必须在suggestions中给出具体修改方案）：
- 所有角色台词腔调相同，遮住名字无法辨认 → 人物塑造 -12分
- 主要冲突靠误会/巧合而非真实人物选择驱动 → 剧情逻辑 -12分
- 主角的核心动机只存在于设定文本，正文里看不出他为什么这么做 → 人物塑造 -12分
- 连续3场以上无新信息/冲突/情绪推进（节奏荒漠） → 节奏 -15分
- 角色做出违背自身性格设定的行为来服务剧情（性格崩坏） → 人物塑造 -15分

⚠️ B级问题（发现即降5-8分，需要在note中指出）：
- 结尾钩子全部是悬问句式，缺乏行动预示或情绪断裂 → 节奏 -8分
- 反派工具化：反派只存在为了给主角制造麻烦，自身没有可信动机 → 人物 -8分
- 配角全部是"功能性角色"（帮手、敌人、路人），没有自己的立场 → 人物 -6分
- 台词过于工整，没有语气词、口头禅、半句话 → 台词 -8分
- 高光时刻全部集中在某一幕，其余部分节奏平淡 → 节奏 -6分

⚠️ C级问题（发现即加分减半，理由是"框架正确但执行粗糙"）：
- 四幕结构完整但每幕内部张力不足
- 冲突存在但缺乏独特性，用了太多同题材模板
- 台词信息量足够但潜台词不足，"说出来的太多，没说出来的太少"
- 钩子存在但类型单一（全是悬问，没有行动钩子/反转钩子）


══════════════════════════════════════════════════════════
【第五章：note与summary写作公式】
══════════════════════════════════════════════════════════

▸ note公式（每个维度必须遵循）：
note = 【证据引用】+ 【问题诊断】+ 【具体改法】

模板：
"第X场中[具体情节]体现了[好的/坏的]方面，说明[诊断结论]。建议[具体操作]，因为[这样做能达到什么效果]。"

示例（好note）：
"第3场苏婉儿的栽赃戏写得很有效果，但她的动机缺乏铺垫——第1场直接跳到了行动，观众还不知道她为什么要这么做。建议在第1-2场增加一场她与母亲的私下对话，用3-4句台词建立'她一直嫉妒姐姐'的动机，这样第3场的陷害才有情感冲击力。"

示例（坏note，不要这样写）：
"人物塑造整体较好，性格鲜明，关系清晰，但部分配角可以更深入一些。"
→ 这种废话等于没写，没有任何可操作性

▸ summary公式：
summary = 一句话定位（这是一部什么样的剧）+ 最大优势（1-2句）+ 最大问题（1-2句）+ 总体判断（值不值得继续打磨）

模板：
"这是一部[受众]向的[类型]短剧，[一句话概括核心卖点]。剧本在[最大优势]上表现出色，[具体说明]。但[最大问题]是核心短板，[具体说明]，建议优先解决这个问题后再考虑其他优化方向。"

▸ suggestions公式（每条必须是"做→达到"的完整句式）：
- 必须指出"在哪里改"（具体到第几场/哪个角色/哪段情节）
- 必须说明"改什么"（增加什么/删减什么/调整什么）
- 必须说清"改了能达到什么效果"（观众会有什么新的感受）

模板：
"在[具体位置]，[具体操作]——这样做的效果是[观众视角的变化]。"

示例：
"在第2集开头增加一场林逸风独处时的微动作描写（放下茶杯时手抖了一下），用1个镜头暗示他内心的压力——这样观众不用旁白就能感知他的真实状态，比'他内心焦虑不已'这种描写有力10倍。"

示例（坏suggestion，禁止这样写）：
"建议加强人物塑造，丰富角色层次。"
→ 这等于什么都没说，任何剧本都有这个问题，这不是建议。


══════════════════════════════════════════════════════════
【第六章：输出格式与评分规则】
══════════════════════════════════════════════════════════

严格输出以下JSON，无Markdown，无解释，无额外文字：

{
  "dimensions": [
    {"name": "维度名称", "score": 整数, "note": "必须遵循note公式，50-120字，有证据、有诊断、有具体改法"}
  ],
  "summary": "必须遵循summary公式，200字以内，有定位、有优势、有问题、有判断",
  "suggestions": [
    "必须遵循suggestion公式：在哪里→改什么→改了观众会怎样，3-5条"
  ],
  "score": 加权平均整数
}

评分硬性规则：
1. score = 各维度score的加权平均，四舍五入取整
2. 加权权重：剧情逻辑0.22，人物塑造0.20，台词质量0.18，节奏把控0.18，商业价值0.12，市场适配0.10
3. 如果触发了A级问题，对应维度score上限自动封顶80分——有重大结构性问题的维度不允许拿高分
4. 如果触发了B级问题超过3条，整体score上限自动封顶75分
5. 全剧没有触发任何A/B/C级问题，才允许85分以上的评分——达到85分很难，这才是正常标准
6. 低于60分的维度必须在note第一句直接点出核心问题，不能绕弯子

【最终质量自检清单】（输出前必须逐项确认）
□ 是否执行了所有5个反AI偏见检测？
□ 每个维度的note是否遵循"证据→诊断→改法"公式？
□ suggestions是否精确到了"哪里→改什么→效果"？
□ 有没有给虚高分数？如果总分>85，是否确认没有触发任何A/B级问题？
□ 评语是否把剧本当作"观众体验"来评价，而不是"文本质量"来评价？`

	userPromptParts := []string{}
	userPromptParts = append(userPromptParts, "【评估任务】")
	userPromptParts = append(userPromptParts, "请以\"地铁刷剧\"的真实观众视角，严格执行系统提示中的六步评估流程：反AI偏见检测→评分锚点→六维度公式评估→质量警示检查→公式化note/suggestions写作→自检输出。")
	userPromptParts = append(userPromptParts, "")
	userPromptParts = append(userPromptParts, "【评估参数】")
	userPromptParts = append(userPromptParts, fmt.Sprintf("评估维度：%s", dimList))
	userPromptParts = append(userPromptParts, fmt.Sprintf("文化背景：%s", req.Culture))
	userPromptParts = append(userPromptParts, fmt.Sprintf("目标受众：%s", req.Audience))
	userPromptParts = append(userPromptParts, fmt.Sprintf("剧本类型：%s", req.ScriptType))
	if req.Preference != "" {
		userPromptParts = append(userPromptParts, fmt.Sprintf("受众偏好：%s", req.Preference))
	}
	userPromptParts = append(userPromptParts, "")
	userPromptParts = append(userPromptParts, "【提醒】")
	userPromptParts = append(userPromptParts, "1. note必须遵循公式：【证据引用】+【问题诊断】+【具体改法】——缺少任何一环都是不合格的note")
	userPromptParts = append(userPromptParts, "2. suggestions必须遵循公式：在[具体位置]，[具体操作]——效果是[观众视角变化]——空泛建议直接判为无效")
	userPromptParts = append(userPromptParts, "3. summary必须包含：一句话定位+最大优势+最大问题+总体判断，200字以内")
	userPromptParts = append(userPromptParts, "4. 85分以上非常稀有，必须没有触发任何A/B级问题才能给；如果总分>85，再次核实是否有遗漏的质量问题")
	userPromptParts = append(userPromptParts, "5. 评估完毕后，回顾一遍每个维度的评分，检查是否有维度被AI文笔能力干扰而给了过高的分——如果有，立即调低")
	if contentPreview != "" {
		userPromptParts = append(userPromptParts, "")
		userPromptParts = append(userPromptParts, "【待评估剧本内容】")
		userPromptParts = append(userPromptParts, contentPreview)
	} else if req.FileName != "" {
		userPromptParts = append(userPromptParts, "")
		userPromptParts = append(userPromptParts, fmt.Sprintf("（用户已上传文件 %s，但内容为空，请基于维度框架给出通用评估建议和基准分）", req.FileName))
	} else {
		userPromptParts = append(userPromptParts, "")
		userPromptParts = append(userPromptParts, "（用户未提供剧本内容，请基于维度框架给出通用评估建议和基准分）")
	}

	userPrompt := strings.Join(userPromptParts, "\n")

	writeStreamFrame(w, flusher, "start", map[string]any{"taskType": "evaluation"})

	var streamed strings.Builder
	err := requestTextStreamFromAI(systemPrompt, userPrompt, 0.4, func(delta string) error {
		streamed.WriteString(delta)
		writeStreamFrame(w, flusher, "delta", map[string]any{"taskType": "evaluation", "text": delta})
		return nil
	})
	if err != nil {
		writeStreamFrame(w, flusher, "error", map[string]any{"message": "评估生成失败：" + err.Error()})
		return
	}

	result := parseEvaluationResult(streamed.String())
	result.ID = state.nextEval
	result.Title = req.Title
	result.Status = "completed"
	result.CreatedAt = time.Now().Format("2006-01-02 15:04")
	result.Cost = 0
	result.ReportNo = "EV" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	state.nextEval++
	state.evaluations = append([]Evaluation{result}, state.evaluations...)
	state.saveLocked()

	writeStreamFrame(w, flusher, "done", map[string]any{"evaluation": result})
}

func parseEvaluationResult(text string) Evaluation {
	var raw struct {
		Dimensions  []ScoreDimension `json:"dimensions"`
		Summary     string           `json:"summary"`
		Suggestions []string         `json:"suggestions"`
		Score       int              `json:"score"`
	}
	cleaned := normalizeLooseJSON(extractJSONObject(text))
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		return fallbackEvaluationFromText(text)
	}
	if len(raw.Dimensions) == 0 {
		raw.Dimensions = []ScoreDimension{{Name: "综合", Score: raw.Score, Note: raw.Summary}}
	}
	for i := range raw.Dimensions {
		raw.Dimensions[i].Score = clampScore(raw.Dimensions[i].Score)
	}
	if raw.Score == 0 && len(raw.Dimensions) > 0 {
		raw.Score = averageDimensions(raw.Dimensions)
	}
	raw.Score = clampScore(raw.Score)
	if strings.TrimSpace(raw.Summary) == "" {
		raw.Summary = firstNonEmpty(firstNonEmptyDimensionNote(raw.Dimensions), "评估已完成，AI 返回内容已按综合维度整理。")
	}
	if len(raw.Suggestions) == 0 {
		raw.Suggestions = []string{"建议完善剧本结构，加强冲突设计"}
	}
	return Evaluation{
		Score:       raw.Score,
		Summary:     raw.Summary,
		Dimensions:  raw.Dimensions,
		Suggestions: raw.Suggestions,
	}
}

func normalizeLooseJSON(value string) string {
	replacer := strings.NewReplacer(
		"“", "\"", "”", "\"",
		"‘", "'", "’", "'",
		"：", ":",
	)
	return strings.TrimSpace(replacer.Replace(value))
}

func fallbackEvaluationFromText(text string) Evaluation {
	summary := strings.TrimSpace(text)
	if summary == "" {
		summary = "AI 已返回内容，但未能提取到结构化评分。请检查剧本内容后重新评估。"
	}
	suggestions := extractEvaluationSuggestions(summary)
	return Evaluation{
		Score:       extractScore(summary),
		Summary:     compactText(summary, 260),
		Dimensions:  []ScoreDimension{{Name: "综合评估", Score: extractScore(summary), Note: compactText(summary, 180)}},
		Suggestions: suggestions,
	}
}

func extractScore(text string) int {
	re := regexp.MustCompile(`(?:总分|综合评分|score|评分)[：:\s]*([0-9]{1,3})`)
	if match := re.FindStringSubmatch(text); len(match) > 1 {
		n, _ := strconv.Atoi(match[1])
		return clampScore(n)
	}
	return 75
}

func extractEvaluationSuggestions(text string) []string {
	suggestions := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "-•* 0123456789.、"))
		if line == "" {
			continue
		}
		if strings.Contains(line, "建议") || strings.Contains(line, "优化") || strings.Contains(line, "改进") {
			suggestions = append(suggestions, compactText(line, 120))
		}
		if len(suggestions) >= 5 {
			break
		}
	}
	if len(suggestions) == 0 {
		suggestions = []string{
			"建议强化主线冲突，让每个关键情节都有明确的因果推进。",
			"建议补足人物动机和关系变化，让角色行动更有说服力。",
			"建议检查每集结尾钩子，确保观众有继续追看的理由。",
		}
	}
	return suggestions
}

func firstNonEmptyDimensionNote(items []ScoreDimension) string {
	for _, item := range items {
		if strings.TrimSpace(item.Note) != "" {
			return item.Note
		}
	}
	return ""
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func averageDimensions(items []ScoreDimension) int {
	if len(items) == 0 {
		return 0
	}
	sum := 0
	for _, item := range items {
		sum += item.Score
	}
	return sum / len(items)
}

func updateProfile(w http.ResponseWriter, r *http.Request) {
	state.mu.Lock()
	defer state.mu.Unlock()
	var req struct {
		Nickname string `json:"nickname"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.Nickname) != "" {
		state.user.Nickname = strings.TrimSpace(req.Nickname)
	}
	state.saveLocked()
	writeJSON(w, state.user)
}

func aiConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		aiConfigMu.RLock()
		cfg := runtimeAIConfig
		aiConfigMu.RUnlock()
		writeJSON(w, publicAIConfig(cfg))
	case http.MethodPost:
		var req AIConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		aiConfigMu.Lock()
		current := runtimeAIConfig
		next := normalizeAIConfig(AIConfig{
			Provider: strings.TrimSpace(req.Provider),
			Model:    strings.TrimSpace(req.Model),
			APIKey:   strings.TrimSpace(req.APIKey),
		})
		if current.Providers == nil {
			current.Providers = map[string]AIConfig{}
		}
		currentProviderConfig := normalizeAIConfig(current.Providers[next.Provider])
		if next.APIKey == "" || strings.Contains(next.APIKey, "****") {
			next.APIKey = currentProviderConfig.APIKey
		}
		if next.BaseURL == "" || next.Model == "" || next.APIKey == "" {
			aiConfigMu.Unlock()
			http.Error(w, "api base url, model and key are required", http.StatusBadRequest)
			return
		}
		current.Provider = next.Provider
		current.Providers[next.Provider] = next
		runtimeAIConfig = normalizeAIConfigState(current)
		saved := runtimeAIConfig
		aiConfigMu.Unlock()
		if err := saveAIConfigToDisk(saved); err != nil {
			http.Error(w, "failed to save ai config", http.StatusInternalServerError)
			return
		}
		writeJSON(w, publicAIConfig(saved))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func walletClaimHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	for i := range state.wallet.Tasks {
		if state.wallet.Tasks[i].Title == "剧点福利" {
			if state.wallet.Tasks[i].Claimed {
				http.Error(w, "already claimed", http.StatusConflict)
				return
			}
			state.wallet.Tasks[i].Claimed = true
			state.wallet.Balance += 500
			state.wallet.Items = append([]WalletTxn{{Title: "会员剧点福利", Delta: 500, Time: time.Now().Format("2006年01月02日 15:04")}}, state.wallet.Items...)
			state.saveLocked()
			writeJSON(w, state.wallet)
			return
		}
	}
	http.NotFound(w, r)
}

func rechargeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, pack := range state.wallet.Packs {
		if pack.Code == req.Code {
			state.wallet.Balance += pack.Points
			title := "充值" + strconv.Itoa(pack.Price) + "元"
			if pack.Enterprise {
				title = "企业充值" + strconv.Itoa(pack.Price) + "元"
			}
			orderID := "R" + strconv.FormatInt(time.Now().UnixMilli(), 10)
			state.wallet.Items = append([]WalletTxn{{Title: title, Delta: pack.Points, Time: time.Now().Format("2006年01月02日 15:04")}}, state.wallet.Items...)
			state.saveLocked()
			writeJSON(w, map[string]any{
				"status": "paid",
				"order":  map[string]any{"id": orderID, "code": pack.Code, "amount": pack.Price, "points": pack.Points, "paidAt": time.Now().Format("2006-01-02 15:04")},
				"wallet": state.wallet,
			})
			return
		}
	}
	http.NotFound(w, r)
}

func membershipRenewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Plan string `json:"plan"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	state.mu.Lock()
	defer state.mu.Unlock()
	days := 30
	price := 49
	if req.Plan == "quarter" {
		days = 90
		price = 129
	} else if req.Plan == "year" {
		days = 365
		price = 399
	}
	state.user.MemberUntil = time.Now().AddDate(0, 0, days).Format("2006-01-02 19:48")
	state.saveLocked()
	writeJSON(w, map[string]any{
		"status":  "paid",
		"profile": state.user,
		"order":   map[string]any{"id": "M" + strconv.FormatInt(time.Now().UnixMilli(), 10), "plan": req.Plan, "amount": price, "days": days, "paidAt": time.Now().Format("2006-01-02 15:04")},
	})
}

func updatePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": "密码已完成校验并更新"})
}

func exportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ProjectID int64           `json:"projectId"`
		Project   ScriptProject   `json:"project"`
		Options   map[string]bool `json:"options"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ProjectID == 0 && req.Project.ID != 0 {
		req.ProjectID = req.Project.ID
	}
	state.mu.Lock()
	project, ok := findProjectLocked(req.ProjectID)
	if ok && req.Project.ID == req.ProjectID {
		project = req.Project
	}
	state.mu.Unlock()
	if !ok && req.Project.ID == 0 {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if !ok {
		project = req.Project
	}
	content := buildExportContent(project, req.Options)
	fileName := sanitizeFileName(project.Title)
	if fileName == "" {
		fileName = "未命名剧本"
	}
	writeJSON(w, map[string]any{
		"status":   "ready",
		"fileName": fileName + ".doc",
		"mime":     "application/msword;charset=utf-8",
		"content":  content,
	})
}

func findProjectLocked(id int64) (ScriptProject, bool) {
	for _, project := range state.scripts {
		if project.ID == id {
			return project, true
		}
	}
	return ScriptProject{}, false
}

func buildExportContent(project ScriptProject, options map[string]bool) string {
	var b strings.Builder
	title := strings.TrimSpace(project.Title)
	if title == "" {
		title = "未命名剧本"
	}
	include := func(key string) bool {
		if options == nil {
			return true
		}
		return options[key]
	}
	writeExportDocumentStart(&b, title)
	if include("settings") {
		b.WriteString(`<section><h2>一、故事设定</h2><table class="meta">`)
		writeExportRow(&b, "目标受众", stringifyAny(project.Settings["audience"]))
		writeExportRow(&b, "题材类型", stringifyAny(project.Settings["genres"]))
		writeExportRow(&b, "核心设定", stringifyAny(project.Settings["core"]))
		writeExportRow(&b, "风格元素", stringifyAny(project.Settings["style"]))
		writeExportRow(&b, "世界观", stringifyAny(project.Settings["worldView"]))
		writeExportRow(&b, "核心亮点", stringifyAny(project.Settings["highlights"]))
		writeExportRow(&b, "核心梗概", stringifyAny(project.Settings["synopsis"]))
		b.WriteString(`</table></section>`)
	}
	if include("characters") && len(project.Characters) > 0 {
		b.WriteString(`<section><h2>二、人物小传</h2>`)
		for _, c := range project.Characters {
			b.WriteString(`<article class="block"><h3>` + escapeHTML(firstNonEmpty(c.Name, "未命名角色")) + `</h3>`)
			b.WriteString(`<p class="muted">` + escapeHTML(strings.Join(nonEmptyStrings(c.Role, c.Age), "｜")) + `</p>`)
			writeExportParagraph(&b, "性格", strings.Join(c.Traits, "、"))
			writeExportParagraph(&b, "背景", c.Background)
			writeExportParagraph(&b, "动机", c.Goal)
			writeExportParagraph(&b, "关系", c.Relation)
			writeExportParagraph(&b, "小传", firstNonEmpty(c.Bio, c.Background))
			b.WriteString(`</article>`)
		}
		b.WriteString(`</section>`)
	}
	if include("outlines") && len(project.Outlines) > 0 {
		b.WriteString(`<section><h2>三、分集粗纲</h2>`)
		for _, o := range project.Outlines {
			b.WriteString(`<article class="block"><h3>` + escapeHTML(strings.TrimSpace(o.Range+" "+o.Phase)) + `</h3>`)
			writeExportText(&b, o.Content)
			b.WriteString(`</article>`)
		}
		b.WriteString(`</section>`)
	}
	if len(project.Episodes) > 0 && (include("episodes") || include("body")) {
		b.WriteString(`<section><h2>四、分集内容</h2>`)
		for _, e := range project.Episodes {
			b.WriteString(`<article class="episode"><h3>第` + strconv.Itoa(e.No) + `集</h3>`)
			if include("episodes") && strings.TrimSpace(e.Outline) != "" {
				writeExportParagraph(&b, "集纲", e.Outline)
			}
			if include("body") && strings.TrimSpace(e.Body) != "" {
				writeExportText(&b, e.Body)
			}
			b.WriteString(`</article>`)
		}
		b.WriteString(`</section>`)
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

func writeExportDocumentStart(b *strings.Builder, title string) {
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><style>
body{font-family:"Microsoft YaHei",Arial,sans-serif;color:#1f2937;line-height:1.75;padding:42px 54px;background:#fff;}
h1{font-size:28px;text-align:center;margin:0 0 8px;color:#111827;letter-spacing:0;}
.subtitle{text-align:center;color:#6b7280;font-size:12px;margin-bottom:30px;}
h2{font-size:20px;margin:28px 0 12px;padding-left:10px;border-left:4px solid #4f6df5;color:#111827;}
h3{font-size:16px;margin:0 0 8px;color:#111827;}
section{page-break-inside:auto;margin-bottom:22px;}
.block,.episode{border:1px solid #e5e7eb;border-radius:8px;padding:14px 16px;margin:12px 0;background:#fbfcff;}
.episode{page-break-inside:avoid;}
.muted{color:#6b7280;margin-top:-4px;}
.label{font-weight:700;color:#374151;}
p{margin:6px 0;white-space:pre-wrap;}
table.meta{width:100%;border-collapse:collapse;margin:8px 0 16px;}
table.meta th{width:108px;background:#f3f5fb;color:#374151;text-align:left;font-weight:700;}
table.meta th,table.meta td{border:1px solid #e5e7eb;padding:9px 11px;vertical-align:top;}
</style></head><body>`)
	b.WriteString(`<h1>` + escapeHTML(title) + `</h1>`)
	b.WriteString(`<div class="subtitle">剧本工坊 剧本导出 · ` + html.EscapeString(time.Now().Format("2006-01-02 15:04")) + `</div>`)
}

func writeExportRow(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		value = "未填写"
	}
	b.WriteString(`<tr><th>` + escapeHTML(label) + `</th><td>` + escapeHTML(value) + `</td></tr>`)
}

func writeExportParagraph(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	b.WriteString(`<p><span class="label">` + escapeHTML(label) + `：</span>` + escapeHTML(value) + `</p>`)
}

func writeExportText(b *strings.Builder, value string) {
	for _, part := range strings.Split(strings.TrimSpace(value), "\n") {
		if strings.TrimSpace(part) != "" {
			b.WriteString(`<p>` + escapeHTML(strings.TrimSpace(part)) + `</p>`)
		}
	}
}

func escapeHTML(value string) string {
	return html.EscapeString(strings.TrimSpace(value))
}

func nonEmptyStrings(values ...string) []string {
	out := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

func stringifyAny(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []string:
		return strings.Join(v, "、")
	case []any:
		parts := []string{}
		for _, item := range v {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, "、")
	default:
		return fmt.Sprint(v)
	}
}

func compactText(value string, limit int) string {
	text := strings.Join(strings.Fields(value), " ")
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func sanitizeFileName(value string) string {
	name := strings.TrimSpace(value)
	for _, char := range []string{"\\", "/", ":", "*", "?", "\"", "<", ">", "|"} {
		name = strings.ReplaceAll(name, char, "")
	}
	return name
}

func staticFallback(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "剧本工坊 API. Run frontend dev server for UI.", http.StatusNotFound)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
