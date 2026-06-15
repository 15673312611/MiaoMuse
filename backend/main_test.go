package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN to run MySQL persistence integration test")
	}
	t.Setenv("DB_DSN", dsn)
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
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN to run MySQL migration integration test")
	}
	t.Setenv("DB_DSN", dsn)
	db, err := openAppDB()
	if err != nil {
		t.Fatalf("openAppDB failed: %v", err)
	}
	defer db.Close()
	if err := migrateAppDB(db); err != nil {
		t.Fatalf("migrateAppDB failed: %v", err)
	}
	rows, err := db.Query(`SHOW TABLES`)
	if err != nil {
		t.Fatalf("show tables failed: %v", err)
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

func TestMySQLAppDSNFromEnvParts(t *testing.T) {
	t.Setenv("DB_DSN", "")
	t.Setenv("MYSQL_DSN", "")
	t.Setenv("DB_HOST", "db.example.test")
	t.Setenv("DB_PORT", "3307")
	t.Setenv("DB_USER", "writer")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "jubentest")
	t.Setenv("DB_LOC", "UTC")
	dsn, dbName, err := mysqlAppDSN()
	if err != nil {
		t.Fatalf("mysqlAppDSN failed: %v", err)
	}
	if dbName != "jubentest" {
		t.Fatalf("dbName = %q", dbName)
	}
	for _, want := range []string{"writer:secret@tcp(db.example.test:3307)/jubentest", "charset=utf8mb4", "parseTime=true"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("dsn %q missing %q", dsn, want)
		}
	}
}

func TestLoadDotEnvFileDoesNotOverrideExistingEnv(t *testing.T) {
	dir := t.TempDir()
	file := dir + string(os.PathSeparator) + ".env"
	if err := os.WriteFile(file, []byte("DB_HOST=from-file\nDB_USER=\"file-user\"\n"), 0o600); err != nil {
		t.Fatalf("write .env failed: %v", err)
	}
	t.Setenv("DB_HOST", "existing")
	if err := loadDotEnvFile(file); err != nil {
		t.Fatalf("loadDotEnvFile failed: %v", err)
	}
	if got := os.Getenv("DB_HOST"); got != "existing" {
		t.Fatalf("DB_HOST = %q", got)
	}
	if got := os.Getenv("DB_USER"); got != "file-user" {
		t.Fatalf("DB_USER = %q", got)
	}
}

func TestNormalizeBeatLinesSplitsInlineBeats(t *testing.T) {
	got := normalizeBeatLines("情节1：开局受辱 情节2：主角反击 情节3：反派加压")
	want := "情节1：开局受辱\n情节2：主角反击\n情节3：反派加压"
	if got != want {
		t.Fatalf("normalizeBeatLines = %q, want %q", got, want)
	}
}

func TestParsePlanningStreamPlansAcceptsAdaptationFieldAliases(t *testing.T) {
	input := `【策划1】
方案标题：《重生后我把宗门踩在脚下》
受众定位：男频
题材：玄幻逆袭、复仇虐渣
创意设定：废脉重铸、宗门审判
改编亮点：把原文慢热成长线压缩成开局受辱、当场反杀的短剧强钩子
故事背景：灵脉为尊的宗门世界，废脉弟子被当成替罪羊逐出山门。
改编梗概：少年被宗门诬陷偷盗秘宝，废去灵脉后丢入寒潭。濒死之际，他觉醒古碑传承，发现所谓秘宝正是宗门长老掩盖血祭真相的关键证据。他回到宗门审判台，从外门杂役开始逐层打脸，逼出长老、少宗主和幕后掌门。每一次反击都揭开一层真相，也让观众看到从废物到审判者的爽感升级。`

	plans := parsePlanningStreamPlans(input)
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d: %#v", len(plans), plans)
	}
	plan := plans[0]
	if plan["audience"] != "男频" {
		t.Fatalf("audience = %#v", plan["audience"])
	}
	if strings.TrimSpace(plan["highlights"].(string)) == "" || strings.TrimSpace(plan["worldView"].(string)) == "" || strings.TrimSpace(plan["synopsis"].(string)) == "" {
		t.Fatalf("plan has empty fields: %#v", plan)
	}
	if got := plan["genres"].([]string); len(got) != 2 || got[0] != "玄幻逆袭" {
		t.Fatalf("genres = %#v", got)
	}
}

func TestParsePlanningStreamPlansAcceptsJSONPlans(t *testing.T) {
	input := `{"plans":[{"title":"《真假千金审判夜》","audience":"女频","genres":["真假千金","复仇虐渣"],"core":["证据翻盘"],"highlights":"开局订婚宴公开处刑","worldView":"现代豪门直播审判场","synopsis":"女主被假千金夺走身份后，在订婚宴被全网嘲笑。她拿出母亲留下的旧手机，逐步放出证据，让豪门、未婚夫和假千金的谎言当场崩塌。"}]}`

	plans := parsePlanningStreamPlans(input)
	if len(plans) != 1 {
		t.Fatalf("expected 1 JSON plan, got %d: %#v", len(plans), plans)
	}
	if plans[0]["title"] != "《真假千金审判夜》" {
		t.Fatalf("title = %#v", plans[0]["title"])
	}
	if got := plans[0]["core"].([]string); len(got) != 1 || got[0] != "证据翻盘" {
		t.Fatalf("core = %#v", got)
	}
}

func TestTaskContextProjectTrimsAdaptationPlanningPayload(t *testing.T) {
	project := ScriptProject{
		Source: "adaptation",
		Settings: map[string]any{
			"synopsis":            "已有梗概",
			"chapterCount":        200,
			"chapterBreakdown":    []map[string]any{{"chapter": 1, "title": "开局", "summary": strings.Repeat("长内容", 100)}},
			"novelChapterOutline": []map[string]any{{"column": "1", "title": "第一章", "summary": strings.Repeat("剧情", 100)}},
		},
	}

	trimmed := taskContextProject(project, "planning")
	if _, ok := trimmed.Settings["chapterCount"]; ok {
		t.Fatalf("chapterCount should be removed: %#v", trimmed.Settings)
	}
	if _, ok := trimmed.Settings["novelChapterOutline"]; ok {
		t.Fatalf("novelChapterOutline should be removed: %#v", trimmed.Settings)
	}
	if sample, ok := trimmed.Settings["novelChapterOutlineSample"].([]map[string]any); !ok || len(sample) != 1 {
		t.Fatalf("missing compact novel sample: %#v", trimmed.Settings)
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
