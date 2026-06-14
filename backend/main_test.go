package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseNovelChaptersCommonFormats(t *testing.T) {
	input := `序章 风起
` + novelChapterBody() + `

第1章 出狱
` + novelChapterBody() + `

第一章 母亲
` + novelChapterBody() + `

Chapter 3 Dragon
` + novelChapterBody() + `

4、发布会
` + novelChapterBody() + `

五、龙主
` + novelChapterBody()

	chapters := parseNovelChapters(input)
	if len(chapters) != 6 {
		t.Fatalf("expected 6 chapters, got %d: %#v", len(chapters), chapters)
	}
	wantTitles := []string{"风起", "出狱", "母亲", "Dragon", "发布会", "龙主"}
	for i, title := range wantTitles {
		if chapters[i].Title != title {
			t.Fatalf("chapter %d title = %q, want %q", i+1, chapters[i].Title, title)
		}
		if chapters[i].Words == 0 {
			t.Fatalf("chapter %d has zero words", i+1)
		}
	}
}

func TestParseNovelChaptersFallbackForLongText(t *testing.T) {
	input := ""
	for i := 0; i < 220; i++ {
		input += "主角进入新的冲突现场，人物关系继续升级，反派压迫不断加深。\n"
	}
	chapters := parseNovelChapters(input)
	if len(chapters) < 2 {
		t.Fatalf("expected fallback split into multiple chapters, got %d", len(chapters))
	}
	if chapters[0].Title == "" || chapters[0].Words == 0 {
		t.Fatalf("fallback chapter missing title or words: %#v", chapters[0])
	}
}

func TestParseNovelChaptersRecognizesSpacedChineseHeadings(t *testing.T) {
	input := `第3章  神鬼手段，以气成针
` + novelChapterBody() + `

  第四章　家族变故
` + novelChapterBody()

	chapters := parseNovelChapters(input)
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d: %#v", len(chapters), chapters)
	}
	wantTitles := []string{"神鬼手段，以气成针", "家族变故"}
	for i, title := range wantTitles {
		if chapters[i].Title != title {
			t.Fatalf("chapter %d title = %q, want %q", i+1, chapters[i].Title, title)
		}
	}
}

func TestParseNovelChaptersKeepsSingleRecognizedChapter(t *testing.T) {
	input := "第3章  神鬼手段，以气成针\n" + strings.Repeat("秦川以气成针，局势继续反转。\n", 500)

	chapters := parseNovelChapters(input)
	if len(chapters) != 1 {
		t.Fatalf("expected recognized single chapter, got %d: %#v", len(chapters), chapters)
	}
	if chapters[0].Title != "神鬼手段，以气成针" {
		t.Fatalf("title = %q", chapters[0].Title)
	}
	if strings.HasPrefix(chapters[0].Title, "自动拆分") {
		t.Fatalf("recognized chapter was replaced by fallback: %#v", chapters[0])
	}
}

func TestParseNovelChaptersRecognizesInlineHeading(t *testing.T) {
	input := "前文收束。第3章  神鬼手段，以气成针\n" + novelChapterBody() + "\n第4章  满堂皆惊\n" + novelChapterBody()

	chapters := parseNovelChapters(input)
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d: %#v", len(chapters), chapters)
	}
	if chapters[0].Title != "神鬼手段，以气成针" || chapters[1].Title != "满堂皆惊" {
		t.Fatalf("unexpected titles: %#v", chapters)
	}
}

func TestParseNovelChaptersMergesTinyFalseChapter(t *testing.T) {
	input := "第1章 开局\n" + novelChapterBody() + "\n第2章 误切短句\n只有几十个字，不应该单独拆成一章。\n第3章 反转\n" + novelChapterBody()

	chapters := parseNovelChapters(input)
	if len(chapters) != 2 {
		t.Fatalf("expected tiny chapter to be merged, got %d: %#v", len(chapters), chapters)
	}
	if chapters[0].Title != "开局" || chapters[1].Title != "反转" {
		t.Fatalf("unexpected titles after merge: %#v", chapters)
	}
	if !strings.Contains(chapters[0].Content, "误切短句") {
		t.Fatalf("merged content lost tiny fragment: %#v", chapters[0])
	}
}

func novelChapterBody() string {
	return strings.Repeat("主角进入新的冲突现场，人物关系继续升级，反派压迫不断加深。\n", 8)
}

func nonEmptyLines(value string) []string {
	lines := []string{}
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestStatePersistenceRoundTrip(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "jubengongfang.db"))
	db, err := openAppDB()
	if err != nil {
		t.Fatalf("openAppDB failed: %v", err)
	}
	if err := migrateAppDB(db); err != nil {
		t.Fatalf("migrateAppDB failed: %v", err)
	}
	state := newState()
	state.db = db
	state.user = User{ID: 9, Nickname: "持久化用户"}
	state.wallet = Wallet{
		Balance:      88,
		Items:        []WalletTxn{{Title: "测试入账", Delta: 88, Time: "2026年06月10日 10:00"}},
		Packs:        newState().wallet.Packs,
		Tasks:        newState().wallet.Tasks,
		ServiceCosts: newState().wallet.ServiceCosts,
	}
	state.scripts = []ScriptProject{{ID: 12, Title: "持久化项目", Type: "AI短剧", Source: "adaptation"}}
	state.nextScript = 13
	state.saveLocked()
	_ = db.Close()

	loaded := loadState()
	defer loaded.db.Close()
	if loaded.user.Nickname != "持久化用户" {
		t.Fatalf("loaded user = %#v", loaded.user)
	}
	if loaded.wallet.Balance != 88 {
		t.Fatalf("loaded wallet = %#v", loaded.wallet)
	}
	if len(loaded.scripts) != 1 || loaded.scripts[0].Title != "持久化项目" {
		t.Fatalf("loaded scripts = %#v", loaded.scripts)
	}
	if loaded.nextScript != 13 {
		t.Fatalf("nextScript = %d", loaded.nextScript)
	}
}

func TestDatabaseMigrationCreatesTables(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "jubengongfang.db"))
	db, err := openAppDB()
	if err != nil {
		t.Fatalf("openAppDB failed: %v", err)
	}
	defer db.Close()
	if err := migrateAppDB(db); err != nil {
		t.Fatalf("migrateAppDB failed: %v", err)
	}
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatalf("query sqlite_master failed: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name failed: %v", err)
		}
		got[name] = true
	}
	for _, name := range []string{"app_meta", "users", "wallet_state", "wallet_transactions", "wallet_tasks", "recharge_packs", "service_costs", "script_projects", "evaluations"} {
		if !got[name] {
			t.Fatalf("missing table %s; got %#v", name, got)
		}
	}
}

func TestAppDatabasePathUsesLegacyDBWhenRenamedDBMissing(t *testing.T) {
	t.Setenv("DB_PATH", "")
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore wd failed: %v", err)
		}
	}()
	if err := os.MkdirAll("data", 0o755); err != nil {
		t.Fatalf("mkdir data failed: %v", err)
	}
	legacyPath := legacyDatabasePath()
	if err := os.WriteFile(legacyPath, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy db failed: %v", err)
	}
	if got := appDatabasePath(); got != legacyPath {
		t.Fatalf("appDatabasePath = %q, want %q", got, legacyPath)
	}
}

func TestNormalizeBeatLinesSplitsInlineBeats(t *testing.T) {
	got := normalizeBeatLines("情节1：开局受辱 情节2：主角反击 情节3：反派加压")
	want := "情节1：开局受辱\n情节2：主角反击\n情节3：反派加压"
	if got != want {
		t.Fatalf("normalizeBeatLines = %q, want %q", got, want)
	}
}

func TestOutlineBeatsHasAtLeastTenItems(t *testing.T) {
	got := outlineBeats("主角被逼入绝境。母亲病危。反派上门羞辱。主角决定反击。")
	count := 0
	for _, line := range nonEmptyLines(got) {
		if len(line) > 0 {
			count++
		}
	}
	if count < 10 {
		t.Fatalf("expected at least 10 beats, got %d: %s", count, got)
	}
}

func TestParseEvaluationResultFallsBackForPlainText(t *testing.T) {
	got := parseEvaluationResult("综合评分：82\n建议：强化主线冲突。\n建议：优化人物动机。")
	if got.Score != 82 {
		t.Fatalf("score = %d, want 82", got.Score)
	}
	if len(got.Dimensions) == 0 || len(got.Suggestions) < 2 {
		t.Fatalf("unexpected fallback evaluation: %#v", got)
	}
}

func TestBuildExportContentUsesWordHTML(t *testing.T) {
	project := ScriptProject{
		Title: "测试剧本",
		Settings: map[string]any{
			"audience": "男频",
			"genres":   []any{"都市", "逆袭"},
			"synopsis": "主角逆袭。",
		},
		Characters: []Character{{Name: "秦川", Role: "主角", Bio: "被误解后反击。"}},
		Episodes:   []Episode{{No: 1, Outline: "开局受辱", Body: "△1-1 日 内 大厅\n秦川：（冷静）该结束了。"}},
	}
	got := buildExportContent(project, nil)
	for _, want := range []string{"<!doctype html>", "<h1>测试剧本</h1>", "一、故事设定", "秦川", "第1集"} {
		if !strings.Contains(got, want) {
			t.Fatalf("export missing %q: %s", want, got)
		}
	}
}

func TestEvaluationsHandlerDeletesRecord(t *testing.T) {
	state.mu.Lock()
	oldEvaluations := state.evaluations
	oldDB := state.db
	state.db = nil
	state.evaluations = []Evaluation{{ID: 77, Title: "待删除"}}
	state.mu.Unlock()
	defer func() {
		state.mu.Lock()
		state.evaluations = oldEvaluations
		state.db = oldDB
		state.mu.Unlock()
	}()

	req := httptest.NewRequest(http.MethodDelete, "/api/evaluations?id=77", nil)
	rec := httptest.NewRecorder()
	evaluationsHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["ok"] != true {
		t.Fatalf("bad body %#v err=%v", body, err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.evaluations) != 0 {
		t.Fatalf("evaluations not deleted: %#v", state.evaluations)
	}
}
