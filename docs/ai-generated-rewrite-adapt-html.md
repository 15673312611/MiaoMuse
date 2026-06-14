下面给出一套**不照抄品牌资源 / 源码**、但可复刻截图信息架构、布局状态和交互逻辑的前端实现规范。整体适用于 Vue 3 / Nuxt / Vite 项目，可直接将 HTML skeleton 嵌入 `.vue` 模板中，再接入状态数据与接口。

---

# 一、全局产品结构规范

## 1.1 公共布局

两个功能共用一套暗色工作台布局：

```txt
app-shell
├── app-sidebar                    左侧固定导航栏
├── app-main                       右侧主工作区
│   ├── page-header                页面标题 / 搜索 / 教程 / 上传
│   ├── page-content               页面主体
│   └── floating-ai-assistant      右下 AI 助手入口
└── global-modal-layer             弹窗层
```

## 1.2 视觉复刻原则

- 背景：深黑色工作台，叠加细斜线纹理。
- 左侧导航：固定宽度约 `108px`，深灰背景。
- 主内容：圆角容器，内容区域左上对齐。
- 卡片：深灰底 `#18191d` / `#1b1c20`，圆角 `12px`。
- 主按钮：
  - 白色按钮：上传 / 确认。
  - 紫蓝渐变按钮：AI 提炼 / 改写策划。
  - 蓝绿渐变 Tab：当前一级激活 Tab。
- 弹窗遮罩：黑色半透明 `rgba(0,0,0,.72)`。
- 不使用截图中的 logo、人物图、机器人图，可用 CSS 图形 / icon font / 占位图替代。

---

# 二、剧本改写 Rewrite

需要输出两个页面状态：

1. `rewrite list/upload`：列表页 + 导入剧本上传状态。
2. `rewrite infoflow/editor`：详情页信息流 + 故事设定编辑器。

---

## 2.1 页面 A：剧本改写列表 / 上传页

### 2.1.1 DOM 区块

```txt
rewrite-list-page
├── app-sidebar
├── rewrite-list-main
│   ├── rewrite-list-header
│   │   ├── page-title: 剧本改写
│   │   ├── rewrite-search
│   │   ├── tutorial-link: 使用教程
│   │   └── import-button: 导入剧本
│   ├── sort-toolbar
│   │   └── sort-button: 排序方式
│   ├── script-list
│   │   └── script-card[]
│   └── empty-state / upload-progress-state
└── floating-ai-assistant
```

### 2.1.2 主要字段

```ts
type RewriteProject = {
  id: string;
  title: string;
  sourceFileName: string;
  fileType: 'docx' | 'pdf' | 'txt';
  coverType: 'gradient' | 'custom';
  coverUrl?: string;
  tag?: string; // 例如：实拍、动画、短剧
  updatedAt: string;
  createdAt: string;
  status: 'ready' | 'uploading' | 'parsing' | 'failed';
  uploadProgress?: number;
  parseProgress?: number;
  synopsis?: string;
};
```

### 2.1.3 状态切换

| 状态 | 触发 | UI 表现 |
|---|---|---|
| 初始有项目 | 页面加载成功 | 显示项目卡片列表 |
| 空列表 | 无项目 | 显示空态 + “导入剧本”按钮 |
| 上传中 | 选择文件后 | 按钮 disabled，项目卡片可出现 loading 遮罩 |
| 解析中 | 上传完成后自动解析 | 显示“解析中...”进度 |
| 解析失败 | 后端返回失败 | 卡片显示失败状态，支持重新上传 |
| 点击卡片 | 用户点击项目 | 跳转到 rewrite detail/infoflow |

### 2.1.4 按钮文案

- 页面标题：`剧本改写`
- 搜索 placeholder：`根据名称、核心梗概进行搜索`
- 排序按钮：`排序方式`
- 教程：`使用教程`
- 主按钮：`导入剧本`
- 上传中：`上传中...`
- 解析中：`解析中...`
- 失败重试：`重新上传`

---

## 2.2 页面 B：剧本改写信息流 / 编辑器

### 2.2.1 DOM 区块

```txt
rewrite-detail-page
├── app-sidebar
├── rewrite-detail-main
│   ├── detail-header
│   │   ├── project-title
│   │   └── project-prefix-icon
│   ├── detail-toolbar
│   │   ├── primary-tabs
│   │   │   ├── 信息流
│   │   │   ├── 故事梗概
│   │   │   ├── 人物小传
│   │   │   ├── 分集大纲
│   │   │   └── 剧本正文
│   │   ├── history-actions
│   │   │   ├── undo
│   │   │   └── redo
│   │   ├── quote-button / rewrite-plan-button
│   │   └── ai-action-button
│   ├── infoflow-panel
│   │   ├── story-summary-card
│   │   ├── character-card
│   │   ├── episode-outline-card
│   │   └── script-text-card
│   ├── story-settings-editor
│   │   ├── gender-selector
│   │   ├── era-select
│   │   ├── theme-select
│   │   ├── core-setting-select
│   │   ├── story-background-input
│   │   ├── core-hook-input
│   │   └── core-summary-textarea
│   └── right-step-anchor
└── floating-ai-assistant
```

### 2.2.2 Tab 状态

一级 Tab：

```ts
type RewriteTab =
  | 'infoflow'
  | 'story'
  | 'characters'
  | 'episodes'
  | 'script';
```

| 当前 Tab | 页面主体 |
|---|---|
| 信息流 | 展示 AI 解析后的聚合信息卡片 |
| 故事梗概 | 展示故事设定编辑表单 |
| 人物小传 | 展示人物列表 / 人物详情编辑 |
| 分集大纲 | 展示分集卡片 |
| 剧本正文 | 展示正文编辑器 |

### 2.2.3 故事设定编辑字段

```ts
type RewriteStorySettings = {
  targetAudience: 'male' | 'female' | '';
  eraBackground: string[];
  themeTypes: string[];
  coreSettings: string[];
  storyBackground: string;
  coreHook: string;
  coreSummary: string;
};
```

### 2.2.4 表单限制

| 字段 | 必填 | 限制 |
|---|---:|---|
| 目标受众 | 是 | 单选：男频 / 女频 |
| 时代背景 | 是 | 最多 3 个 |
| 题材类型 | 是 | 最多 3 个 |
| 核心设定 | 是 | 最多 5 个 |
| 故事背景 | 否 | 建议 300 字内 |
| 核心亮点 | 否 | 建议 200 字内 |
| 核心梗概 | 是 | 建议 800 字内 |

### 2.2.5 右侧锚点

```ts
const rewriteStorySteps = [
  '目标受众',
  '时代背景',
  '题材类型',
  '核心设定',
  '故事背景',
  '核心亮点',
  '核心梗概'
];
```

交互：

- 页面滚动时高亮当前编辑区。
- 点击锚点滚动到对应表单块。
- 未完成必填项可显示红点或错误态。

---

# 三、网文改编 Adapt

需要覆盖：

1. `adapt empty/upload`
2. `type modal`
3. `chapter range modal`
4. `novel outline editor`

---

## 3.1 页面 A：网文改编空态 / 上传

### 3.1.1 DOM 区块

```txt
adapt-list-page
├── app-sidebar
├── adapt-main
│   ├── adapt-header
│   │   ├── page-title: 网文改编
│   │   ├── search-box
│   │   └── tutorial-link
│   ├── adapt-empty-state
│   │   ├── document-icon
│   │   ├── title: 您暂无改编剧本
│   │   ├── description
│   │   └── upload-button
│   └── copyright-tip
└── floating-ai-assistant
```

### 3.1.2 按钮文案

- 页面标题：`网文改编`
- 空态标题：`您暂无改编剧本`
- 空态说明：`点击按钮上传小说文件，即刻进行章纲拆解`
- 上传按钮：`上传文件`
- 底部提示：`您拥有剧本所有权与控制，我们承诺不利用数据进行任何商业用途`

### 3.1.3 上传后流程

```txt
点击上传文件
→ 打开文件选择器
→ 上传小说文件
→ 上传成功
→ 弹出改编类型选择弹窗
→ 选择类型
→ 弹出拆解范围弹窗
→ 确认拆解
→ 进入小说章纲生成页
```

---

## 3.2 弹窗 A：改编类型选择 Type Modal

截图中弹窗是一个居中的大图选择卡片，左侧偏动画感，右侧偏真人感。这里不要使用原始图，可使用占位图或 CSS 渐变图片区。

### 3.2.1 DOM 区块

```txt
adapt-type-modal
├── modal-mask
└── modal-panel.type-select-panel
    ├── type-card.animation
    │   ├── image-placeholder
    │   └── label: 动画改编
    └── type-card.live-action
        ├── image-placeholder
        └── label: 实拍改编
```

### 3.2.2 字段

```ts
type AdaptType = 'animation' | 'live_action';

type AdaptUploadSession = {
  uploadId: string;
  fileName: string;
  fileSize: number;
  fileType: 'txt' | 'docx' | 'pdf';
  adaptType?: AdaptType;
};
```

### 3.2.3 交互

| 操作 | 结果 |
|---|---|
| 点击动画区域 | `adaptType = animation`，关闭类型弹窗，打开拆解范围弹窗 |
| 点击实拍区域 | `adaptType = live_action`，关闭类型弹窗，打开拆解范围弹窗 |
| ESC / 点击遮罩 | 关闭弹窗，可回到空态 |
| 鼠标悬浮卡片 | 卡片高亮、边框变亮、轻微放大 |

---

## 3.3 弹窗 B：章节范围拆解 Chapter Range Modal

### 3.3.1 DOM 区块

```txt
chapter-range-modal
├── modal-mask
└── modal-panel.range-panel
    ├── modal-close
    ├── modal-title: 拆解范围
    ├── modal-subtitle: 小说共 x 章，总计 x 字，请选择需要拆解的范围
    ├── range-body
    │   ├── range-slider-column
    │   │   ├── start-chapter-input
    │   │   ├── vertical-slider
    │   │   └── end-chapter-input
    │   ├── chapter-list
    │   │   └── chapter-item[]
    │   └── chapter-preview
    │       ├── chapter-title
    │       ├── word-count-badge
    │       └── chapter-content
    ├── cost-summary
    │   ├── credit-cost
    │   └── cost-note
    └── modal-footer
        ├── format-button
        ├── split-button
        └── confirm-button
```

### 3.3.2 字段

```ts
type NovelChapter = {
  chapterNo: number;
  title: string;
  wordCount: number;
  contentPreview: string;
  selected: boolean;
};

type ChapterRangeState = {
  totalChapters: number;
  totalWords: number;
  startChapter: number;
  endChapter: number;
  selectedChapterNos: number[];
  costCredits: number;
};
```

### 3.3.3 交互规则

- 默认选中全部章节。
- 拖动竖向 slider 的上下端点，改变 `startChapter` 和 `endChapter`。
- 左侧章节列表只显示被选范围内的章节勾选态。
- 点击某一章，右侧预览区域展示该章标题、字数、正文片段。
- 点击 `格式`：
  - 触发小说格式检测。
  - 若章节识别失败，展示错误提示。
- 点击 `拆解`：
  - 可先执行章节切分预处理。
- 点击 `确认拆解`：
  - 创建改编项目。
  - 进入 `novel outline editor`。
  - 按范围发起 AI 章纲生成任务。

### 3.3.4 按钮状态

| 按钮 | 默认 | loading | disabled |
|---|---|---|---|
| 格式 | 可点击 | `检测中...` | 上传未完成 |
| 拆解 | 可点击 | `拆解中...` | 未选择章节 |
| 确认拆解 | 可点击 | `生成中...` | 积分不足 / 未选章节 |

---

## 3.4 页面 B：网文改编小说章纲 / 编辑器

### 3.4.1 DOM 区块

```txt
adapt-editor-page
├── app-sidebar
├── adapt-editor-main
│   ├── detail-header
│   │   ├── project-title
│   ├── editor-toolbar
│   │   ├── primary-tabs
│   │   │   ├── 小说章纲
│   │   │   ├── 故事梗概
│   │   │   ├── 人物小传
│   │   │   ├── 分集大纲
│   │   │   └── 剧本正文
│   │   ├── history-actions
│   │   │   ├── undo
│   │   │   └── redo
│   │   └── ai-generate-button / ai-polish-button
│   ├── outline-canvas
│   │   └── outline-card[]
│   ├── story-settings-editor
│   └── right-chapter-ruler
```

### 3.4.2 小说章纲卡片字段

```ts
type NovelOutlineCard = {
  id: string;
  chapterNo: number;
  chapterTitle: string;
  logline: string;
  beats: {
    id: string;
    label: string; // 情节1 / 情节2
    content: string;
  }[];
  status: 'generating' | 'done' | 'failed';
  selected?: boolean;
};
```

### 3.4.3 生成中状态

- 页面右上按钮显示：`生成中`
- 按钮带旋转 loading icon。
- 当前正在生成的章纲卡片正常亮显。
- 已生成但非当前视区的卡片可降低透明度。
- 右侧章节标尺显示当前生成位置。

### 3.4.4 生成完成状态

- 卡片以瀑布流 / 双列布局展示。
- 当前选中卡片高亮。
- 右侧章节标尺可从 1 到 10 或实际章节数。
- 点击标尺数字滚动到对应章节卡片。
- 卡片支持：
  - 单击选中。
  - 双击进入编辑。
  - hover 显示更多操作：重新生成 / 删除 / 复制。

---

# 四、后端接口契约

以下接口为建议协议，可按实际网关调整。

---

## 4.1 文件上传接口

### `POST /api/files/upload`

`Content-Type: multipart/form-data`

### Request

```ts
{
  file: File;
  bizType: 'rewrite' | 'adapt';
}
```

### Response

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "uploadId": "upl_123",
    "fileName": "新建 DOCX 文档.docx",
    "fileType": "docx",
    "fileSize": 238912,
    "url": "https://cdn.example.com/files/upl_123.docx"
  }
}
```

---

## 4.2 剧本改写项目列表

### `GET /api/rewrite/projects`

### Query

```ts
{
  keyword?: string;
  sortBy?: 'updatedAt' | 'createdAt' | 'title';
  order?: 'asc' | 'desc';
  page?: number;
  pageSize?: number;
}
```

### Response

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "id": "rw_001",
        "title": "新建 DOCX 文档",
        "sourceFileName": "新建 DOCX 文档.docx",
        "fileType": "docx",
        "tag": "实拍",
        "updatedAt": "2025-06-07T10:08:00+08:00",
        "status": "ready",
        "coverType": "gradient"
      }
    ],
    "total": 1
  }
}
```

---

## 4.3 创建剧本改写项目

### `POST /api/rewrite/projects`

### Request

```json
{
  "uploadId": "upl_123",
  "title": "新建 DOCX 文档"
}
```

### Response

```json
{
  "code": 0,
  "data": {
    "projectId": "rw_001",
    "taskId": "task_parse_001",
    "status": "parsing"
  }
}
```

---

## 4.4 获取剧本改写详情

### `GET /api/rewrite/projects/{projectId}`

### Response

```json
{
  "code": 0,
  "data": {
    "id": "rw_001",
    "title": "新建 DOCX 文档",
    "activeTab": "infoflow",
    "infoflow": {
      "story": {
        "targetAudience": "female",
        "eraBackground": ["古代"],
        "themeTypes": ["宅斗"],
        "coreSettings": ["真假千金", "逆袭", "虐恋", "大女主", "重生"],
        "storyBackground": "在等级森严的古代世家社会中...",
        "coreHook": "重生嫡女以逆袭身份开启复仇...",
        "coreSummary": "在灵脉天赋决定一切的世家..."
      },
      "characters": [
        {
          "id": "ch_001",
          "name": "苏明远",
          "age": "45岁",
          "role": "苏家家主",
          "description": "注重家族颜面..."
        }
      ],
      "episodes": [],
      "scriptText": []
    }
  }
}
```

---

## 4.5 保存剧本改写故事设定

### `PUT /api/rewrite/projects/{projectId}/story-settings`

### Request

```json
{
  "targetAudience": "female",
  "eraBackground": ["古代"],
  "themeTypes": ["宅斗"],
  "coreSettings": ["真假千金", "逆袭"],
  "storyBackground": "故事背景文本",
  "coreHook": "核心亮点文本",
  "coreSummary": "核心梗概文本"
}
```

### Response

```json
{
  "code": 0,
  "data": {
    "savedAt": "2025-06-07T12:00:00+08:00"
  }
}
```

---

## 4.6 发起剧本改写策划

### `POST /api/rewrite/projects/{projectId}/rewrite-plan`

### Request

```json
{
  "settings": {
    "targetAudience": "female",
    "eraBackground": ["古代"],
    "themeTypes": ["宅斗"],
    "coreSettings": ["真假千金", "逆袭"],
    "storyBackground": "故事背景",
    "coreHook": "核心亮点",
    "coreSummary": "核心梗概"
  }
}
```

### Response

```json
{
  "code": 0,
  "data": {
    "taskId": "task_rewrite_plan_001",
    "status": "queued"
  }
}
```

---

## 4.7 网文上传后解析章节

### `POST /api/adapt/novels/parse`

### Request

```json
{
  "uploadId": "upl_999"
}
```

### Response

```json
{
  "code": 0,
  "data": {
    "novelId": "novel_001",
    "title": "剧本名称",
    "totalChapters": 5,
    "totalWords": 1324,
    "chapters": [
      {
        "chapterNo": 1,
        "title": "第1章 出狱",
        "wordCount": 277,
        "contentPreview": "沈临风命令保镖动手..."
      }
    ]
  }
}
```

---

## 4.8 创建网文改编项目并生成章纲

### `POST /api/adapt/projects`

### Request

```json
{
  "novelId": "novel_001",
  "title": "剧本名称",
  "adaptType": "live_action",
  "range": {
    "startChapter": 1,
    "endChapter": 5,
    "selectedChapterNos": [1, 2, 3, 4, 5]
  }
}
```

### Response

```json
{
  "code": 0,
  "data": {
    "projectId": "ad_001",
    "taskId": "task_outline_001",
    "status": "generating"
  }
}
```

---

## 4.9 获取网文改编章纲

### `GET /api/adapt/projects/{projectId}/outline`

### Response

```json
{
  "code": 0,
  "data": {
    "projectId": "ad_001",
    "title": "剧本名称",
    "generationStatus": "done",
    "currentChapterNo": 5,
    "outline": [
      {
        "id": "oc_001",
        "chapterNo": 1,
        "chapterTitle": "第1章 出狱",
        "logline": "秦川出狱即遭林家保镖威胁...",
        "beats": [
          {
            "id": "beat_001",
            "label": "情节1",
            "content": "秦川提着旧布包走出江城第一监狱。"
          }
        ],
        "status": "done"
      }
    ]
  }
}
```

---

# 五、AI 后端应生成的 JSON 字段

## 5.1 剧本改写 AI 解析 JSON

AI 需要从上传剧本中生成：

```json
{
  "projectTitle": "新建 DOCX 文档",
  "story": {
    "targetAudience": "female",
    "eraBackground": ["古代"],
    "themeTypes": ["宅斗"],
    "coreSettings": ["真假千金", "逆袭", "虐恋", "大女主", "重生"],
    "storyBackground": "故事背景，说明世界、社会规则、主角处境。",
    "coreHook": "一句或一段高概念卖点。",
    "coreSummary": "完整故事梗概，包含主角、冲突、目标、反转和结局方向。"
  },
  "characters": [
    {
      "id": "ch_001",
      "name": "角色名",
      "age": "年龄或年龄段",
      "gender": "male/female/unknown",
      "role": "身份定位",
      "personality": "性格关键词",
      "motivation": "人物目标",
      "relationship": "与主角关系",
      "description": "人物小传"
    }
  ],
  "episodeOutline": [
    {
      "episodeNo": 1,
      "title": "第1集标题",
      "summary": "本集剧情摘要",
      "turningPoint": "关键转折",
      "cliffhanger": "结尾钩子"
    }
  ],
  "scriptSections": [
    {
      "sectionId": "sc_001",
      "title": "场次或章节标题",
      "content": "剧本正文或拆分后的文本",
      "characters": ["角色A", "角色B"],
      "location": "地点",
      "timeOfDay": "日/夜/未知"
    }
  ]
}
```

---

## 5.2 网文改编 AI 章纲 JSON

AI 需要从小说章节中生成：

```json
{
  "projectTitle": "剧本名称",
  "adaptType": "live_action",
  "outline": [
    {
      "chapterNo": 1,
      "chapterTitle": "第1章 出狱",
      "logline": "一句话概括该章戏剧冲突。",
      "dramaticGoal": "本章改编目标",
      "mainConflict": "主要冲突",
      "characters": ["秦川", "林雪"],
      "scenes": [
        {
          "sceneNo": 1,
          "location": "江城第一监狱门口",
          "timeOfDay": "清晨",
          "summary": "场景摘要",
          "visualPoint": "影像化看点"
        }
      ],
      "beats": [
        {
          "label": "情节1",
          "content": "可直接进入剧本分场的情节点。",
          "function": "铺垫/冲突/反转/高潮/钩子"
        }
      ],
      "cliffhanger": "本章结尾悬念",
      "estimatedDuration": 90
    }
  ],
  "storySettings": {
    "targetAudience": "male",
    "themeTypes": ["逆袭", "都市", "复仇"],
    "coreSettings": ["出狱归来", "隐藏身份"],
    "styleElements": ["强爽点", "高反差", "快节奏"],
    "worldview": "世界观说明",
    "coreHook": "核心亮点",
    "coreSummary": "核心梗概"
  },
  "characters": [
    {
      "name": "秦川",
      "role": "男主",
      "description": "人物简介",
      "arc": "人物成长线"
    }
  ]
}
```

---

# 六、可直接嵌入 Vue 的 HTML Skeleton

> 说明：以下只提供结构，不绑定具体品牌资源。`v-if`、`v-for`、`:class`、`@click` 可直接接入 Vue 状态。

---

## 6.1 公共布局 Skeleton

```html
<div class="app-shell theme-dark">
  <aside class="app-sidebar">
    <div class="sidebar-logo" aria-label="应用标识">剧</div>

    <nav class="sidebar-nav">
      <button class="sidebar-nav__item" type="button" aria-label="剧本工具">
        <span class="icon icon-script"></span>
      </button>
      <button class="sidebar-nav__item is-active" type="button" aria-label="剧本改写">
        <span class="icon icon-rewrite"></span>
      </button>
      <button class="sidebar-nav__item" type="button" aria-label="网文改编">
        <span class="icon icon-book"></span>
      </button>

      <div class="sidebar-divider"></div>

      <button class="sidebar-nav__item" type="button" aria-label="安全">
        <span class="icon icon-shield"></span>
      </button>
      <button class="sidebar-nav__item" type="button" aria-label="热点">
        <span class="icon icon-fire"></span>
      </button>
      <button class="sidebar-nav__item" type="button" aria-label="会员">
        <span class="icon icon-diamond"></span>
      </button>
      <button class="sidebar-nav__item" type="button" aria-label="钱包">
        <span class="icon icon-wallet"></span>
      </button>
      <button class="sidebar-nav__item" type="button" aria-label="团队">
        <span class="icon icon-users"></span>
      </button>
      <button class="sidebar-nav__item" type="button" aria-label="设置">
        <span class="icon icon-setting"></span>
      </button>
    </nav>

    <div class="sidebar-footer">
      <div class="credit-balance">
        <strong>{{ userCredits }}</strong>
        <span>点</span>
      </div>
      <button class="user-avatar" type="button" aria-label="用户中心">
        <img :src="userAvatar" alt="" />
      </button>
    </div>
  </aside>

  <main class="app-main">
    <!-- 具体页面插槽 -->
  </main>

  <button class="floating-ai-assistant" type="button" aria-label="AI 助手">
    <span class="assistant-face"></span>
  </button>
</div>
```

---

## 6.2 剧本改写：列表 / 上传页 Skeleton

```html
<section class="rewrite-list-page">
  <header class="page-header rewrite-list-header">
    <h1 class="page-title">剧本改写</h1>

    <div class="header-search">
      <input
        v-model="rewriteQuery.keyword"
        class="header-search__input"
        type="search"
        placeholder="根据名称、核心梗概进行搜索"
        @keyup.enter="fetchRewriteProjects"
      />
      <button class="header-search__button" type="button" @click="fetchRewriteProjects">
        <span class="icon icon-search"></span>
      </button>
    </div>

    <a class="header-link" href="/help/rewrite" target="_blank">使用教程</a>

    <button
      class="primary-upload-button"
      type="button"
      :disabled="uploadState.status === 'uploading'"
      @click="openRewriteFilePicker"
    >
      <span class="icon icon-upload"></span>
      <span>{{ uploadState.status === 'uploading' ? '上传中...' : '导入剧本' }}</span>
    </button>

    <input
      ref="rewriteFileInput"
      class="visually-hidden"
      type="file"
      accept=".doc,.docx,.pdf,.txt"
      @change="handleRewriteFileChange"
    />
  </header>

  <section class="rewrite-list-content">
    <div class="sort-toolbar">
      <button class="sort-button" type="button" @click="toggleSortPopover">
        <span class="icon icon-menu"></span>
        <span>排序方式</span>
      </button>

      <div v-if="sortPopoverVisible" class="sort-popover">
        <button type="button" @click="setRewriteSort('updatedAt')">最近修改</button>
        <button type="button" @click="setRewriteSort('createdAt')">创建时间</button>
        <button type="button" @click="setRewriteSort('title')">名称</button>
      </div>
    </div>

    <div v-if="rewriteProjects.length" class="script-list">
      <article
        v-for="project in rewriteProjects"
        :key="project.id"
        class="script-card"
        :class="{
          'is-uploading': project.status === 'uploading',
          'is-parsing': project.status === 'parsing',
          'is-failed': project.status === 'failed'
        }"
        @click="goRewriteDetail(project.id)"
      >
        <div class="script-card__cover">
          <span v-if="project.tag" class="script-card__tag">{{ project.tag }}</span>
          <div class="script-card__cover-pattern"></div>

          <div v-if="project.status === 'uploading'" class="script-card__progress-mask">
            <span>上传中 {{ project.uploadProgress || 0 }}%</span>
          </div>

          <div v-if="project.status === 'parsing'" class="script-card__progress-mask">
            <span>解析中...</span>
          </div>
        </div>

        <div class="script-card__body">
          <h2 class="script-card__title">{{ project.title }}</h2>
          <p class="script-card__meta">
            上次修改于 {{ formatDate(project.updatedAt) }}
          </p>

          <button
            v-if="project.status === 'failed'"
            class="script-card__retry"
            type="button"
            @click.stop="retryRewriteUpload(project)"
          >
            重新上传
          </button>
        </div>
      </article>
    </div>

    <div v-else class="empty-state rewrite-empty-state">
      <div class="empty-state__icon">
        <span class="icon icon-file-search"></span>
      </div>
      <h2 class="empty-state__title">暂无改写剧本</h2>
      <p class="empty-state__desc">导入剧本文件后，即可进行故事信息提取与改写策划。</p>
      <button class="empty-state__button" type="button" @click="openRewriteFilePicker">
        导入剧本
      </button>
    </div>
  </section>
</section>
```

---

## 6.3 剧本改写：信息流 / 编辑器 Skeleton

```html
<section class="rewrite-detail-page">
  <header class="detail-header">
    <h1 class="detail-title">
      <span class="detail-title__icon">✦</span>
      <span>{{ rewriteProject.title }}</span>
    </h1>
  </header>

  <section class="detail-toolbar">
    <nav class="primary-tabs">
      <button
        class="primary-tab is-gradient"
        :class="{ 'is-active': activeRewriteTab === 'infoflow' }"
        type="button"
        @click="activeRewriteTab = 'infoflow'"
      >
        信息流
      </button>
      <button
        class="primary-tab"
        :class="{ 'is-active': activeRewriteTab === 'story' }"
        type="button"
        @click="activeRewriteTab = 'story'"
      >
        故事梗概
      </button>
      <button
        class="primary-tab"
        :class="{ 'is-active': activeRewriteTab === 'characters' }"
        type="button"
        @click="activeRewriteTab = 'characters'"
      >
        人物小传
      </button>
      <button
        class="primary-tab"
        :class="{ 'is-active': activeRewriteTab === 'episodes' }"
        type="button"
        @click="activeRewriteTab = 'episodes'"
      >
        分集大纲
      </button>
      <button
        class="primary-tab"
        :class="{ 'is-active': activeRewriteTab === 'script' }"
        type="button"
        @click="activeRewriteTab = 'script'"
      >
        剧本正文
      </button>
    </nav>

    <div class="toolbar-actions">
      <button class="icon-button" type="button" @click="undoRewrite">
        <span class="icon icon-undo"></span>
      </button>
      <button class="icon-button" type="button" @click="redoRewrite">
        <span class="icon icon-redo"></span>
      </button>

      <button
        v-if="activeRewriteTab === 'infoflow'"
        class="secondary-button"
        type="button"
        @click="quoteAllInfo"
      >
        <span class="icon icon-copy"></span>
        一键引用
      </button>

      <button
        v-else
        class="gradient-action-button"
        type="button"
        :disabled="rewritePlanState.loading"
        @click="generateRewritePlan"
      >
        <span class="icon icon-lightbulb"></span>
        {{ rewritePlanState.loading ? '生成中' : '改写策划' }}
      </button>
    </div>
  </section>

  <section v-if="activeRewriteTab === 'infoflow'" class="infoflow-panel">
    <article class="info-card story-info-card">
      <h2 class="info-card__title">故事梗概</h2>

      <div class="story-meta-grid">
        <div class="story-meta-item">
          <strong>目标受众</strong>
          <span>{{ rewriteInfo.story.targetAudienceLabel }}</span>
        </div>
        <div class="story-meta-item">
          <strong>时代背景</strong>
          <span>{{ rewriteInfo.story.eraBackground.join('、') }}</span>
        </div>
        <div class="story-meta-item">
          <strong>题材类型</strong>
          <span>{{ rewriteInfo.story.themeTypes.join('、') }}</span>
        </div>
        <div class="story-meta-item">
          <strong>核心设定</strong>
          <span>{{ rewriteInfo.story.coreSettings.join('、') }}</span>
        </div>
      </div>

      <section class="info-section">
        <h3>故事背景</h3>
        <p>{{ rewriteInfo.story.storyBackground }}</p>
      </section>

      <section class="info-section">
        <h3>核心亮点</h3>
        <p>{{ rewriteInfo.story.coreHook }}</p>
      </section>

      <section class="info-section">
        <h3>核心梗概</h3>
        <p>{{ rewriteInfo.story.coreSummary }}</p>
      </section>
    </article>

    <article class="info-card character-info-card">
      <h2 class="info-card__title">人物设定</h2>

      <div
        v-for="character in rewriteInfo.characters"
        :key="character.id"
        class="character-row"
      >
        <div class="character-row__head">
          <strong>{{ character.name }}</strong>
          <span>{{ character.age }}</span>
        </div>
        <p>{{ character.description }}</p>
      </div>
    </article>
  </section>

  <section v-if="activeRewriteTab === 'story'" class="story-settings-layout">
    <form class="story-settings-form" @submit.prevent="saveRewriteStorySettings">
      <h2 class="form-section-title">故事设定</h2>

      <div class="form-row two-columns">
        <fieldset class="form-field">
          <legend>目标受众 <em>*</em></legend>
          <div class="segmented-options">
            <button
              class="audience-option male"
              :class="{ 'is-selected': rewriteSettings.targetAudience === 'male' }"
              type="button"
              @click="rewriteSettings.targetAudience = 'male'"
            >
              ♂ 男频
            </button>
            <button
              class="audience-option female"
              :class="{ 'is-selected': rewriteSettings.targetAudience === 'female' }"
              type="button"
              @click="rewriteSettings.targetAudience = 'female'"
            >
              ♀ 女频
            </button>
          </div>
        </fieldset>

        <label class="form-field">
          <span>时代背景 <em>*</em>（不超过3个）</span>
          <div class="select-like-input" @click="openOptionPicker('eraBackground')">
            <span>{{ rewriteSettings.eraBackground.length ? rewriteSettings.eraBackground.join('、') : '选择或自定义输入背景' }}</span>
            <span class="icon icon-chevron-down"></span>
          </div>
        </label>
      </div>

      <div class="form-row two-columns">
        <label class="form-field">
          <span>题材类型 <em>*</em>（不超过3个）</span>
          <div class="select-like-input" @click="openOptionPicker('themeTypes')">
            <span>{{ rewriteSettings.themeTypes.length ? rewriteSettings.themeTypes.join('、') : '选择或自定义输入类型' }}</span>
            <span class="icon icon-chevron-down"></span>
          </div>
        </label>

        <label class="form-field">
          <span>核心设定 <em>*</em>（不超过5个）</span>
          <div class="select-like-input" @click="openOptionPicker('coreSettings')">
            <span>{{ rewriteSettings.coreSettings.length ? rewriteSettings.coreSettings.join('、') : '选择或自定义输入设定' }}</span>
            <span class="icon icon-chevron-down"></span>
          </div>
        </label>
      </div>

      <label class="form-field full-width" data-anchor="故事背景">
        <span>故事背景（选填）</span>
        <textarea
          v-model="rewriteSettings.storyBackground"
          class="dark-textarea compact"
          placeholder="例：1932年，伪满洲国成立前夕的北平。表面繁华依旧，实则暗流涌动，人物界人人自危。"
        ></textarea>
      </label>

      <label class="form-field full-width" data-anchor="核心亮点">
        <span>核心亮点（选填）</span>
        <textarea
          v-model="rewriteSettings.coreHook"
          class="dark-textarea compact"
          placeholder="例：穿书成作死女配，她反向操作，用沙雕和直球硬核攻略哑巴老公，结果把冰山霸总撩到开口说话！"
        ></textarea>
      </label>

      <label class="form-field full-width" data-anchor="核心梗概">
        <span>核心梗概 <em>*</em></span>
        <textarea
          v-model="rewriteSettings.coreSummary"
          class="dark-textarea large"
        ></textarea>
      </label>
    </form>

    <aside class="right-step-anchor">
      <button
        v-for="step in rewriteStorySteps"
        :key="step"
        class="step-anchor-item"
        :class="{ 'is-active': currentStoryStep === step }"
        type="button"
        @click="scrollToStoryStep(step)"
      >
        <span class="step-anchor-item__dot"></span>
        <span>{{ step }}</span>
      </button>
    </aside>
  </section>
</section>
```

---

## 6.4 网文改编：空态 / 上传 Skeleton

```html
<section class="adapt-list-page">
  <header class="page-header adapt-list-header">
    <h1 class="page-title">网文改编</h1>

    <div class="header-search">
      <input
        v-model="adaptQuery.keyword"
        class="header-search__input"
        type="search"
        placeholder="根据名称、核心梗概进行搜索"
        @keyup.enter="fetchAdaptProjects"
      />
      <button class="header-search__button" type="button" @click="fetchAdaptProjects">
        <span class="icon icon-search"></span>
      </button>
    </div>

    <a class="header-link" href="/help/adapt" target="_blank">使用教程</a>
  </header>

  <section class="adapt-empty-state">
    <div class="empty-document-icon">
      <span class="icon icon-file-search"></span>
    </div>

    <h2>您暂无改编剧本</h2>
    <p>点击按钮上传小说文件，即刻进行章纲拆解</p>

    <button
      class="white-action-button"
      type="button"
      :disabled="adaptUploadState.status === 'uploading'"
      @click="openAdaptFilePicker"
    >
      {{ adaptUploadState.status === 'uploading' ? '上传中...' : '上传文件' }}
    </button>

    <input
      ref="adaptFileInput"
      class="visually-hidden"
      type="file"
      accept=".txt,.doc,.docx,.pdf"
      @change="handleAdaptFileChange"
    />
  </section>

  <footer class="copyright-tip">
    <span class="icon icon-info"></span>
    <span>您拥有剧本所有权与控制，我们承诺不利用数据进行任何商业用途</span>
  </footer>
</section>
```

---

## 6.5 网文改编：类型选择弹窗 Skeleton

```html
<div
  v-if="adaptTypeModalVisible"
  class="modal-layer adapt-type-modal-layer"
  @click.self="closeAdaptTypeModal"
>
  <section class="adapt-type-modal" role="dialog" aria-modal="true">
    <button class="modal-close visually-hidden" type="button" @click="closeAdaptTypeModal">
      关闭
    </button>

    <button
      class="adapt-type-card adapt-type-card--animation"
      type="button"
      @click="selectAdaptType('animation')"
    >
      <div class="adapt-type-card__image gradient-animation-placeholder"></div>
      <span class="adapt-type-card__label">动画改编</span>
    </button>

    <button
      class="adapt-type-card adapt-type-card--live"
      type="button"
      @click="selectAdaptType('live_action')"
    >
      <div class="adapt-type-card__image gradient-live-placeholder"></div>
      <span class="adapt-type-card__label">实拍改编</span>
    </button>
  </section>
</div>
```

---

## 6.6 网文改编：章节范围弹窗 Skeleton

```html
<div
  v-if="chapterRangeModalVisible"
  class="modal-layer chapter-range-modal-layer"
  @click.self="closeChapterRangeModal"
>
  <section class="chapter-range-modal" role="dialog" aria-modal="true">
    <button class="modal-close" type="button" @click="closeChapterRangeModal">
      ×
    </button>

    <header class="chapter-range-header">
      <h2>拆解范围</h2>
      <p>
        小说共<strong>{{ chapterRange.totalChapters }}</strong>章，
        总计<strong>{{ chapterRange.totalWords }}</strong>字，
        请选择需要拆解的范围
      </p>
    </header>

    <div class="chapter-range-body">
      <aside class="range-slider-column">
        <input
          v-model.number="chapterRange.startChapter"
          class="chapter-number-input"
          type="number"
          min="1"
          :max="chapterRange.endChapter"
          @change="syncChapterRange"
        />

        <div class="vertical-range-slider">
          <input
            v-model.number="chapterRange.startChapter"
            type="range"
            min="1"
            :max="chapterRange.totalChapters"
            orient="vertical"
            @input="syncChapterRange"
          />
          <input
            v-model.number="chapterRange.endChapter"
            type="range"
            min="1"
            :max="chapterRange.totalChapters"
            orient="vertical"
            @input="syncChapterRange"
          />
        </div>

        <input
          v-model.number="chapterRange.endChapter"
          class="chapter-number-input"
          type="number"
          :min="chapterRange.startChapter"
          :max="chapterRange.totalChapters"
          @change="syncChapterRange"
        />
      </aside>

      <nav class="chapter-check-list">
        <button
          v-for="chapter in parsedNovel.chapters"
          :key="chapter.chapterNo"
          class="chapter-check-item"
          :class="{
            'is-selected': chapterRange.selectedChapterNos.includes(chapter.chapterNo),
            'is-current': previewChapter.chapterNo === chapter.chapterNo
          }"
          type="button"
          @click="selectPreviewChapter(chapter)"
        >
          <input
            type="checkbox"
            :checked="chapterRange.selectedChapterNos.includes(chapter.chapterNo)"
            @change.stop="toggleChapterSelected(chapter.chapterNo)"
          />
          <span>第{{ chapter.chapterNo }}章</span>
        </button>
      </nav>

      <article class="chapter-preview">
        <header class="chapter-preview__header">
          <h3>{{ previewChapter.title }}</h3>
          <span class="word-count-badge">{{ previewChapter.wordCount }}字</span>
        </header>
        <p class="chapter-preview__content">
          {{ previewChapter.contentPreview }}
        </p>
      </article>
    </div>

    <footer class="chapter-range-footer">
      <div class="credit-cost">
        <strong>{{ chapterRange.costCredits }}</strong>
        <span>点</span>
        <p>消耗剧点（400字/剧点，已选{{ chapterRange.selectedChapterNos.length }}章，共{{ selectedWords }}字）</p>
      </div>

      <div class="range-footer-actions">
        <button
          class="ghost-action-button"
          type="button"
          :disabled="rangeActionState.formatting"
          @click="detectNovelFormat"
        >
          <span class="icon icon-alert"></span>
          {{ rangeActionState.formatting ? '检测中...' : '格式' }}
        </button>

        <button
          class="ghost-action-button"
          type="button"
          :disabled="!chapterRange.selectedChapterNos.length || rangeActionState.splitting"
          @click="splitNovelChapters"
        >
          <span class="icon icon-alert"></span>
          {{ rangeActionState.splitting ? '拆解中...' : '拆解' }}
        </button>

        <button
          class="white-action-button"
          type="button"
          :disabled="!canConfirmAdaptRange"
          @click="confirmChapterRange"
        >
          {{ rangeActionState.confirming ? '生成中...' : '确认拆解' }}
        </button>
      </div>
    </footer>
  </section>
</div>
```

---

## 6.7 网文改编：小说章纲编辑器 Skeleton

```html
<section class="adapt-editor-page">
  <header class="detail-header">
    <h1 class="detail-title">
      <span class="detail-title__icon">✦</span>
      <span>{{ adaptProject.title || '剧本名称' }}</span>
    </h1>
  </header>

  <section class="editor-toolbar">
    <nav class="primary-tabs">
      <button
        class="primary-tab is-gradient"
        :class="{ 'is-active': activeAdaptTab === 'outline' }"
        type="button"
        @click="activeAdaptTab = 'outline'"
      >
        小说章纲
      </button>
      <button
        class="primary-tab"
        :class="{ 'is-active': activeAdaptTab === 'story' }"
        type="button"
        @click="activeAdaptTab = 'story'"
      >
        故事梗概
      </button>
      <button
        class="primary-tab"
        :class="{ 'is-active': activeAdaptTab === 'characters' }"
        type="button"
        @click="activeAdaptTab = 'characters'"
      >
        人物小传
      </button>
      <button
        class="primary-tab"
        :class="{ 'is-active': activeAdaptTab === 'episodes' }"
        type="button"
        @click="activeAdaptTab = 'episodes'"
      >
        分集大纲
      </button>
      <button
        class="primary-tab"
        :class="{ 'is-active': activeAdaptTab === 'script' }"
        type="button"
        @click="activeAdaptTab = 'script'"
      >
        剧本正文
      </button>
    </nav>

    <div class="toolbar-actions">
      <button class="icon-button" type="button" @click="undoAdapt">
        <span class="icon icon-undo"></span>
      </button>
      <button class="icon-button" type="button" @click="redoAdapt">
        <span class="icon icon-redo"></span>
      </button>

      <button
        v-if="activeAdaptTab === 'outline'"
        class="generation-button"
        :class="{ 'is-loading': outlineGeneration.status === 'generating' }"
        type="button"
        :disabled="outlineGeneration.status === 'generating'"
      >
        <span class="loading-ring" v-if="outlineGeneration.status === 'generating'"></span>
        {{ outlineGeneration.status === 'generating' ? '生成中' : '重新生成' }}
      </button>

      <button
        v-if="activeAdaptTab === 'story'"
        class="gradient-action-button"
        type="button"
        @mouseenter="showAiPolishTip = true"
        @mouseleave="showAiPolishTip = false"
        @click="aiPolishAdaptSettings"
      >
        <span class="icon icon-sparkle"></span>
        AI提炼 50点
      </button>

      <div v-if="showAiPolishTip" class="ai-polish-popover">
        <strong>点击AI，一键提炼全部设定</strong>
        <p>根据原小说章纲，提炼全部设定内容</p>
        <button type="button" @click="showAiPolishTip = false">知道了</button>
      </div>
    </div>
  </section>

  <section v-if="activeAdaptTab === 'outline'" class="outline-editor-layout">
    <div class="outline-canvas">
      <article
        v-for="card in novelOutline"
        :key="card.id"
        class="outline-card"
        :class="{
          'is-generating': card.status === 'generating',
          'is-selected': selectedOutlineId === card.id,
          'is-muted': outlineGeneration.status === 'generating' && card.status !== 'generating'
        }"
        @click="selectedOutlineId = card.id"
        @dblclick="editOutlineCard(card)"
      >
        <header class="outline-card__header">
          <h2>第{{ card.chapterNo }}章 {{ card.chapterTitle }}</h2>
        </header>

        <p class="outline-card__logline">
          {{ card.logline }}
        </p>

        <ul class="outline-card__beats">
          <li v-for="beat in card.beats" :key="beat.id">
            <strong>{{ beat.label }}：</strong>
            <span>{{ beat.content }}</span>
          </li>
        </ul>

        <div class="outline-card__actions">
          <button type="button" @click.stop="regenerateOutlineCard(card)">重新生成</button>
          <button type="button" @click.stop="copyOutlineCard(card)">复制</button>
          <button type="button" @click.stop="deleteOutlineCard(card)">删除</button>
        </div>
      </article>
    </div>

    <aside class="right-chapter-ruler">
      <button
        v-for="chapterNo in chapterRulerNumbers"
        :key="chapterNo"
        class="ruler-number"
        :class="{ 'is-active': currentVisibleChapter === chapterNo }"
        type="button"
        @click="scrollToChapter(chapterNo)"
      >
        {{ chapterNo }}
      </button>
      <div class="ruler-line">
        <span
          v-for="tick in chapterRulerTicks"
          :key="tick"
          class="ruler-tick"
        ></span>
      </div>
    </aside>
  </section>

  <section v-if="activeAdaptTab === 'story'" class="story-settings-layout adapt-story-settings">
    <form class="story-settings-form" @submit.prevent="saveAdaptStorySettings">
      <h2 class="form-section-title">故事设定</h2>

      <div class="form-row two-columns">
        <fieldset class="form-field">
          <legend>目标受众 <em>*</em></legend>
          <div class="segmented-options">
            <button
              class="audience-option male"
              :class="{ 'is-selected': adaptSettings.targetAudience === 'male' }"
              type="button"
              @click="adaptSettings.targetAudience = 'male'"
            >
              ♂ 男频
            </button>
            <button
              class="audience-option female"
              :class="{ 'is-selected': adaptSettings.targetAudience === 'female' }"
              type="button"
              @click="adaptSettings.targetAudience = 'female'"
            >
              ♀ 女频
            </button>
          </div>
        </fieldset>

        <label class="form-field">
          <span>题材类型 <em>*</em>（不超过3个）</span>
          <div class="select-like-input" @click="openAdaptOptionPicker('themeTypes')">
            <span>{{ adaptSettings.themeTypes.length ? adaptSettings.themeTypes.join('、') : '选择或自定义输入背景' }}</span>
            <span class="icon icon-chevron-down"></span>
          </div>
        </label>
      </div>

      <div class="form-row two-columns">
        <label class="form-field">
          <span>核心设定 <em>*</em>（不超过3个）</span>
          <div class="select-like-input" @click="openAdaptOptionPicker('coreSettings')">
            <span>{{ adaptSettings.coreSettings.length ? adaptSettings.coreSettings.join('、') : '选择或自定义输入类型' }}</span>
            <span class="icon icon-chevron-down"></span>
          </div>
        </label>

        <label class="form-field">
          <span>风格元素 <em>*</em>（不超过5个）</span>
          <div class="select-like-input" @click="openAdaptOptionPicker('styleElements')">
            <span>{{ adaptSettings.styleElements.length ? adaptSettings.styleElements.join('、') : '选择或自定义输入设定' }}</span>
            <span class="icon icon-chevron-down"></span>
          </div>
        </label>
      </div>

      <label class="form-field full-width">
        <span>世界观（选填）</span>
        <textarea
          v-model="adaptSettings.worldview"
          class="dark-textarea compact"
          placeholder="例：世界存在一种失传的织墨术，顶尖修复师能以特制丝线绣补古画..."
        ></textarea>
      </label>

      <label class="form-field full-width">
        <span>核心亮点（选填）</span>
        <textarea
          v-model="adaptSettings.coreHook"
          class="dark-textarea compact"
          placeholder="例：穿书成作死女配，她反向操作，用沙雕和直球硬核攻略哑巴老公..."
        ></textarea>
      </label>

      <label class="form-field full-width">
        <span>核心梗概 <em>*</em></span>
        <textarea
          v-model="adaptSettings.coreSummary"
          class="dark-textarea large"
        ></textarea>
      </label>
    </form>

    <aside class="right-step-anchor">
      <button
        v-for="step in adaptStorySteps"
        :key="step"
        class="step-anchor-item"
        :class="{ 'is-active': currentAdaptStoryStep === step }"
        type="button"
        @click="scrollToAdaptStoryStep(step)"
      >
        <span class="step-anchor-item__dot"></span>
        <span>{{ step }}</span>
      </button>
    </aside>
  </section>
</section>
```

---

# 七、推荐 Vue 状态结构

```ts
const state = reactive({
  userCredits: 463,

  rewrite: {
    query: {
      keyword: '',
      sortBy: 'updatedAt',
      order: 'desc'
    },
    upload: {
      status: 'idle',
      progress: 0
    },
    projects: [],
    activeTab: 'infoflow',
    project: null,
    info: null,
    settings: {
      targetAudience: '',
      eraBackground: [],
      themeTypes: [],
      coreSettings: [],
      storyBackground: '',
      coreHook: '',
      coreSummary: ''
    }
  },

  adapt: {
    query: {
      keyword: ''
    },
    upload: {
      status: 'idle',
      uploadId: '',
      fileName: ''
    },
    typeModalVisible: false,
    chapterRangeModalVisible: false,
    adaptType: '',
    parsedNovel: {
      novelId: '',
      title: '',
      totalChapters: 0,
      totalWords: 0,
      chapters: []
    },
    range: {
      startChapter: 1,
      endChapter: 1,
      selectedChapterNos: [],
      costCredits: 0
    },
    activeTab: 'outline',
    outlineGeneration: {
      status: 'idle',
      taskId: '',
      currentChapterNo: 1
    },
    outline: [],
    settings: {
      targetAudience: '',
      themeTypes: [],
      coreSettings: [],
      styleElements: [],
      worldview: '',
      coreHook: '',
      coreSummary: ''
    }
  }
});
```

---

# 八、关键交互事件清单

## 剧本改写

```ts
openRewriteFilePicker()
handleRewriteFileChange(file)
uploadRewriteFile(file)
createRewriteProject(uploadId)
fetchRewriteProjects()
goRewriteDetail(projectId)
fetchRewriteDetail(projectId)
saveRewriteStorySettings()
generateRewritePlan()
quoteAllInfo()
undoRewrite()
redoRewrite()
```

## 网文改编

```ts
openAdaptFilePicker()
handleAdaptFileChange(file)
uploadAdaptFile(file)
parseNovelChapters(uploadId)
openAdaptTypeModal()
selectAdaptType(type)
openChapterRangeModal()
syncChapterRange()
toggleChapterSelected(chapterNo)
selectPreviewChapter(chapter)
detectNovelFormat()
splitNovelChapters()
confirmChapterRange()
createAdaptProject()
pollOutlineGeneration(taskId)
fetchAdaptOutline(projectId)
saveAdaptStorySettings()
aiPolishAdaptSettings()
scrollToChapter(chapterNo)
```

---

# 九、最小 CSS 命名建议

```css
.app-shell {}
.app-sidebar {}
.sidebar-nav__item {}
.app-main {}

.page-header {}
.page-title {}
.header-search {}
.primary-upload-button {}
.white-action-button {}
.gradient-action-button {}

.script-list {}
.script-card {}
.script-card__cover {}
.script-card__body {}

.primary-tabs {}
.primary-tab {}
.primary-tab.is-active {}

.infoflow-panel {}
.info-card {}
.story-meta-grid {}

.story-settings-layout {}
.story-settings-form {}
.form-field {}
.select-like-input {}
.dark-textarea {}
.right-step-anchor {}

.modal-layer {}
.adapt-type-modal {}
.adapt-type-card {}

.chapter-range-modal {}
.chapter-range-body {}
.chapter-check-list {}
.chapter-preview {}

.outline-editor-layout {}
.outline-canvas {}
.outline-card {}
.outline-card.is-generating {}
.right-chapter-ruler {}
```

以上结构可以完整覆盖截图中的核心页面：**剧本改写列表 / 上传、剧本改写信息流 / 故事设定编辑、网文改编空态 / 上传、类型选择弹窗、章节范围弹窗、小说章纲生成与编辑器**。