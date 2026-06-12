# StoryPlay Clone Current Business Logic

Date: 2026-06-06

This document describes the current commercial clone scope after removing lapian and community.

## Product Scope

Retained modules:

- Original script creation
- Script rewrite upload and parsing
- Novel adaptation upload and parsing
- Script evaluation
- Membership
- Point wallet
- Account settings

Removed modules:

- Lapian
- Community

## Interaction Logic

### Workspace

The left rail is the persistent primary navigation. The bottom rail shows current point balance and profile entry. Each retained module is a direct one-click route. Cost-bearing actions always expose their cost in the button label and are validated again by the backend.

### Original Script

The creation page lists draft projects with title, type, and last update time. `新建剧本` opens a type modal with `AI短剧` and `真人实拍`. Creating a project stores a structured project with story settings, characters, outlines, and episodes.

Observed correction from the real site: opening a script project does not switch to a full white document workspace. The editor remains inside the black patterned workspace shell with the same left workbench navigation. The form area uses dark controls; only planning/ranking modals and guide bubbles use light panels.

The editor has four tabs:

- `故事梗概`: audience, genre, core setting, style, world view, highlights, synopsis.
- `人物小传`: manual character cards and `AI生成 50`.
- `分集大纲`: rough outline, episode outline, guide bubbles, `AI生成全部粗纲 80`.
- `剧本正文`: episode bodies and Word export. The backend still supports `AI生成正文 120`, but the current UI keeps the real-site top action surface to `导出为Word`.

`灵感策划` is an editor modal. It can use manual prompt/options or ranking benchmark data to run `开始策划50`. Ranking data is used only as creative benchmark material, not as a retained lapian feature.

Real interaction details captured on 2026-06-06:

- The planning button is disabled until the user enters free text or selects at least one option.
- During generation, the underlying editor fields and script title are disabled.
- The editor top action changes from `灵感策划` to `生成中`.
- The modal shows a `StoryPlay` status block and progressive copy: first plan completed, second running, then third running/completed.
- The AI returns three planning cards. Each card contains title, target audience, genre, core setting, style elements, highlights, world view, and synopsis.
- Each card has `再次策划` and `引用`. Generation consumes points, but it does not immediately overwrite the editor. `引用` first shows `处理中，请稍后...`, then writes that card into the story settings form and saves the draft without a second point debit.
- `人物小传` empty state shows `点击右侧按钮创建新角色，自定义角色信息 / 或由AI一键生成所有角色`. `AI生成 50` and `一键生成50` trigger the same paid task. During generation, the title and character fields are disabled and the button text changes to `生成中`. The result is a role-detail workspace rather than a flat list: current role fields include `姓名`, `定位`, `年龄`, `性格` tags, tag input placeholder `空格添加标签，双击删除`, and `背景`; other generated role names are listed as a side index for switching.
- `分集大纲` has a paid pre-confirmation. Clicking `AI生成全部粗纲 80` first opens `选择集数范围` with `30集`, `60集`, `80集`, `100集`, `取消`, and `确认生成`. The point cost is only executed after confirmation. During generation, the title is disabled, the button changes to `生成中`, and the page remains in the dark editor with `粗纲*` and `集纲*` sections.
- `分集大纲` content model separates rough outline blocks from episode outline cards. Episode cards show a readonly episode number and the `起 - 承 - 转 - 合` structure.

### Rewrite And Adaptation

Rewrite and adaptation use the same ingestion path. The UI exposes an empty state with upload entry and rights statement. Uploading creates a parsed project:

- rewrite source: `rewriting`
- adaptation source: `adaptation`

Real interaction details captured on 2026-06-07:

- Both pages keep the same workspace shell, search box, tutorial link, upload entry, and data rights statement.
- The rewrite empty state says the uploaded full script will be split into script structure; invalid short/non-script text stays on the page and shows an import failure state.
- The adaptation empty state says the uploaded novel will be split into chapter outlines (`章纲拆解`).
- Novel adaptation expects chapters to be recognisable by `第N章` headings. After upload it opens a `拆解范围` dialog with start/end chapter inputs, chapter cards, total chapters, total words, and a cost estimate.
- The range dialog estimates cost as `400字/剧点`, caps the effective novel outline range at 150 chapters, and warns users to choose only the chapters they need.
- Confirming the range writes the selected chapter range back into project settings and seeds episode outlines/bodies for continued editing.

The current local implementation reads the selected file in the browser and posts `title`, `fileName`, and `content` to the backend. Text files are parsed directly. Binary formats are accepted into the same contract with a placeholder parse body so a production object-storage parser can replace it without changing the API.

The backend creates a structured project from the upload:

- `settings.sourceFile` and `settings.sourceLength`
- `settings.importStatus`, `settings.importType`, and import check steps
- synopsis and highlights derived from uploaded text
- rewrite/adaptation-specific rough outline blocks
- rewrite-specific scene breakdown and dialogue count
- adaptation-specific chapter count, selected chapter range, and chapter breakdown
- inferred character placeholders when dialogue/name patterns exist
- a first episode card seeded from the uploaded content

Production flow should still add object storage, antivirus/content safety checks, OCR/docx/pdf parsing, length limits, and async parse status polling.

### Evaluation

Evaluation collects culture, audience, script type, preference, and script file. Submitting costs 2500 points. The backend checks balance before creating a completed evaluation record.

The current report object includes total score, report number, summary, five scored dimensions, and actionable suggestions. The history drawer renders the summary and dimension scores. Production should queue a long-running report job, store report artifacts, and expose status polling.

### Membership

Membership shows current plan and expiry. Renewal now returns an order object with plan, amount, days, paid status, and the updated profile. Membership controls recharge discount and welfare eligibility. Production should replace the local paid response with payment-provider order creation and callback reconciliation.

### Wallet

Wallet shows balance, estimated script capacity, point tasks, and ledger records. Current supported actions:

- New user gift record
- Member welfare claim: +500 points
- Normal recharge order: credits selected point package and writes a ledger entry
- Enterprise recharge order: credits selected enterprise package and writes a ledger entry

All point changes are ledger entries. Production should derive or audit balance through point transactions.

### Settings

Profile supports nickname save. Account security shows phone and password status, and password reset modal validates confirmation. Contact shows three QR blocks. General settings stores local UI preferences.

## Backend API

- `GET /api/profile`
- `GET /api/wallet`
- `POST /api/wallet/claim`
- `GET /api/scripts`
- `POST /api/scripts`
- `GET /api/scripts/{id}`
- `PUT /api/scripts/{id}`
- `DELETE /api/scripts/{id}`
- `POST /api/upload?purpose=rewriting|adaptation&title=...`
- `GET /api/evaluations`
- `POST /api/evaluations`
- `POST /api/ai-task`
- `POST /api/recharge`
- `POST /api/membership/renew`
- `POST /api/settings/profile`
- `POST /api/settings/password`
- `POST /api/export`

Important local payload contracts:

- Rewrite upload accepts `{ title, fileName, content }` at `POST /api/upload?purpose=rewriting` and returns `{ status, taskId, summary, project }`. The backend first asks the configured OpenAI-compatible model for `settings`, `characters`, `infoflow`, `outlines`, and `episodes`; if that fails it falls back to deterministic text parsing.
- Adaptation upload is a two-step frontend flow. The browser reads chapters, opens the script-type modal, then opens the range modal. Confirming sends `{ title, fileName, content, fullContent, scriptType, chapterStart, chapterEnd, cost }` to `POST /api/upload?purpose=adaptation`; the backend deducts the server wallet by `cost`, generates `novelChapterOutline`, and returns `{ status, taskId, summary, project, wallet }`.
- Evaluation accepts `{ title, culture, audience, scriptType, preference, fileName, content }` and returns `{ evaluation, wallet }`.
- Recharge accepts `{ code }` and returns `{ status, order, wallet }`.
- Membership renewal accepts `{ plan }` and returns `{ status, order, profile }`.
- Export accepts `{ projectId, project }` and returns `{ status, fileName, mime, content }`; the frontend downloads the returned content as a `.doc` file.

## Production Table Design

Core tables:

- `users`
- `user_sessions`
- `memberships`
- `point_wallets`
- `point_transactions`
- `recharge_orders`
- `script_projects`
- `script_settings`
- `script_characters`
- `script_outlines`
- `script_episodes`
- `uploads`
- `evaluations`
- `ai_tasks`
- `inspiration_plans`
- `ranking_items`

Removed table groups:

- lapian library and unlock tables
- community/course/social tables

## AI Task Contract

AI operations must be idempotent and auditable:

1. Validate required inputs.
2. Calculate server-side cost.
3. Reserve points in a transaction.
4. Create `ai_tasks` with input snapshot, model config, and client request id.
5. Worker calls the OpenAI-compatible provider.
6. Normalize output into structured project fields.
7. Commit generated content and convert reservation to expense.
8. On failure, release reserved points and store retryable error.

Recommended `ai_tasks` fields:

- `id`
- `user_id`
- `project_id`
- `task_type`
- `client_request_id`
- `input_hash`
- `model`
- `model_config_json`
- `input_snapshot`
- `cost_points`
- `reserved_transaction_id`
- `status`
- `result_json`
- `error`
- `created_at`
- `finished_at`

## Local Implementation Notes

- The local Go API recalculates task costs server-side for `planning`, `characters`, `outline`, `episode`, and `body`.
- `POST /api/ai-task` returns `taskId`, `status`, `cost`, `result`, `project`, and `wallet`.
- Unknown AI task types are rejected server-side; the client cost is not trusted.
- AI tasks validate project existence before deducting points.
- Planning results are returned as structured `plans` and are only applied after `引用`.
- Characters use commercial fields: name, role, age, traits, background, goal, relation, and bio.
- Editor changes are silently persisted on a debounce and before tab switches, exports, and AI tasks.
- Undo/redo uses a local snapshot stack and persists the restored version.
- Upload, evaluation, recharge, membership renewal, and export all return concrete domain data instead of placeholder links.
- Source projects now have source-specific editor entry tabs: `rewriting` opens `信息流`, `adaptation` opens `小说章纲`, and `original` keeps `故事梗概`.
- Rewriting settings uses target-site labels: `改写策划`, `目标受众`, `时代背景`, `题材类型`, `核心设定`, `故事背景`, `核心亮点`, `核心梗概`.
- Adaptation range cost follows the observed target rule: `ceil(selectedWords / 400)` points.
- The current clone deliberately removes all main navigation and backend routes for lapian and community. The only remaining ranking-related UI is the editor's `爆款榜单` reference modal used as creative benchmark input for `灵感策划`; it is not a retained lapian module.
