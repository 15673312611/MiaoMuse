package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	return filepath.Join("data", "storyplay.db")
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
	log.Println("StoryPlay clone API listening on :" + port)
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
		systemPrompt := "你是 StoryPlay 短剧人物小传创作师。按用户指定格式直接输出中文内容，不要 Markdown，不要 JSON，不要解释。"
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
		systemPrompt := "你是 StoryPlay 短剧分集大纲策划师。按用户指定格式直接输出中文内容，不要 Markdown，不要 JSON，不要解释。"
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
		systemPrompt := "你是 StoryPlay 短剧单集集纲策划师。按用户指定格式直接输出中文内容，不要 Markdown，不要 JSON，不要解释。"
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
		systemPrompt := "你是 StoryPlay 短剧正文创作师。按用户指定格式直接输出中文短剧正文，不要 Markdown，不要 JSON，不要解释。"
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
		systemPrompt := "你是 StoryPlay 短剧剧本创作平台的结构化生成引擎。必须只返回合法 JSON，不要 Markdown，不要解释。字段必须符合用户给出的 schema。"
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

	systemPrompt := "你是 StoryPlay 短剧策划师。按用户指定格式直接输出中文内容，不要 Markdown，不要 JSON，不要解释。"
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
	BaseURL string `json:"baseURL"`
	Model   string `json:"model"`
	APIKey  string `json:"apiKey"`
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
	result, err := requestJSONFromAI("你是 StoryPlay 短剧剧本创作平台的结构化生成引擎。必须只返回合法 JSON，不要 Markdown，不要解释。字段必须符合用户给出的 schema。", prompt, 0.8)
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
	url := strings.TrimRight(cfg.BaseURL, "/") + "/v1/chat/completions"
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
	url := strings.TrimRight(cfg.BaseURL, "/") + "/v1/chat/completions"
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
	cfg := runtimeAIConfig
	aiConfigMu.RUnlock()
	if aiConfigReady(cfg) {
		return cfg, nil
	}
	cfg = readAIConfigFromEnvAndFile()
	if aiConfigReady(cfg) {
		return cfg, nil
	}
	return cfg, fmt.Errorf("missing ai config; set API base url, model and key in settings")
}

func readAIConfigFromEnvAndFile() AIConfig {
	for _, path := range []string{aiConfigFile, filepath.Join("..", aiConfigFile)} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var saved AIConfig
		if json.Unmarshal(data, &saved) == nil && aiConfigReady(saved) {
			return saved
		}
	}
	cfg := AIConfig{
		BaseURL: strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")),
		Model:   strings.TrimSpace(os.Getenv("OPENAI_MODEL")),
		APIKey:  strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
	}
	if aiConfigReady(cfg) {
		return cfg
	}
	return cfg
}

func aiConfigReady(cfg AIConfig) bool {
	return strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "" && strings.TrimSpace(cfg.APIKey) != ""
}

func publicAIConfig(cfg AIConfig) map[string]any {
	return map[string]any{
		"baseURL":    strings.TrimSpace(cfg.BaseURL),
		"model":      strings.TrimSpace(cfg.Model),
		"configured": aiConfigReady(cfg),
		"apiKeySet":  strings.TrimSpace(cfg.APIKey) != "",
	}
}

func saveAIConfigToDisk(cfg AIConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
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
		return "制作方式：真人实拍。生成内容要优先考虑低成本可拍摄、有限场景、真实演员表演、台词调度、动作可执行、服化道易落地；避免大规模奇观、复杂特效、无法实拍的超现实镜头。"
	}
	return "制作方式：AI短剧。生成内容要强化视觉奇观、强情绪画面、可由AI视频生成的镜头描述、角色造型辨识度、反转爽点和高频钩子；允许更强风格化、幻想化和镜头冲击力。"
}

func buildTaskPrompt(project ScriptProject, taskType, userPrompt string) (string, error) {
	contextProject := taskContextProject(project, taskType)
	context, _ := json.Marshal(contextProject)
	base := fmt.Sprintf("项目快照 JSON：%s\n%s\n用户补充：%s\n", string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt))
	switch taskType {
	case "planning":
		if project.Source == "rewriting" {
			return base + `基于上传剧本的信息流和已填写的改写方向，生成 3 个“改写策划”候选。必须返回：
{"plans":[{"title":"《标题》","audience":"男频或女频","genres":["不超过3个"],"core":["不超过3个"],"style":["不超过5个"],"highlights":"改写后的强爽点","worldView":"改写后的故事背景","synopsis":"改写后的核心梗概"}]}
要求：保留原剧核心关系和冲突，但允许重构开场、爽点、反派压迫和短剧钩子；字段不能为空。`, nil
		}
		if project.Source == "adaptation" {
			return base + `基于小说提炼内容生成 3 个短剧改编策划候选。必须返回：
{"plans":[{"title":"《标题》","audience":"男频或女频","genres":["不超过3个"],"core":["不超过3个"],"style":["不超过5个"],"highlights":"短剧化强爽点","worldView":"改编后的故事背景","synopsis":"改编后的核心梗概"}]}
要求：保留原文主线，强化短剧节奏、人物欲望和每集钩子；字段不能为空。`, nil
		}
		return base + `生成 3 个短剧灵感策划候选，必须返回：
{"plans":[{"title":"《标题》","audience":"男频或女频","genres":["不超过3个"],"core":["不超过3个"],"style":["不超过5个"],"highlights":"一句强爽点","worldView":"完整世界观设定","synopsis":"核心梗概"}]}
要求：适合竖屏短剧，强反转、强钩子、可拍摄，字段不能为空。`, nil
	case "extract":
		if project.Source == "adaptation" {
			return base + `请对当前网文改编项目重新做 AI 提炼，必须返回：
{"settings":{"audience":"男频或女频","genres":["题材1","题材2"],"core":["核心设定"],"style":["风格"],"worldView":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"},"characters":[{"name":"姓名","role":"定位","age":"年龄","meta":"年龄 / 定位","traits":["标签"],"background":"背景","goal":"目标","relation":"关系","bio":"人物小传"}],"novelChapterOutline":[{"column":"1","title":"章节标题","synopsis":"本章一句话梗概","summary":"情节1：...\n情节2：..."}],"outlines":[{"range":"第1-10集","phase":"起","content":"短剧粗纲"}],"episodes":[{"no":1,"outline":"第1集集纲","body":""}]}
要求：提炼故事概梗、人物小传和短剧化结构，不照搬原文；字段不能为空。`, nil
		}
		return base + `请重新提炼当前项目核心信息，必须返回：
{"settings":{"audience":"男频或女频","genres":["题材1"],"core":["核心设定"],"style":["风格"],"worldView":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"},"characters":[{"name":"姓名","role":"定位","age":"年龄","meta":"年龄 / 定位","traits":["标签"],"background":"背景","goal":"目标","relation":"关系","bio":"人物小传"}],"outlines":[{"range":"第1-10集","phase":"起","content":"粗纲"}],"episodes":[{"no":1,"outline":"第1集集纲","body":""}]}
要求：字段不能为空。`, nil
	case "synopsis":
		return base + `只提炼当前项目的故事概梗，不要改人物小传、小说章纲、粗纲和集纲。必须返回：
{"settings":{"audience":"男频或女频","genres":["题材1","题材2"],"core":["核心设定"],"worldView":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"}}
要求：synopsis 写成完整故事概梗，包含主角处境、核心冲突、反派压力、爽点反转和短剧追看方向；字段不能为空。`, nil
	case "characters":
		return base + `基于故事设定生成 8-12 个主要人物小传，必须返回：
{"characters":[{"name":"姓名","role":"定位","age":"年龄","meta":"年龄 / 定位","traits":["3-5个性格标签"],"background":"背景经历","goal":"核心动机","relation":"人物关系","bio":"综合小传"}]}
要求：第一个角色必须是主角；反派、助攻、亲属/权力角色齐全；字段不能为空。`, nil
	case "outline":
		return base + `只根据故事概梗和人物小传，AI全新生成四阶段粗纲；不要参考小说章纲；不要生成单集集纲。必须返回：
{"outlines":[{"range":"第1-N集","phase":"起","content":"只写开端阶段的大致剧情，不要写单集标题"},{"range":"第N-N集","phase":"承","content":"只写推进阶段的大致剧情，不要写单集标题"},{"range":"第N-N集","phase":"转","content":"只写反转升级阶段的大致剧情，不要写单集标题"},{"range":"第N-N集","phase":"合","content":"只写收束爆发阶段的大致剧情，不要写单集标题"}]}
要求：必须只有四个粗纲，phase 固定为“起、承、转、合”；range 必须按用户要求的总集数规划为连续的 x-x 区间；粗纲 content 只描述阶段剧情，不要堆集数明细；不要返回 episodes 字段。`, nil
	case "episode":
		return base + `基于现有故事设定、人物小传和四阶段粗纲，为用户指定范围补齐或优化集纲。必须返回：
{"episodes":[{"no":1,"outline":"本集核心事件、人物目标、冲突推进、关键反转或爽点、结尾追看钩子","body":""}]}
要求：no 必须对应用户要求的集数范围；“起、承、转、合”只代表四阶段粗纲的大方向，不要在每一集里套写起承转合结构，不要输出“本集起/承/转/合”小标题；每集集纲都要贴合当前阶段粗纲，写清本集发生什么、谁推动、冲突如何升级、有什么反转或爽点、结尾为什么让人追下一集；不要改正文 body。`, nil
	case "body":
		if fmt.Sprint(project.Settings["adaptationMode"]) == "original" {
			return base + `按照原文改编模式：直接根据小说章纲和故事概梗生成或补齐短剧正文；不要另行改写人物小传或分集大纲。必须返回：
{"episodes":[{"no":1,"body":"△1-1 日 内 场景名\n人物：角色A、角色B\n△动作描写\n角色A：（语气）台词"}]}
要求：必须只返回 JSON。body 字符串必须使用这种剧本格式：每场以“△集数-场次 日/夜 内/外 场景名”开头；下一行写“人物：”；动作行以“△”开头；台词行写“角色：（语气）台词”。正文顺序贴合小说章纲，保留原文主线冲突，短剧化表达。`, nil
		}
		return base + `为项目中已有 episodes 生成或补齐短剧正文；优先处理没有 body 的集数。必须返回：
{"episodes":[{"no":1,"body":"△1-1 日 内 场景名\n人物：角色A、角色B\n△动作描写\n角色A：（语气）台词"}]}
要求：必须只返回 JSON。body 字符串必须使用这种剧本格式：每场以“△集数-场次 日/夜 内/外 场景名”开头；下一行写“人物：”；动作行以“△”开头；台词行写“角色：（语气）台词”。只根据故事概梗、人物小传和已有分集集纲生成，不要参考小说章纲；no 必须对应已有集数；正文可拍摄，节奏快，台词短。生成多集时必须按集数顺序连续生成，后一集要承接上一集正文结尾的人物状态、情绪和未解决冲突，避免割裂；每集结尾留钩子。`, nil
	default:
		return "", fmt.Errorf("unsupported task type %s", taskType)
	}
}

func buildPlanningStreamPrompt(project ScriptProject, userPrompt string) string {
	contextProject := taskContextProject(project, "planning")
	context, _ := json.Marshal(contextProject)
	return fmt.Sprintf(`项目快照 JSON：%s
%s
用户选择和输入：
%s

请生成 3 个完全不同的短剧策划方案，边思考边输出，但最终必须严格使用下面的文本结构。每个策划都要完整，不要写 JSON，不要 Markdown。

【策划1】
标题：短剧标题
目标受众：男频或女频
题材类型：题材1、题材2
时代背景：现代都市/古代/民国/近未来等
核心设定：核心1、核心2
核心亮点：一句强钩子亮点
世界观：故事背景/人物处境/核心规则
核心梗概：完整故事梗概，包含主角处境、核心冲突、反派压力、爽点反转和追看方向

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

要求：三套策划必须差异明显；核心梗概写得可直接进入故事概梗页；字段不能为空。`, string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt))
}

func buildCharacterStreamPrompt(project ScriptProject, userPrompt string) string {
	contextProject := taskContextProject(project, "characters")
	context, _ := json.Marshal(contextProject)
	return fmt.Sprintf(`项目快照 JSON：%s
%s
用户补充：%s

请生成 8-12 个主要人物小传。边生成边输出，但必须严格使用下面的文本结构，不要 JSON，不要 Markdown。

【人物1】
姓名：角色姓名
定位：主角/反派/助攻/亲属/竞争者等
年龄：年龄或年龄段
性格标签：标签1、标签2、标签3
背景：背景经历、身份反转、过往创伤或秘密
核心动机：角色最想得到什么，为什么
人物关系：与主角和其他关键角色的关系
人物小传：完整人物小传，包含欲望、缺陷、关系变化和反转节点

【人物2】
姓名：
定位：
年龄：
性格标签：
背景：
核心动机：
人物关系：
人物小传：

要求：第一个角色必须是主角；后续角色按剧情重要性输出；每个人物字段不能为空；每写完一个字段就立刻换行继续下一个字段；生成完一个人物再生成下一个人物。`, string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt))
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
	return fmt.Sprintf(`项目快照 JSON：%s
%s
用户补充：%s

请只生成四阶段粗纲，不要生成单集集纲，不要写 JSON。必须按下面顺序边思考边输出，每个阶段输出完再进入下个阶段。

【起】
范围：第1-X集
粗纲：用较完整的段落概括这一集数区间的剧情推进。要写清主角处境、引爆事件、核心矛盾如何建立、反派或阻力如何压迫、第一轮爽点和阶段结尾钩子；剧情要有趣、有反转、有可拍的情绪场面，不能太短，不能只写一句方向。

【承】
范围：第X-X集
粗纲：用较完整的段落概括这一集数区间的剧情推进。要写清关系推进、阵营变化、反派压力、误会或秘密如何扩大、爽点如何升级、阶段性反转如何发生；要让用户看完能知道这几集大概会发生哪些连续事件，不能空泛。

【转】
范围：第X-X集
粗纲：用较完整的段落概括这一集数区间的剧情推进。要写清真相揭露、危机升级、人物选择、关键背叛或误判、强反转和情绪爆点；要有连续的因果链，不要写成概念词堆砌。

【合】
范围：第X-X集
粗纲：用较完整的段落概括这一集数区间的剧情推进。要写清终局对抗、反派清算、人物关系落点、情绪释放、核心爽点兑现和结局余味；如果适合续作可留轻钩子，但不能牺牲本季收束。

要求：phase 只能是 起、承、转、合；每个“粗纲”写成可直接放入粗纲输入框的正文，每段建议 350-700 字，要概括该区间所有集的主要戏剧推进；不要逐集编号，不要列 JSON 字段，不要写单集标题，不要只写“主角成长、矛盾升级”这类空话。`, string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt))
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
	return fmt.Sprintf(`项目快照 JSON：%s
%s
用户补充：%s

请只生成用户指定范围内的单集集纲，不要写 JSON。必须按集数顺序边生成边输出，每一集输出完再进入下一集。

【第1集】
集纲：本集核心事件、人物目标、冲突推进、关键反转或爽点、结尾追看钩子

【第2集】
集纲：

要求：集数必须对应用户指定范围；“起、承、转、合”只代表四阶段粗纲的大方向，不要在每一集里套写起承转合结构，不要输出“本集起/承/转/合”小标题；每集集纲写成可直接放入输入框的一段话，贴合当前阶段粗纲，写清本集发生什么、谁推动、冲突如何升级、有什么反转或爽点、结尾为什么让人追下一集。`, string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt))
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
	sourceRule := "只根据故事概梗、人物小传和已有分集集纲生成，不要参考小说章纲；正文要可拍摄、节奏快、台词短。"
	if fmt.Sprint(project.Settings["adaptationMode"]) == "original" {
		sourceRule = "按照原文改编模式生成：可以参考小说章纲和故事概梗，保留原文主线冲突，但必须短剧化表达。"
	}
	return fmt.Sprintf(`项目快照 JSON：%s
%s
用户补充：%s

请只生成用户指定集数范围内的短剧正文，不要写 JSON，不要 Markdown，不要解释，不要写“正在生成”等说明文字。必须按集数顺序边生成边输出，每一集输出完再进入下一集。

【第1集】
正文：
△1-1 日 内 场景名
人物：角色A、角色B
△可拍摄的动作、调度或画面描写
角色A：（语气）短台词
角色B：（语气）短台词

【第2集】
正文：
△2-1 夜 外 场景名
人物：角色A、角色C
△动作描写
角色C：（语气）短台词

要求：集数必须对应用户指定范围；每场以“△集数-场次 日/夜 内/外 场景名”开头；下一行写“人物：”；动作行以“△”开头；台词行写“角色：（语气）台词”。%s 生成多集时后一集要承接上一集正文结尾的人物状态、情绪和未解决冲突，避免割裂；每集结尾留追看钩子。`, string(context), productionGuidance(project.Type), strings.TrimSpace(userPrompt), sourceRule)
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
	re := regexp.MustCompile(`【策划\s*([123])】`)
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
	systemPrompt := `你是 StoryPlay 的字段级流式剧本结构化拆解引擎。你必须按指定隐藏字段标记输出，不要输出 JSON，不要 Markdown，不要解释。字段标记不会展示给用户；标记后面的正文会直接流式写进页面。`
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
		scopeLine = "本次用户选择的拆解范围：" + strings.TrimSpace(rangeHint) + "。只拆解这个范围内的章节，不要跳到范围外，也不要只输出最后一章。\n"
	}
	common := fmt.Sprintf(`文件名：%s
%s
%s
正文：
%s

输出协议：
使用隐藏字段标记切换页面字段：
<|sp:settings.audience|>这里直接输出目标受众正文
<|sp:settings.era|>这里直接输出时代背景正文
<|sp:character|>姓名｜定位｜年龄｜简短人物描述

标记和字段名不会展示给用户；标记之后、下一个标记之前的文字会直接流式显示在页面对应字段里。
不要输出 JSON，不要 Markdown，不要解释，不要使用标题符号。
	`, fileName, productionGuidance(scriptType), scopeLine, limited)
	if purpose == "adaptation" {
		if adaptMode == "original" {
			return common + `
请按顺序流式输出这些字段：
<|sp:settings.synopsis|>
<|sp:chapter|> 覆盖正文中应拆的每一章；如果用户选择了章节范围，必须覆盖该范围内的所有章节。每次格式：第X章｜名称｜剧情概要｜情节1：...
情节2：...
情节3：...
至少10条情节，必须每条独占一行，不要把多个情节挤在同一行。
要求：这是“按照原文改编”，只基于原文拆出故事概梗和小说章纲，不要输出人物小传、短剧粗纲、集纲或正文。`
		}
		return common + `
请按顺序流式输出这些字段：
<|sp:settings.audience|>
<|sp:settings.genres|>
<|sp:settings.core|>
<|sp:settings.worldView|>
<|sp:settings.highlights|>
<|sp:settings.synopsis|>
<|sp:character|> 至少5次，每次格式：姓名｜定位｜年龄｜60-100字简短描述。
<|sp:chapter|> 覆盖正文中应拆的每一章；如果用户选择了章节范围，必须覆盖该范围内的所有章节。每次格式：第X章｜名称｜剧情概要｜情节1：...
情节2：...
情节3：...
至少10条情节，必须每条独占一行，不要把多个情节挤在同一行。
<|sp:outline|> 至少3次，每次格式：第X-Y集｜起承转合阶段｜粗纲内容。
<|sp:episode|> 至少3次，每次格式：第X集｜集纲内容。
要求：这是网文改编，必须短剧化，保留主线但强化爽点、反转和每集钩子。`
	}
	return common + `
请按顺序流式输出这些字段：
<|sp:settings.audience|>
<|sp:settings.era|>
<|sp:settings.genres|>
<|sp:settings.core|>
<|sp:infoflow.background|>
<|sp:settings.highlights|>
<|sp:settings.synopsis|>
<|sp:character|> 至少5次，每次格式：姓名｜定位｜年龄｜60-100字简短描述。
<|sp:outline|> 必须4次，每次格式：阶段｜粗纲内容。阶段固定为起、承、转、合；不要写具体集数。
要求：这是剧本改写拆解，只提取原剧核心人物关系、主线矛盾、故事背景、情绪爽点、四阶段粗纲和可改写方向；不要输出集纲或正文，后续由用户在页面单独生成。`
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
		prompt = fmt.Sprintf(`请对上传小说进行“AI提炼”，生成 StoryPlay 网文改编项目需要的数据。
标题：%s
文件名：%s
%s
正文：
%s

必须只返回 JSON：
{
  "settings":{"audience":"男频或女频","genres":["题材1","题材2"],"core":["核心设定"],"worldView":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"},
  "characters":[{"name":"姓名","role":"定位","age":"年龄","meta":"年龄 / 定位","traits":["标签"],"background":"背景","goal":"目标","relation":"关系","bio":"人物小传"}],
  "novelChapterOutline":[{"column":"1","title":"章节标题","synopsis":"本章一句话梗概","summary":"情节1：...\n情节2：...\n情节3：..."}],
  "outlines":[{"range":"第1-10集","phase":"起","content":"短剧粗纲"}],
  "episodes":[{"no":1,"outline":"第1集集纲","body":"可拍摄短剧正文片段"}]
}
要求：
1. 先提炼故事核心梗概、主线矛盾、爽点机制和世界观，不要照搬原文。
2. characters 必须生成人物小传，包含主角、核心对手、关键关系角色。
3. novelChapterOutline 覆盖正文中出现的每一章，summary 用“情节N：”分行。
4. outlines 和 episodes 要按短剧改编方式重组节奏，保留追看钩子。`, title, fileName, productionGuidance(scriptType), limited)
	} else {
		prompt = fmt.Sprintf(`请将上传剧本拆解成 StoryPlay 剧本改写的信息流和编辑项目。你需要先提取原剧核心，再发挥想象做可继续创作的改写策划。
标题：%s
文件名：%s
%s
正文：
%s

必须只返回 JSON：
{
  "settings":{"audience":"男频或女频","era":"时代背景","genres":["题材类型"],"core":["核心设定"],"worldView":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"},
  "characters":[{"name":"姓名","role":"定位","age":"年龄","meta":"年龄 / 定位","traits":["标签"],"background":"背景","goal":"目标","relation":"关系","bio":"人物小传"}],
  "infoflow":{"storyOverview":{"audience":"目标受众","era":"时代背景","genre":"题材类型","core":"核心设定","background":"故事背景","highlights":"核心亮点","synopsis":"核心梗概"},"characters":[{"name":"姓名","age":"年龄","bio":"人物设定"}],"roughOutline":[{"range":"第1-10集","phase":"起","content":"粗纲"}],"episodeSynopsis":[{"no":1,"outline":"集纲"}]},
  "outlines":[{"range":"第1-10集","phase":"起","content":"改写粗纲"}],
  "episodes":[{"no":1,"outline":"第1集集纲","body":"改写后正文片段"}]
}
要求：
1. 保留原剧最核心的人物关系、主线矛盾和情绪爽点。
2. 可以重构开场、压缩松散场次、强化反转和结尾钩子。
3. infoflow 必须完整，能直接作为“信息流”页展示。
4. settings 里写的是改写后的策划方向，不是简单摘要。`, title, fileName, productionGuidance(scriptType), limited)
	}
	result, err := requestJSONFromAI("你是 StoryPlay 上传解析引擎。你只输出合法 JSON，不写 Markdown，不解释，不省略 schema 中的字段。", prompt, 0.35)
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
	if r.Method == http.MethodPost {
		var req struct {
			Title      string `json:"title"`
			Culture    string `json:"culture"`
			Audience   string `json:"audience"`
			ScriptType string `json:"scriptType"`
			Preference string `json:"preference"`
			FileName   string `json:"fileName"`
			Content    string `json:"content"`
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
	}
	evals := state.evaluations
	state.mu.Unlock()
	writeJSON(w, evals)
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
		"plot":        "剧情逻辑",
		"character":   "人物塑造",
		"commercial":  "商业价值",
		"market":      "市场适配",
		"pacing":      "节奏把控",
		"dialogue":    "台词质量",
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

	systemPrompt := `你是 StoryPlay 专业剧本评估师。你需要根据用户提供的剧本内容和参数，从指定维度进行专业评估，并严格按照以下 JSON 格式输出结果。不要输出任何 Markdown 标记、解释或额外文字，只输出纯 JSON。

输出格式严格如下：
{
  "dimensions": [
    {"name": "维度名称", "score": 85, "note": "该维度的详细评估说明，指出优点和不足，给出改进建议"}
  ],
  "summary": "200字以内的综合评估摘要，概括剧本整体质量、核心优势和主要问题",
  "suggestions": [
    "具体的改进建议1",
    "具体的改进建议2",
    "具体的改进建议3"
  ],
  "score": 82
}

评分规则：
- 每个维度评分 0-100，必须基于剧本实际内容客观评判
- score 是所有维度的加权平均分（取整）
- note 是每个维度的 30-80 字详细点评
- summary 是整体评价
- suggestions 列出 3-5 条具体可操作的改进建议
- 不要给虚高分数，低于60分的维度要明确指出问题`

	userPromptParts := []string{}
	userPromptParts = append(userPromptParts, fmt.Sprintf("请评估以下剧本。"))
	userPromptParts = append(userPromptParts, fmt.Sprintf("评估维度：%s", dimList))
	userPromptParts = append(userPromptParts, fmt.Sprintf("文化背景：%s", req.Culture))
	userPromptParts = append(userPromptParts, fmt.Sprintf("目标受众：%s", req.Audience))
	userPromptParts = append(userPromptParts, fmt.Sprintf("剧本类型：%s", req.ScriptType))
	if req.Preference != "" {
		userPromptParts = append(userPromptParts, fmt.Sprintf("受众偏好：%s", req.Preference))
	}
	if contentPreview != "" {
		userPromptParts = append(userPromptParts, fmt.Sprintf("\n---剧本内容---\n%s\n---内容结束---", contentPreview))
	} else if req.FileName != "" {
		userPromptParts = append(userPromptParts, fmt.Sprintf("\n（用户已上传文件 %s，但内容为空）", req.FileName))
	} else {
		userPromptParts = append(userPromptParts, "\n（用户未提供剧本内容，请基于维度框架给出通用评估建议和基准分）")
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
	cleaned := strings.TrimSpace(text)
	// Try to extract JSON from markdown code blocks
	if idx := strings.Index(cleaned, "```json"); idx >= 0 {
		cleaned = cleaned[idx+7:]
		if endIdx := strings.Index(cleaned, "```"); endIdx >= 0 {
			cleaned = cleaned[:endIdx]
		}
	} else if idx := strings.Index(cleaned, "```"); idx >= 0 {
		cleaned = cleaned[idx+3:]
		if endIdx := strings.Index(cleaned, "```"); endIdx >= 0 {
			cleaned = cleaned[:endIdx]
		}
	}
	cleaned = strings.TrimSpace(cleaned)

	// Try to find JSON object boundaries
	startIdx := strings.Index(cleaned, "{")
	endIdx := strings.LastIndex(cleaned, "}")
	if startIdx < 0 || endIdx < 0 || endIdx <= startIdx {
		return Evaluation{Score: 0, Summary: "AI 返回格式异常，请重试", Dimensions: []ScoreDimension{}, Suggestions: []string{}}
	}
	cleaned = cleaned[startIdx : endIdx+1]

	var raw struct {
		Dimensions  []ScoreDimension `json:"dimensions"`
		Summary     string           `json:"summary"`
		Suggestions []string         `json:"suggestions"`
		Score       int              `json:"score"`
	}
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		// Fallback: return what we can
		return Evaluation{Score: 0, Summary: "评估结果解析失败：" + err.Error(), Dimensions: []ScoreDimension{}, Suggestions: []string{}}
	}
	if len(raw.Dimensions) == 0 {
		raw.Dimensions = []ScoreDimension{{Name: "综合", Score: raw.Score, Note: raw.Summary}}
	}
	if raw.Score == 0 && len(raw.Dimensions) > 0 {
		raw.Score = averageDimensions(raw.Dimensions)
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
		next := AIConfig{
			BaseURL: strings.TrimSpace(req.BaseURL),
			Model:   strings.TrimSpace(req.Model),
			APIKey:  strings.TrimSpace(req.APIKey),
		}
		if next.APIKey == "" || strings.Contains(next.APIKey, "****") {
			next.APIKey = current.APIKey
		}
		if next.BaseURL == "" || next.Model == "" || next.APIKey == "" {
			aiConfigMu.Unlock()
			http.Error(w, "api base url, model and key are required", http.StatusBadRequest)
			return
		}
		runtimeAIConfig = next
		aiConfigMu.Unlock()
		if err := saveAIConfigToDisk(next); err != nil {
			http.Error(w, "failed to save ai config", http.StatusInternalServerError)
			return
		}
		writeJSON(w, publicAIConfig(next))
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
	b.WriteString(title + "\n\n")
	if include("settings") {
		b.WriteString("一、故事设定\n")
		b.WriteString("目标受众：" + stringifyAny(project.Settings["audience"]) + "\n")
		b.WriteString("题材类型：" + stringifyAny(project.Settings["genres"]) + "\n")
		b.WriteString("核心设定：" + stringifyAny(project.Settings["core"]) + "\n")
		b.WriteString("风格元素：" + stringifyAny(project.Settings["style"]) + "\n")
		b.WriteString("世界观：" + stringifyAny(project.Settings["worldView"]) + "\n")
		b.WriteString("核心亮点：" + stringifyAny(project.Settings["highlights"]) + "\n")
		b.WriteString("核心梗概：" + stringifyAny(project.Settings["synopsis"]) + "\n\n")
	}
	if include("characters") && len(project.Characters) > 0 {
		b.WriteString("二、人物小传\n")
		for _, c := range project.Characters {
			b.WriteString(c.Name + "｜" + c.Role + "｜" + c.Age + "\n")
			b.WriteString("性格：" + strings.Join(c.Traits, "、") + "\n")
			b.WriteString("背景：" + c.Background + "\n")
			b.WriteString("动机：" + c.Goal + "\n")
			b.WriteString("关系：" + c.Relation + "\n\n")
		}
	}
	if include("outlines") && len(project.Outlines) > 0 {
		b.WriteString("三、分集粗纲\n")
		for _, o := range project.Outlines {
			b.WriteString(o.Range + " " + o.Phase + "\n" + o.Content + "\n\n")
		}
	}
	if len(project.Episodes) > 0 && (include("episodes") || include("body")) {
		b.WriteString("四、分集内容\n")
		for _, e := range project.Episodes {
			b.WriteString("第" + strconv.Itoa(e.No) + "集\n")
			if include("episodes") && strings.TrimSpace(e.Outline) != "" {
				b.WriteString("集纲：" + e.Outline + "\n")
			}
			if include("body") && strings.TrimSpace(e.Body) != "" {
				b.WriteString(e.Body + "\n")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
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
	http.Error(w, "StoryPlay clone API. Run frontend dev server for UI.", http.StatusNotFound)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
