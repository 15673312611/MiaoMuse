# 剧本工坊 UI Research

Research date: 2026-06-05

## 2026-06-06 Reshoot And HTML Reference

The valid screenshot set for the rebuild is `screenshots/reshoot-2048/`. It contains 34 viewport screenshots, all captured from the logged-in 剧本工坊 session at CSS viewport `2050x968`, `devicePixelRatio=1.25`, producing PNG files of `2563x1210`. The earlier smaller/old screenshot set should not be used for layout matching.

Generated HTML references are in `docs/generated-html-reshoot/`. Each HTML file was produced from exactly one original PNG request using a local OpenAI-compatible API config file and the required prompt. PNG files were sent as original PNG bytes in data URLs; no compression, resizing, conversion, or multi-image bundling was used.

Important correction: generated HTML must use CSS canvas `2050x968`, not PNG physical pixels `2563x1210`. Using `2563x1210` as CSS layout width causes the right side of the UI to overflow and appear missing on the current browser. The generation script now tells the model that the PNG is high-DPI and maps to `2050x968` CSS pixels.

Valid generated references:

- `01-creation-dashboard.html` through `06-coverage-history-panel.html`: workbench, creation modal, rewrite/adaptation upload, evaluation form/history.
- `07-hot-grid.html` through `09-hot-custom-modal.html`: lapian grid, hover state, custom lapian modal.
- `10-premium.html` through `16-wallet-enterprise-modal.html`: membership, renewal, wallet, point help, recharge, enterprise recharge.
- `17-community.html` through `24-settings-contact.html`: community, QR modals, profile/security/password/contact settings.
- `25-editor-story-settings.html` through `34-editor-ranking-modal.html`: internal editor, character empty state, outline, guide tips, script body, inspiration panel, audience expansion, ranking modal.

Local validation screenshots from the clone are saved as:

- `screenshots/local-clone-creation.png`
- `screenshots/local-clone-new-script-modal.png`
- `screenshots/local-clone-editor-settings.png`
- `screenshots/local-clone-editor-ranking.png`

All validation checks reported `body/document` dimensions of `2050x968`, so the right-side clipping issue is resolved.

## Screenshot Index

- `screenshots/01-home-full.png`: public homepage
- `screenshots/02-home-logged-full.png`: logged-in homepage
- `screenshots/03-creation-dashboard.png`: workbench
- `screenshots/04-new-script-type-modal.png`: new script type modal
- `screenshots/05-original-planning-editor.png`: inspiration planning
- `screenshots/06-original-planning-tags.png`: planning tag expansion
- `screenshots/07-original-script-steps.png`: original script workflow
- `screenshots/08-original-story-settings-form.png`: story settings form
- `screenshots/09-original-characters-empty.png`: character empty state
- `screenshots/10-original-outline-empty.png`: outline empty state
- `screenshots/11-original-script-body-empty.png`: script body empty state
- `screenshots/12-original-episode-editor-guide.png`: episode editor guide
- `screenshots/13-rewriting-empty-upload.png`: script rewrite upload
- `screenshots/14-adaptation-empty-upload.png`: novel adaptation upload
- `screenshots/15-coverage-evaluation-form.png`: script evaluation
- `screenshots/16-coverage-history-panel.png`: evaluation history drawer
- `screenshots/17-hot-lapian-grid.png`: lapian grid
- `screenshots/18-custom-lapian-modal.png`: custom lapian service
- `screenshots/19-premium-membership.png`: membership page
- `screenshots/20-wallet-points.png`: wallet
- `screenshots/21-wallet-points-help-modal.png`: points help
- `screenshots/22-wallet-recharge-modal.png`: points recharge
- `screenshots/23-wallet-enterprise-recharge-modal.png`: enterprise recharge
- `screenshots/24-community-courses-full.png`: community courses
- `screenshots/25-community-teacher-modal.png`: teacher consultation
- `screenshots/26-community-editor-modal.png`: editor consultation
- `screenshots/27-community-group-modal.png`: official group
- `screenshots/28-settings-profile.png`: profile settings
- `screenshots/29-settings-account-security.png`: account security
- `screenshots/30-settings-password-modal.png`: password modal
- `screenshots/31-settings-contact.png`: contact settings
- `screenshots/32-hot-lapian-detail-hover.png`: lapian hover detail

## Global Layout

The logged-in product uses a fixed left sidebar and a large light workspace. The sidebar carries two groups: workbench actions and common modules. The bottom of the sidebar is a persistent wallet/user panel with current points, estimated script capacity, avatar, VIP badge, nickname, and membership expiry.

Navigation is direct and low-friction. Every primary module is reachable without top navigation. The product keeps commercial status visible at all times because most AI actions consume points or membership quota.

## Homepage

The public homepage is a conversion page for an AI short-drama writing platform. It uses a clear registration offer: new users receive 24-hour membership and 1000 points. Logged-in users see calls to action for new script, web novel adaptation, script rewrite, and script evaluation.

The homepage explains the product through feature blocks: short-drama breakdown, script evaluation, inspiration planning, script creation, provider ecosystem, testimonials, and footer QR codes. The primary conversion path is to enter the workbench.

## Original Script Flow

The workbench starts with a `新建剧本` button. It opens a centered type modal with `AI短剧` and `真人实拍`. Selecting `AI短剧` enters inspiration planning.

Inspiration planning has a large text prompt area, tag groups, and two exits: `跳过` and `开始策划 50`. The paid planning button stays disabled until enough input exists. Tag groups expand inline, for example audience tags reveal `男频` and `女频`. This keeps the form compact but makes supported taxonomy discoverable.

Skipping creates a script workspace and lands on a four-step editor:

1. `故事梗概`
2. `人物小传`
3. `分集大纲`
4. `剧本正文`

The story settings page uses required fields for audience, genre, core setting, style, and synopsis. Optional fields include world view and highlights. A right-side anchor list mirrors the form sections.

Character, outline, and body pages use empty states with AI generation actions. Costs are displayed directly in button labels, such as `AI生成 50`, `一键生成50`, and `AI生成全部粗纲 80`. The body page supports export to Word and creating a new episode without payment. First-time guide overlays explain the episode generation, batch generation, and polishing actions.

The 2026-06-06 editor reshoot showed the internal creation page at `/creation/{id}/1` through `/4`. The editor keeps the left icon rail, but the main area switches to a white document workspace. The top has a script-name input, four route-like tabs, two disabled utility buttons, and a dark `灵感策划` button. The `故事梗概` tab currently renders a `故事设定` form with required target audience, genre, core setting, style element, and synopsis fields, plus optional world view and highlights. A right-side anchor list mirrors these sections.

`人物小传` is an empty state with `AI生成 50` and `一键生成50`, both cost-labelled. `分集大纲` includes `AI生成全部粗纲 80`, a rough outline rich-text placeholder, and a first episode card with 起-承-转-合 phases. It also triggers first-time guide bubbles: first for generating all rough outlines, then for generating episode outlines. `剧本正文` shows `导出为Word`, a `第1集` body textarea, and bottom page/episode indicators `1` and `10`.

`灵感策划` opens a centered white modal over the editor. It has `爆款榜单`, `跳过`, an AI planning title, a prompt/idea area, four wide option rows, and a disabled `开始策划50` button until prerequisites exist. Clicking `目标受众` expands inline to show `男频 / 女频`. Clicking `爆款榜单` opens a second large ranking modal with update date `2026年05月31日`, data-source copy, category tabs, ranked cards, heat values, and `对标策划` buttons. `对标策划` appears like a paid/AI action and was not executed.

## Rewrite And Adaptation

`剧本改写` and `网文改编` share the same upload-centered layout. Rewrite asks for a complete script and adaptation asks for a novel. Both include rights and safety copy stating the user retains ownership and the platform will not commercially use uploaded content.

The upload area validates file type and content asynchronously. A dummy text upload briefly showed `加载中，请稍后` and then returned to empty state, implying backend validation rejected the file or could not parse it.

The likely shared pipeline is: upload file, store object, create parse task, validate format and length, extract metadata, then create a rewrite/adaptation project when parsing succeeds.

## Script Evaluation

Evaluation is a marketing and form page. It highlights a `剧本评估系统 V1.0` style evaluation engine and shows example graphs. Form fields include culture, target audience, script type, audience preference, and upload. The file guidance is explicit: docx/txt, under 100k words, under 10M, including character/settings, outline, and at least 10 episodes.

`开始剧本评估` costs 2500 points and promises a 3-6 minute result. History opens as a right drawer with title, search box, and empty state. This implies evaluations are long-running jobs with status and report artifacts.

## Lapian

The lapian page is a poster grid with filters:

- Type: all, live action, AI
- Audience: all, male, female
- Time: all, last 14 days, last 30 days, older
- Background: modern, urban, ancient, village, era, workplace, fictional, palace, republic, campus, island

The first row also has search, page controls, `定制拉片`, selected-filter chips, and `我的拉片`. Poster cards reveal a hover detail panel rather than navigating to a separate page. The hover detail shows cover, title, episode count, audience/background/tags, launch date, platform, story background, highlights, synopsis, partial characters, partial rough outline, and partial episode outline.

Full details are gated by `免费解锁 剩1次`. The text indicates a quota-based unlock flow for members. I did not consume the free unlock.

Custom lapian opens a service modal, not an online payment flow. It explains manual custom service tiers, turnaround, QR code contact, and that custom results are protected for seven days before being published to the library.

## Membership

Membership lives at `/premium`. It shows current membership expiry, current role `剧本专家`, point recharge discount, and a prominent `续费会员` button. The rights list includes creation, storage, Word export, AI naming, AI settings, characters, outline, body generation, rewrite, adaptation, lapian, recharge discount, every-30-day point claim, script submission, editor services, custom lapian, inspiration planning, and future features.

The record table includes membership source, duration, and time. The current account has a registration gift record.

## Wallet

Wallet lives at `/wallet`. It shows the point balance, estimated script count, points help, recharge, enterprise recharge, point tasks, membership welfare, and income/expense records.

The points help modal explains:

- New users receive 1000 points.
- Recharge packages grant points.
- 100 yuan gives an extra 500 points; 300 yuan gives an extra 3000 points.
- Members can claim 500 points every 30 days.
- Member lapian consumes 100 points.
- AI functions consume their displayed points.

The recharge modal shows the member discount as 6.7x and three packages: 20 yuan for 2000 points, 100 yuan for 10500 points, 300 yuan for 33000 points. The recharge button is disabled until the recharge agreement checkbox is selected. I did not submit payment.

Enterprise recharge is a separate modal with 5000, 30000, and 100000 yuan packages, larger bonus percentages, business QR code, contract/payment/invoice flow, and one-workday point arrival.

## Community

Community is a course and ecosystem page, not a social feed. It contains a Feishu user tutorial link, a short-drama writer incubation program, text course information, video lesson cards, member-only lessons, and writer talks.

The bottom has three consultation cards: training, script submission, and official group. All three open QR-code modals with a single confirmation-style button.

## Settings

Settings has four tabs: profile, account security, contact us, and general settings.

Profile includes avatar and nickname. Account security shows the bound phone number, states it cannot be changed, shows masked password, offers password reset, and has logout. Password reset opens a modal requiring phone, SMS code, new password, and confirmation. Contact us shows QR codes for official service account, support, and official group. General settings did not switch content during testing, so it appears unfinished or unbound.

## Interaction Patterns

The platform consistently exposes cost before action. Paid or quota-consuming controls include the point amount in the button label and are disabled until prerequisites are met. Longer tasks are represented as async work: upload parsing, evaluation, AI generation, and likely lapian unlocks.

Feedback is mostly modal, drawer, disabled-state, empty-state, and guide-overlay based. There is little inline validation until an operation is attempted. The UI favors immediate discoverability over dense configuration.

The clone preserves this interaction contract: cost-bearing controls are visible but either disabled or intercepted with a toast. Recharge, payment, `开始策划50`, `AI生成50`, `AI生成全部粗纲80`, lapian unlock, and `对标策划` are not submitted in the local UI.

## Backend Behavior Inferred

The observed network calls include token refresh, user profile, point balance, script project lists, and homepage lapian loading. The product likely uses short-lived access tokens with refresh, a point wallet service, membership entitlement service, and async AI task workers.

Script generation likely stores structured sections rather than a single document: settings, characters, outlines, episodes, and generated versions. AI operations should be idempotent, reserve points before dispatch, and commit or release points based on task outcome.
