下面给出一套可落地的 Vue 实现方案，用于还原截图中的「网文改编」流程：

1. 类型选择页 / 类型弹窗  
2. 拆解范围弹窗  
3. 确认拆解后进入「小说章纲」编辑器结果页  
4. 交互状态机  
5. 后端 AI JSON 契约  

整体风格：暗黑底、左侧固定导航、主区域黑色斜纹背景、居中弹窗、亮色选中态、小说章纲卡片式编辑。

---

# 一、页面结构总览

## 业务流程

```txt
进入网文改编页
  ↓
展示类型选择 Type Modal
  ↓ 用户选择「小说改编 / 动漫转真人 / 真人转动漫」等类型
  ↓
打开拆解范围 Range Modal
  ↓ 用户选择章节范围
  ↓ 点击「确认拆解」
  ↓
后端 AI 拆解小说内容
  ↓
进入结果页 Novel Outline Editor
  ↓
用户编辑「小说章纲 / 故事梗概 / 人物小传 / 分集大纲 / 剧本正文」
```

---

# 二、Vue HTML Skeleton

建议组件拆分：

```txt
src/
 ├─ views/
 │   └─ AdaptPage.vue
 ├─ components/adapt/
 │   ├─ AppShell.vue
 │   ├─ SideNav.vue
 │   ├─ TopSearch.vue
 │   ├─ TypeModal.vue
 │   ├─ RangeModal.vue
 │   ├─ NovelOutlineEditor.vue
 │   ├─ OutlineCard.vue
 │   └─ RightChapterIndex.vue
 └─ api/
     └─ adapt.ts
```

---

## 1. AdaptPage.vue 页面容器

```vue
<template>
  <AppShell>
    <!-- 顶部搜索和教程入口 -->
    <template #header>
      <TopSearch
        v-model="keyword"
        placeholder="根据名称、核心梗概进行搜索"
        @search="handleSearch"
      />

      <button class="tutorial-btn">
        使用教程
      </button>
    </template>

    <!-- 初始类型选择背景态 -->
    <section
      v-if="uiState === 'TYPE_SELECTING'"
      class="adapt-type-stage"
    >
      <div class="type-hero-card">
        <div class="type-hero-image left">
          <!-- 动漫图 -->
        </div>
        <div class="type-hero-split"></div>
        <div class="type-hero-image right">
          <!-- 真人图 -->
        </div>
      </div>
    </section>

    <!-- 结果页 -->
    <NovelOutlineEditor
      v-if="uiState === 'OUTLINE_READY'"
      :script-name="scriptName"
      :outline-data="outlineData"
      :active-tab="activeTab"
      :active-chapter-id="activeChapterId"
      @change-tab="activeTab = $event"
      @select-chapter="activeChapterId = $event"
      @update-section="handleUpdateSection"
      @undo="handleUndo"
      @redo="handleRedo"
    />

    <!-- 类型选择弹窗/浮层 -->
    <TypeModal
      v-if="showTypeModal"
      :selected-type="selectedAdaptType"
      @select="handleSelectType"
      @next="openRangeModal"
    />

    <!-- 拆解范围弹窗 -->
    <RangeModal
      v-if="showRangeModal"
      :chapters="chapters"
      :selected-range="selectedRange"
      :loading="isDecomposing"
      @close="closeRangeModal"
      @change-range="handleRangeChange"
      @confirm="confirmDecompose"
    />

    <!-- 右下角 AI 助手 -->
    <button class="ai-float-btn">
      <span class="robot-icon">🤖</span>
    </button>

    <!-- 底部版权提示 -->
    <div class="copyright-tip">
      <span>ⓘ</span>
      您拥有剧本所有权与控制，我们承诺不利用数据进行任何商业用途
    </div>
  </AppShell>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import AppShell from '@/components/adapt/AppShell.vue'
import TopSearch from '@/components/adapt/TopSearch.vue'
import TypeModal from '@/components/adapt/TypeModal.vue'
import RangeModal from '@/components/adapt/RangeModal.vue'
import NovelOutlineEditor from '@/components/adapt/NovelOutlineEditor.vue'

type UIState =
  | 'TYPE_SELECTING'
  | 'RANGE_SELECTING'
  | 'DECOMPOSING'
  | 'OUTLINE_READY'
  | 'ERROR'

const uiState = ref<UIState>('TYPE_SELECTING')

const keyword = ref('')
const scriptName = ref('剧本名称')
const selectedAdaptType = ref<'anime_to_live' | 'live_to_anime' | 'novel_to_script' | null>(null)

const showTypeModal = computed(() => uiState.value === 'TYPE_SELECTING')
const showRangeModal = computed(() => uiState.value === 'RANGE_SELECTING' || uiState.value === 'DECOMPOSING')
const isDecomposing = computed(() => uiState.value === 'DECOMPOSING')

const selectedRange = ref({
  startChapterNo: 1,
  endChapterNo: 5
})

const activeTab = ref<'novel_outline' | 'story_summary' | 'characters' | 'episode_outline' | 'script_body'>('novel_outline')
const activeChapterId = ref('chapter-5')

const chapters = ref([
  {
    id: 'chapter-1',
    chapterNo: 1,
    title: '第1章',
    wordCount: 238,
    content: '秦川回到秦家旧宅……'
  },
  {
    id: 'chapter-2',
    chapterNo: 2,
    title: '第2章',
    wordCount: 251,
    content: '灵堂外传来粗烈的砸门声……'
  },
  {
    id: 'chapter-3',
    chapterNo: 3,
    title: '第3章',
    wordCount: 265,
    content: '陈虎带着一群林家打手强行闯了进来……'
  },
  {
    id: 'chapter-4',
    chapterNo: 4,
    title: '第4章',
    wordCount: 293,
    content: '秦川背对众人……'
  },
  {
    id: 'chapter-5',
    chapterNo: 5,
    title: '第5章 龙主',
    wordCount: 277,
    content:
      '沈临风命令保镖动手。可保镖刚迈出一步，会场外便涌入一队黑衣人，胸口全都佩着龙形徽章。为首男人单膝跪地，声音震动全场：“恭迎龙主出狱！”'
  }
])

const outlineData = ref({
  novelOutline: [
    {
      id: 'chapter-1',
      chapterNo: 1,
      title: '第1章 灵堂归来',
      summary:
        '秦川在旧宅为母设灵堂，陈虎带人奉沈临风之命前来灭口。秦川隐忍未动，直到母亲遗像前被辱，怒意爆发。',
      beats: [
        '夜里，秦川回到秦家旧宅，为母亲设置了简单的灵堂。',
        '灵堂外传来砸门声，打破夜的寂静。',
        '陈虎带着林家打手闯入，声称奉沈临风之命要除掉秦川。'
      ]
    },
    {
      id: 'chapter-5',
      chapterNo: 5,
      title: '第5章 龙主',
      summary:
        '沈临风欲令保镖动手，会场外突然涌入佩戴龙形徽章的黑衣人，向秦川跪拜高呼“恭迎龙主”。秦川揭露身份，林雪交出沈临风策划放弃抢救的录音证据。',
      beats: [
        '沈临风恼羞成怒，命令现场保镖对秦川动手，试图控制混乱局面。',
        '保镖刚欲行动，会场外迅速涌入一队气势森严的黑衣人，制服统一。',
        '黑衣人胸口均佩戴龙形徽章，行动整齐划一，训练有素。',
        '为首的黑衣人径直走到秦川面前，单膝跪地，高呼：“恭迎龙主出狱！”'
      ]
    }
  ],
  storySummary: '',
  characters: [],
  episodeOutline: [],
  scriptBody: ''
})

function handleSearch() {
  console.log('search:', keyword.value)
}

function handleSelectType(type: typeof selectedAdaptType.value) {
  selectedAdaptType.value = type
}

function openRangeModal() {
  if (!selectedAdaptType.value) return
  uiState.value = 'RANGE_SELECTING'
}

function closeRangeModal() {
  uiState.value = 'TYPE_SELECTING'
}

function handleRangeChange(range: { startChapterNo: number; endChapterNo: number }) {
  selectedRange.value = range
}

async function confirmDecompose() {
  uiState.value = 'DECOMPOSING'

  try {
    // await api.decomposeNovel(...)
    await new Promise(resolve => setTimeout(resolve, 1200))
    uiState.value = 'OUTLINE_READY'
  } catch (err) {
    uiState.value = 'ERROR'
  }
}

function handleUpdateSection(payload: any) {
  console.log('update section:', payload)
}

function handleUndo() {}
function handleRedo() {}
</script>
```

---

## 2. AppShell.vue 外壳布局

```vue
<template>
  <div class="app-shell">
    <SideNav />

    <main class="main-panel">
      <header class="page-header">
        <div class="page-title">
          <slot name="title">
            <h1>网文改编</h1>
          </slot>
        </div>

        <div class="header-center">
          <slot name="header" />
        </div>
      </header>

      <div class="page-content">
        <slot />
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import SideNav from './SideNav.vue'
</script>
```

---

## 3. SideNav.vue 左侧导航

```vue
<template>
  <aside class="side-nav">
    <div class="side-logo">剧</div>

    <nav class="side-menu">
      <button class="side-icon">▧</button>
      <button class="side-icon">🛠</button>
      <button class="side-icon active">▥</button>

      <div class="side-divider"></div>

      <button class="side-icon">⌾</button>
      <button class="side-icon">♨</button>
      <button class="side-icon">◇</button>
      <button class="side-icon">▣</button>
      <button class="side-icon">👥</button>
      <button class="side-icon">⚙</button>
    </nav>

    <div class="side-bottom">
      <div class="credit">467</div>
      <div class="avatar"></div>
    </div>
  </aside>
</template>
```

---

## 4. TopSearch.vue

```vue
<template>
  <div class="top-search">
    <input
      :value="modelValue"
      :placeholder="placeholder"
      @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
      @keyup.enter="$emit('search')"
    />

    <button @click="$emit('search')">
      🔍
    </button>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  modelValue: string
  placeholder?: string
}>()

defineEmits<{
  'update:modelValue': [value: string]
  search: []
}>()
</script>
```

---

# 三、类型选择 Type Modal

截图 `target-adapt-02-type-modal.png` 的重点是：

- 暗黑主背景。
- 中央一张大图卡片。
- 左侧为动漫女性脸部，右侧为真人男性脸部。
- 中间有斜切割线。
- 整体像「类型选择入口」。
- 当前阶段还没有明显按钮，但产品实际可加 hover 选择区域。

## TypeModal.vue

```vue
<template>
  <div class="type-modal-layer">
    <div class="type-select-card">
      <button
        class="type-half type-half-left"
        :class="{ selected: selectedType === 'anime_to_live' }"
        @click="$emit('select', 'anime_to_live')"
      >
        <div class="type-image anime"></div>
        <div class="type-mask"></div>
        <div class="type-label">
          <strong>动漫转真人</strong>
          <span>将二次元角色设定转为真人影视风格</span>
        </div>
      </button>

      <button
        class="type-half type-half-right"
        :class="{ selected: selectedType === 'live_to_anime' }"
        @click="$emit('select', 'live_to_anime')"
      >
        <div class="type-image live"></div>
        <div class="type-mask"></div>
        <div class="type-label">
          <strong>真人转动漫</strong>
          <span>将真人剧本转为动漫表现方式</span>
        </div>
      </button>

      <div class="diagonal-cut"></div>
    </div>

    <div class="type-action-bar">
      <button
        class="secondary-type-btn"
        :class="{ selected: selectedType === 'novel_to_script' }"
        @click="$emit('select', 'novel_to_script')"
      >
        网文改编为剧本
      </button>

      <button
        class="primary-btn"
        :disabled="!selectedType"
        @click="$emit('next')"
      >
        下一步
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  selectedType: 'anime_to_live' | 'live_to_anime' | 'novel_to_script' | null
}>()

defineEmits<{
  select: ['anime_to_live' | 'live_to_anime' | 'novel_to_script']
  next: []
}>()
</script>
```

---

# 四、拆解范围 Range Modal

截图 `target-adapt-03-range-modal.png` 的重点：

- 居中大弹窗。
- 标题：拆解范围。
- 副标题：小说共 5 章，总计 1324 字，请选择需要拆解的范围。
- 左侧是范围滑杆，顶部数字 1，底部数字 5。
- 中间是章节列表，每章有勾选框。
- 当前第 5 章高亮。
- 右侧显示章节正文。
- 右上角关闭按钮。
- 底部左侧显示消耗剧点。
- 底部右侧有「格式」「拆解」提示和「确认拆解」按钮。

## RangeModal.vue

```vue
<template>
  <div class="modal-mask">
    <section class="range-modal">
      <button
        class="modal-close"
        @click="$emit('close')"
      >
        ×
      </button>

      <header class="range-modal-header">
        <h2>拆解范围</h2>
        <p>
          小说共<span>{{ chapters.length }}</span>章，
          总计<span>{{ totalWords }}</span>字，
          请选择需要拆解的范围
        </p>
      </header>

      <main class="range-modal-body">
        <!-- 左侧范围数字和竖向滑杆 -->
        <div class="range-slider-panel">
          <div class="range-number-box">
            {{ selectedRange.startChapterNo }}
          </div>

          <div class="vertical-range">
            <div class="vertical-track"></div>
            <div
              class="vertical-selected"
              :style="selectedTrackStyle"
            ></div>
            <button
              class="range-thumb start"
              :style="startThumbStyle"
              aria-label="start"
            ></button>
            <button
              class="range-thumb end"
              :style="endThumbStyle"
              aria-label="end"
            ></button>
          </div>

          <div class="range-number-box">
            {{ selectedRange.endChapterNo }}
          </div>
        </div>

        <!-- 中间章节列表 -->
        <aside class="chapter-list">
          <button
            v-for="chapter in chapters"
            :key="chapter.id"
            class="chapter-list-item"
            :class="{
              checked: isChapterChecked(chapter.chapterNo),
              active: activeChapterId === chapter.id
            }"
            @click="handleSelectChapter(chapter)"
          >
            <span class="fake-checkbox">
              <span v-if="isChapterChecked(chapter.chapterNo)">✓</span>
            </span>
            <span>{{ chapter.title }}</span>
          </button>
        </aside>

        <!-- 右侧章节正文 -->
        <article class="chapter-preview">
          <header>
            <h3>{{ activeChapter?.title }}</h3>
            <span class="word-count">{{ activeChapter?.wordCount }}字</span>
          </header>

          <div class="chapter-content">
            {{ activeChapter?.content }}
          </div>
        </article>
      </main>

      <footer class="range-modal-footer">
        <div class="cost-info">
          <strong>{{ cost }}</strong>
          <span>消耗剧点（400字/剧点，已选{{ selectedCount }}章，共{{ selectedWords }}字）</span>
        </div>

        <div class="footer-actions">
          <button class="ghost-action">
            <span>!</span>
            格式
          </button>

          <button class="ghost-action">
            <span>!</span>
            拆解
          </button>

          <button
            class="confirm-btn"
            :disabled="loading || selectedCount <= 0"
            @click="$emit('confirm')"
          >
            {{ loading ? '拆解中...' : '确认拆解' }}
          </button>
        </div>
      </footer>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

interface Chapter {
  id: string
  chapterNo: number
  title: string
  wordCount: number
  content: string
}

const props = defineProps<{
  chapters: Chapter[]
  selectedRange: {
    startChapterNo: number
    endChapterNo: number
  }
  loading?: boolean
}>()

const emit = defineEmits<{
  close: []
  confirm: []
  changeRange: [
    range: {
      startChapterNo: number
      endChapterNo: number
    }
  ]
}>()

const activeChapterId = ref(props.chapters[props.chapters.length - 1]?.id)

const totalWords = computed(() => {
  return props.chapters.reduce((sum, item) => sum + item.wordCount, 0)
})

const selectedChapters = computed(() => {
  return props.chapters.filter(item => {
    return (
      item.chapterNo >= props.selectedRange.startChapterNo &&
      item.chapterNo <= props.selectedRange.endChapterNo
    )
  })
})

const selectedCount = computed(() => selectedChapters.value.length)

const selectedWords = computed(() => {
  return selectedChapters.value.reduce((sum, item) => sum + item.wordCount, 0)
})

const cost = computed(() => {
  return Math.ceil(selectedWords.value / 400)
})

const activeChapter = computed(() => {
  return props.chapters.find(item => item.id === activeChapterId.value) || props.chapters[0]
})

function isChapterChecked(chapterNo: number) {
  return (
    chapterNo >= props.selectedRange.startChapterNo &&
    chapterNo <= props.selectedRange.endChapterNo
  )
}

function handleSelectChapter(chapter: Chapter) {
  activeChapterId.value = chapter.id

  // 点击章节时可扩展为重设范围
  // 当前仅作为预览激活
}

const selectedTrackStyle = computed(() => {
  const max = props.chapters.length
  const startPercent = ((props.selectedRange.startChapterNo - 1) / (max - 1)) * 100
  const endPercent = ((props.selectedRange.endChapterNo - 1) / (max - 1)) * 100

  return {
    top: `${startPercent}%`,
    height: `${endPercent - startPercent}%`
  }
})

const startThumbStyle = computed(() => {
  const max = props.chapters.length
  const percent = ((props.selectedRange.startChapterNo - 1) / (max - 1)) * 100

  return {
    top: `${percent}%`
  }
})

const endThumbStyle = computed(() => {
  const max = props.chapters.length
  const percent = ((props.selectedRange.endChapterNo - 1) / (max - 1)) * 100

  return {
    top: `${percent}%`
  }
})
</script>
```

---

# 五、小说章纲编辑器 NovelOutlineEditor

截图 `target-adapt-05-novel-outline-result.png` 的重点：

- 左侧导航白色 active。
- 主区域标题：剧本名称。
- 标题左侧有小魔法图标。
- 顶部 Tab：
  - 小说章纲 active，渐变绿蓝。
  - 故事梗概
  - 人物小传
  - 分集大纲
  - 剧本正文
- 主内容为多张章纲卡片。
- 卡片暗色、圆角、文字灰白。
- 正在编辑的章节更亮。
- 右侧有撤销、重做按钮。
- 右侧还有章节索引刻度 1-10。

## NovelOutlineEditor.vue

```vue
<template>
  <section class="outline-editor">
    <header class="outline-header">
      <div class="script-title">
        <span class="magic-icon">✦</span>
        <h1>{{ scriptName }}</h1>
      </div>

      <div class="history-actions">
        <button @click="$emit('undo')">↶</button>
        <button @click="$emit('redo')">↷</button>
      </div>
    </header>

    <nav class="outline-tabs">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        :class="{ active: activeTab === tab.key }"
        @click="$emit('changeTab', tab.key)"
      >
        {{ tab.label }}
      </button>
    </nav>

    <main class="outline-main">
      <section class="outline-card-grid">
        <OutlineCard
          v-for="chapter in outlineData.novelOutline"
          :key="chapter.id"
          :chapter="chapter"
          :active="activeChapterId === chapter.id"
          @click="$emit('selectChapter', chapter.id)"
          @update="handleUpdateChapter(chapter.id, $event)"
        />
      </section>

      <RightChapterIndex
        :total="10"
        :active="getActiveChapterNo()"
        @select="$emit('selectChapter', `chapter-${$event}`)"
      />
    </main>
  </section>
</template>

<script setup lang="ts">
import OutlineCard from './OutlineCard.vue'
import RightChapterIndex from './RightChapterIndex.vue'

const props = defineProps<{
  scriptName: string
  activeTab: string
  activeChapterId: string
  outlineData: {
    novelOutline: Array<{
      id: string
      chapterNo: number
      title: string
      summary: string
      beats: string[]
    }>
  }
}>()

const emit = defineEmits<{
  changeTab: [tab: string]
  selectChapter: [chapterId: string]
  updateSection: [payload: any]
  undo: []
  redo: []
}>()

const tabs = [
  {
    key: 'novel_outline',
    label: '小说章纲'
  },
  {
    key: 'story_summary',
    label: '故事梗概'
  },
  {
    key: 'characters',
    label: '人物小传'
  },
  {
    key: 'episode_outline',
    label: '分集大纲'
  },
  {
    key: 'script_body',
    label: '剧本正文'
  }
]

function handleUpdateChapter(chapterId: string, data: any) {
  emit('updateSection', {
    section: 'novel_outline',
    chapterId,
    data
  })
}

function getActiveChapterNo() {
  const chapter = props.outlineData.novelOutline.find(
    item => item.id === props.activeChapterId
  )
  return chapter?.chapterNo || 1
}
</script>
```

---

## OutlineCard.vue

```vue
<template>
  <article
    class="outline-card"
    :class="{ active }"
    @click="$emit('click')"
  >
    <h2
      contenteditable
      @blur="handleTitleBlur"
    >
      {{ chapter.title }}
    </h2>

    <p
      class="chapter-summary"
      contenteditable
      @blur="handleSummaryBlur"
    >
      {{ chapter.summary }}
    </p>

    <div class="beats">
      <p
        v-for="(beat, index) in chapter.beats"
        :key="index"
        contenteditable
        @blur="handleBeatBlur(index, $event)"
      >
        <span>情节{{ index + 1 }}：</span>{{ beat }}
      </p>
    </div>
  </article>
</template>

<script setup lang="ts">
const props = defineProps<{
  active?: boolean
  chapter: {
    id: string
    chapterNo: number
    title: string
    summary: string
    beats: string[]
  }
}>()

const emit = defineEmits<{
  click: []
  update: [payload: any]
}>()

function handleTitleBlur(event: FocusEvent) {
  emit('update', {
    title: (event.target as HTMLElement).innerText
  })
}

function handleSummaryBlur(event: FocusEvent) {
  emit('update', {
    summary: (event.target as HTMLElement).innerText
  })
}

function handleBeatBlur(index: number, event: FocusEvent) {
  emit('update', {
    beatIndex: index,
    beatText: (event.target as HTMLElement).innerText.replace(/^情节\d+：/, '')
  })
}
</script>
```

---

## RightChapterIndex.vue

```vue
<template>
  <aside class="right-chapter-index">
    <button
      class="index-number top"
      @click="$emit('select', 1)"
    >
      1
    </button>

    <div class="index-ruler">
      <span
        v-for="i in total"
        :key="i"
        :class="{ active: i === active }"
      ></span>
    </div>

    <button
      class="index-number bottom"
      @click="$emit('select', total)"
    >
      {{ total }}
    </button>
  </aside>
</template>

<script setup lang="ts">
defineProps<{
  total: number
  active: number
}>()

defineEmits<{
  select: [chapterNo: number]
}>()
</script>
```

---

# 六、CSS 结构说明

建议使用一个 `adapt.scss` 或每个组件 scoped CSS。下面给出核心样式结构。

---

## 1. 全局暗黑背景与斜纹

```css
:root {
  --bg-root: #050505;
  --bg-panel: #090909;
  --bg-card: #1b1c20;
  --bg-card-soft: rgba(27, 28, 32, 0.86);
  --border-subtle: rgba(255, 255, 255, 0.08);
  --text-main: #f4f6fb;
  --text-secondary: #9ca3af;
  --text-muted: #5f6673;
  --accent-green: #98f58f;
  --accent-blue: #43b8ff;
  --accent-purple: #8a7cff;
}

body {
  margin: 0;
  background: var(--bg-root);
  color: var(--text-main);
  font-family:
    Inter,
    "PingFang SC",
    "Microsoft YaHei",
    sans-serif;
}

.app-shell {
  width: 100vw;
  height: 100vh;
  display: flex;
  overflow: hidden;
  background: #000;
}

.main-panel {
  flex: 1;
  margin-left: 108px;
  min-width: 0;
  border-radius: 26px 0 0 26px;
  background:
    repeating-linear-gradient(
      135deg,
      rgba(255, 255, 255, 0.025) 0,
      rgba(255, 255, 255, 0.025) 1px,
      transparent 1px,
      transparent 8px
    ),
    #030303;
  position: relative;
  overflow: hidden;
}

.page-header {
  height: 106px;
  display: flex;
  align-items: center;
  padding: 0 60px 0 32px;
  position: relative;
}

.page-title h1 {
  font-size: 28px;
  margin: 0;
  color: #7c8798;
}

.header-center {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
}

.page-content {
  position: relative;
  height: calc(100vh - 106px);
}
```

---

## 2. 左侧导航

```css
.side-nav {
  width: 108px;
  height: 100vh;
  background: #17181c;
  position: fixed;
  left: 0;
  top: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  z-index: 20;
}

.side-logo {
  margin-top: 50px;
  font-size: 32px;
  font-weight: 800;
  color: #ffffff;
}

.side-menu {
  margin-top: 76px;
  display: flex;
  flex-direction: column;
  gap: 20px;
  align-items: center;
}

.side-icon {
  width: 58px;
  height: 58px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: rgba(255, 255, 255, 0.4);
  font-size: 24px;
  cursor: pointer;
}

.side-icon.active {
  background: #ffffff;
  color: #000000;
}

.side-divider {
  width: 58px;
  height: 1px;
  background: rgba(255, 255, 255, 0.16);
  margin: 12px 0;
}

.side-bottom {
  margin-top: auto;
  margin-bottom: 28px;
  text-align: center;
}

.credit {
  font-weight: 700;
  color: rgba(255, 255, 255, 0.7);
  margin-bottom: 40px;
}

.avatar {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: linear-gradient(135deg, #56380f, #1c1f26);
  border: 1px solid rgba(255, 255, 255, 0.12);
}
```

---

## 3. 顶部搜索

```css
.top-search {
  width: 448px;
  height: 52px;
  border-radius: 8px;
  background: rgba(13, 14, 17, 0.9);
  display: flex;
  align-items: center;
  padding-left: 16px;
  border: 1px solid rgba(255, 255, 255, 0.04);
}

.top-search input {
  flex: 1;
  border: 0;
  outline: none;
  background: transparent;
  color: var(--text-main);
  font-size: 16px;
}

.top-search input::placeholder {
  color: rgba(255, 255, 255, 0.18);
}

.top-search button {
  width: 42px;
  height: 42px;
  margin-right: 4px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  background: transparent;
  color: rgba(255, 255, 255, 0.4);
}

.tutorial-btn {
  position: fixed;
  right: 60px;
  top: 54px;
  border: 0;
  background: transparent;
  color: #7f8796;
  font-size: 16px;
}
```

---

## 4. 类型选择卡片

```css
.type-modal-layer {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.type-select-card {
  width: 838px;
  height: 464px;
  border-radius: 6px;
  overflow: hidden;
  position: relative;
  border: 2px solid rgba(255, 255, 255, 0.12);
  display: flex;
  background: #111;
}

.type-half {
  width: 50%;
  height: 100%;
  padding: 0;
  border: 0;
  position: relative;
  overflow: hidden;
  cursor: pointer;
  background: transparent;
}

.type-half-left {
  clip-path: polygon(0 0, 100% 0, 84% 100%, 0 100%);
  z-index: 2;
}

.type-half-right {
  margin-left: -70px;
  width: calc(50% + 70px);
  clip-path: polygon(17% 0, 100% 0, 100% 100%, 0 100%);
  z-index: 1;
}

.type-image {
  position: absolute;
  inset: 0;
  background-size: cover;
  background-position: center;
  transition: transform 0.35s ease;
}

.type-image.anime {
  background:
    linear-gradient(135deg, rgba(255, 122, 61, 0.3), rgba(80, 210, 255, 0.15)),
    url("/mock/anime-face.jpg");
}

.type-image.live {
  background:
    linear-gradient(135deg, rgba(0, 0, 0, 0.15), rgba(255, 185, 90, 0.08)),
    url("/mock/live-face.jpg");
}

.type-half:hover .type-image,
.type-half.selected .type-image {
  transform: scale(1.04);
}

.type-half.selected {
  outline: 2px solid var(--accent-blue);
  outline-offset: -2px;
}

.type-mask {
  position: absolute;
  inset: 0;
  background: linear-gradient(to top, rgba(0, 0, 0, 0.55), transparent 50%);
}

.type-label {
  position: absolute;
  left: 32px;
  bottom: 28px;
  text-align: left;
  color: #fff;
  opacity: 0;
  transform: translateY(10px);
  transition: 0.2s ease;
}

.type-half:hover .type-label,
.type-half.selected .type-label {
  opacity: 1;
  transform: translateY(0);
}

.type-label strong {
  display: block;
  font-size: 22px;
  margin-bottom: 8px;
}

.type-label span {
  color: rgba(255, 255, 255, 0.72);
  font-size: 14px;
}

.diagonal-cut {
  position: absolute;
  left: 49%;
  top: -20px;
  width: 4px;
  height: 520px;
  transform: rotate(9deg);
  background: rgba(0, 0, 0, 0.75);
  z-index: 3;
}

.type-action-bar {
  margin-top: 28px;
  display: flex;
  gap: 16px;
}

.secondary-type-btn,
.primary-btn {
  height: 44px;
  padding: 0 24px;
  border-radius: 8px;
  border: 0;
  cursor: pointer;
}

.secondary-type-btn {
  background: rgba(255, 255, 255, 0.08);
  color: #fff;
}

.secondary-type-btn.selected {
  background: rgba(67, 184, 255, 0.18);
  color: #9ee7ff;
}

.primary-btn {
  background: linear-gradient(90deg, var(--accent-green), var(--accent-blue));
  color: #fff;
  font-weight: 700;
}

.primary-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
```

---

## 5. 拆解范围弹窗

```css
.modal-mask {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.62);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 30;
}

.range-modal {
  width: 1015px;
  height: 845px;
  background: #1a1b1f;
  border-radius: 6px;
  position: relative;
  padding: 32px 30px;
  box-sizing: border-box;
  box-shadow: 0 30px 100px rgba(0, 0, 0, 0.45);
}

.modal-close {
  position: absolute;
  right: 20px;
  top: 16px;
  border: 0;
  background: transparent;
  color: rgba(255, 255, 255, 0.35);
  font-size: 42px;
  cursor: pointer;
}

.range-modal-header h2 {
  margin: 0;
  font-size: 26px;
  color: #fff;
}

.range-modal-header p {
  margin: 12px 0 22px;
  color: #75839a;
  font-size: 16px;
}

.range-modal-header span {
  color: #fff;
  font-weight: 700;
}

.range-modal-body {
  display: grid;
  grid-template-columns: 50px 130px 1fr;
  height: 615px;
}

.range-slider-panel {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.range-number-box {
  width: 45px;
  height: 45px;
  border: 1px solid #ccd5ea;
  border-radius: 8px;
  display: grid;
  place-items: center;
  color: #fff;
  font-weight: 700;
  font-size: 20px;
}

.vertical-range {
  flex: 1;
  width: 24px;
  margin: 16px 0;
  position: relative;
}

.vertical-track {
  position: absolute;
  left: 50%;
  top: 0;
  bottom: 0;
  width: 10px;
  transform: translateX(-50%);
  background: #ffffff;
  border-radius: 999px;
}

.vertical-selected {
  position: absolute;
  left: 50%;
  width: 10px;
  transform: translateX(-50%);
  background: linear-gradient(to bottom, var(--accent-blue), var(--accent-purple));
  border-radius: 999px;
}

.range-thumb {
  position: absolute;
  left: 50%;
  width: 24px;
  height: 24px;
  transform: translate(-50%, -50%);
  border: 2px solid #9c99ff;
  background: #fff;
  border-radius: 50%;
}

.chapter-list {
  border-right: 1px solid rgba(255, 255, 255, 0.08);
  padding-left: 18px;
  padding-right: 20px;
}

.chapter-list-item {
  height: 37px;
  width: 110px;
  display: flex;
  align-items: center;
  gap: 8px;
  color: #fff;
  border: 0;
  border-radius: 4px;
  background: transparent;
  font-size: 18px;
  cursor: pointer;
  padding: 0 10px;
}

.chapter-list-item.active {
  background: rgba(255, 255, 255, 0.12);
}

.fake-checkbox {
  width: 14px;
  height: 14px;
  border: 1px solid #fff;
  border-radius: 2px;
  display: grid;
  place-items: center;
  font-size: 12px;
}

.chapter-preview {
  padding-left: 14px;
  padding-right: 8px;
  color: #fff;
}

.chapter-preview header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.chapter-preview h3 {
  margin: 0;
  font-size: 20px;
}

.word-count {
  background: #373c45;
  color: #fff;
  border-radius: 4px;
  padding: 2px 8px;
  font-size: 14px;
}

.chapter-content {
  margin-top: 12px;
  line-height: 1.55;
  font-size: 19px;
  white-space: pre-wrap;
}

.range-modal-footer {
  height: 110px;
  display: flex;
  align-items: end;
  justify-content: space-between;
}

.cost-info strong {
  font-size: 40px;
  color: #fff;
  display: block;
}

.cost-info span {
  color: rgba(255, 255, 255, 0.25);
}

.footer-actions {
  display: flex;
  align-items: center;
  gap: 24px;
}

.ghost-action {
  border: 0;
  background: transparent;
  color: rgba(255, 255, 255, 0.65);
  font-size: 18px;
}

.ghost-action span {
  display: inline-grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border: 2px solid currentColor;
  border-radius: 50%;
  margin-right: 6px;
}

.confirm-btn {
  height: 50px;
  padding: 0 26px;
  border-radius: 7px;
  border: 0;
  background: #fff;
  color: #111;
  font-size: 18px;
  cursor: pointer;
}

.confirm-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
```

---

## 6. 小说章纲结果页

```css
.outline-editor {
  height: 100%;
  padding: 0 108px;
  box-sizing: border-box;
  position: relative;
}

.outline-header {
  height: 68px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.script-title {
  display: flex;
  align-items: center;
  gap: 12px;
}

.magic-icon {
  width: 26px;
  height: 26px;
  border-radius: 4px;
  display: grid;
  place-items: center;
  color: #bda6ff;
  background: rgba(138, 124, 255, 0.12);
}

.script-title h1 {
  font-size: 30px;
  color: #8b95a5;
  margin: 0;
}

.history-actions {
  position: fixed;
  right: 110px;
  top: 120px;
  display: flex;
  background: rgba(28, 30, 35, 0.72);
  border-radius: 4px;
  overflow: hidden;
}

.history-actions button {
  width: 60px;
  height: 48px;
  border: 0;
  background: transparent;
  color: rgba(255, 255, 255, 0.55);
  font-size: 24px;
  cursor: pointer;
}

.outline-tabs {
  margin-top: 12px;
  display: flex;
  gap: 0;
}

.outline-tabs button {
  height: 48px;
  padding: 0 24px;
  border: 0;
  background: #191b21;
  color: #fff;
  font-size: 17px;
  font-weight: 700;
  cursor: pointer;
}

.outline-tabs button:first-child {
  border-radius: 8px 0 0 8px;
}

.outline-tabs button:last-child {
  border-radius: 0 8px 8px 0;
}

.outline-tabs button.active {
  border-radius: 8px;
  background: linear-gradient(100deg, #93f58a, #36a7ff);
}

.outline-main {
  display: flex;
  margin-top: 42px;
  position: relative;
}

.outline-card-grid {
  width: 1160px;
  display: grid;
  grid-template-columns: 550px 550px;
  gap: 54px 74px;
}

.outline-card {
  height: 310px;
  overflow: hidden;
  border-radius: 0;
  background: rgba(29, 31, 36, 0.62);
  color: rgba(255, 255, 255, 0.42);
  padding: 18px 20px;
  box-sizing: border-box;
  line-height: 1.55;
  cursor: pointer;
}

.outline-card.active {
  height: 408px;
  border-radius: 12px;
  background: #1c1d22;
  color: rgba(255, 255, 255, 0.75);
}

.outline-card h2 {
  margin: 0 0 12px;
  color: #fff;
  font-size: 22px;
}

.chapter-summary {
  color: #dce8ff;
  font-size: 17px;
  font-weight: 700;
}

.beats p {
  margin: 6px 0;
  font-size: 17px;
}

.beats span {
  color: rgba(255, 255, 255, 0.55);
}

[contenteditable] {
  outline: none;
}

[contenteditable]:focus {
  background: rgba(67, 184, 255, 0.08);
  box-shadow: 0 0 0 1px rgba(67, 184, 255, 0.35);
  border-radius: 4px;
}
```

---

## 7. 右侧章节索引

```css
.right-chapter-index {
  position: fixed;
  right: 108px;
  top: 210px;
  width: 90px;
  height: 130px;
  color: #fff;
}

.index-number {
  border: 0;
  background: transparent;
  color: #fff;
  font-size: 22px;
  font-weight: 700;
  cursor: pointer;
}

.index-ruler {
  position: absolute;
  right: 0;
  top: 12px;
  width: 42px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.index-ruler span {
  height: 1px;
  background: rgba(255, 255, 255, 0.7);
}

.index-ruler span.active {
  background: var(--accent-blue);
  height: 2px;
}

.index-number.bottom {
  position: absolute;
  left: 0;
  bottom: 0;
}
```

---

## 8. 右下角 AI 按钮和版权提示

```css
.ai-float-btn {
  position: fixed;
  right: 70px;
  bottom: 250px;
  width: 64px;
  height: 64px;
  border-radius: 50%;
  border: 0;
  background: radial-gradient(circle, #3d2b8f, #17122d);
  color: #fff;
  font-size: 28px;
  cursor: pointer;
  box-shadow: 0 0 28px rgba(119, 91, 255, 0.4);
}

.copyright-tip {
  position: fixed;
  bottom: 28px;
  left: 50%;
  transform: translateX(-50%);
  color: rgba(255, 255, 255, 0.25);
  font-size: 15px;
}
```

---

# 七、交互状态机

## 状态定义

```ts
type AdaptState =
  | 'INIT'
  | 'TYPE_SELECTING'
  | 'TYPE_SELECTED'
  | 'RANGE_SELECTING'
  | 'RANGE_VALID'
  | 'DECOMPOSING'
  | 'OUTLINE_READY'
  | 'OUTLINE_EDITING'
  | 'SAVING'
  | 'SAVE_SUCCESS'
  | 'ERROR'
```

---

## 状态转移表

| 当前状态 | 事件 | 下一个状态 | 说明 |
|---|---|---|---|
| INIT | page.loaded | TYPE_SELECTING | 页面加载完成，显示类型选择 |
| TYPE_SELECTING | type.select | TYPE_SELECTED | 用户选择改编类型 |
| TYPE_SELECTED | next.click | RANGE_SELECTING | 进入拆解范围弹窗 |
| RANGE_SELECTING | range.change | RANGE_VALID | 选择范围有效 |
| RANGE_VALID | confirm.click | DECOMPOSING | 调用后端 AI 拆解 |
| DECOMPOSING | ai.success | OUTLINE_READY | 返回章纲结果 |
| DECOMPOSING | ai.failed | ERROR | AI 处理失败 |
| OUTLINE_READY | card.focus | OUTLINE_EDITING | 用户开始编辑章纲 |
| OUTLINE_EDITING | edit.blur | SAVING | 自动保存 |
| SAVING | save.success | SAVE_SUCCESS | 保存成功 |
| SAVE_SUCCESS | idle | OUTLINE_READY | 回到结果态 |
| ERROR | retry.click | DECOMPOSING | 重试 |
| ERROR | close.click | RANGE_SELECTING | 回到范围选择 |

---

## XState 风格伪代码

```ts
const adaptMachine = {
  id: 'adapt',
  initial: 'typeSelecting',
  states: {
    typeSelecting: {
      on: {
        SELECT_TYPE: {
          target: 'typeSelected',
          actions: ['setAdaptType']
        }
      }
    },

    typeSelected: {
      on: {
        NEXT: 'rangeSelecting',
        SELECT_TYPE: {
          target: 'typeSelected',
          actions: ['setAdaptType']
        }
      }
    },

    rangeSelecting: {
      on: {
        CHANGE_RANGE: {
          target: 'rangeValid',
          actions: ['setRange']
        },
        CLOSE: 'typeSelecting'
      }
    },

    rangeValid: {
      on: {
        CHANGE_RANGE: {
          target: 'rangeValid',
          actions: ['setRange']
        },
        CONFIRM: 'decomposing',
        CLOSE: 'typeSelecting'
      }
    },

    decomposing: {
      invoke: {
        src: 'decomposeNovel',
        onDone: {
          target: 'outlineReady',
          actions: ['setOutlineResult']
        },
        onError: {
          target: 'error',
          actions: ['setError']
        }
      }
    },

    outlineReady: {
      on: {
        CHANGE_TAB: {
          target: 'outlineReady',
          actions: ['setActiveTab']
        },
        SELECT_CHAPTER: {
          target: 'outlineReady',
          actions: ['setActiveChapter']
        },
        EDIT_START: 'outlineEditing',
        UNDO: {
          target: 'outlineReady',
          actions: ['undo']
        },
        REDO: {
          target: 'outlineReady',
          actions: ['redo']
        }
      }
    },

    outlineEditing: {
      on: {
        EDIT_COMMIT: 'saving',
        CANCEL_EDIT: 'outlineReady'
      }
    },

    saving: {
      invoke: {
        src: 'saveOutlinePatch',
        onDone: 'outlineReady',
        onError: 'error'
      }
    },

    error: {
      on: {
        RETRY: 'decomposing',
        BACK_TO_RANGE: 'rangeSelecting'
      }
    }
  }
}
```

---

# 八、后端 AI JSON 契约

下面给出前后端可对接的 JSON 契约。

---

## 1. 获取小说章节列表

### Request

```http
GET /api/adapt/novels/{novelId}/chapters
```

### Response

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "novelId": "novel_001",
    "title": "龙主归来",
    "chapterCount": 5,
    "totalWordCount": 1324,
    "chapters": [
      {
        "id": "chapter-1",
        "chapterNo": 1,
        "title": "第1章",
        "wordCount": 238,
        "contentPreview": "秦川回到秦家旧宅，为母亲设置了简单的灵堂……"
      },
      {
        "id": "chapter-5",
        "chapterNo": 5,
        "title": "第5章 龙主",
        "wordCount": 277,
        "contentPreview": "沈临风命令保镖动手。可保镖刚迈出一步，会场外便涌入一队黑衣人……"
      }
    ]
  }
}
```

---

## 2. 获取单章正文

### Request

```http
GET /api/adapt/chapters/{chapterId}
```

### Response

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": "chapter-5",
    "chapterNo": 5,
    "title": "第5章 龙主",
    "wordCount": 277,
    "content": "沈临风命令保镖动手。可保镖刚迈出一步，会场外便涌入一队黑衣人，胸口全都佩着龙形徽章。为首男人单膝跪地，声音震动全场：“恭迎龙主出狱！”……"
  }
}
```

---

## 3. 提交 AI 拆解任务

### Request

```http
POST /api/adapt/tasks/decompose
```

```json
{
  "novelId": "novel_001",
  "projectId": "project_001",
  "adaptType": "novel_to_script",
  "range": {
    "startChapterNo": 1,
    "endChapterNo": 5,
    "chapterIds": [
      "chapter-1",
      "chapter-2",
      "chapter-3",
      "chapter-4",
      "chapter-5"
    ]
  },
  "options": {
    "targetFormat": "screenplay_outline",
    "language": "zh-CN",
    "style": "dark_drama",
    "preserveOriginalNames": true,
    "outputSections": [
      "novel_outline",
      "story_summary",
      "characters",
      "episode_outline",
      "script_body"
    ]
  },
  "billing": {
    "wordCount": 1324,
    "costPoint": 4
  }
}
```

### Response

```json
{
  "code": 0,
  "message": "task created",
  "data": {
    "taskId": "task_202501010001",
    "status": "queued",
    "estimatedSeconds": 15
  }
}
```

---

## 4. 查询 AI 拆解任务状态

### Request

```http
GET /api/adapt/tasks/{taskId}
```

### Processing Response

```json
{
  "code": 0,
  "message": "processing",
  "data": {
    "taskId": "task_202501010001",
    "status": "processing",
    "progress": 62,
    "currentStep": "正在生成小说章纲"
  }
}
```

### Success Response

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "taskId": "task_202501010001",
    "status": "success",
    "projectId": "project_001",
    "scriptId": "script_001",
    "scriptName": "剧本名称",
    "result": {
      "storySummary": {
        "id": "summary_001",
        "content": "秦川在母亲死后回归都市，发现昔日仇敌沈临风与林雪订婚，并试图彻底抹除秦家旧案。秦川以龙主身份强势现身，逐步揭开母亲死亡真相，展开复仇。"
      },
      "novelOutline": [
        {
          "id": "outline_chapter_1",
          "sourceChapterId": "chapter-1",
          "chapterNo": 1,
          "title": "第1章 灵堂归来",
          "summary": "秦川在旧宅为母设灵堂，陈虎带人奉沈临风之命前来灭口。秦川隐忍未动，直到母亲遗像前被辱，怒意爆发。",
          "beats": [
            {
              "id": "beat_1_1",
              "index": 1,
              "content": "夜里，秦川回到秦家旧宅，为母亲设置了简单的灵堂，独自守灵。"
            },
            {
              "id": "beat_1_2",
              "index": 2,
              "content": "灵堂外传来粗烈砸门声，打破夜的寂静，预示着麻烦上门。"
            },
            {
              "id": "beat_1_3",
              "index": 3,
              "content": "陈虎带着林家打手强行闯入，声称奉沈临风之命要让秦川今晚消失。"
            }
          ],
          "dramaticFunction": "建立主角回归、母亲死亡悬念与复仇动机。",
          "charactersInvolved": ["秦川", "陈虎"],
          "conflict": "秦川守灵与仇敌灭口之间的冲突。"
        },
        {
          "id": "outline_chapter_5",
          "sourceChapterId": "chapter-5",
          "chapterNo": 5,
          "title": "第5章 龙主",
          "summary": "沈临风欲令保镖动手，会场外突然涌入佩戴龙形徽章的黑衣人，向秦川跪拜高呼“恭迎龙主”。秦川揭露身份，林雪交出沈临风策划放弃抢救的录音证据。",
          "beats": [
            {
              "id": "beat_5_1",
              "index": 1,
              "content": "沈临风恼羞成怒，命令现场保镖对秦川动手，试图控制混乱局面。"
            },
            {
              "id": "beat_5_2",
              "index": 2,
              "content": "保镖刚欲行动，会场外迅速涌入一队气势森严的黑衣人，制服统一。"
            },
            {
              "id": "beat_5_3",
              "index": 3,
              "content": "黑衣人胸口均佩戴龙形徽章，行动整齐划一，训练有素。"
            },
            {
              "id": "beat_5_4",
              "index": 4,
              "content": "为首黑衣人径直来到秦川面前，单膝跪地，高呼：“恭迎龙主出狱！”"
            }
          ],
          "dramaticFunction": "完成主角身份反转，提升爽点，推动复仇线进入明面冲突。",
          "charactersInvolved": ["秦川", "沈临风", "林雪", "黑衣人"],
          "conflict": "沈临风试图压制秦川，秦川真实身份反制全场。"
        }
      ],
      "characters": [
        {
          "id": "char_001",
          "name": "秦川",
          "roleType": "protagonist",
          "identity": "龙主，秦家遗孤",
          "goal": "查清母亲死亡真相，向沈临风复仇。",
          "personality": ["隐忍", "冷静", "强势", "重情"],
          "arc": "从隐忍归来者转为公开复仇的掌控者。"
        },
        {
          "id": "char_002",
          "name": "沈临风",
          "roleType": "antagonist",
          "identity": "林氏集团准女婿，旧案参与者",
          "goal": "掩盖秦母死亡真相，维护自身权势。",
          "personality": ["虚伪", "狠辣", "自负"],
          "arc": "从婚礼赢家跌落为罪行暴露者。"
        }
      ],
      "episodeOutline": [
        {
          "id": "episode_1",
          "episodeNo": 1,
          "title": "龙主归来",
          "coveredChapters": [1, 2, 3, 4, 5],
          "summary": "秦川守灵遭辱，追查母亲死亡真相，最终在沈临风订婚现场以龙主身份震慑全场。",
          "keyScenes": [
            "秦川旧宅守灵",
            "陈虎带人闯入",
            "林氏发布会",
            "黑衣人跪迎龙主"
          ],
          "endingHook": "林雪交出录音证据，沈临风罪行即将曝光。"
        }
      ],
      "scriptBody": [
        {
          "id": "scene_001",
          "sceneNo": 1,
          "title": "秦家旧宅夜",
          "location": "秦家旧宅灵堂",
          "timeOfDay": "夜",
          "characters": ["秦川"],
          "content": "昏暗的灵堂中，秦川跪坐在母亲遗像前。香灰落下，他的眼神平静却压着沉重怒意。"
        }
      ]
    }
  }
}
```

---

## 5. AI 拆解失败响应

```json
{
  "code": 50001,
  "message": "AI_DECOMPOSE_FAILED",
  "data": {
    "taskId": "task_202501010001",
    "status": "failed",
    "reason": "模型生成超时，请稍后重试",
    "retryable": true
  }
}
```

---

## 6. 保存章纲编辑补丁

### Request

```http
PATCH /api/adapt/scripts/{scriptId}/outline
```

```json
{
  "section": "novel_outline",
  "chapterId": "outline_chapter_5",
  "patch": {
    "title": "第5章 龙主现身",
    "summary": "沈临风欲令保镖动手，龙形徽章黑衣人突然入场，跪迎秦川为龙主。林雪交出关键录音，沈临风罪行浮出水面。",
    "beats": [
      {
        "id": "beat_5_1",
        "index": 1,
        "content": "沈临风恼羞成怒，命令保镖控制秦川。"
      }
    ]
  },
  "clientVersion": 7
}
```

### Response

```json
{
  "code": 0,
  "message": "saved",
  "data": {
    "scriptId": "script_001",
    "section": "novel_outline",
    "serverVersion": 8,
    "updatedAt": "2025-01-01T12:00:00.000Z"
  }
}
```

---

# 九、前端数据模型建议

```ts
export interface AdaptProject {
  projectId: string
  novelId: string
  scriptId?: string
  scriptName: string
  adaptType: AdaptType
  status: AdaptProjectStatus
}

export type AdaptType =
  | 'novel_to_script'
  | 'anime_to_live'
  | 'live_to_anime'

export type AdaptProjectStatus =
  | 'draft'
  | 'decomposing'
  | 'outline_ready'
  | 'editing'
  | 'failed'

export interface NovelChapter {
  id: string
  chapterNo: number
  title: string
  wordCount: number
  content?: string
  contentPreview?: string
}

export interface NovelOutlineChapter {
  id: string
  sourceChapterId: string
  chapterNo: number
  title: string
  summary: string
  beats: OutlineBeat[]
  dramaticFunction?: string
  charactersInvolved?: string[]
  conflict?: string
}

export interface OutlineBeat {
  id: string
  index: number
  content: string
}

export interface CharacterProfile {
  id: string
  name: string
  roleType: 'protagonist' | 'antagonist' | 'supporting'
  identity: string
  goal: string
  personality: string[]
  arc: string
}
```

---

# 十、关键还原点总结

## 类型选择页

需要优先还原：

```txt
暗黑斜纹背景
左侧固定导航
顶部搜索框
中央双图卡片
左右图斜切
hover/selected 高亮
右下角 AI 助手
底部版权提示
```

## 拆解范围弹窗

需要优先还原：

```txt
1015px 左右宽度的深色弹窗
标题和小说统计信息
左侧竖向范围选择条
章节列表带 checkbox
当前章节灰底高亮
右侧正文预览
底部剧点消耗
确认拆解按钮
```

## 小说章纲编辑器

需要优先还原：

```txt
顶部剧本名称
渐变 active tab
卡片式章纲
非 active 卡片偏暗
active 卡片更高、更亮、圆角更明显
右侧撤销/重做按钮
右侧章节刻度索引
contenteditable 或 textarea 支持编辑
编辑后自动保存
```

以上骨架和契约可以直接进入 Vue 项目实现，并能比较准确地复刻三张截图中的核心布局与业务交互。