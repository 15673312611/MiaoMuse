# 剧本工坊 System Design

## Stack

- Backend: Go 1.22+, standard HTTP API or Gin, PostgreSQL in production, Redis for queues and locks, object storage for uploads/assets.
- Frontend: Vue 3 + Vite, componentized SPA, route-level pages matching `/creation`, `/rewriting`, `/adaptation`, `/coverage`, `/premium`, `/wallet`, and `/settings`.
- AI provider: OpenAI-compatible chat completions or responses adapter. Provider credentials are server-only.
- Storage: OSS/S3 for uploaded scripts, generated reports, Word exports, user avatars, payment QR codes, and contact QR codes.
- Async jobs: Redis queue, database job table, workers for parsing, AI generation, report generation, Word export, and payment reconciliation.

## Core Domains

### Identity

Users authenticate by phone/password or SMS. Access tokens are short-lived and refresh tokens rotate. Profile data includes nickname, avatar, phone, membership state, and wallet summary.

### Wallet And Membership

Point operations are ledger-based. The wallet balance is derived from transactions or maintained as a locked row with transaction audit. AI actions reserve points before queueing work. If the job fails, the reservation is released; if it succeeds, the reservation becomes a debit.

Membership owns entitlements such as recharge discount, every-30-day point welfare, priority AI queue, and higher upload/export quotas.

### Script Creation

Original script projects are structured as multiple editable sections:

- Inspiration planning inputs and selected tags
- Story settings
- Characters
- Rough outline
- Episode outline
- Script body episodes
- Generated versions and revision history

Every AI action should create an `ai_tasks` row with input snapshot, model, cost, status, and result artifact. This gives retry safety and lets the UI show task progress.

### Rewrite And Adaptation

Rewrite and adaptation share upload ingestion. The first phase validates file type, size, and parseability. The second phase creates a project and decomposes content into structured source sections. Rewrite targets script-to-script transformation; adaptation targets novel-to-outline/script transformation.

### Evaluation

Evaluation is a long-running report job. The uploaded file is parsed and normalized, then an evaluation task produces scores, diagnostic text, charts, and suggestions. The UI shows a history drawer searchable by title/status.

### Removed Domains

Lapian and community are intentionally out of scope for the clone. They must not appear in primary navigation, backend routes, production tables, wallet costs, membership benefits, or payment order sources. The editor may keep a lightweight `爆款榜单` reference modal only as a creative benchmark input for inspiration planning; it is not a retained lapian library.

## Database Tables

### users

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | user id |
| phone | varchar unique | login account |
| password_hash | varchar | bcrypt/argon2 |
| nickname | varchar | display name |
| avatar_url | text | selected/uploaded avatar |
| status | varchar | active, disabled |
| created_at | timestamptz | |
| updated_at | timestamptz | |

### user_sessions

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| user_id | bigint | |
| refresh_token_hash | varchar | rotate on refresh |
| user_agent | text | |
| ip | inet | |
| expires_at | timestamptz | |
| revoked_at | timestamptz | |

### memberships

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| user_id | bigint | |
| plan_code | varchar | player, expert, enterprise |
| starts_at | timestamptz | |
| ends_at | timestamptz | |
| source | varchar | gift, purchase, admin |
| created_at | timestamptz | |

### point_wallets

| Field | Type | Notes |
| --- | --- | --- |
| user_id | bigint pk | |
| balance | int | available points |
| frozen | int | reserved points |
| updated_at | timestamptz | optimistic lock |

### point_transactions

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| user_id | bigint | |
| type | varchar | income, expense, reserve, release |
| amount | int | signed or positive with type |
| balance_after | int | audit |
| source_type | varchar | ai_task, recharge, gift, welfare, membership |
| source_id | bigint | |
| title | varchar | UI label |
| created_at | timestamptz | |

### recharge_orders

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| user_id | bigint | |
| package_code | varchar | 20,100,300,enterprise |
| amount_cents | int | |
| base_points | int | |
| bonus_points | int | |
| status | varchar | pending, paid, closed, refunded |
| payment_provider | varchar | |
| paid_at | timestamptz | |

### script_projects

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| user_id | bigint | |
| title | varchar | default generated title |
| type | varchar | ai_short, live_action |
| source | varchar | original, rewrite, adaptation |
| status | varchar | draft, generating, archived |
| created_at | timestamptz | |
| updated_at | timestamptz | |

### script_settings

| Field | Type | Notes |
| --- | --- | --- |
| project_id | bigint pk | |
| audience | varchar | male, female |
| genres | jsonb | max 3 |
| core_settings | jsonb | max 3 |
| style_elements | jsonb | max 5 |
| world_view | text | optional |
| highlights | text | optional |
| synopsis | text | required |

### script_characters

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| project_id | bigint | |
| name | varchar | |
| age | varchar | |
| role | varchar | |
| bio | text | |
| sort_order | int | |

### script_outlines

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| project_id | bigint | |
| range_label | varchar | e.g. 第1-10集 |
| phase | varchar | 起承转合 |
| content | text | |
| sort_order | int | |

### script_episodes

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| project_id | bigint | |
| episode_no | int | |
| outline | text | |
| body | text | |
| status | varchar | draft, generated |
| updated_at | timestamptz | |

### ai_tasks

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| user_id | bigint | |
| project_id | bigint nullable | |
| task_type | varchar | planning, character, outline, episode, polish, rewrite, adaptation, evaluation |
| model | varchar | |
| input_snapshot | jsonb | prompt and structured inputs |
| cost_points | int | |
| status | varchar | queued, running, succeeded, failed, cancelled |
| result_ref | text/jsonb | output artifact |
| error | text | |
| created_at | timestamptz | |
| finished_at | timestamptz | |

### uploads

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| user_id | bigint | |
| purpose | varchar | rewrite, adaptation, evaluation |
| file_name | varchar | |
| mime_type | varchar | |
| size_bytes | bigint | |
| object_key | text | |
| parse_status | varchar | pending, succeeded, failed |
| parse_error | text | |

### evaluations

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| user_id | bigint | |
| upload_id | bigint | |
| title | varchar | |
| audience | varchar | |
| culture | varchar | |
| script_type | varchar | |
| preference | text | |
| status | varchar | queued, running, completed, failed |
| score_json | jsonb | |
| report_url | text | |
| created_at | timestamptz | |

### service_leads

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| user_id | bigint | |
| type | varchar | enterprise_recharge, customer_support, account_help |
| contact_note | text | |
| status | varchar | viewed, contacted, converted |
| created_at | timestamptz | |

## AI Task Flow

1. Frontend validates required fields and calls a quote endpoint or uses fixed displayed cost.
2. Backend starts a transaction, verifies membership/points, reserves points, and creates `ai_tasks`.
3. Worker consumes queued task and calls the OpenAI-compatible provider.
4. Worker validates and normalizes the response into structured sections.
5. On success, backend commits generated content and converts reserved points into expense.
6. On failure, backend releases reserved points and stores an error for UI retry.

## Payment Flow

1. User selects package and accepts agreement.
2. Backend creates `recharge_orders` in pending status.
3. Payment provider returns QR/payment URL.
4. Payment callback verifies signature and idempotently marks order paid.
5. Points are credited through `point_transactions`.
6. Frontend polls order status until paid or closed.

## UI State Contracts

Every long operation returns a stable task id. The UI polls task status and displays loading, success, failure, and retry. Empty states must be first-class states, not missing-data accidents. Locked content uses previews and explicit unlock controls. Buttons that can cost points must show cost and disabled reasons before submission.

## 2026-06-06 Clone Implementation Notes

The local clone is implemented as a Go API mock plus Vue 3/Vite UI. The current visual rebuild uses the valid reshoot set in `screenshots/reshoot-2048/` and generated HTML references in `docs/generated-html-reshoot/`.

The design viewport is CSS `2050x968`. The captured PNG files are `2563x1210` because the browser was running at `devicePixelRatio=1.25`. Any generated HTML reference or future UI work must keep this CSS/PNG distinction. Treating `2563x1210` as CSS layout dimensions causes right-side overflow and creates the “right side missing” symptom.

Editor routes should be modeled as section routes or section state:

- `/creation/{project_id}/1`: story synopsis/settings. Stores audience, genre, core settings, style elements, world view, highlights, and synopsis.
- `/creation/{project_id}/2`: character biographies. Supports manual character creation and `AI生成 50` / `一键生成50`.
- `/creation/{project_id}/3`: rough outline and episode outline. Supports `AI生成全部粗纲 80`, rough outline editing, episode cards, and first-time guide state.
- `/creation/{project_id}/4`: script body. Stores per-episode body text, supports Word export and non-paid episode navigation.

`灵感策划` is a modal workflow attached to the editor rather than a separate full route. It can use typed options plus free text to create three planning candidates. `爆款榜单` is a nested ranking modal that can seed a planning task through `对标策划`; that action should be treated as a paid AI task and require confirmation/reservation before execution.

Cost-bearing UI actions retained in the clone include `开始策划50`, `AI生成 50`, `一键生成50`, `AI生成全部粗纲 80`, `AI生成正文 120`, `开始剧本评估 2500`, recharge/payment, membership renewal, and ranking `对标策划`. The backend should never rely on disabled frontend state. Every request must recalculate cost and entitlement server-side, reserve points in one transaction, then dispatch the job.

Additional tables recommended for the editor and ranking modal:

### ranking_items

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| period_start | date | ranking window start |
| period_end | date | ranking window end |
| channel | varchar | ai_short, live_action |
| list_type | varchar | hot, ai_real, comic, sand_comic, hongguo, douyin, kuaishou |
| rank_no | int | |
| title | varchar | |
| cover_url | text | |
| episode_count | int | |
| launched_at | date | |
| tags | jsonb | |
| synopsis | text | |
| heat_value | varchar | display value such as 6.5亿 |
| source_json | jsonb | raw platform aggregation |

### inspiration_plans

| Field | Type | Notes |
| --- | --- | --- |
| id | bigint pk | |
| project_id | bigint | |
| source | varchar | manual, ranking_benchmark |
| selected_audience | varchar | |
| selected_genres | jsonb | |
| selected_core_settings | jsonb | |
| selected_styles | jsonb | |
| user_prompt | text | |
| candidate_json | jsonb | generated planning candidates |
| selected_candidate | int | nullable |
| created_by_task_id | bigint | links ai_tasks |
| created_at | timestamptz | |

Recommended idempotency fields for `ai_tasks`: `client_request_id`, `reserved_transaction_id`, `input_hash`, and `model_config_json`. These cover double-clicks, retry safety, point reservation linkage, and provider/prompt audit.

Production UI contracts:

- Editor sections autosave draft fields independently.
- Paid buttons show exact cost and disabled reasons before submission.
- Paid execution shows confirmation or pending state before reserving points.
- Modals and drawers do not mutate domain data until final submit.
- Guide overlays are stored per user/project so they do not repeat after dismissal.
