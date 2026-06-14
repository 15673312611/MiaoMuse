<template>
  <div class="app-frame">
    <aside class="rail">
      <button class="logo-mark" title="剧本工坊" @click="go('creation')">剧</button>
      <nav class="rail-nav">
        <button v-for="item in workNav" :key="item.key" :class="{ active: activeWorkKey === item.key }" :title="item.label" @click="go(item.key)">
          <component :is="item.icon" :size="25" />
          <span class="rail-label">{{ item.label }}</span>
        </button>
      </nav>
      <span class="rail-line"></span>
      <nav class="rail-nav rail-common">
        <button v-for="item in commonNav" :key="item.key" :class="{ active: route === item.key }" :title="item.label" @click="go(item.key)">
          <component :is="item.icon" :size="25" />
          <span class="rail-label">{{ item.label }}</span>
        </button>
      </nav>
      <div class="rail-bottom">
        <button class="rail-points" title="剧点" @click="go('wallet')"><small>剧点余额</small><b>{{ wallet.balance }}</b><i class="rail-point-mark" aria-hidden="true"></i></button>
        <button class="rail-account" :title="profile.nickname" @click="go('settings')">
          <i class="rail-avatar"></i>
          <span><b>{{ profile.nickname }}</b><small>{{ profile.phone }}</small></span>
        </button>
      </div>
    </aside>

    <main class="stage" :class="{ editor: route === 'editor' }">
      <template v-if="route !== 'editor'">
        <header class="page-head">
          <h1>{{ pageTitle }}</h1>
          <div v-if="showsSearch" class="search-box">
            <input v-model="searchText" placeholder="根据名称、核心梗概进行搜索" />
            <Search :size="20" />
          </div>
          <div v-if="showsSearch" class="page-head-actions">
            <button class="tutorial" @click="toast('使用教程为外部飞书文档入口')">使用教程</button>
            <button v-if="route === 'creation'" class="top-btn" @click="modal = 'newScript'">
              <FilePlus2 :size="18" />新建剧本
            </button>
            <label v-else-if="route === 'rewrite' || route === 'adapt'" class="top-btn upload-label" :class="{ disabled: busy }">
              <component :is="route === 'rewrite' ? Wand2 : UploadCloud" :size="18" />{{ route === 'rewrite' ? '导入剧本' : '导入小说' }}
              <input type="file" accept=".txt,.doc,.docx,.pdf" :disabled="busy" @change="uploadSource($event, sourceName)" />
            </label>
          </div>
        </header>

        <section v-if="route === 'creation'" class="creation-screen">
          <div class="sort-row"><ListFilter :size="16" />排序方式</div>
          <p v-if="loading" class="center-empty">正在加载数据...</p>
          <p v-else-if="visibleScripts.length === 0" class="center-empty">暂无原创剧本</p>
          <article v-for="card in visibleScripts" :key="card.id" class="script-row" tabindex="0" @click="openEditor(card)" @keydown.enter="openEditor(card)">
            <span :class="['script-cover', projectCoverClass(card)]">
              <b>{{ projectBadge(card) }}</b>
              <span class="script-cover-mark"><component :is="projectCoverIcon(card)" :size="36" /></span>
            </span>
            <span><strong>{{ card.title }}</strong><small>上次修改于 {{ formatProjectTime(card.updatedAt) }}</small></span>
            <span class="script-row-side" @click.stop>
              <em>{{ card.type || 'AI短剧' }}</em>
              <button class="script-more" title="更多操作" @click="toggleRowMenu(card.id)">
                <MoreVertical :size="18" />
              </button>
              <div v-if="rowMenuOpen === card.id" class="script-menu">
                <button @click.stop="exportListProject(card)"><Download :size="15" />导出</button>
                <button class="danger" @click.stop="deleteListProject(card)"><Trash2 :size="15" />删除</button>
              </div>
            </span>
          </article>
        </section>

        <section v-else-if="route === 'rewrite' || route === 'adapt'" class="source-screen target-source" :class="`target-${route}`">
          <div class="sort-row source-sort"><ListFilter :size="16" />排序方式</div>

          <article v-if="sourceProjects.length === 0" class="source-uploader target-empty">
            <div class="source-icon"><component :is="route === 'rewrite' ? Wand2 : PanelLeft" :size="38" /></div>
            <h2>{{ sourceCopy.emptyTitle }}</h2>
            <p>{{ sourceCopy.emptyText }}</p>
            <label class="primary-btn upload-label" :class="{ disabled: busy }">
              {{ route === 'rewrite' ? '导入剧本' : '导入小说' }}
              <input type="file" accept=".txt,.doc,.docx,.pdf" :disabled="busy" @change="uploadSource($event, sourceName)" />
            </label>
            <small><Info :size="15" />您拥有剧本所有权与控制，我们承诺不利用数据进行任何商业用途</small>
          </article>

          <div v-else class="source-script-list">
            <article v-for="item in sourceProjects" :key="item.id" class="script-row source-script-row" tabindex="0" @click="openEditor(item)" @keydown.enter="openEditor(item)">
              <span :class="['script-cover', 'source-script-cover', projectCoverClass(item)]">
                <b>{{ projectBadge(item) }}</b>
                <span class="script-cover-mark"><component :is="projectCoverIcon(item)" :size="36" /></span>
              </span>
              <span>
                <strong>{{ item.title }}</strong>
                <small>{{ sourceProjectSummary(item) }}</small>
              </span>
              <span class="script-row-side" @click.stop>
                <em>{{ item.source === 'adaptation' ? '已拆解' : '已解析' }}</em>
                <button class="script-more" title="更多操作" @click="toggleRowMenu(item.id)">
                  <MoreVertical :size="18" />
                </button>
                <div v-if="rowMenuOpen === item.id" class="script-menu">
                  <button @click.stop="exportListProject(item)"><Download :size="15" />导出</button>
                  <button class="danger" @click.stop="deleteListProject(item)"><Trash2 :size="15" />删除</button>
                </div>
              </span>
            </article>
          </div>
        </section>

        <section v-else-if="route === 'coverage'" class="coverage-screen">
          <article class="coverage-visual">
            <div class="coverage-header">
              <div class="coverage-badge">
                <ShieldCheck :size="14" />
                <span>剧本评估系统 V1.0</span>
              </div>
              <h2>核心评估维度</h2>
              <p>结合 AI 从多维度、图形化等视角完成剧本评估。涵盖剧情逻辑、人物塑造、商业价值、市场适配等维度，全方位洞察剧本潜力。</p>
            </div>
            <!-- 流式评估进度 -->
            <div class="coverage-stream-panel" v-if="evalStreaming">
              <div class="coverage-stream-header">
                <LoaderCircle :size="18" class="spin-loader" />
                <span>AI 正在深度评估中...</span>
              </div>
              <pre class="coverage-stream-text">{{ evalStreamText }}</pre>
            </div>
            <!-- 雷达图 + 维度分数 -->
            <template v-else>
              <div class="radar-chart">
                <div class="radar-ring radar-ring-1"></div>
                <div class="radar-ring radar-ring-2"></div>
                <div class="radar-ring radar-ring-3"></div>
                <div class="radar-axis radar-axis-1"></div>
                <div class="radar-axis radar-axis-2"></div>
                <div class="radar-axis radar-axis-3"></div>
                <div class="radar-score">
                  <span class="radar-score-num">{{ latestEvaluation?.score || '--' }}</span>
                  <span class="radar-score-label">综合评分</span>
                </div>
                <div class="radar-dimensions">
                  <template v-if="latestEvaluation?.dimensions?.length">
                    <span v-for="(dim, i) in latestEvaluation.dimensions.slice(0, 6)" :key="dim.name" :class="['radar-dim', 'dim-' + (i + 1)]">{{ dim.name }}</span>
                  </template>
                  <template v-else>
                    <span class="radar-dim dim-1">剧情逻辑</span>
                    <span class="radar-dim dim-2">人物塑造</span>
                    <span class="radar-dim dim-3">商业价值</span>
                    <span class="radar-dim dim-4">市场适配</span>
                    <span class="radar-dim dim-5">节奏把控</span>
                    <span class="radar-dim dim-6">台词质量</span>
                  </template>
                </div>
              </div>
              <div class="coverage-stats" v-if="latestEvaluation?.dimensions?.length">
                <div class="coverage-stat" v-for="dim in latestEvaluation.dimensions" :key="dim.name">
                  <b>{{ dim.score }}</b>
                  <span>{{ dim.name }}</span>
                </div>
              </div>
              <div class="coverage-empty-hint" v-else>
                <Sparkles :size="20" />
                <p>完成评估后，将在此展示您的多维度评估结果</p>
              </div>
              <!-- 评估摘要 + 建议 -->
              <div class="coverage-report" v-if="latestEvaluation?.summary">
                <div class="coverage-report-section">
                  <h3>综合评价</h3>
                  <p>{{ latestEvaluation.summary }}</p>
                </div>
                <div class="coverage-report-section" v-if="latestEvaluation.dimensions?.length">
                  <h3>维度点评</h3>
                  <div class="coverage-dim-notes">
                    <div class="coverage-dim-note" v-for="dim in latestEvaluation.dimensions" :key="dim.name">
                      <b>{{ dim.name }} <em>{{ dim.score }}分</em></b>
                      <p>{{ dim.note }}</p>
                    </div>
                  </div>
                </div>
                <div class="coverage-report-section" v-if="latestEvaluation.suggestions?.length">
                  <h3>改进建议</h3>
                  <ul class="coverage-suggestions">
                    <li v-for="(sug, i) in latestEvaluation.suggestions" :key="i">{{ sug }}</li>
                  </ul>
                </div>
              </div>
            </template>
          </article>
          <form class="coverage-form" @submit.prevent="submitEvaluation">
            <div class="coverage-form-header">
              <button type="button" class="history-btn" @click="modal = 'coverageHistory'">
                <History :size="16" />
                <span>评估历史</span>
                <em v-if="evaluations.length">{{ evaluations.length }}</em>
              </button>
            </div>
            <div class="coverage-form-body">
              <h2>设置评估参数</h2>
              <p class="coverage-form-desc">配置剧本基本信息，AI 将根据您的目标受众和制作方式生成针对性评估报告</p>

              <div class="coverage-field-group">
                <label>
                  <span>评估标题</span>
                  <input v-model="evaluationForm.title" placeholder="输入评估标题，便于后续查找" />
                </label>
              </div>

              <div class="coverage-field-row">
                <label>
                  <span>文化背景</span>
                  <div class="coverage-select-wrap">
                    <select v-model="evaluationForm.culture">
                      <option value="国内">国内市场</option>
                      <option value="海外">海外市场</option>
                      <option value="东南亚">东南亚市场</option>
                      <option value="欧美">欧美市场</option>
                    </select>
                    <ChevronDown :size="14" />
                  </div>
                </label>
                <label>
                  <span>目标受众</span>
                  <div class="coverage-select-wrap">
                    <select v-model="evaluationForm.audience">
                      <option value="男频">男频受众</option>
                      <option value="女频">女频受众</option>
                      <option value="全年龄">全年龄</option>
                      <option value="下沉市场">下沉市场</option>
                    </select>
                    <ChevronDown :size="14" />
                  </div>
                </label>
              </div>

              <div class="coverage-field-group">
                <label>
                  <span>剧本类型</span>
                  <div class="script-type-choice">
                    <button type="button" :class="{ selected: evaluationForm.scriptType === 'AI短剧' }" @click="evaluationForm.scriptType = 'AI短剧'">
                      <Sparkles :size="16" />
                      AI短剧
                    </button>
                    <button type="button" :class="{ selected: evaluationForm.scriptType === '真人实拍' }" @click="evaluationForm.scriptType = '真人实拍'">
                      <Camera :size="16" />
                      真人实拍
                    </button>
                  </div>
                </label>
              </div>

              <div class="coverage-field-group">
                <label>
                  <span>评估维度</span>
                  <div class="dimension-tags">
                    <span :class="{ active: evaluationForm.dimensions.includes('plot') }" @click="toggleDimension('plot')">
                      <CheckCircle2 v-if="evaluationForm.dimensions.includes('plot')" :size="12" />
                      <Circle v-else :size="12" />
                      剧情逻辑
                    </span>
                    <span :class="{ active: evaluationForm.dimensions.includes('character') }" @click="toggleDimension('character')">
                      <CheckCircle2 v-if="evaluationForm.dimensions.includes('character')" :size="12" />
                      <Circle v-else :size="12" />
                      人物塑造
                    </span>
                    <span :class="{ active: evaluationForm.dimensions.includes('commercial') }" @click="toggleDimension('commercial')">
                      <CheckCircle2 v-if="evaluationForm.dimensions.includes('commercial')" :size="12" />
                      <Circle v-else :size="12" />
                      商业价值
                    </span>
                    <span :class="{ active: evaluationForm.dimensions.includes('market') }" @click="toggleDimension('market')">
                      <CheckCircle2 v-if="evaluationForm.dimensions.includes('market')" :size="12" />
                      <Circle v-else :size="12" />
                      市场适配
                    </span>
                    <span :class="{ active: evaluationForm.dimensions.includes('pacing') }" @click="toggleDimension('pacing')">
                      <CheckCircle2 v-if="evaluationForm.dimensions.includes('pacing')" :size="12" />
                      <Circle v-else :size="12" />
                      节奏把控
                    </span>
                    <span :class="{ active: evaluationForm.dimensions.includes('dialogue') }" @click="toggleDimension('dialogue')">
                      <CheckCircle2 v-if="evaluationForm.dimensions.includes('dialogue')" :size="12" />
                      <Circle v-else :size="12" />
                      台词质量
                    </span>
                  </div>
                </label>
              </div>

              <div class="coverage-field-group">
                <label>
                  <span>受众喜好 / 补充说明</span>
                  <textarea v-model="evaluationForm.preference" rows="3" placeholder="描述目标受众的偏好特征，如：喜欢爽文、偏好甜宠、注重家国情怀等。留空则由 AI 自动识别"></textarea>
                </label>
              </div>

              <div class="coverage-field-group">
                <label class="file-drop" :class="{ 'has-file': evaluationForm.fileName }">
                  <div class="file-drop-content">
                    <UploadCloud :size="28" />
                    <div class="file-drop-text">
                      <b>{{ evaluationForm.fileName || '点击或拖拽上传剧本文件' }}</b>
                      <p v-if="!evaluationForm.fileName">支持 .txt、.doc、.docx 格式，最大 10MB</p>
                      <p v-else class="file-drop-success">
                        <CheckCircle2 :size="14" /> 文件已就绪，可开始评估
                      </p>
                    </div>
                  </div>
                  <input type="file" accept=".txt,.doc,.docx" @change="evaluationFileChanged" />
                </label>
              </div>

              <button class="coverage-submit" :class="{ 'is-ready': evaluationForm.fileName && evaluationForm.title }" :disabled="busy || !evaluationForm.content.trim()">
                <span class="coverage-submit-main">
                  <LoaderCircle v-if="busy" :size="18" class="spin-loader" />
                  <Sparkles v-else :size="18" />
                  {{ busy ? 'AI 正在深度评估中...' : '开始剧本评估' }}
                </span>
              </button>

              <div class="coverage-hints">
                <p><Info :size="13" /> 评估耗时约 30 秒，AI 将从选定维度深度分析您的剧本，生成包含评分、点评和改进建议的结构化报告</p>
              </div>
            </div>
          </form>
        </section>

        <section v-else-if="route === 'wallet'" class="wallet-screen">
          <article class="wallet-hero">
            <div><small>当前剧点</small><h2>{{ wallet.balance }}<span>⌘</span></h2><p>大约还能生成 {{ Math.floor(wallet.balance / 3000) }} 篇剧本</p><button class="link-btn" @click="modal = 'walletHelp'">查看剧点说明</button></div>
            <div class="coin-stack"><i v-for="n in 8" :key="n"></i></div>
            <div class="wallet-actions">
              <button class="primary-btn" @click="modal = 'recharge'">充值剧点</button>
              <button @click="modal = 'enterprise'">企业充值</button>
            </div>
          </article>
          <article class="wallet-membership">
            <div>
              <small>会员权益</small>
              <h3>{{ profile.membership || '剧本专家' }}</h3>
              <p>{{ profile.memberUntil }} 到期；会员可享充值折扣、任务福利和高峰期优先队列。</p>
            </div>
            <button class="gold-btn" @click="modal = 'premiumRenew'">续费会员</button>
          </article>
          <h3 class="wallet-subtitle">剧点任务</h3>
          <div class="task-grid">
            <article v-for="task in wallet.tasks" :key="task.title">
              <b>{{ task.title }}</b><p>{{ task.description }}</p>
              <button v-if="!task.claimed" @click="claimWalletTask">点击领取</button>
              <span v-else>已完成</span>
            </article>
          </div>
          <article class="wallet-table">
            <h3>收入 / 消费明细</h3>
            <div v-for="item in wallet.items" :key="item.title + item.time" class="table-row">
              <span>{{ item.title }}</span><b :class="{ expense: item.delta < 0 }">{{ item.delta > 0 ? '+' : '' }}{{ item.delta }}</b><small>{{ item.time }}</small>
            </div>
          </article>
        </section>

        <section v-else-if="route === 'settings'" class="settings-screen">
          <nav class="settings-tabs">
            <button v-for="tab in settingsTabs" :key="tab.key" :class="{ active: settingsTab === tab.key }" @click="settingsTab = tab.key">{{ tab.label }}</button>
          </nav>
          <article class="settings-panel">
            <template v-if="settingsTab === 'profile'">
              <section class="api-config-card">
                <header>
                  <div>
                    <small>大模型接口</small>
                    <h2>API 配置</h2>
                  </div>
                  <b :class="{ ready: selectedProviderConfigured }">{{ selectedProviderConfigured ? '已配置' : '未配置' }}</b>
                </header>
                <div class="provider-grid">
                  <button
                    v-for="provider in aiProviders"
                    :key="provider.key"
                    type="button"
                    :class="{ active: aiConfigForm.provider === provider.key }"
                    @click="selectAIProvider(provider.key)"
                  >
                    <b>{{ provider.label }}</b>
                    <span>{{ provider.hint }}</span>
                  </button>
                </div>
                <div class="provider-help">
                  <span>官方接口：{{ selectedAIProvider.baseURL }}</span>
                  <a :href="selectedAIProvider.consoleURL" target="_blank" rel="noreferrer">打开 {{ selectedAIProvider.label }} API Key 页面</a>
                </div>
                <ol class="provider-guide">
                  <li>选择服务商后，点击右侧链接去官方控制台创建 API Key。</li>
                  <li>复制 Key 回来粘贴到下方输入框。</li>
                  <li>选择模型；如果官方新模型还没出现在列表里，就手动添加模型 ID。</li>
                </ol>
                <label>
                  <span>模型</span>
                  <select v-model="aiConfigForm.model">
                    <option v-for="model in selectedProviderModels" :key="model" :value="model">{{ model }}</option>
                  </select>
                </label>
                <div class="custom-model-row">
                  <input v-model.trim="customModelName" placeholder="手动添加模型名，例如官方新发布的模型 ID" @keydown.enter.prevent="addCustomModel" />
                  <button type="button" class="top-btn" @click="addCustomModel">添加模型</button>
                </div>
                <label><span>API Key</span><input v-model="aiConfigForm.apiKey" type="password" :placeholder="selectedProviderConfigured ? `${selectedAIProvider.label} 已保存，留空不修改` : 'sk-...'" /></label>
                <button class="primary-btn" :disabled="busy" @click="saveAIConfig">保存 API 配置</button>
              </section>
              <h2>我的资料</h2>
              <label><span>头像</span><button class="rail-avatar large"></button></label>
              <label><span>昵称</span><input v-model="profileForm.nickname" /></label>
              <label><span>绑定手机</span><input :value="profile.phone" readonly /></label>
              <button class="primary-btn" @click="saveProfile">保存</button>
            </template>
            <template v-else-if="settingsTab === 'security'">
              <h2>账号安全</h2>
              <div class="security-row"><span>登录密码</span><b>已设置</b><button @click="modal = 'password'">修改</button></div>
              <div class="security-row"><span>绑定手机</span><b>{{ profile.phone }}</b><button disabled>不可更换</button></div>
              <div class="security-row"><span>登录状态</span><b>当前设备在线</b><button @click="toast('已退出当前设备')">退出登录</button></div>
            </template>
            <template v-else-if="settingsTab === 'contact'">
              <h2>联系我们</h2>
              <article class="contact-card-single">
                <img src="/contact-wechat.jpg" alt="微信二维码" />
                <div>
                  <small>人工客服</small>
                  <h3>微信号 soe303</h3>
                  <p>添加微信后可咨询账号、生成、导出和项目协作问题。</p>
                </div>
              </article>
            </template>
            <template v-else>
              <h2>通用设置</h2>
              <label class="check-line"><input type="checkbox" v-model="generalSettings.autosave" /> 自动保存草稿</label>
              <label class="check-line"><input type="checkbox" v-model="generalSettings.guides" /> 显示新手引导</label>
              <button class="primary-btn" @click="toast('通用设置已保存')">保存</button>
            </template>
          </article>
        </section>
      </template>

      <template v-else>
        <EditorView
          v-if="currentProject"
          :project="currentProject"
          :busy="busy"
          @back="go(editorBackRoute)"
          @save="saveProject"
          @ai="runAiTask"
          @export="exportWord"
          @toast="toast"
        />
      </template>
    </main>

    <div v-if="modal" class="modal-layer" @click.self="closeModal">
      <section v-if="modal === 'newScript'" class="type-modal" aria-label="选择剧本类型">
        <button class="modal-close" @click="closeModal"><X :size="18" /></button>
        <div class="script-type-split create">
          <button class="script-type-image ai" aria-label="AI短剧" @click="createAndOpen('AI短剧')">
            <img src="/type-assets/ai-short.png" alt="" />
            <span>AI短剧</span>
          </button>
          <button class="script-type-image live" aria-label="真人实拍" @click="createAndOpen('真人实拍')">
            <img src="/type-assets/live-action.png" alt="" />
            <span>真人实拍</span>
          </button>
        </div>
      </section>

      <section v-else-if="modal === 'exportProject'" class="info-modal export-select-modal">
        <button class="modal-close" @click="closeModal"><X :size="18" /></button>
        <h2>选择导出内容</h2>
        <p class="export-project-name">{{ exportProjectDraft?.title || '未命名剧本' }}</p>
        <div class="export-option-grid">
          <button v-for="opt in exportOptionDefs" :key="opt.key"
            :class="['export-opt-card', { active: exportOptions[opt.key] }]"
            @click="exportOptions[opt.key] = !exportOptions[opt.key]">
            <component :is="opt.icon" :size="16" />
            <span>{{ opt.label }}</span>
            <CheckCircle2 v-if="exportOptions[opt.key]" :size="13" class="opt-check" />
          </button>
        </div>
        <button class="primary-btn" :disabled="busy" @click="confirmExportProject"><Download :size="16" />导出 Word</button>
      </section>

      <section v-else-if="modal === 'coverageHistory'" class="drawer">
        <button class="modal-close" @click="closeModal"><X :size="18" /></button>
        <h2>评估历史</h2>
        <input v-model="historySearch" placeholder="搜索评估记录" />
        <article v-for="item in filteredEvaluations" :key="item.id" class="evaluation-record">
          <div class="eval-record-head">
            <b>{{ item.title }}</b>
            <div class="eval-record-actions">
              <span :class="['eval-status', item.status === 'completed' ? 'eval-done' : 'eval-pending']">{{ item.status === 'completed' ? '已完成' : '评估中' }}</span>
              <button type="button" title="删除评估" @click="deleteEvaluationRecord(item)"><Trash2 :size="14" /></button>
            </div>
          </div>
          <div class="eval-record-meta">
            <small>{{ item.createdAt }}</small>
            <em v-if="item.score">{{ item.score }}分</em>
          </div>
          <p v-if="item.summary">{{ item.summary }}</p>
          <div v-if="item.dimensions?.length" class="dimension-list">
            <span v-for="dim in item.dimensions" :key="dim.name" class="dim-tag">{{ dim.name }} <b>{{ dim.score }}</b></span>
          </div>
          <ul v-if="item.suggestions?.length" class="eval-suggestions">
            <li v-for="(sug, i) in item.suggestions" :key="i">{{ sug }}</li>
          </ul>
        </article>
        <p v-if="filteredEvaluations.length === 0" class="drawer-empty">暂无评估记录</p>
      </section>

      <section v-else-if="modal === 'premiumRenew'" class="info-modal">
        <button class="modal-close" @click="closeModal"><X :size="18" /></button>
        <h2>会员续费</h2>
        <div class="price-grid">
          <button :class="{ selected: renewPlan === 'month' }" @click="renewPlan = 'month'">月卡 <b>¥49</b></button>
          <button :class="{ selected: renewPlan === 'quarter' }" @click="renewPlan = 'quarter'">季卡 <b>¥129</b></button>
          <button :class="{ selected: renewPlan === 'year' }" @click="renewPlan = 'year'">年卡 <b>¥399</b></button>
        </div>
        <label class="check-line"><input type="checkbox" v-model="agreement" /> 我已阅读并同意会员服务协议</label>
        <button class="primary-btn" :disabled="!agreement" @click="renewMembership">立即支付</button>
      </section>

      <section v-else-if="modal === 'walletHelp'" class="info-modal">
        <button class="modal-close" @click="closeModal"><X :size="18" /></button>
        <h2>剧点说明</h2>
        <p>剧点用于 AI 生成、剧本评估、短剧拆解类能力和自定义策划。不同能力按按钮显示的点数消耗，服务端在执行前重新校验余额和权益。</p>
      </section>

      <section v-else-if="modal === 'recharge'" class="info-modal wide">
        <button class="modal-close" @click="closeModal"><X :size="18" /></button>
        <h2>充值剧点</h2>
        <div class="price-grid">
          <button v-for="pack in normalPacks" :key="pack.code" :class="{ selected: selectedPack === pack.code }" @click="selectedPack = pack.code">
            <b>¥{{ pack.price }}</b><span>{{ pack.points }} 剧点</span><small>{{ pack.bonus ? `赠送${pack.bonus}` : '入门体验' }}</small>
          </button>
        </div>
        <label class="check-line"><input type="checkbox" v-model="agreement" /> 我已阅读并同意充值协议</label>
        <button class="primary-btn" :disabled="!agreement" @click="recharge">确认充值</button>
      </section>

      <section v-else-if="modal === 'enterprise'" class="info-modal wide">
        <button class="modal-close" @click="closeModal"><X :size="18" /></button>
        <h2>企业充值</h2>
        <p>适合团队批量创作、机构账号、剧本工作室和企业采购。订单确认后记录企业付款单，开票与到账状态由财务流程同步。</p>
        <div class="price-grid">
          <button v-for="pack in enterprisePacks" :key="pack.code" @click="selectedPack = pack.code"><b>¥{{ pack.price }}</b><span>{{ pack.points }} 剧点</span></button>
        </div>
        <button class="primary-btn" @click="recharge">提交企业充值</button>
      </section>

      <section v-else-if="modal === 'password'" class="info-modal">
        <button class="modal-close" @click="closeModal"><X :size="18" /></button>
        <h2>修改密码</h2>
        <label><span>绑定手机</span><input :value="profile.phone" readonly /></label>
        <label><span>验证码</span><input v-model="passwordForm.code" placeholder="请输入短信验证码" /></label>
        <label><span>新密码</span><input v-model="passwordForm.password" type="password" /></label>
        <label><span>确认密码</span><input v-model="passwordForm.confirm" type="password" /></label>
        <button class="primary-btn" @click="savePassword">确认修改</button>
      </section>

      <section v-else-if="modal === 'adaptType'" class="adapt-type-modal">
        <button class="modal-close" @click="closeModal"><X :size="18" /></button>
        <h2>选择改编方式</h2>
        <div class="adapt-mode-grid">
          <button :class="{ selected: adaptMode === 'full' }" @click="adaptMode = 'full'"><b>全部改编</b><small>提炼概梗、人物小传、章纲，再重组短剧结构</small></button>
          <button :class="{ selected: adaptMode === 'original' }" @click="adaptMode = 'original'"><b>按照原文</b><small>保留小说拆解章纲，只补故事概梗</small></button>
        </div>
        <div class="script-type-split">
          <button class="script-type-image ai" aria-label="AI短剧" @click="chooseAdaptType('AI短剧')">
            <img src="/type-assets/ai-short.png" alt="" />
            <span>AI短剧</span>
          </button>
          <button class="script-type-image live" aria-label="真人实拍" @click="chooseAdaptType('真人实拍')">
            <img src="/type-assets/live-action.png" alt="" />
            <span>真人实拍</span>
          </button>
        </div>
      </section>

      <section v-else-if="modal === 'rewriteType'" class="rewrite-confirm-modal">
        <button class="modal-close" @click="closeModal"><X :size="18" /></button>
        <h2>选择改写类型</h2>
        <p>系统会先提取原剧核心信息流，再按所选制作方式进行改写策划和结构拆解。</p>
        <div class="script-type-split rewrite">
          <button class="script-type-image ai" aria-label="AI短剧" :class="{ selected: rewriteType === 'AI短剧' }" @click="rewriteType = 'AI短剧'">
            <img src="/type-assets/ai-short.png" alt="" />
            <span>AI短剧</span>
          </button>
          <button class="script-type-image live" aria-label="真人实拍" :class="{ selected: rewriteType === '真人实拍' }" @click="rewriteType = '真人实拍'">
            <img src="/type-assets/live-action.png" alt="" />
            <span>真人实拍</span>
          </button>
        </div>
        <footer>
          <span><b>{{ rewriteImportCost }}</b> 预计消耗剧点（500字/剧点，{{ pendingRewriteUpload?.words || 0 }}字）</span>
          <button @click="closeModal">取消</button>
          <button class="primary-btn" :disabled="busy || rewriteImportCost > wallet.balance" @click="confirmRewriteUpload">确认拆解</button>
        </footer>
      </section>

      <section v-else-if="modal === 'chapterRange'" class="chapter-modal">
        <button class="modal-close" @click="closeModal"><X :size="18" /></button>
        <h2>拆解范围</h2>
        <p>小说共<b>{{ chapterRange.total }}</b>章，总计<b>{{ chapterRange.totalWords }}</b>字，请选择需要拆解的范围</p>
        <div class="adapt-mode-grid chapter-mode-grid">
          <button :class="{ selected: adaptMode === 'full' }" @click="adaptMode = 'full'"><b>全部改编</b><small>提炼概梗、人物小传、章纲，再重组短剧结构</small></button>
          <button :class="{ selected: adaptMode === 'original' }" @click="adaptMode = 'original'"><b>按照原文</b><small>保留小说拆解章纲，只补故事概梗</small></button>
        </div>
        <div class="chapter-split-layout">
          <div class="chapter-range-rail">
            <input v-model.number="chapterRange.start" type="number" min="1" :max="chapterRange.end" @change="normalizeChapterRange" />
            <span></span>
            <input v-model.number="chapterRange.end" type="number" :min="chapterRange.start" :max="chapterRange.total" @change="normalizeChapterRange" />
          </div>
          <div class="chapter-check-list">
            <label v-for="chapter in chapterRange.chapters" :key="chapter.no" :class="{ selected: chapter.no >= chapterRange.start && chapter.no <= chapterRange.end }" @click="pickChapter(chapter.no)">
              <input type="checkbox" :checked="chapter.no >= chapterRange.start && chapter.no <= chapterRange.end" readonly />
              第{{ chapter.no }}章
            </label>
          </div>
          <article class="chapter-preview">
            <header><b>第{{ currentChapter?.no || chapterRange.start }}章 {{ currentChapter?.title || '未命名章节' }}</b><small>{{ currentChapter?.words || 0 }}字</small></header>
            <textarea readonly :value="currentChapter?.content || ''"></textarea>
          </article>
        </div>
        <footer>
          <span class="chapter-cost"><b>{{ chapterRangeCost }}</b> 剧点<span>400字/剧点 · 已选 {{ selectedChapterCount }} 章 · {{ selectedChapterWords }} 字</span></span>
          <div class="chapter-footer-actions">
            <button @click="closeModal">取消</button>
            <button class="primary-btn" :disabled="busy || chapterRangeCost > wallet.balance" @click="confirmChapterRange">确认拆解</button>
          </div>
        </footer>
      </section>
    </div>

    <div v-if="uploadOverlay" class="global-loading"><LoaderCircle :size="22" class="spin-loader" />文件处理中，请稍后</div>
    <div v-if="toastText" class="toast">{{ toastText }}</div>
  </div>
</template>

<script setup>
import { computed, defineComponent, h, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import {
  AlertTriangle,
  BookOpen,
  Camera,
  CheckCircle2,
  ChevronDown,
  Circle,
  FilePenLine,
  FilePlus2,
  History,
  Info,
  ListFilter,
  LoaderCircle,
  Maximize2,
  Coins,
  MoreVertical,
  PanelLeft,
  Plus,
  Download,
  RotateCcw,
  RotateCw,
  Search,
  Settings,
  ShieldCheck,
  Sparkles,
  Trash2,
  UploadCloud,
  Wallet,
  Wand2,
  X
} from 'lucide-vue-next'

const API_BASE = import.meta.env.VITE_API_BASE || 'http://localhost:18080'

const route = ref('creation')
const modal = ref('')
const loading = ref(false)
const busy = ref(false)
const uploadBusy = ref(false)
const toastText = ref('')
const searchText = ref('')
const historySearch = ref('')
const settingsTab = ref('profile')
const renewPlan = ref('month')
const selectedPack = ref('p100')
const agreement = ref(true)
const rowMenuOpen = ref(null)
const exportProjectDraft = ref(null)
const exportOptions = reactive({ settings: true, characters: true, outlines: true, episodes: true, body: true })
const exportOptionDefs = [
  { key: 'settings',   label: '故事设定', icon: BookOpen },
  { key: 'characters', label: '人物小传', icon: Circle },
  { key: 'outlines',   label: '粗纲',     icon: ListFilter },
  { key: 'episodes',   label: '集纲',     icon: History },
  { key: 'body',       label: '正文',     icon: FilePenLine },
]

const profile = reactive({ id: 1, phone: '未绑定', nickname: '创作者', memberUntil: '2026-06-06 19:48', membership: '剧本专家' })
const profileForm = reactive({ nickname: '创作者' })
const aiProviders = [
  {
    key: 'deepseek',
    label: 'DeepSeek',
    hint: '推理与通用写作',
    baseURL: 'https://api.deepseek.com',
    consoleURL: 'https://platform.deepseek.com/api_keys',
    models: ['deepseek-v4-pro', 'deepseek-v4-flash']
  },
  {
    key: 'kimi',
    label: 'Kimi',
    hint: '长文本与创作',
    baseURL: 'https://api.moonshot.ai/v1',
    consoleURL: 'https://platform.moonshot.cn/console/api-keys',
    models: ['kimi-k2.7-code', 'kimi-k2.6', 'kimi-k2.5', 'moonshot-v1-128k']
  },
  {
    key: 'doubao',
    label: '豆包',
    hint: '火山方舟',
    baseURL: 'https://ark.cn-beijing.volces.com/api/v3',
    consoleURL: 'https://console.volcengine.com/ark/region:ark+cn-beijing/apiKey',
    models: ['doubao-seed-2-0-pro-260215', 'doubao-seed-2-0-lite-260215', 'doubao-seed-2-0-mini-260428', 'doubao-seed-code']
  },
  {
    key: 'zhipu',
    label: '智谱',
    hint: 'GLM 系列',
    baseURL: 'https://open.bigmodel.cn/api/paas/v4',
    consoleURL: 'https://bigmodel.cn/usercenter/proj-mgmt/apikeys',
    models: ['glm-5.1', 'glm-5-turbo', 'glm-5', 'glm-4.7']
  },
  {
    key: 'qwen',
    label: '通义千问',
    hint: '阿里百炼',
    baseURL: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    consoleURL: 'https://bailian.console.aliyun.com/?tab=model#/api-key',
    models: ['qwen3.7-max', 'qwen3.7-plus', 'qwen3.6-flash']
  }
]
const aiConfig = reactive({ provider: 'deepseek', baseURL: '', model: '', configured: false })
const aiConfigForm = reactive({ provider: 'deepseek', model: '', apiKey: '' })
const aiProviderConfigs = reactive({})
const customModels = reactive({})
const customModelName = ref('')
const wallet = reactive({ balance: 1000, frozen: 0, items: [], packs: [], tasks: [], serviceCosts: {} })
const scripts = ref([])
const evaluations = ref([])
const currentProject = ref(null)
const uploadJobs = ref([])
const pendingRewriteUpload = ref(null)
const pendingAdaptUpload = ref(null)
const adaptMode = ref('full')
const rewriteType = ref('AI短剧')
const evaluationForm = reactive({ title: '未命名评估', culture: '国内', audience: '男频', scriptType: '真人实拍', preference: '', fileName: '', content: '', dimensions: ['plot', 'character', 'commercial', 'market', 'pacing', 'dialogue'] })
const generalSettings = reactive({ autosave: true, guides: true })
const passwordForm = reactive({ code: '', password: '', confirm: '' })
const chapterRange = reactive({ projectId: 0, start: 1, end: 1, total: 0, totalWords: 0, chapters: [] })

const workNav = [
  { key: 'creation', label: '剧本原创', icon: FilePenLine },
  { key: 'rewrite', label: '剧本改写', icon: Wand2 },
  { key: 'adapt', label: '网文改编', icon: PanelLeft }
]

const commonNav = [
  { key: 'coverage', label: '评估', icon: ShieldCheck },
  { key: 'wallet', label: '剧点', icon: Wallet },
  { key: 'settings', label: '设置', icon: Settings }
]

const pageTitle = computed(() => ({
  creation: '剧本原创',
  rewrite: '剧本改写',
  adapt: '网文改编',
  coverage: '剧本评估',
  wallet: '剧点',
  settings: '设置'
}[route.value] || '剧本原创'))

const showsSearch = computed(() => ['creation', 'rewrite', 'adapt'].includes(route.value))
const sourceName = computed(() => route.value === 'rewrite' ? 'rewriting' : 'adaptation')
const visibleScripts = computed(() => filterProjects('original'))
const sourceProjects = computed(() => filterProjects(sourceName.value))
const activeImport = computed(() => uploadJobs.value.find((item) => item.purpose === sourceName.value) || null)
const currentProjectRoute = computed(() => {
  if (currentProject.value?.source === 'rewriting') return 'rewrite'
  if (currentProject.value?.source === 'adaptation') return 'adapt'
  return 'creation'
})
const editorBackRoute = computed(() => currentProjectRoute.value)
const activeWorkKey = computed(() => route.value === 'editor' ? currentProjectRoute.value : route.value)
const sourceCopy = computed(() => route.value === 'rewrite'
  ? {
    emptyTitle: '您暂无改写剧本',
    emptyText: '点击按钮上传完整剧本，即刻进行剧本拆解。系统会识别场景、人物、对白和节奏问题，生成可继续编辑的改写项目。'
  }
  : {
    emptyTitle: '您暂无改编剧本',
    emptyText: '点击按钮上传小说文件，即刻进行章纲拆解。系统会先识别章节，再选择拆解范围并转成短剧化大纲。'
  })

const selectedChapterCount = computed(() => Math.max(0, chapterRange.end - chapterRange.start + 1))
const selectedChapterWords = computed(() => chapterRange.chapters
  .filter((chapter) => chapter.no >= chapterRange.start && chapter.no <= chapterRange.end)
  .reduce((sum, chapter) => sum + chapter.words, 0))
const chapterRangeCost = computed(() => Math.max(1, Math.ceil(selectedChapterWords.value / 400)))
const rewriteImportCost = computed(() => Math.max(20, Math.ceil((pendingRewriteUpload.value?.words || 0) / 500)))
const normalPacks = computed(() => wallet.packs.filter((pack) => !pack.enterprise))
const enterprisePacks = computed(() => wallet.packs.filter((pack) => pack.enterprise))
const evaluationCost = computed(() => Number(wallet.serviceCosts?.evaluation || 2500))
const latestEvaluation = computed(() => evaluations.value[0] || null)
const filteredEvaluations = computed(() => evaluations.value.filter((item) => !historySearch.value || item.title.includes(historySearch.value)))
const uploadOverlay = computed(() => uploadBusy.value && ['rewrite', 'adapt'].includes(route.value))
const currentChapter = computed(() => chapterRange.chapters.find((chapter) => chapter.no === chapterRange.start) || chapterRange.chapters[0] || null)
const selectedAIProvider = computed(() => aiProviders.find((item) => item.key === aiConfigForm.provider) || aiProviders[0])
const selectedProviderModels = computed(() => {
  const defaults = selectedAIProvider.value.models || []
  const added = customModels[aiConfigForm.provider] || []
  return [...new Set([...defaults, ...added].filter(Boolean))]
})
const selectedProviderConfig = computed(() => aiProviderConfigs[aiConfigForm.provider] || {})
const selectedProviderConfigured = computed(() => Boolean(selectedProviderConfig.value.apiKeySet))

const settingsTabs = [
  { key: 'profile', label: '我的资料' },
  { key: 'security', label: '账号安全' },
  { key: 'contact', label: '联系我们' },
  { key: 'general', label: '通用设置' }
]

onMounted(loadApp)

watch(() => profile.nickname, (value) => {
  profileForm.nickname = value
})

async function api(path, options = {}) {
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) }
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || res.statusText)
  }
  return res.json()
}

async function streamApi(path, payload, onFrame) {
  const res = await fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload)
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || res.statusText)
  }
  if (!res.body) throw new Error('浏览器不支持流式读取')
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  while (true) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() || ''
    for (const line of lines) {
      const frame = parseStreamFrame(line)
      if (frame) onFrame(frame.event, frame.data)
    }
  }
  if (buffer.trim()) {
    const frame = parseStreamFrame(buffer)
    if (frame) onFrame(frame.event, frame.data)
  }
}

function parseStreamFrame(line) {
  const text = String(line || '').trim()
  if (!text.startsWith('@@')) return null
  const space = text.indexOf(' ')
  if (space < 3) return null
  const event = text.slice(2, space)
  const bytes = Uint8Array.from(atob(text.slice(space + 1)), (char) => char.charCodeAt(0))
  return { event, data: JSON.parse(new TextDecoder().decode(bytes)) }
}

async function loadApp() {
  loading.value = true
  try {
    const [profileData, walletData, scriptData, evalData, aiConfigData] = await Promise.all([
      api('/api/profile'),
      api('/api/wallet'),
      api('/api/scripts'),
      api('/api/evaluations'),
      api('/api/settings/ai-config')
    ])
    Object.assign(profile, profileData)
    Object.assign(profileForm, { nickname: profileData.nickname })
    syncAIConfig(aiConfigData)
    syncWallet(walletData)
    scripts.value = scriptData
    evaluations.value = evalData
  } catch (err) {
    toast(`加载失败：${cleanError(err)}`)
  } finally {
    loading.value = false
  }
}

function syncAIConfig(data = {}) {
  const provider = aiProviders.some((item) => item.key === data.provider) ? data.provider : inferAIProvider(data.baseURL)
  Object.keys(aiProviderConfigs).forEach((key) => delete aiProviderConfigs[key])
  Object.entries(data.providers || {}).forEach(([key, value]) => {
    if (aiProviders.some((item) => item.key === key)) {
      aiProviderConfigs[key] = {
        provider: key,
        baseURL: value?.baseURL || '',
        model: value?.model || '',
        apiKeySet: Boolean(value?.apiKeySet)
      }
    }
  })
  if (!aiProviderConfigs[provider]) {
    aiProviderConfigs[provider] = {
      provider,
      baseURL: data.baseURL || selectedAIProvider.value.baseURL || '',
      model: data.model || '',
      apiKeySet: Boolean(data.apiKeySet)
    }
  }
  aiConfig.baseURL = data.baseURL || ''
  aiConfig.model = data.model || ''
  aiConfig.provider = provider
  aiConfig.configured = Boolean(data.configured)
  aiConfigForm.provider = provider
  applyProviderConfig(provider)
}

function inferAIProvider(baseURL = '') {
  const value = String(baseURL).toLowerCase()
  if (value.includes('moonshot') || value.includes('kimi')) return 'kimi'
  if (value.includes('volces') || value.includes('volcengine')) return 'doubao'
  if (value.includes('bigmodel')) return 'zhipu'
  if (value.includes('dashscope')) return 'qwen'
  return 'deepseek'
}

function selectAIProvider(key) {
  if (!aiProviders.some((item) => item.key === key)) return
  aiConfigForm.provider = key
  customModelName.value = ''
  applyProviderConfig(key)
}

function applyProviderConfig(provider) {
  const savedModel = aiProviderConfigs[provider]?.model || ''
  const models = [
    ...(aiProviders.find((item) => item.key === provider)?.models || []),
    ...(customModels[provider] || [])
  ]
  aiConfigForm.model = models.includes(savedModel) ? savedModel : (models[0] || '')
  aiConfigForm.apiKey = ''
}

function addCustomModel() {
  const value = customModelName.value.trim()
  if (!value) return
  const key = aiConfigForm.provider
  customModels[key] = [...new Set([...(customModels[key] || []), value])]
  aiConfigForm.model = value
  customModelName.value = ''
}

function syncWallet(data) {
  wallet.balance = data.balance || 0
  wallet.frozen = data.frozen || 0
  wallet.items = data.items || []
  wallet.packs = data.packs || []
  wallet.tasks = data.tasks || []
  wallet.serviceCosts = data.serviceCosts || {}
}

function filterProjects(source) {
  return scripts.value.filter((item) => item.source === source && (!searchText.value || item.title.includes(searchText.value) || JSON.stringify(item.settings || {}).includes(searchText.value)))
}

function go(next) {
  route.value = next
  modal.value = ''
  searchText.value = ''
  rowMenuOpen.value = null
}

function closeModal() {
  if (modal.value === 'rewriteType') pendingRewriteUpload.value = null
  if (modal.value === 'adaptType' || modal.value === 'chapterRange') pendingAdaptUpload.value = null
  if (modal.value === 'exportProject') exportProjectDraft.value = null
  modal.value = ''
}

function formatProjectTime(value) {
  if (!value) return '刚刚'
  if (value.includes(' ')) {
    const parts = value.split(' ')
    return parts.length > 1 ? parts[0].slice(5).replace('-', '月') + '日 ' + parts[1] : value
  }
  return value
}

async function createAndOpen(type) {
  busy.value = true
  try {
    const project = await api('/api/scripts', { method: 'POST', body: JSON.stringify({ title: '未命名', type, source: 'original' }) })
    scripts.value.unshift(project)
    modal.value = ''
    openEditor(project)
  } catch (err) {
    toast(`创建失败：${cleanError(err)}`)
  } finally {
    busy.value = false
  }
}

function openEditor(project) {
  rowMenuOpen.value = null
  currentProject.value = cloneData(project)
  route.value = 'editor'
}

function toggleRowMenu(id) {
  rowMenuOpen.value = rowMenuOpen.value === id ? null : id
}

function projectCoverIcon(project) {
  if (project?.source === 'rewriting') return Wand2
  if (project?.source === 'adaptation') return BookOpen
  return FilePenLine
}

function projectCoverClass(project) {
  if (project?.source === 'rewriting') return 'cover-rewrite'
  if (project?.source === 'adaptation') return 'cover-adapt'
  return 'cover-original'
}

function projectBadge(project) {
  if (project?.source === 'rewriting') return '改写'
  if (project?.source === 'adaptation') return '网文'
  return '原创'
}

async function exportListProject(project) {
  rowMenuOpen.value = null
  exportProjectDraft.value = cloneData(project)
  Object.assign(exportOptions, { settings: true, characters: true, outlines: true, episodes: true, body: true })
  modal.value = 'exportProject'
}

async function confirmExportProject() {
  const project = exportProjectDraft.value
  if (!project?.id) return
  try {
    await exportWord(project, cloneData(exportOptions))
    closeModal()
  } catch (err) {
    toast(`导出失败：${cleanError(err)}`)
  }
}

async function deleteListProject(project) {
  rowMenuOpen.value = null
  if (!project?.id) return
  if (!window.confirm(`确认删除《${project.title || '未命名'}》？`)) return
  busy.value = true
  try {
    await api(`/api/scripts/${project.id}`, { method: 'DELETE' })
    scripts.value = scripts.value.filter((item) => item.id !== project.id)
    if (currentProject.value?.id === project.id) {
      currentProject.value = null
      route.value = project.source === 'rewriting' ? 'rewrite' : (project.source === 'adaptation' ? 'adapt' : 'creation')
    }
    toast('项目已删除')
  } catch (err) {
    toast(`删除失败：${cleanError(err)}`)
  } finally {
    busy.value = false
  }
}

async function deleteEvaluationRecord(item) {
  if (!item?.id) return
  if (!window.confirm(`确认删除评估《${item.title || '未命名评估'}》？`)) return
  try {
    await api(`/api/evaluations?id=${encodeURIComponent(item.id)}`, { method: 'DELETE' })
    evaluations.value = evaluations.value.filter((record) => record.id !== item.id)
    toast('评估记录已删除')
  } catch (err) {
    toast(`删除失败：${cleanError(err)}`)
  }
}

async function saveProject(payload) {
  const project = payload?.project || payload
  if (!project?.id) return
  const silent = Boolean(payload?.silent)
  if (!silent) busy.value = true
  try {
    const updated = await api(`/api/scripts/${project.id}`, { method: 'PUT', body: JSON.stringify(project) })
    if (!silent) currentProject.value = cloneData(updated)
    upsertScript(updated)
    if (!silent) toast('草稿已保存')
    payload?.onDone?.(updated)
  } catch (err) {
    if (!silent) toast(`保存失败：${cleanError(err)}`)
    payload?.onError?.(err)
  } finally {
    if (!silent) busy.value = false
  }
}

async function runAiTask({ taskType, cost, project, prompt, onResult, onError, onStream }) {
  busy.value = true
  try {
    if (onStream && ['planning', 'characters', 'outline', 'episode', 'body'].includes(taskType)) {
      let streamError = ''
      await streamApi(`/api/ai-task/stream`, { taskType, cost, projectId: project.id, project, prompt }, (event, data) => {
        if (event === 'error') {
          streamError = data?.message || '生成失败'
        } else if (event === 'frame' || event === 'body' || event === 'episode' || event === 'delta' || event === 'character' || event === 'outline') {
          onStream(data)
        } else if (event === 'done' || event === 'complete') {
          if (data?.wallet) syncWallet(data.wallet)
          if (data?.project) {
            currentProject.value = cloneData(data.project)
            upsertScript(data.project)
          }
          onResult?.(data)
        } else if (data?.wallet) {
          syncWallet(data.wallet)
        }
      })
      if (streamError) throw new Error(streamError)
      toast('AI生成已完成')
      return
    }
    const data = await api('/api/ai-task', { method: 'POST', body: JSON.stringify({ taskType, cost, projectId: project.id, project, prompt }) })
    syncWallet(data.wallet)
    currentProject.value = cloneData(data.project)
    upsertScript(data.project)
    onResult?.(data)
    toast(data.status === 'succeeded' ? 'AI生成已完成' : '任务已提交')
    return data
  } catch (err) {
    toast(`AI生成失败：${cleanAiError(err)}`)
    onError?.(err)
    return null
  } finally {
    busy.value = false
  }
}

async function exportWord(project = currentProject.value, options = null) {
  const data = await api('/api/export', { method: 'POST', body: JSON.stringify({ projectId: project?.id, project, options }) })
  if (data.content) {
    downloadTextFile(data.fileName, data.content, data.mime)
  }
  toast(`${data.fileName} 已生成`)
}

async function uploadSource(event, purpose) {
  const file = event.target.files?.[0]
  const title = fileTitle(event) || (purpose === 'adaptation' ? '网文改编项目' : '剧本改写项目')
  if (!file) return
  busy.value = true
  uploadBusy.value = true
  try {
    const content = await readTextFile(file)
    validateSourceContent(purpose, file, content)
    const chapters = purpose === 'adaptation' ? parseNovelChapters(content) : []

    if (purpose === 'adaptation') {
      adaptMode.value = 'full'
      pendingAdaptUpload.value = { title, fileName: file?.name || title, content, chapters, scriptType: 'AI短剧', adaptMode: 'full' }
      seedChapterRange(null, chapters)
      modal.value = 'adaptType'
      return
    }

    pendingRewriteUpload.value = { title, fileName: file?.name || title, content, words: content.replace(/\s/g, '').length }
    rewriteType.value = 'AI短剧'
    modal.value = 'rewriteType'
  } catch (err) {
    toast(`上传解析失败：${cleanError(err)}`)
  } finally {
    busy.value = false
    uploadBusy.value = false
    event.target.value = ''
  }
}

async function confirmRewriteUpload() {
  if (!pendingRewriteUpload.value) {
    closeModal()
    return
  }
  busy.value = true
  uploadBusy.value = true
  try {
    const pending = pendingRewriteUpload.value
    pendingRewriteUpload.value = null
    closeModal()
    await runUploadStream('rewriting', {
      title: pending.title,
      fileName: pending.fileName,
      content: pending.content,
      scriptType: rewriteType.value,
      cost: rewriteImportCost.value
    })
  } catch (err) {
    toast(`拆解失败：${cleanError(err)}`)
  } finally {
    busy.value = false
    uploadBusy.value = false
  }
}

function createImportJob(purpose, title) {
  return {
    id: Date.now(),
    purpose,
    title,
    status: 'queued',
    message: '等待读取文件',
    steps: importSteps('queued'),
    project: null
  }
}

function setImportJob(job) {
  uploadJobs.value = [job, ...uploadJobs.value.filter((item) => item.purpose !== job.purpose)].slice(0, 4)
}

function updateImportJob(id, patch) {
  const index = uploadJobs.value.findIndex((item) => item.id === id)
  if (index >= 0) uploadJobs.value[index] = { ...uploadJobs.value[index], ...patch }
}

function importSteps(status) {
  const order = ['格式校验', '上传读取', 'AI拆解', '生成项目']
  const doneMap = {
    queued: -1,
    checking: 0,
    parsing: 2,
    done: 4,
    failed: -1
  }
  const doneIndex = doneMap[status] ?? -1
  return order.map((label, index) => ({
    label,
    state: status === 'failed' && index === 0 ? 'failed' : index < doneIndex ? 'done' : index === doneIndex ? 'active' : 'pending'
  }))
}

function validateSourceContent(purpose, file, content) {
  const lower = file.name.toLowerCase()
  if (!lower.endsWith('.txt')) return
  const text = String(content || '').trim()
  if (text.length < 40) {
    throw new Error(purpose === 'adaptation' ? '小说正文过短，请上传包含章节正文的文件' : '剧本正文过短，请上传完整剧本')
  }
  if (purpose === 'adaptation') {
    const chapters = parseNovelChapters(text)
    if (chapters.length === 0) {
      throw new Error('小说正文过短或缺少可拆分段落，请上传包含章节正文的文件')
    }
    return
  }
  const scriptScore = ['场景', '人物', '第1集', '第1场', '：', ':'].reduce((sum, token) => sum + (text.includes(token) ? 1 : 0), 0)
  if (scriptScore < 2) {
    throw new Error('导入的文件可能非剧本内容，请检查后重试')
  }
}

function importFailureMessage(purpose, err) {
  const text = cleanError(err)
  if (text) return text
  return purpose === 'adaptation' ? '导入的文件可能非小说内容，请检查章节格式后重试' : '导入的文件可能非剧本内容，请检查后重试'
}

function parseNovelChapters(content) {
  const text = normalizeNovelChapterText(content)
  const headings = []
  let offset = 0
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    const title = parseChapterTitle(line)
    if (title !== null) headings.push({ index: offset, title })
    offset += raw.length + 1
  }
  if (!headings.length) return fallbackChaptersByLength(text, 2200)
  const chapters = headings.slice(0, 150).map((heading, index) => {
    const start = heading.index
    const end = headings[index + 1]?.index ?? text.length
    const body = text.slice(start, end).trim()
    return {
      no: index + 1,
      title: heading.title,
      content: body,
      words: body.replace(/\s/g, '').length
    }
  })
  return compactShortNovelChapters(chapters)
}

const MIN_PARSED_NOVEL_CHAPTER_WORDS = 120

function compactShortNovelChapters(chapters) {
  if (chapters.length <= 1) return chapters
  const result = []
  let leadingShort = null
  for (const chapter of chapters) {
    if (chapter.words < MIN_PARSED_NOVEL_CHAPTER_WORDS) {
      if (result.length) {
        result[result.length - 1] = appendNovelChapterContent(result[result.length - 1], chapter.content)
      } else {
        leadingShort = leadingShort
          ? appendNovelChapterContent(leadingShort, chapter.content)
          : { ...chapter }
      }
      continue
    }
    const next = leadingShort
      ? prependNovelChapterContent(chapter, leadingShort.content)
      : { ...chapter }
    leadingShort = null
    result.push(next)
  }
  if (leadingShort) {
    if (result.length) {
      result[result.length - 1] = appendNovelChapterContent(result[result.length - 1], leadingShort.content)
    } else {
      result.push(leadingShort)
    }
  }
  return result.map((chapter, index) => ({
    ...chapter,
    no: index + 1,
    words: chapter.content.replace(/\s/g, '').length
  }))
}

function appendNovelChapterContent(chapter, extraContent) {
  const content = [chapter.content, extraContent].map((item) => String(item || '').trim()).filter(Boolean).join('\n')
  return { ...chapter, content, words: content.replace(/\s/g, '').length }
}

function prependNovelChapterContent(chapter, prefixContent) {
  const content = [prefixContent, chapter.content].map((item) => String(item || '').trim()).filter(Boolean).join('\n')
  return { ...chapter, content, words: content.replace(/\s/g, '').length }
}

function normalizeNovelChapterText(content) {
  return String(content || '')
    .replace(/\r\n?/g, '\n')
    .replace(/\u3000/g, ' ')
    .replace(/([^\n])(\s*(?:第\s*[0-9零〇一二两三四五六七八九十百千万]+\s*[章节回卷集部话幕]|(?:chapter|chap\.?)\s*[0-9ivxlcdm]+|序章|楔子|引子|前言|尾声|后记|番外(?:\s*[0-9零〇一二两三四五六七八九十]+)?)(?=\s*[:：、.．\-—]?\s*[^\n]{0,48}(?:\n|$)))/gi, '$1\n$2')
}

function parseChapterTitle(line) {
  line = String(line || '').replace(/\u3000/g, ' ').trim()
  if (!line || line.length > 96) return null
  const patterns = [
    /^第\s*[0-9零〇一二两三四五六七八九十百千万]+\s*[章节回卷集部]\s*[:：、.．\-—]?\s*(.*)$/,
    /^第\s*[0-9零〇一二两三四五六七八九十百千万]+\s*[话幕]\s*[:：、.．\-—]?\s*(.*)$/,
    /^(?:chapter|chap\.?)\s*[0-9ivxlcdm]+\s*[:：、.．\-—]?\s*(.*)$/i,
    /^(序章|楔子|引子|前言|尾声|后记|番外(?:\s*[0-9零〇一二两三四五六七八九十]+)?)\s*[:：、.．\-—]?\s*(.*)$/,
    /^[0-9]{1,4}\s*[、.．]\s*(.{0,36})$/,
    /^[0-9]{1,4}\s+([^\s].{0,36})$/,
    /^[零〇一二两三四五六七八九十百千万]{1,6}\s*[、.．]\s*(.{0,36})$/
  ]
  for (const pattern of patterns) {
    const match = line.match(pattern)
    if (!match) continue
    const picked = (match[2] || match[1] || '').trim()
    return picked.replace(/^[:：、.．\-—]+/, '').trim()
  }
  return null
}

function fallbackChaptersByLength(content, targetWords = 2200) {
  const text = String(content || '').trim()
  if (text.replace(/\s/g, '').length < 80) return []
  const chapters = []
  let buffer = ''
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (!line) continue
    buffer += `${buffer ? '\n' : ''}${line}`
    if (buffer.replace(/\s/g, '').length >= targetWords && chapters.length < 150) {
      chapters.push(makeFallbackChapter(chapters.length + 1, buffer))
      buffer = ''
    }
  }
  if (buffer.trim() && chapters.length < 150) chapters.push(makeFallbackChapter(chapters.length + 1, buffer))
  if (chapters.length === 1 && chapters[0].words > targetWords * 2) {
    const runes = Array.from(text)
    const split = []
    for (let start = 0; start < runes.length && split.length < 150; start += targetWords) {
      split.push(makeFallbackChapter(split.length + 1, runes.slice(start, start + targetWords).join('')))
    }
    return split
  }
  return chapters
}

function makeFallbackChapter(no, content) {
  return { no, title: `自动拆分 ${no}`, content: content.trim(), words: content.replace(/\s/g, '').length }
}

function seedChapterRange(project, chapters) {
  chapterRange.projectId = project?.id || 0
  chapterRange.start = 1
  chapterRange.end = Math.max(1, Math.min(chapters.length, 20))
  chapterRange.total = chapters.length
  chapterRange.totalWords = chapters.reduce((sum, chapter) => sum + chapter.words, 0)
  chapterRange.chapters = chapters
}

function chooseAdaptType(type) {
  if (!pendingAdaptUpload.value) {
    closeModal()
    return
  }
  pendingAdaptUpload.value.scriptType = type
  pendingAdaptUpload.value.adaptMode = adaptMode.value
  modal.value = 'chapterRange'
}

function normalizeChapterRange() {
  chapterRange.start = clampNumber(chapterRange.start, 1, Math.max(1, chapterRange.total))
  chapterRange.end = clampNumber(chapterRange.end, chapterRange.start, Math.max(chapterRange.start, chapterRange.total))
}

function pickChapter(no) {
  const picked = clampNumber(no, 1, Math.max(1, chapterRange.total))
  if (picked < chapterRange.start) {
    chapterRange.start = picked
  } else {
    chapterRange.end = picked
  }
  normalizeChapterRange()
}

async function confirmChapterRange() {
  const selected = chapterRange.chapters.filter((chapter) => chapter.no >= chapterRange.start && chapter.no <= chapterRange.end)
  if (!selected.length || !pendingAdaptUpload.value) {
    closeModal()
    return
  }
  busy.value = true
  uploadBusy.value = true
  try {
    const pending = pendingAdaptUpload.value
    const selectedContent = selected.map((chapter) => chapter.content).join('\n\n')
    pendingAdaptUpload.value = null
    closeModal()
    await runUploadStream('adaptation', {
      title: pending.title,
      fileName: pending.fileName,
      content: selectedContent || pending.content,
      fullContent: pending.content,
      scriptType: pending.scriptType,
      chapterStart: chapterRange.start,
      chapterEnd: chapterRange.end,
      adaptMode: adaptMode.value || pending.adaptMode || 'full',
      cost: chapterRangeCost.value
    })
  } catch (err) {
    toast(`拆解失败：${cleanError(err)}`)
  } finally {
    busy.value = false
    uploadBusy.value = false
  }
}

async function runUploadStream(purpose, payload) {
  let opened = false
  let streamError = ''
  openEditor({
    id: 0,
    title: payload.title || (purpose === 'adaptation' ? '网文改编项目' : '剧本改写项目'),
    type: payload.scriptType || 'AI短剧',
    source: purpose,
    status: 'parsing',
    settings: {
      importStatus: 'parsing',
      importProgress: '拆解中',
      sourceFile: payload.fileName || '',
      sourceLength: String(payload.content || '').replace(/\s/g, '').length
    },
    characters: [],
    outlines: [],
    episodes: [{ no: 1, outline: '', body: '' }]
  })
  busy.value = false
  uploadBusy.value = false
  await streamApi(`/api/upload/stream?purpose=${purpose}`, payload, (event, data) => {
    if (event === 'error') {
      streamError = data?.message || '拆解失败'
      patchCurrentImportFailed(streamError, data)
      return
    }
    if (event === 'project') {
      if (data.wallet) syncWallet(data.wallet)
      const project = markProjectProgress(data.project, '拆解中')
      upsertScript(project)
      openEditor(project)
      opened = true
      busy.value = false
      uploadBusy.value = false
      toast('拆解中')
      return
    }
    if (event === 'progress') {
      patchCurrentImportProgress('拆解中', data)
      return
    }
    if (event === 'patch') {
      const project = markProjectProgress(data.project, '拆解中')
      currentProject.value = cloneData(project)
      upsertScript(project)
      return
    }
    if (event === 'chapter') {
      appendCurrentChapter(data.chapter, data.index, data.total)
      patchCurrentImportProgress('拆解中', data)
      return
    }
    if (event === 'scene') {
      appendCurrentScene(data.scene, data.index, data.total)
      patchCurrentImportProgress('拆解中', data)
      return
    }
    if (event === 'warning') {
      patchCurrentImportProgress(data.message || '本地拆解已完成，AI增强失败', data)
      toast(data.message || 'AI增强失败，已保留本地拆解结果')
      return
    }
    if (event === 'done') {
      const project = markProjectProgress(data.project, data.message || '拆解完成', true)
      currentProject.value = cloneData(project)
      upsertScript(project)
      toast(data.message || '拆解完成')
    }
  })
  if (streamError) throw new Error(streamError)
  if (!opened) throw new Error('未收到项目创建结果')
}

function markProjectProgress(project, message, done = false) {
  const next = cloneData(project || {})
  next.status = done ? 'parsed' : (next.status || 'parsing')
  next.settings = { ...(next.settings || {}), importStatus: done ? 'parsed' : 'parsing', importProgress: message }
  return next
}

function patchCurrentImportProgress(message, extra = {}) {
  if (!currentProject.value) return
  currentProject.value = {
    ...currentProject.value,
    status: 'parsing',
    settings: {
      ...(currentProject.value.settings || {}),
      importStatus: 'parsing',
      importProgress: message,
      streamProgress: extra
    }
  }
  upsertScript(currentProject.value)
}

function patchCurrentImportFailed(message, extra = {}) {
  if (!currentProject.value) return
  currentProject.value = {
    ...currentProject.value,
    status: 'failed',
    settings: {
      ...(currentProject.value.settings || {}),
      importStatus: 'failed',
      importProgress: message || '拆解失败',
      streamProgress: extra
    }
  }
  upsertScript(currentProject.value)
}

function appendCurrentChapter(chapter, index, total) {
  if (!currentProject.value || !chapter) return
  const settings = { ...(currentProject.value.settings || {}) }
  const outlines = Array.isArray(settings.novelChapterOutline) ? [...settings.novelChapterOutline] : []
  const key = String(chapter.column || index)
  const existing = outlines.findIndex((item) => String(item.column) === key)
  if (existing >= 0) outlines[existing] = chapter
  else outlines.push(chapter)
  settings.novelChapterOutline = outlines
  settings.chapterCount = total || settings.chapterCount || outlines.length
  currentProject.value = { ...currentProject.value, settings }
  upsertScript(currentProject.value)
}

function appendCurrentScene(scene, index, total) {
  if (!currentProject.value || !scene) return
  const settings = { ...(currentProject.value.settings || {}) }
  const scenes = Array.isArray(settings.sceneBreakdown) ? [...settings.sceneBreakdown] : []
  if (!scenes[index - 1]) scenes[index - 1] = scene
  settings.sceneBreakdown = scenes.filter(Boolean)
  settings.sceneCount = total || settings.sceneBreakdown.length
  currentProject.value = { ...currentProject.value, settings }
  upsertScript(currentProject.value)
}

function clampNumber(value, min, max) {
  const parsed = Number.parseInt(value, 10)
  if (Number.isNaN(parsed)) return min
  return Math.min(max, Math.max(min, parsed))
}

function importStatusLabel(status) {
  return ({ queued: '等待中', checking: '校验中', parsing: '拆解中', done: '已完成', failed: '导入失败' })[status] || status
}

function stepIcon(state) {
  if (state === 'done') return CheckCircle2
  if (state === 'failed') return AlertTriangle
  if (state === 'active') return LoaderCircle
  return Circle
}

function sourceProjectSummary(project) {
  const settings = project.settings || {}
  const file = settings.sourceFile || '上传文件'
  const length = settings.sourceLength ? `${settings.sourceLength}字` : '待统计'
  const range = settings.chapterRange ? ` · ${settings.chapterRange}` : ''
  return `${file} · ${length}${range}`
}

function compactText(value, limit) {
  const text = String(value || '').replace(/\s+/g, ' ').trim()
  if (text.length <= limit) return text
  return `${text.slice(0, limit)}...`
}

function fileTitle(event) {
  const file = event.target.files?.[0]
  return file?.name?.replace(/\.[^.]+$/, '') || ''
}

async function evaluationFileChanged(event) {
  const file = event.target.files?.[0]
  evaluationForm.title = fileTitle(event) || evaluationForm.title
  evaluationForm.fileName = file?.name || ''
  evaluationForm.content = await readTextFile(file)
  event.target.value = ''
}

function toggleDimension(key) {
  const dims = evaluationForm.dimensions
  const idx = dims.indexOf(key)
  if (idx >= 0) {
    if (dims.length > 1) dims.splice(idx, 1)
  } else {
    dims.push(key)
  }
}

const evalStreaming = ref(false)
const evalStreamText = ref('')

async function submitEvaluation() {
  if (busy.value || evalStreaming.value) return
  evalStreaming.value = true
  evalStreamText.value = ''
  busy.value = true
  try {
    await streamApi('/api/evaluations/stream', { ...evaluationForm }, (event, data) => {
      if (event === 'delta' && data?.text) {
        evalStreamText.value += data.text
      } else if (event === 'done' && data?.evaluation) {
        evaluations.value.unshift(data.evaluation)
        toast('评估完成，已生成报告')
      } else if (event === 'error') {
        toast(`评估失败：${data?.message || '未知错误'}`)
      }
    })
  } catch (err) {
    toast(`评估失败：${cleanError(err)}`)
  } finally {
    busy.value = false
    evalStreaming.value = false
  }
}

async function claimWalletTask() {
  try {
    const data = await api('/api/wallet/claim', { method: 'POST', body: '{}' })
    syncWallet(data)
    toast('已领取500剧点')
  } catch (err) {
    toast(`领取失败：${cleanError(err)}`)
  }
}

async function recharge() {
  try {
    const data = await api('/api/recharge', { method: 'POST', body: JSON.stringify({ code: selectedPack.value }) })
    syncWallet(data.wallet)
    closeModal()
    toast(data.order?.id ? `订单${data.order.id}已支付到账` : '充值已到账')
  } catch (err) {
    toast(`充值失败：${cleanError(err)}`)
  }
}

async function renewMembership() {
  try {
    const data = await api('/api/membership/renew', { method: 'POST', body: JSON.stringify({ plan: renewPlan.value }) })
    Object.assign(profile, data.profile || data)
    closeModal()
    toast(data.order?.id ? `会员订单${data.order.id}已完成` : '会员续费已完成')
  } catch (err) {
    toast(`续费失败：${cleanError(err)}`)
  }
}

async function saveProfile() {
  try {
    const data = await api('/api/settings/profile', { method: 'POST', body: JSON.stringify(profileForm) })
    Object.assign(profile, data)
    toast('资料已保存')
  } catch (err) {
    toast(`保存失败：${cleanError(err)}`)
  }
}

async function saveAIConfig() {
  if (!aiConfigForm.provider || !aiConfigForm.model.trim()) {
    toast('请选择服务商和模型')
    return
  }
  if (!selectedProviderConfigured.value && !aiConfigForm.apiKey.trim()) {
    toast(`请填写 ${selectedAIProvider.value.label} API Key`)
    return
  }
  busy.value = true
  try {
    const data = await api('/api/settings/ai-config', { method: 'POST', body: JSON.stringify(aiConfigForm) })
    syncAIConfig(data)
    toast('API 配置已保存')
  } catch (err) {
    toast(`API 配置保存失败：${cleanError(err)}`)
  } finally {
    busy.value = false
  }
}

async function savePassword() {
  if (!passwordForm.password || passwordForm.password !== passwordForm.confirm) {
    toast('请确认两次密码一致')
    return
  }
  try {
    await api('/api/settings/password', { method: 'POST', body: JSON.stringify(passwordForm) })
    closeModal()
    toast('密码已修改')
  } catch (err) {
    toast(`修改失败：${cleanError(err)}`)
  }
}

function upsertScript(project) {
  const index = scripts.value.findIndex((item) => item.id === project.id)
  if (index >= 0) scripts.value[index] = project
  else scripts.value.unshift(project)
}

function toast(text) {
  toastText.value = text
  window.setTimeout(() => {
    toastText.value = ''
  }, 2400)
}

function cleanError(err) {
  return String(err?.message || err).replace(/\s+/g, ' ').trim()
}

function cleanAiError(err) {
  const text = cleanError(err)
  if (/剧本工坊 API|大模型|model|stream|流式|connection|timeout|fetch/i.test(text)) {
    return '生成暂时不可用，请稍后重试'
  }
  return text || '生成暂时不可用，请稍后重试'
}

function cloneData(value) {
  return JSON.parse(JSON.stringify(value))
}

async function readTextFile(file) {
  if (!file) return ''
  if (file.size > 10 * 1024 * 1024) {
    throw new Error('文件不能超过10M')
  }
  const lower = file.name.toLowerCase()
  if (!/\.(txt|doc|docx|pdf)$/.test(lower)) {
    throw new Error('仅支持 txt、doc、docx、pdf 文件')
  }
  if (lower.endsWith('.txt') || lower.endsWith('.doc')) {
    return file.text()
  }
  return `${file.name} 已上传，服务端将解析该文件内容。`
}

function downloadTextFile(fileName, content, mime = 'text/plain;charset=utf-8') {
  const blob = new Blob([content], { type: mime || 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = fileName || '剧本导出.doc'
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

const EditorView = defineComponent({
  props: {
    project: { type: Object, required: true },
    busy: { type: Boolean, default: false }
  },
  emits: ['back', 'save', 'ai', 'export', 'toast'],
  setup(props, { emit }) {
    const local = ref(cloneData(props.project))
    const editorTab = ref(initialEditorTab(props.project))
    const inspirationOpen = ref(false)
    const rankingOpen = ref(false)
    const guide = ref('')
    const ideaPrompt = ref('')
    const audienceOpen = ref(false)
    const optionOpen = ref('')
    const settingsOptionOpen = ref('')
    const selectedAudience = ref('')
    const selectedGenres = ref([])
    const selectedCore = ref([])
    const selectedStyle = ref([])
    const selectedEra = ref([])
    const planningOptionGroup = ref({})
    const planningGenerating = ref(false)
    const planningProgress = ref('')
    const planningPlans = ref([])
    const planningStreamPlans = ref([])
    const planningListRef = ref(null)
    const aiStreamText = ref('')
    const planSavePending = ref(false)
    const planningWriteTimers = []
    const activeAi = ref('')
    const outlineRangeOpen = ref(false)
    const selectedEpisodeCount = ref(40)
    const selectedOutlinePhase = ref(0)
    const outlineCollapsed = ref(false)
    const expandedBodyEpisode = ref(null)
    const selectedBodyEpisode = ref(1)
    const bodyRangeOpen = ref(false)
    const bodyRangeStart = ref(1)
    const bodyRangeEnd = ref(1)
    const guideSeen = ref(window.localStorage?.getItem('jubengongfang:outlineGuideSeen') === '1')
    const characterTagDraft = ref('')
    const novelOutlineRefs = ref([])
    const historyStack = ref([])
    const futureStack = ref([])
    const autosaveTimer = ref(null)
    const applyingRemote = ref(false)

    watch(() => props.project, (value) => {
      const previousId = local.value?.id
      applyingRemote.value = true
      local.value = cloneData(value)
      if (previousId !== value?.id) {
        editorTab.value = initialEditorTab(value)
      }
      window.setTimeout(() => {
        applyingRemote.value = false
      }, 0)
    }, { deep: true })

    watch(local, () => {
      if (applyingRemote.value) return
      if (autosaveTimer.value) window.clearTimeout(autosaveTimer.value)
      autosaveTimer.value = window.setTimeout(() => {
        persistLocal(true)
      }, 1200)
    }, { deep: true })

    onUnmounted(() => {
      if (autosaveTimer.value) window.clearTimeout(autosaveTimer.value)
      clearPlanningWriteTimers()
    })

    const tabs = computed(() => {
      const common = [
        ['settings', '故事梗概'],
        ['characters', local.value.source === 'original' ? 'AI生成人物' : '人物小传'],
        ['outline', '分集大纲'],
        ['body', '剧本正文']
      ]
      if (local.value.source === 'rewriting') return [['infoflow', '信息流'], ...common]
      if (local.value.source === 'adaptation') {
        const next = [['novelOutline', '小说章纲'], ['settings', '故事梗概'], ['body', '剧本正文']]
        if (local.value.settings?.adaptationMode !== 'original') {
          next.splice(2, 0, ['characters', '人物小传'], ['outline', '分集大纲'])
        }
        return next
      }
      return common
    })
    const importProgress = computed(() => {
      const settings = local.value.settings || {}
      if (settings.importStatus !== 'parsing' && local.value.status !== 'parsing') return ''
      return settings.importProgress || '拆解中'
    })
    const anchors = computed(() => local.value.source === 'rewriting'
      ? ['目标受众', '时代背景', '题材类型', '核心设定', '故事背景', '核心亮点', '核心梗概']
      : ['目标受众', '时代背景', '题材类型', '核心设定', '世界观', '核心亮点', '核心梗概'])
    const eraOptions = [
      ['现代现实', ['现代都市', '当代家庭', '职场商战', '校园青春', '乡村振兴', '直播短视频']],
      ['年代地域', ['民国风云', '九零年代', '八零年代', '港风年代', '边境小城', '海岛小镇']],
      ['古装架空', ['古代权谋', '宫廷宅斗', '江湖武侠', '仙侠大陆', '架空王朝', '女尊世界']],
      ['未来幻想', ['近未来', '末世废土', '星际文明', '赛博都市', '时空循环', '平行世界']]
    ]
    const genreOptions = [
      ['热门题材', ['都市日常', '家庭伦理', '豪门甜宠', '悬疑反转', '逆袭爽剧', '先婚后爱']],
      ['奇幻设定', ['玄幻奇遇', '仙侠奇缘', '都市异能', '末世重生', '穿书系统', '时空循环']],
      ['情感关系', ['婚恋拉扯', '亲情救赎', '萌宝助攻', '女性成长', '职场情感', '青春校园']],
      ['强情节', ['复仇虐渣', '真假千金', '豪门争产', '刑侦悬疑', '商战博弈', '医术逆袭']]
    ]
    const coreOptions = [
      ['身份反转', ['隐藏身份', '马甲流', '真假继承人', '替身上位', '落魄归来', '豪门认亲']],
      ['能力成长', ['传承觉醒', '异能觉醒', '医术封神', '商业逆袭', '系统任务', '重生预知']],
      ['关系冲突', ['亲情反转', '误会追妻', '契约婚姻', '双强对抗', '家族压迫', '背叛反击']],
      ['爽点机制', ['打脸虐渣', '证据翻盘', '全员后悔', '大佬撑腰', '公开处刑', '极限反杀']]
    ]
    const styleOptions = [
      ['节奏', ['强爽点', '快节奏', '连续钩子', '高密反转', '开局暴击', '短平快']],
      ['情绪', ['情绪拉满', '温情收束', '极致委屈', '高甜治愈', '虐恋拉扯', '群像热血']],
      ['表达', ['轻喜沙雕', '悬疑压迫', '甜宠恋爱', '现实质感', '古早狗血', '隐藏大佬']]
    ]
    const characterRoleOptions = [
      ['主角', ['女主', '男主', '双女主', '双男主', '群像主角']],
      ['亲密关系', ['恋人', '前任', '夫妻', '青梅竹马', '契约对象']],
      ['家庭关系', ['父亲', '母亲', '兄弟姐妹', '子女', '养父母']],
      ['剧情功能', ['反派', '助攻', '导师', '竞争者', '线索人物']]
    ]
    const rankingItems = []
    function field(key, fallback = '') {
      return local.value.settings?.[key] ?? fallback
    }

    function setField(key, value) {
      recordHistory()
      local.value.settings = { ...(local.value.settings || {}), [key]: value }
    }

    function setLimitedListField(key, value, limit) {
      setField(key, splitList(value).slice(0, limit))
    }

    function toggleSettingChoice(key, item, limit) {
      const current = splitList(field(key, []))
      const next = current.includes(item)
        ? current.filter((value) => value !== item)
        : [...current, item].slice(0, limit)
      setField(key, next)
    }

    function optionGroups(options) {
      return Array.isArray(options?.[0]) ? options : [['常用', options || []]]
    }

    function fieldDone(key) {
      const value = field(key, key === 'audience' ? '' : [])
      return Array.isArray(value) ? value.length > 0 : Boolean(String(value || '').trim())
    }

    function anchorDone(item) {
      return ({
        目标受众: fieldDone('audience'),
        时代背景: fieldDone('era'),
        题材类型: fieldDone('genres'),
        核心设定: fieldDone('core'),
        故事背景: fieldDone('worldView'),
        世界观: fieldDone('worldView'),
        核心亮点: fieldDone('highlights'),
        核心梗概: fieldDone('synopsis')
      })[item]
    }

    function settingSelect(key, label, hint, placeholder, options, limit, required = true) {
      const selected = splitList(field(key, []))
      const open = settingsOptionOpen.value === key || String(settingsOptionOpen.value).startsWith(`${key}:`)
      return h('label', { class: ['select-field', { open }] }, [
        h('b', [label, required ? h('span', { class: 'req' }, '*') : null, hint ? h('small', hint) : null]),
        h('div', { class: 'select-input-wrap' }, [
          h('input', {
            disabled: props.busy || planningGenerating.value,
            value: stringifyList(selected),
            placeholder,
            onFocus: () => { settingsOptionOpen.value = `${key}:${optionGroups(options)[0][0]}` },
            onInput: (event) => setLimitedListField(key, event.target.value, limit)
          }),
          h('button', {
            type: 'button',
            class: 'select-toggle',
            disabled: props.busy || planningGenerating.value,
            'aria-label': `${label}选项`,
            onClick: () => { settingsOptionOpen.value = open ? '' : `${key}:${optionGroups(options)[0][0]}` }
          })
        ]),
        open && h('div', { class: 'setting-options grouped-options' }, [
          h('div', { class: 'option-cats' }, optionGroups(options).map(([group]) => h('button', {
            type: 'button',
            class: { active: settingsOptionOpen.value === `${key}:${group}` },
            onMousedown: (event) => event.preventDefault(),
            onClick: () => { settingsOptionOpen.value = `${key}:${group}` }
          }, group))),
          h('div', { class: 'option-values' }, (optionGroups(options).find(([group]) => settingsOptionOpen.value === `${key}:${group}`) || optionGroups(options)[0])[1].map((item) => h('button', {
            type: 'button',
            class: { selected: selected.includes(item) },
            disabled: props.busy || planningGenerating.value,
            onMousedown: (event) => event.preventDefault(),
            onClick: () => toggleSettingChoice(key, item, limit)
          }, item)))
        ])
      ].filter(Boolean))
    }

    function setTitle(value) {
      recordHistory()
      local.value.title = value
    }

    function persistLocal(silent = true, onDone) {
      emit('save', { project: cloneData(local.value), silent, onDone })
    }

    function recordHistory() {
      const snapshot = JSON.stringify(local.value)
      if (historyStack.value[historyStack.value.length - 1] === snapshot) return
      historyStack.value.push(snapshot)
      if (historyStack.value.length > 40) historyStack.value.shift()
      futureStack.value = []
    }

    function undo() {
      const previous = historyStack.value.pop()
      if (!previous) return
      futureStack.value.push(JSON.stringify(local.value))
      local.value = JSON.parse(previous)
      persistLocal(true)
    }

    function redo() {
      const next = futureStack.value.pop()
      if (!next) return
      historyStack.value.push(JSON.stringify(local.value))
      local.value = JSON.parse(next)
      persistLocal(true)
    }

    function switchTab(key) {
      persistLocal(true)
      editorTab.value = key
      if (key === 'outline' && !guideSeen.value && !(local.value.outlines || []).length) {
        guideSeen.value = true
        window.localStorage?.setItem('jubengongfang:outlineGuideSeen', '1')
        guide.value = 'coarse'
      } else {
        guide.value = ''
      }
    }

    function isSourceTab(key) {
      return (local.value.source === 'rewriting' && key === 'infoflow') || (local.value.source === 'adaptation' && key === 'novelOutline')
    }

    function leaveEditor() {
      persistLocal(true)
      emit('back')
    }

    function exportCurrent() {
      persistLocal(true)
      emit('export', cloneData(local.value))
    }

    function addCharacter() {
      recordHistory()
      characterTagDraft.value = ''
      local.value.characters = [{
        name: '新角色',
        role: '定位',
        age: '',
        meta: '年龄 / 身份',
        traits: ['待补充'],
        background: '填写人物背景、过往经历和身份反转。',
        goal: '填写人物核心目标。',
        relation: '填写与主角和其他角色的关系。',
        bio: '填写人物目标、缺陷、关系和反转节点。'
      }, ...(local.value.characters || [])]
    }

    function ensureEpisode() {
      if (!local.value.episodes?.length) local.value.episodes = [{ no: 1, outline: '', body: '' }]
    }

    function bodyOnlyEpisodeNos() {
      return new Set(splitList(local.value.settings?.bodyOnlyEpisodes || []).map((item) => Number(item)).filter(Boolean))
    }

    function markBodyOnlyEpisode(no) {
      const target = Number(no) || 0
      if (!target) return
      const next = new Set(bodyOnlyEpisodeNos())
      next.add(target)
      local.value.settings = { ...(local.value.settings || {}), bodyOnlyEpisodes: [...next].sort((a, b) => a - b) }
    }

    function unmarkBodyOnlyEpisode(no) {
      const target = Number(no) || 0
      if (!target) return
      const next = [...bodyOnlyEpisodeNos()].filter((item) => item !== target)
      local.value.settings = { ...(local.value.settings || {}), bodyOnlyEpisodes: next }
    }

    function bodyCardNos() {
      return new Set(splitList(local.value.settings?.bodyCardEpisodes || []).map((item) => Number(item)).filter(Boolean))
    }

    function markBodyCardEpisode(no) {
      const target = Number(no) || 0
      if (!target) return
      const next = new Set(bodyCardNos())
      next.add(target)
      local.value.settings = { ...(local.value.settings || {}), bodyCardEpisodes: [...next].sort((a, b) => a - b) }
    }

    function unmarkBodyCardEpisode(no) {
      const target = Number(no) || 0
      if (!target) return
      const next = [...bodyCardNos()].filter((item) => item !== target)
      local.value.settings = { ...(local.value.settings || {}), bodyCardEpisodes: next }
    }

    function seedEmptyEpisodeOutlines(total) {
      const count = Math.max(1, Number(total) || 1)
      local.value.episodes = Array.from({ length: count }, (_, index) => {
        const no = index + 1
        const existing = (local.value.episodes || []).find((item) => Number(item.no) === no)
        return { no, outline: '', body: existing?.body || '' }
      })
    }

    function addEpisode(anchorEpisode = null) {
      recordHistory()
      ensureEpisode()
      const episodes = local.value.episodes || []
      const existingNos = new Set(episodes.map((item) => Number(item.no) || 0))
      const anchorNo = Number(anchorEpisode?.no) || Number(selectedBodyEpisode.value) || Math.max(1, ...existingNos)
      // 加号只按当前正文卡片的集数向后补一个空集，不改已有分集大纲内容。
      const newNo = anchorNo + 1
      if (!existingNos.has(newNo)) episodes.push({ no: newNo, outline: '', body: '' })
      episodes.sort((a, b) => Number(a.no || 0) - Number(b.no || 0))
      markBodyCardEpisode(newNo)
      const nextEpisode = episodes.find((item) => Number(item.no) === newNo)
      if (!String(nextEpisode?.outline || '').trim()) markBodyOnlyEpisode(newNo)
      selectedBodyEpisode.value = newNo
      bodyRangeStart.value = newNo
      bodyRangeEnd.value = newNo
      expandedBodyEpisode.value = null
      nextTick(() => scrollStreamingBodyToBottom([newNo]))
    }

    function createFirstBodyEpisode() {
      recordHistory()
      ensureEpisode()
      let episode = (local.value.episodes || []).find((item) => Number(item.no) === 1)
      if (!episode) {
        episode = { no: 1, outline: '', body: '' }
        local.value.episodes.push(episode)
        local.value.episodes.sort((a, b) => Number(a.no || 0) - Number(b.no || 0))
        markBodyOnlyEpisode(1)
      } else if (!String(episode.outline || '').trim()) {
        markBodyOnlyEpisode(1)
      }
      markBodyCardEpisode(1)
      selectedBodyEpisode.value = 1
      bodyRangeStart.value = 1
      bodyRangeEnd.value = 1
      expandedBodyEpisode.value = null
      nextTick(() => scrollStreamingBodyToBottom([1]))
    }

    function setCharacterField(index, key, value) {
      if (!local.value.characters?.[index]) return
      recordHistory()
      local.value.characters[index][key] = value
    }

    function selectCharacter(index) {
      if (!local.value.characters?.[index] || index === 0) return
      recordHistory()
      characterTagDraft.value = ''
      const next = [...local.value.characters]
      const picked = next.splice(index, 1)[0]
      local.value.characters = [picked, ...next]
    }

    function deleteCharacter(index) {
      if (activeAi.value === 'characters' || !local.value.characters?.[index]) return
      recordHistory()
      characterTagDraft.value = ''
      local.value.characters = local.value.characters.filter((_, itemIndex) => itemIndex !== index)
    }

    function activeCharacterIndex() {
      if (activeAi.value === 'characters') {
        return Math.max(0, (local.value.characters || []).length - 1)
      }
      return 0
    }

    function activeCharacter() {
      return (local.value.characters || [])[activeCharacterIndex()] || {}
    }

    function addCharacterTrait(value) {
      const trait = String(value || '').trim()
      if (!trait || !local.value.characters?.[0]) return
      const traits = local.value.characters[0].traits || []
      if (traits.includes(trait)) return
      recordHistory()
      local.value.characters[0].traits = [...traits, trait].slice(0, 8)
      characterTagDraft.value = ''
    }

    function removeCharacterTrait(trait) {
      if (!local.value.characters?.[0]) return
      recordHistory()
      local.value.characters[0].traits = (local.value.characters[0].traits || []).filter((item) => item !== trait)
    }

    function setEpisodeOutline(index, value) {
      ensureEpisode()
      if (!local.value.episodes[index]) return
      recordHistory()
      local.value.episodes[index].outline = value
    }

    function setEpisodeOutlineByNo(no, value) {
      ensureEpisode()
      recordHistory()
      const targetNo = Number(no) || 1
      const index = local.value.episodes.findIndex((item) => Number(item.no) === targetNo)
      if (index >= 0) {
        local.value.episodes[index].outline = value
        if (String(value || '').trim()) unmarkBodyOnlyEpisode(targetNo)
        return
      }
      if (String(value || '').trim()) unmarkBodyOnlyEpisode(targetNo)
      local.value.episodes = [...local.value.episodes, { no: targetNo, outline: value, body: '' }]
        .sort((a, b) => Number(a.no || 0) - Number(b.no || 0))
    }

    function setEpisodeBody(index, value) {
      ensureEpisode()
      if (!local.value.episodes[index]) return
      recordHistory()
      local.value.episodes[index].body = value
    }

    function setEpisodeBodyByNo(no, value) {
      ensureEpisode()
      recordHistory()
      const targetNo = Number(no) || 1
      markBodyCardEpisode(targetNo)
      const index = local.value.episodes.findIndex((item) => Number(item.no) === targetNo)
      if (index >= 0) {
        local.value.episodes[index].body = value
        return
      }
      local.value.episodes = [...local.value.episodes, { no: targetNo, outline: '', body: value }]
        .sort((a, b) => Number(a.no || 0) - Number(b.no || 0))
    }

    function deleteEpisodeByNo(no) {
      ensureEpisode()
      if (local.value.episodes.length <= 1) return
      recordHistory()
      local.value.episodes = local.value.episodes.filter((item) => Number(item.no) !== Number(no))
      unmarkBodyCardEpisode(no)
      unmarkBodyOnlyEpisode(no)
      if (expandedBodyEpisode.value === no) expandedBodyEpisode.value = null
      if (selectedBodyEpisode.value === no) selectedBodyEpisode.value = local.value.episodes[0]?.no || 1
    }

    function deleteBodyEpisode(no) {
      ensureEpisode()
      const targetNo = Number(no) || 1
      const episode = (local.value.episodes || []).find((item) => Number(item.no) === targetNo)
      if (!episode) return
      const hasOutline = String(episode.outline || '').trim()
      if (!hasOutline && (local.value.episodes || []).length > 1) {
        deleteEpisodeByNo(targetNo)
        return
      }
      recordHistory()
      episode.body = ''
      unmarkBodyCardEpisode(targetNo)
      if (!hasOutline) markBodyOnlyEpisode(targetNo)
      if (expandedBodyEpisode.value === targetNo) expandedBodyEpisode.value = null
      if (selectedBodyEpisode.value === targetNo) {
        const nextVisible = visibleBodyEpisodes().find((item) => Number(item.no) !== targetNo)
        selectedBodyEpisode.value = Number(nextVisible?.no) || 1
      }
    }

    function infoflow() {
      const settings = local.value.settings || {}
      return settings.infoflow || {
        storyOverview: {
          audience: settings.audience || '',
          era: settings.era || settings.worldView || '',
          genre: stringifyList(settings.genres),
          core: stringifyList(settings.core),
          background: settings.worldView || '',
          highlights: settings.highlights || '',
          synopsis: settings.synopsis || ''
        },
        characters: local.value.characters || [],
        roughOutline: local.value.outlines || [],
        episodeSynopsis: local.value.episodes || []
      }
    }

    function filledInfoflowOutlines() {
      return (infoflow().roughOutline || []).filter((item) => String(item?.content || item?.summary || '').trim())
    }

    function filledInfoflowEpisodes() {
      return (infoflow().episodeSynopsis || []).filter((item) => String(item?.outline || item?.summary || '').trim())
    }

    function novelOutlines() {
      const settings = local.value.settings || {}
      const fromAI = Array.isArray(settings.novelChapterOutline) ? settings.novelChapterOutline : []
      if (fromAI.length) return fromAI
      const breakdown = Array.isArray(settings.chapterBreakdown) ? settings.chapterBreakdown : []
      return breakdown.map((item) => ({
        column: String(item.column || item.chapter || ''),
        title: item.title || '',
        synopsis: item.synopsis || item.summary || '',
        summary: item.summary || ''
      }))
    }

    function setNovelOutlineField(index, key, value) {
      recordHistory()
      const items = novelOutlines().map((item) => ({ ...item }))
      if (!items[index]) return
      items[index][key] = value
      local.value.settings = { ...(local.value.settings || {}), novelChapterOutline: items }
    }

    function setNovelOutlineRef(index, el) {
      if (el) novelOutlineRefs.value[index] = el
    }

    function scrollToNovelChapter(index) {
      const el = novelOutlineRefs.value[index]
      if (el?.scrollIntoView) {
        el.scrollIntoView({ behavior: 'smooth', block: 'start' })
      }
    }

    function novelRulerTicks() {
      const count = Math.max(1, novelOutlines().length)
      const max = Math.max(10, Math.ceil(count / 10) * 10)
      return Array.from({ length: max }, (_, index) => {
        const no = index + 1
        return {
          no,
          label: no === 1 || no % 10 === 0 ? String(no) : '',
          targetIndex: Math.min(no - 1, count - 1),
          major: no === 1 || no % 10 === 0,
          mid: no % 5 === 0 && no % 10 !== 0,
          ghost: no > count
        }
      })
    }

    function bodyRulerTicks() {
      const maxEpisode = Math.max(1, ...(local.value.episodes || []).map((item) => Number(item.no) || 0))
      const max = Math.max(10, Math.ceil(Math.max(totalEpisodeCount(), maxEpisode) / 10) * 10)
      return Array.from({ length: Math.min(max, 100) }, (_, index) => {
        const no = index + 1
        return {
          no,
          label: no === 1 || no % 10 === 0 ? String(no) : '',
          major: no === 1 || no % 10 === 0,
          mid: no % 5 === 0 && no % 10 !== 0,
          ghost: no > maxEpisode
        }
      })
    }

    function storyOverview() {
      return infoflow().storyOverview || {}
    }

    function storySynopsis() {
      return storyOverview().synopsis || field('synopsis') || ''
    }

    function hasSynopsisContent() {
      return Boolean(String(storySynopsis() || '').trim())
    }

    function hasCharactersContent() {
      return (local.value.characters || []).some((item) => (
        String(item?.name || '').trim() ||
        String(item?.bio || '').trim() ||
        String(item?.background || '').trim() ||
        String(item?.goal || '').trim()
      ))
    }

    function hasEpisodeOutlineContent() {
      return (local.value.episodes || []).some((item) => String(item?.outline || '').trim())
    }

    function requireStep(ok, message, tab) {
      if (ok) return true
      emit('toast', message)
      if (tab) editorTab.value = tab
      return false
    }

    function canGenerateCharacters() {
      return requireStep(hasSynopsisContent(), '请先生成或填写故事概梗，再生成人物小传', 'settings')
    }

    function canGenerateOutline() {
      if (!requireStep(hasSynopsisContent(), '请先生成或填写故事概梗，再生成分集大纲', 'settings')) return false
      return requireStep(hasCharactersContent(), '请先生成或填写人物小传，再生成分集大纲', 'characters')
    }

    function canGenerateEpisodeOutline() {
      return requireStep(hasPhaseOutlineContent(), '请先生成粗纲，再生成集纲', 'outline')
    }

    function canGenerateBody() {
      if (!requireStep(hasSynopsisContent(), '请先生成或填写故事概梗，再生成剧本正文', 'settings')) return false
      if (!requireStep(hasCharactersContent(), '请先生成或填写人物小传，再生成剧本正文', 'characters')) return false
      return requireStep(hasEpisodeOutlineContent(), '请先生成分集大纲，再生成剧本正文', 'outline')
    }

    function setStorySynopsis(value) {
      recordHistory()
      const settings = { ...(local.value.settings || {}) }
      settings.synopsis = value
      const flow = settings.infoflow || infoflow()
      const story = { ...(flow.storyOverview || {}), synopsis: value }
      settings.infoflow = { ...flow, storyOverview: story }
      local.value.settings = settings
    }

    function setPhaseOutlineContent(value) {
      recordHistory()
      const range = currentPhaseRange()
      const items = [...(local.value.outlines || [])]
      const index = items.findIndex((item) => String(item.phase || '').includes(range.phase))
      const next = {
        ...(index >= 0 ? items[index] : {}),
        phase: range.phase,
        range: `第${range.start}-${range.end}集`,
        content: value
      }
      if (index >= 0) items[index] = next
      else items[selectedOutlinePhase.value] = next
      local.value.outlines = items.filter(Boolean)
    }

    function generateCharacters() {
      if (!canGenerateCharacters()) return
      recordHistory()
      local.value.characters = []
      characterTagDraft.value = ''
      ai('characters', 50, '基于当前故事概梗、世界观设定和已有全部设定，为本项目生成8-12个主要人物的完整小传。每个人物必须有独特的性格标签、说话方式、行为模式和记忆点。第一个角色是主角，后续按剧情重要性排序。反派要有合理动机和逐级递增的压迫感，助攻角色要为主角的爽点时刻服务。所有角色构成完整的人物关系网。')
    }

    function openOutlineGeneration() {
      if (!canGenerateOutline()) return
      outlineRangeOpen.value = true
    }

    function generateCurrentEpisodeOutline() {
      if (!canGenerateEpisodeOutline()) return
      ai('episode', 60, `只生成或优化第${currentPhaseRange().start}-${currentPhaseRange().end}集集纲，必须严格贴合”${currentPhaseRange().phase}”阶段粗纲中的剧情规划。注意：”起、承、转、合”只代表当前阶段的大方向，不要在每一集里套写起承转合结构。每集集纲必须包含五要素：①本集核心事件（发生了什么）②人物目标（谁想要什么）③冲突推进（遇到什么阻力、如何对抗）④关键反转或爽点（预期被打破或情绪释放）⑤结尾追看钩子（为什么必须看下一集）。前后集之间要有因果衔接，不能割裂；每集的爽点要有变化和升级，不能千篇一律。`)
    }

    function ai(taskType, cost, prompt = '') {
      persistLocal(true)
      activeAi.value = taskType
      aiStreamText.value = ''
      emit('ai', {
        taskType,
        cost,
        prompt,
        project: local.value,
        onResult: (data) => {
          local.value = cloneData(data.project)
          if (taskType === 'outline') selectedOutlinePhase.value = 0
          activeAi.value = ''
          aiStreamText.value = ''
        },
        onError: () => {
          activeAi.value = ''
          aiStreamText.value = ''
        },
        onStream: ['characters', 'outline', 'episode', 'body'].includes(taskType) ? (frame) => {
          if (!frame) return
          if (taskType === 'characters' && Array.isArray(frame.characters)) {
            applyStreamingCharacters(frame.characters)
            if (frame.text) aiStreamText.value += frame.text
            return
          }
          if (taskType === 'outline' && Array.isArray(frame.outlines)) {
            applyStreamingOutlines(frame.outlines)
            return
          }
          if (taskType === 'episode') {
            if (Array.isArray(frame.episodes)) applyStreamingEpisodes(frame.episodes)
            if (frame.text) aiStreamText.value += frame.text
            return
          }
          if (taskType === 'body') {
            if (Array.isArray(frame.episodes)) applyStreamingBodies(frame.episodes)
            return
          }
          if (frame.text) {
            aiStreamText.value += frame.text
            return
          }
          const episodes = local.value.episodes || []
          if (frame.episodeNo != null) {
            const ep = episodes.find(e => Number(e.no) === Number(frame.episodeNo))
            if (ep && frame.body) ep.body = frame.body
          } else if (frame.body != null) {
            // If no specific episode number, append to the first episode in range
            const startNo = Number(selectedBodyEpisode.value) || 1
            const ep = episodes.find(e => Number(e.no) >= startNo)
            if (ep) ep.body = frame.body
          }
        } : undefined
      })
    }

    function normalizeStreamingCharacter(item, index) {
      return {
        name: item?.name || `角色${index + 1}`,
        role: item?.role || item?.meta || '',
        age: item?.age || '',
        meta: item?.meta || [item?.age, item?.role].filter(Boolean).join(' / '),
        traits: splitList(item?.traits || item?.meta || '').slice(0, 5),
        background: item?.background || item?.bio || '',
        goal: item?.goal || '',
        relation: item?.relation || '',
        bio: item?.bio || ''
      }
    }

    function applyStreamingCharacters(characters) {
      const next = characters.map((item, index) => normalizeStreamingCharacter(item, index))
      if (!next.length) return
      local.value.characters = next
    }

    function outlinePhaseIndex(phase) {
      const value = String(phase || '').trim()
      return ['起', '承', '转', '合'].findIndex((item) => value.includes(item))
    }

    function applyStreamingOutlines(outlines) {
      const phases = phaseRanges()
      const current = [...(local.value.outlines || [])]
      let lastIndex = -1
      outlines.forEach((item) => {
        const index = outlinePhaseIndex(item.phase)
        if (index < 0) return
        const range = phases[index] || {}
        current[index] = {
          ...(current[index] || {}),
          phase: ['起', '承', '转', '合'][index],
          range: item.range || `第${range.start || 1}-${range.end || range.start || 1}集`,
          content: String(item.content || '').replace(/^生成中$/, '')
        }
        lastIndex = index
      })
      local.value.outlines = current.filter(Boolean)
      if (lastIndex >= 0) {
        selectedOutlinePhase.value = lastIndex
        outlineCollapsed.value = false
      }
    }

    function applyStreamingEpisodes(episodes) {
      if (!Array.isArray(episodes) || !episodes.length) return
      const current = [...(local.value.episodes || [])]
      episodes.forEach((item) => {
        const no = Number(item?.no) || 0
        if (!no) return
        const index = current.findIndex((episode) => Number(episode.no) === no)
        const next = {
          ...(index >= 0 ? current[index] : { no, body: '' }),
          no,
          outline: String(item?.outline || '').replace(/^生成中$/, '')
        }
        if (index >= 0) current[index] = next
        else current.push(next)
      })
      local.value.episodes = current.sort((a, b) => Number(a.no || 0) - Number(b.no || 0))
    }

    function applyStreamingBodies(episodes) {
      if (!Array.isArray(episodes) || !episodes.length) return
      const current = [...(local.value.episodes || [])]
      const changedNos = []
      episodes.forEach((item) => {
        const no = Number(item?.no) || 0
        if (!no || item?.body == null) return
        const index = current.findIndex((episode) => Number(episode.no) === no)
        const next = {
          ...(index >= 0 ? current[index] : { no, outline: '' }),
          no,
          body: String(item.body || '')
        }
        if (index >= 0) current[index] = next
        else current.push(next)
        markBodyCardEpisode(no)
        changedNos.push(no)
      })
      local.value.episodes = current.sort((a, b) => Number(a.no || 0) - Number(b.no || 0))
      nextTick(() => scrollStreamingBodyToBottom(changedNos))
    }

    function scrollStreamingBodyToBottom(changedNos = []) {
      const numbers = changedNos.length ? changedNos : [Number(selectedBodyEpisode.value) || Number(bodyRangeStart.value) || 1]
      numbers.forEach((no) => {
        const paper = document.querySelector(`[data-body-episode="${no}"]`)
        const textarea = paper?.querySelector('textarea.body-textarea')
        if (!textarea) return
        textarea.scrollTop = textarea.scrollHeight
        paper.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
      })
    }

    function confirmOutlineGeneration() {
      if (!canGenerateOutline()) return
      outlineRangeOpen.value = false
      guide.value = ''
      recordHistory()
      local.value.settings = { ...(local.value.settings || {}), episodeCount: selectedEpisodeCount.value }
      seedEmptyEpisodeOutlines(selectedEpisodeCount.value)
      selectedOutlinePhase.value = 0
      outlineCollapsed.value = false
      ai('outline', 80, `预计创作 ${selectedEpisodeCount.value} 集。请只生成四个阶段粗纲（起、承、转、合），每个阶段内部规划对应的 x-x 集区间；不要生成任何单集集纲或 episodes，页面会先创建空集纲输入框，后续由用户按阶段点击生成本阶段集纲。每个阶段粗纲必须是对该区间多集剧情的详细概括（400-800字），要有：①具体事件名称和人物行动②连续因果链（前一阶段的结果是后一阶段的原因）③反派压力逐级递增④爽点层层递进⑤情绪场面和可拍的高光场景⑥阶段钩子（驱动观众追看下一阶段的悬念）。不要太短，不要空泛，不要写"主角成长、矛盾升级"等方向性空话。`)
    }

    function seedInspiration() {
      selectedAudience.value = field('audience') || selectedAudience.value
      selectedEra.value = splitList(field('era', [])).slice(0, 3)
      selectedGenres.value = splitList(field('genres', [])).slice(0, 3)
      selectedCore.value = splitList(field('core', [])).slice(0, 3)
      inspirationOpen.value = true
    }

    function hasPlanningInput() {
      return Boolean(
        ideaPrompt.value.trim() ||
        selectedAudience.value ||
        selectedEra.value.length ||
        selectedGenres.value.length ||
        selectedCore.value.length
      )
    }

    function togglePlanningOption(target, key, item, limit) {
      const exists = target.value.includes(item)
      const next = exists ? target.value.filter((value) => value !== item) : [...target.value, item].slice(0, limit)
      target.value = next
      setField(key, next)
    }

    function planningGroupedOptions(key, options, selected, limit) {
      const groups = optionGroups(options)
      const active = planningOptionGroup.value[key] || groups[0]?.[0]
      const current = groups.find(([group]) => group === active) || groups[0]
      return [
        h('div', { class: 'planning-pop-cats' }, groups.map(([group]) => h('button', {
          type: 'button',
          class: { active: active === group },
          disabled: planningGenerating.value,
          onClick: () => { planningOptionGroup.value = { ...planningOptionGroup.value, [key]: group } }
        }, group))),
        h('div', { class: 'planning-pop-values' }, (current?.[1] || []).map((item) => h('button', {
          type: 'button',
          class: { selected: selected.value.includes(item) },
          disabled: planningGenerating.value,
          onClick: () => togglePlanningOption(selected, key, item, limit)
        }, item)))
      ]
    }

    function planningPrompt() {
      return [
        selectedAudience.value && `目标受众：${selectedAudience.value}`,
        selectedEra.value.length && `时代背景：${selectedEra.value.join('、')}`,
        selectedGenres.value.length && `题材类型：${selectedGenres.value.join('、')}`,
        selectedCore.value.length && `核心设定：${selectedCore.value.join('、')}`,
        ideaPrompt.value.trim() && `补充想法：${ideaPrompt.value.trim()}`
      ].filter(Boolean).join('\n')
    }

    function planningButtonText() {
      if (local.value.source === 'rewriting') return '改写策划'
      if (local.value.source === 'adaptation') return '改编策划'
      return '灵感策划'
    }

    function optionRows() {
      const rows = [
        ['era', '时代背景', eraOptions, selectedEra, 3],
        ['genres', '题材类型', genreOptions, selectedGenres, 3],
        ['core', '核心设定', coreOptions, selectedCore, 3]
      ]
      return rows.flatMap(([key, label, options, selected, limit]) => [
        h('button', {
          class: ['wide-option', { selected: selected.value.length }],
          disabled: planningGenerating.value,
          onClick: () => { optionOpen.value = optionOpen.value === key ? '' : key }
        }, selected.value.length ? `${label}  ${selected.value.join('  ')}` : label),
        optionOpen.value === key && h('div', { class: 'option-pop option-pop-wrap' }, optionGroups(options).flatMap(([, items]) => items).map((item) => h('button', {
          class: { selected: selected.value.includes(item) },
          disabled: planningGenerating.value,
          onClick: () => togglePlanningOption(selected, key, item, limit)
        }, item)))
      ]).filter(Boolean)
    }

    function totalEpisodeCount() {
      const configured = Number(local.value.settings?.episodeCount || 0)
      const maxEpisode = Math.max(0, ...(local.value.episodes || []).map((item) => Number(item.no) || 0))
      return Math.max(1, configured || selectedEpisodeCount.value || maxEpisode || 1)
    }

    function parseEpisodeRange(value) {
      const match = String(value || '').match(/(\d+)\s*[-至到~—]\s*(\d+)/)
      if (!match) return null
      const start = Math.max(1, Number(match[1]) || 1)
      const end = Math.max(start, Number(match[2]) || start)
      return { start, end }
    }

    function phaseRanges() {
      const total = Math.max(4, totalEpisodeCount())
      const firstEnd = Math.max(1, Math.round(total / 6))
      const secondEnd = Math.max(firstEnd + 1, Math.round(total / 2))
      const thirdEnd = Math.max(secondEnd + 1, Math.round(total * 5 / 6))
      const defaults = [
        { phase: '起', start: 1, end: firstEnd },
        { phase: '承', start: firstEnd + 1, end: secondEnd },
        { phase: '转', start: secondEnd + 1, end: thirdEnd },
        { phase: '合', start: thirdEnd + 1, end: total }
      ]
      return defaults.map((item, index) => {
        const outline = (local.value.outlines || []).find((entry) => String(entry.phase || '').includes(item.phase)) || (local.value.outlines || [])[index]
        const parsed = parseEpisodeRange(outline?.range)
        return parsed ? { ...item, ...parsed } : item
      })
    }

    function currentPhaseRange() {
      return phaseRanges()[selectedOutlinePhase.value] || phaseRanges()[0]
    }

    function outlineForPhase(index = selectedOutlinePhase.value) {
      const phases = phaseRanges()
      const phase = phases[index] || phases[0]
      const items = local.value.outlines || []
      return items.find((item) => String(item.phase || '').includes(phase.phase)) || items[index] || {
        phase: phase.phase,
        range: `第${phase.start}-${phase.end}集`,
        content: ''
      }
    }

    function episodesForCurrentPhase() {
      ensureEpisode()
      const range = currentPhaseRange()
      const bodyOnly = bodyOnlyEpisodeNos()
      const inRange = (local.value.episodes || []).filter((item) => {
        const no = Number(item.no) || 1
        return no >= range.start && no <= range.end && !bodyOnly.has(no)
      })
      return inRange.length ? inRange : [{ no: range.start, outline: '', body: '' }]
    }

    function hasEpisodeBody(episode) {
      return Boolean(String(episode?.body || '').trim())
    }

    function isGeneratingBodyEpisode(episode) {
      if (activeAi.value !== 'body') return false
      const no = Number(episode?.no) || 1
      const start = Math.max(1, Number(bodyRangeStart.value) || Number(selectedBodyEpisode.value) || 1)
      const end = Math.max(start, Number(bodyRangeEnd.value) || start)
      return no >= start && no <= end
    }

    function visibleBodyEpisodes() {
      ensureEpisode()
      const cardNos = bodyCardNos()
      const visible = (local.value.episodes || [])
        .filter((episode) => {
          const no = Number(episode.no) || 1
          return hasEpisodeBody(episode) || cardNos.has(no) || isGeneratingBodyEpisode(episode)
        })
        .sort((a, b) => Number(a.no || 0) - Number(b.no || 0))
      if (expandedBodyEpisode.value) {
        const picked = visible.find((item) => Number(item.no) === Number(expandedBodyEpisode.value))
        return picked ? [picked] : []
      }
      return visible
    }

    function currentBodyEpisode() {
      ensureEpisode()
      const current = Math.max(1, Number(selectedBodyEpisode.value) || 1)
      let episode = (local.value.episodes || []).find((item) => Number(item.no) === current)
      if (!episode) {
        episode = (local.value.episodes || [])[0] || { no: 1, outline: '', body: '' }
      }
      return episode
    }

    function bodyEditorEpisodes() {
      return visibleBodyEpisodes()
    }

    function bodyGenerateRange() {
      const start = Math.max(1, Number(bodyRangeStart.value) || Number(selectedBodyEpisode.value) || 1)
      const end = Math.max(start, Number(bodyRangeEnd.value) || start)
      return start === end ? `第${start}集` : `第${start}-${end}集`
    }

    function nextBodyGenerationStart() {
      const bodyNos = (local.value.episodes || [])
        .filter((episode) => hasEpisodeBody(episode))
        .map((episode) => Number(episode.no) || 0)
        .filter(Boolean)
      if (bodyNos.length) return Math.min(500, Math.max(...bodyNos) + 1)
      const currentVisible = visibleBodyEpisodes().find((episode) => Number(episode.no) === Number(selectedBodyEpisode.value))
      return Math.max(1, Number(currentVisible?.no) || Number(selectedBodyEpisode.value) || 1)
    }

    function bodyRangeHint() {
      const bodyNos = (local.value.episodes || [])
        .filter((episode) => hasEpisodeBody(episode))
        .map((episode) => Number(episode.no) || 0)
        .filter(Boolean)
      if (!bodyNos.length) return '默认从当前正文卡片开始，可手动调整区间'
      const lastNo = Math.max(...bodyNos)
      return `已写到第${lastNo}集，默认从第${Math.min(500, lastNo + 1)}集继续`
    }

    function selectBodyEpisode(no) {
      const target = Number(no) || 1
      const numbers = (local.value.episodes || []).map((item) => Number(item.no) || 1)
      selectedBodyEpisode.value = numbers.includes(target) ? target : Math.max(...numbers, 1)
      expandedBodyEpisode.value = null
    }

    function bodyPrompt(rangeText = bodyGenerateRange()) {
      return `一键生成${rangeText}正文。请严格按下面本次集纲顺序逐集生成：\n${bodyOutlineContext(bodyRangeStart.value, bodyRangeEnd.value)}\n\n创作要求：\n1. 正文必须使用专业剧本格式（场景头→人物→动作→台词）\n2. 台词要短而有力，符合人物性格和当前情绪，善用潜台词\n3. 动作描写要精确到可拍摄——谁、做什么、怎么做、什么表情\n4. 生成后一集时，必须承接上一集正文结尾的人物状态、情绪和未解决冲突，保持连贯\n5. 每集结尾必须有追看钩子——最后30秒决定观众是否点下一集\n6. 每场都有存在的戏剧理由，没有"废场"和"日常场"\n7. 只生成当前已有集纲对应的正文，不要新增无关集数`
    }

    function bodyOutlineContext(start, end) {
      const from = Math.max(1, Number(start) || 1)
      const to = Math.max(from, Number(end) || from)
      return Array.from({ length: to - from + 1 }, (_, index) => {
        const no = from + index
        const episode = (local.value.episodes || []).find((item) => Number(item.no) === no)
        return `第${no}集集纲：${String(episode?.outline || '').trim()}`
      }).join('\n')
    }

    function missingEpisodeOutlines(start, end) {
      const from = Math.max(1, Number(start) || 1)
      const to = Math.max(from, Number(end) || from)
      const missing = []
      for (let no = from; no <= to; no++) {
        const episode = (local.value.episodes || []).find((item) => Number(item.no) === no)
        const hasOutline = String(episode?.outline || '').trim()
        const canUseSourceOutline = local.value.source === 'adaptation' && local.value.settings?.adaptationMode === 'original'
        if (!hasOutline && !canUseSourceOutline) missing.push(no)
      }
      return missing
    }

    function remindMissingEpisodeOutlines(start, end) {
      const missing = missingEpisodeOutlines(start, end)
      if (!missing.length) return false
      const text = missing.length === 1 ? `第${missing[0]}集缺少集纲` : `第${missing.slice(0, 5).join('、')}集缺少集纲`
      emit('toast', `${text}，请先填写或生成对应集纲`)
      return true
    }

    function openBodyGenerateRange() {
      if (!canGenerateBody()) return
      ensureEpisode()
      const start = nextBodyGenerationStart()
      bodyRangeStart.value = start
      bodyRangeEnd.value = start
      bodyRangeOpen.value = true
    }

    function setBodyRange(key, value) {
      const parsed = Number.parseInt(value, 10)
      if (Number.isNaN(parsed)) {
        if (key === 'start') bodyRangeStart.value = ''
        else bodyRangeEnd.value = ''
        return
      }
      const next = Math.min(500, Math.max(1, parsed))
      if (key === 'start') {
        bodyRangeStart.value = next
      } else {
        bodyRangeEnd.value = next
      }
    }

    function ensureEpisodeRange(start, end) {
      ensureEpisode()
      recordHistory()
      const existing = new Set((local.value.episodes || []).map((item) => Number(item.no) || 1))
      for (let no = start; no <= end; no++) {
        if (!existing.has(no)) {
          local.value.episodes.push({ no, outline: '', body: '' })
          markBodyOnlyEpisode(no)
        }
        markBodyCardEpisode(no)
      }
      local.value.episodes.sort((a, b) => Number(a.no || 0) - Number(b.no || 0))
    }

    function confirmBodyGeneration() {
      if (!canGenerateBody()) return
      const start = Math.max(1, Number(bodyRangeStart.value) || 1)
      const end = Math.max(start, Number(bodyRangeEnd.value) || start)
      bodyRangeStart.value = start
      bodyRangeEnd.value = end
      if (remindMissingEpisodeOutlines(start, end)) return
      ensureEpisodeRange(start, end)
      for (let no = start; no <= end; no++) markBodyCardEpisode(no)
      selectedBodyEpisode.value = start
      expandedBodyEpisode.value = null
      bodyRangeOpen.value = false
      ai('body', 120, bodyPrompt(start === end ? `第${start}集` : `第${start}-${end}集`))
    }

    function generateSingleBodyEpisode(episode) {
      if (!canGenerateBody()) return
      const no = Number(episode?.no) || Number(selectedBodyEpisode.value) || 1
      if (remindMissingEpisodeOutlines(no, no)) return
      ensureEpisodeRange(no, no)
      markBodyCardEpisode(no)
      selectedBodyEpisode.value = no
      bodyRangeStart.value = no
      bodyRangeEnd.value = no
      ai('body', 40, regenerateBodyPrompt(episode || currentBodyEpisode(), no - 1))
    }

    function setSelectedEpisodeCount(value) {
      selectedEpisodeCount.value = Math.max(10, clampNumber(value, 10, 500))
    }

    function hasPhaseOutlineContent() {
      return (local.value.outlines || []).some((item) => String(item?.content || '').trim())
    }

    function phaseNavLabel(item, index) {
      return hasPhaseOutlineContent() ? `${item.start}-${item.end}` : String(index + 1)
    }

    function aiButtonLabel(label, cost, icon = Sparkles) {
      return [h(icon, { size: 15 }), h('span', label), h('em', { class: 'ai-cost' }, [h(Coins, { size: 13 }), String(cost)])]
    }

    function loadingIcon(size = 16) {
      return h(LoaderCircle, { size, class: 'spin-loader' })
    }

    function loadingClass(active, extra = '') {
      return [extra, { 'is-loading': Boolean(active) }]
    }

    function regenerateBodyPrompt(episode, index) {
      const previous = (local.value.episodes || [])[index - 1]
      return `只重新生成第${episode.no || index + 1}集正文。本集集纲：${String(episode?.outline || '').trim()}。必须参考本集集纲、故事梗概、人物小传${previous?.body ? '，并承接上一集正文结尾：\n' + previous.body.slice(-500) : ''}。创作要求：①使用专业剧本格式②台词短而有力、符合人物性格③动作描写精确到可拍摄④每集结尾留追看钩子⑤不要改其他集。`
    }

    function useBenchmark(item) {
      ideaPrompt.value = `深度分析爆款《${item[1]}》的成功要素——它的核心爽点机制是什么、节奏设计有什么特点、人物塑造为什么让人上头、钩子设计为什么让人停不下来。然后保留这些爆款基因，重新策划一个全新的原创短剧方向。要求：①核心设定要有记忆点②强反转、快节奏、连续钩子③要有明确的受众定位④标题要有短剧感和吸引力。`
      rankingOpen.value = false
      startPlanning()
    }

    function parsePlanningText(text) {
      const source = String(text || '').replace(/\r\n/g, '\n')
      const matches = [...source.matchAll(/【\s*(?:策划|方案)\s*([123１２３一二三])(?:\s*[：:｜|·—_、\s（(\-][^】]*)?】/g)]
      if (!matches.length) {
        return []
      }
      return matches.map((match, index) => {
        const start = match.index + match[0].length
        const end = matches[index + 1]?.index ?? source.length
        return { ...parsePlanningBlock(source.slice(start, end)), streaming: true }
      }).filter((plan) => plan.title || plan.synopsis || plan.worldView || plan.highlights)
    }

    function parsePlanningBlock(block) {
      const fields = {}
      let current = ''
      String(block || '').split('\n').forEach((raw) => {
        const line = raw.trim()
        if (!line) return
        const matched = line.match(/^([^：:]{2,8})[：:]\s*(.*)$/)
        if (matched && ['标题', '目标受众', '时代背景', '题材类型', '核心设定', '风格元素', '核心亮点', '世界观', '核心梗概'].includes(matched[1].trim())) {
          current = matched[1].trim()
          fields[current] = matched[2].trim()
          return
        }
        if (current) fields[current] = `${fields[current] || ''}\n${line}`.trim()
      })
      return {
        title: fields['标题'] || '未命名策划',
        audience: fields['目标受众'] || '',
        era: splitList(fields['时代背景'] || fields['风格元素'] || ''),
        genres: splitList(fields['题材类型'] || ''),
        core: splitList(fields['核心设定'] || ''),
        style: [],
        highlights: fields['核心亮点'] || '',
        worldView: fields['世界观'] || '',
        synopsis: fields['核心梗概'] || fields['世界观'] || ''
      }
    }

    function scrollPlanningToActive() {
      nextTick(() => {
        const list = planningListRef.value
        if (!list) return
        window.requestAnimationFrame(() => {
          const cards = Array.from(list.querySelectorAll('.plan-card'))
          if (!cards.length) return
          const index = Math.max(0, Math.min(cards.length - 1, brainstormPlans().length - 1))
          cards[index]?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
        })
      })
    }

    function startPlanning() {
      if (!hasPlanningInput()) return
      persistLocal(true)
      planningGenerating.value = true
      planningPlans.value = []
      planningStreamPlans.value = []
      planningProgress.value = ''
      activeAi.value = 'planning'
      emit('ai', {
        taskType: 'planning',
        cost: 50,
        prompt: planningPrompt(),
        project: local.value,
        onStream: (frame) => {
          if (!frame?.text) return
          planningProgress.value += frame.text
          planningStreamPlans.value = parsePlanningText(planningProgress.value)
          scrollPlanningToActive()
        },
        onResult: (data) => {
          const plans = data?.result?.plans || []
          local.value = cloneData(data.project)
          planningPlans.value = plans
          planningStreamPlans.value = plans
          planningGenerating.value = false
          activeAi.value = ''
          scrollPlanningToActive()
        },
        onError: () => {
          planningStreamPlans.value = []
          planningGenerating.value = false
          activeAi.value = ''
        }
      })
    }

    function clearPlanningWriteTimers() {
      while (planningWriteTimers.length) {
        window.clearTimeout(planningWriteTimers.pop())
      }
    }

    function writePlanningPlan(plan) {
      if (!plan) return
      clearPlanningWriteTimers()
      recordHistory()
      inspirationOpen.value = false
      editorTab.value = 'settings'
      local.value.title = plan.title || local.value.title
      local.value.settings = {
        ...(local.value.settings || {}),
        audience: plan.audience || field('audience'),
        era: plan.era || field('era', []),
        genres: plan.genres || field('genres', []),
        core: plan.core || field('core', []),
        worldView: plan.worldView || '',
        highlights: plan.highlights || '',
        synopsis: plan.synopsis || ''
      }
      const flow = infoflow()
      local.value.settings.infoflow = {
        ...flow,
        storyOverview: {
          ...(flow.storyOverview || {}),
          audience: plan.audience || field('audience'),
          era: stringifyList(plan.era || field('era', [])),
          genre: stringifyList(plan.genres || field('genres', [])),
          core: stringifyList(plan.core || field('core', [])),
          background: plan.worldView || '',
          highlights: plan.highlights || '',
          synopsis: plan.synopsis || ''
        }
      }
      persistLocal(true)
    }

    function applyPlanningPlan(plan, close = true) {
      if (!plan) return
      recordHistory()
      local.value.title = plan.title || local.value.title
      local.value.settings = {
        ...(local.value.settings || {}),
        audience: plan.audience || field('audience'),
        era: plan.era || field('era', []),
        genres: plan.genres || field('genres', []),
        core: plan.core || field('core', []),
        worldView: plan.worldView || field('worldView'),
        highlights: plan.highlights || field('highlights'),
        synopsis: plan.synopsis || field('synopsis')
      }
      persistLocal(true, () => {
        if (close) inspirationOpen.value = false
      })
    }

    function applyPlan(plan) {
      planSavePending.value = true
      writePlanningPlan(plan)
      window.setTimeout(() => {
        planSavePending.value = false
        emit('toast', '已使用该策划')
      }, 1500)
    }

    function planCard(plan, index) {
      return h('article', { class: 'plan-card', 'data-plan-index': index }, [
        h('header', [h('span', `策划 ${index + 1}`), h('button', { class: { 'is-loading': plan.streaming || planningGenerating.value }, disabled: plan.streaming || planningGenerating.value || planSavePending.value, onClick: () => applyPlan(plan) }, plan.streaming || planningGenerating.value ? loadingIcon(14) : (planSavePending.value ? '使用中' : '使用'))]),
        h('h3', plan.title || `策划 ${index + 1}`),
        h('div', { class: 'plan-setting-grid' }, [
          h('section', [h('b', '目标受众'), h('p', plan.audience || '待补充')]),
          h('section', [h('b', '时代背景'), h('p', stringifyList(plan.era) || '待补充')]),
          h('section', [h('b', '题材类型'), h('p', stringifyList(plan.genres) || '待补充')]),
          h('section', [h('b', '核心设定'), h('p', stringifyList(plan.core) || '待补充')])
        ]),
        h('h4', '故事背景'),
        h('p', plan.worldView || '待补充'),
        h('h4', '核心亮点'),
        h('p', plan.highlights || '待补充'),
        h('h4', '核心梗概'),
        h('p', plan.synopsis || '待补充'),
        h('div', { class: 'plan-tags-line' }, [
          ...splitList(plan.audience).map((item) => h('em', `#${item}`)),
          ...splitList(plan.era).slice(0, 3).map((item) => h('em', `#${item}`)),
          ...splitList(plan.genres).slice(0, 3).map((item) => h('em', `#${item}`)),
          ...splitList(plan.core).slice(0, 3).map((item) => h('em', `#${item}`))
        ])
      ])
    }

    function brainstormPlans() {
      return planningPlans.value.length ? planningPlans.value : planningStreamPlans.value
    }

    return () => h('article', { class: 'editor-page' }, [
      h('button', { class: 'editor-back', title: '返回', 'aria-label': '返回', onClick: leaveEditor }, '✦'),
      h('input', {
        class: 'script-title',
        value: local.value.title,
        placeholder: '剧本名称',
        disabled: props.busy || planningGenerating.value,
        onInput: (event) => setTitle(event.target.value)
      }),
      importProgress.value && h('div', { class: 'import-progress-banner' }, [
        loadingIcon(16),
        h('span', importProgress.value)
      ]),
      h('nav', { class: 'editor-tabs' }, tabs.value.map(([key, label]) => h('button', {
        class: { active: editorTab.value === key, 'source-tab': isSourceTab(key) },
        onClick: () => switchTab(key)
      }, label))),
      h('div', { class: ['editor-actions', `actions-${editorTab.value}`] }, editorTab.value === 'body'
        ? [
          h('div', { class: 'body-generate-group' }, [
            h('button', { class: loadingClass(activeAi.value === 'body', 'cost-btn inline body-generate-btn'), disabled: props.busy || activeAi.value === 'body', onClick: openBodyGenerateRange }, [
              activeAi.value === 'body' ? loadingIcon(16) : h(Sparkles, { size: 16 }),
              activeAi.value === 'body' ? null : '一键生成'
            ]),
            !expandedBodyEpisode.value && h('aside', { class: 'body-ruler novel-ruler' }, bodyRulerTicks().map((tick) => h('button', {
              class: { major: tick.major, mid: tick.mid, ghost: tick.ghost, active: Number(selectedBodyEpisode.value) === tick.no },
              title: tick.label ? `第${tick.label}集` : '集数刻度',
              onClick: () => selectBodyEpisode(tick.no)
            }, [h('i'), tick.label && h('span', tick.label)])))
          ]),
          h('button', { class: 'export-btn inline', disabled: props.busy || activeAi.value === 'body', onClick: exportCurrent }, [h(Download, { size: 18 }), '导出Word'])
        ]
        : [
          h('button', { class: 'icon-action', disabled: !historyStack.value.length, onClick: undo }, h(RotateCcw, { size: 20 })),
          h('button', { class: 'icon-action', disabled: !futureStack.value.length, onClick: redo }, h(RotateCw, { size: 20 })),
          editorTab.value === 'settings' && h('button', { class: loadingClass(activeAi.value === 'planning' || planningGenerating.value, 'inspiration-btn'), disabled: props.busy || planningGenerating.value, onClick: seedInspiration }, activeAi.value === 'planning' || planningGenerating.value ? loadingIcon(16) : planningButtonText()),
          editorTab.value === 'infoflow' && h('button', { class: 'quote-btn', disabled: true }, '一键引用'),
          editorTab.value === 'characters' && local.value.source !== 'original' && h('button', { class: 'icon-action', title: '新增角色', 'aria-label': '新增角色', disabled: props.busy || activeAi.value === 'characters', onClick: addCharacter }, h(Plus, { size: 20 })),
          editorTab.value === 'characters' && h('button', { class: loadingClass(activeAi.value === 'characters', 'cost-btn inline'), disabled: props.busy || activeAi.value === 'characters', onClick: generateCharacters }, activeAi.value === 'characters' ? loadingIcon(16) : aiButtonLabel(local.value.source === 'original' ? 'AI生成人物' : 'AI提炼', 50)),
          editorTab.value === 'outline' && h('div', { class: 'guide-anchor action-guide-anchor' }, [
            h('button', { class: loadingClass(activeAi.value === 'outline', 'cost-btn inline'), disabled: props.busy || activeAi.value === 'outline', onClick: openOutlineGeneration }, activeAi.value === 'outline' ? loadingIcon(16) : aiButtonLabel('AI生成粗纲', 80)),
            guide.value === 'coarse' && h('div', { class: 'guide-tip first' }, [h('h3', '点击AI，一键生成全部粗纲'), h('p', '根据梗概&人物一键生成所有粗纲'), h('button', { onClick: () => guide.value = 'episode' }, '下一步')])
          ])
        ].filter(Boolean)
      ),
      editorTab.value === 'infoflow' && h('section', { class: 'infoflow-editor' }, [
        h('article', { class: 'infoflow-block' }, [
          h('h2', '故事梗概'),
          h('div', { class: 'infoflow-meta' }, [
            ['目标受众', infoflow().storyOverview?.audience || field('audience')],
            ['时代背景', infoflow().storyOverview?.era || field('era')],
            ['题材类型', infoflow().storyOverview?.genre || stringifyList(field('genres', []))],
            ['核心设定', infoflow().storyOverview?.core || stringifyList(field('core', []))]
          ].map(([label, value]) => h('span', [h('b', label), h('em', value || '待补充')]))),
          h('h3', '故事背景'), h('p', infoflow().storyOverview?.background || field('worldView') || '待补充'),
          h('h3', '核心亮点'), h('p', infoflow().storyOverview?.highlights || field('highlights') || '待补充'),
          h('h3', '核心梗概'), h('p', infoflow().storyOverview?.synopsis || field('synopsis') || '待补充')
        ]),
        h('article', { class: 'infoflow-block' }, [
          h('h2', '人物设定'),
          (infoflow().characters || []).map((item) => h('section', { class: 'infoflow-person' }, [
            h('b', `${item.name || '角色'} ${item.age || item.meta || ''}`),
            h('p', item.bio || item.background || item.goal || '待补充')
          ]))
        ]),
        (local.value.source === 'rewriting' || filledInfoflowOutlines().length) && h('article', { class: 'infoflow-block' }, [
          h('h2', '粗纲'),
          filledInfoflowOutlines().length
            ? filledInfoflowOutlines().map((item) => h('p', [h('b', `${item.range || ''} ${item.phase || ''}`), ` ${item.content || item.summary || ''}`]))
            : h('p', '待生成')
        ]),
        filledInfoflowEpisodes().length > 0 ? h('article', { class: 'infoflow-block' }, [
          h('h2', '集纲'),
          filledInfoflowEpisodes().map((item, index) => h('p', [h('b', `第${item.no || index + 1}集`), ` ${item.outline || item.summary || ''}`]))
        ]) : null
      ].filter(Boolean)),
      editorTab.value === 'novelOutline' && h('section', { class: 'novel-outline-editor' }, [
        h('div', { class: 'novel-outline-main' }, [
          h('div', { class: 'novel-section-title' }, [h('b', '小说章纲'), h('span', '每章情节独立拆分')]),
          h('div', { class: 'novel-outline-grid' }, novelOutlines().map((chapter, index) => h('article', { class: 'novel-chapter-card', ref: (el) => setNovelOutlineRef(index, el) }, [
          h('label', { class: 'novel-title-line' }, [
            h('b', `第${chapter.column || index + 1}章`),
            h('input', { value: chapter.title, disabled: props.busy, onInput: (event) => setNovelOutlineField(index, 'title', event.target.value) })
          ]),
          h('h3', chapter.synopsis || chapter.title || '章节梗概'),
          h('textarea', { value: chapter.summary, disabled: props.busy, placeholder: '章节大纲', onInput: (event) => setNovelOutlineField(index, 'summary', event.target.value) })
          ])))
        ]),
        h('aside', { class: 'novel-ruler' }, novelRulerTicks().map((tick) => h('button', {
          class: { major: tick.major, mid: tick.mid, ghost: tick.ghost },
          title: tick.label ? `跳到第${tick.label}章` : `跳到附近章节`,
          onClick: () => scrollToNovelChapter(tick.targetIndex)
        }, [h('i'), tick.label && h('span', tick.label)])))
      ]),
      editorTab.value === 'settings' && h('section', { class: 'settings-editor' }, [
        h('div', { class: 'edit-form' }, [
          h('h1', local.value.source === 'rewriting' ? '改写策划' : '故事设定'),
          h('div', { class: 'setting-grid' }, [
            h('label', { class: 'audience-field' }, [h('b', ['目标受众', h('span', { class: 'req' }, '*')]), h('div', { class: 'pill-line' }, ['男频', '女频'].map((item) => h('button', { type: 'button', disabled: props.busy || planningGenerating.value, class: ['audience-pill', item === '男频' ? 'male' : 'female', { selected: field('audience') === item }], onClick: () => setField('audience', item) }, [h('span', item === '男频' ? '♂' : '♀'), item])))]),
            settingSelect('era', '时代背景', '（不超过3个）', '选择或自定义输入时代背景', eraOptions, 3),
            settingSelect('genres', '题材类型', '（不超过3个）', '选择或自定义输入背景', genreOptions, 3),
            settingSelect('core', '核心设定', '（不超过3个）', '选择或自定义输入类型', coreOptions, 3)
          ].filter(Boolean)),
          h('label', { class: 'wide-field' }, [h('b', local.value.source === 'rewriting' ? '故事背景' : '世界观（选填）'), h('textarea', { disabled: props.busy || planningGenerating.value, value: field('worldView'), placeholder: '例：世界存在一种失传的“织墨术”...', onInput: (event) => setField('worldView', event.target.value) })]),
          h('label', { class: 'wide-field compact' }, [h('b', '核心亮点（选填）'), h('textarea', { disabled: props.busy || planningGenerating.value, value: field('highlights'), placeholder: '例：穿书成作死女配，她反向操作...', onInput: (event) => setField('highlights', event.target.value) })]),
          h('label', { class: 'wide-field synopsis-field' }, [
            h('div', { class: 'field-heading-row' }, [
              h('b', ['核心梗概', h('span', { class: 'req' }, '*')]),
              local.value.source === 'adaptation' && h('button', { type: 'button', class: 'cost-btn small', disabled: props.busy || activeAi.value === 'synopsis', onClick: () => ai('synopsis', 80, '只提炼故事概梗，不要改人物小传、小说章纲、粗纲和集纲。') }, activeAi.value === 'synopsis' ? '提炼中' : aiButtonLabel('AI提炼', 80))
            ]),
            h('textarea', { disabled: props.busy || activeAi.value === 'synopsis', value: storySynopsis(), onInput: (event) => setStorySynopsis(event.target.value) })
          ])
        ]),
        h('aside', { class: 'anchor-list' }, anchors.value.map((item) => h('span', { class: { done: anchorDone(item) } }, item)))
      ]),
      editorTab.value === 'characters' && h('section', { class: (local.value.characters || []).length || activeAi.value === 'characters' ? 'characters-editor' : 'empty-editor' }, [
        (local.value.characters || []).length || activeAi.value === 'characters'
          ? [h('div', { class: ['character-workbench', { streaming: activeAi.value === 'characters' }] }, [
            h('article', { class: 'character-card' }, [
              h('div', { class: 'character-grid' }, [
                h('label', [h('h3', '姓名'), h('input', { disabled: props.busy || activeAi.value === 'characters', value: activeCharacter().name || '', placeholder: '', onInput: (event) => setCharacterField(activeCharacterIndex(), 'name', event.target.value) })]),
                h('label', { class: 'character-role-field' }, [h('h3', '定位'), activeAi.value === 'characters'
                  ? h('input', { disabled: true, value: activeCharacter().role || '', placeholder: '' })
                  : h('select', { disabled: props.busy, value: activeCharacter().role || activeCharacter().meta, onChange: (event) => setCharacterField(activeCharacterIndex(), 'role', event.target.value) }, optionGroups(characterRoleOptions).flatMap(([group, items]) => [h('option', { disabled: true }, group), ...items.map((item) => h('option', { value: item }, item))]))]),
                h('label', [h('h3', '年龄'), h('input', { disabled: props.busy || activeAi.value === 'characters', value: activeCharacter().age || '', placeholder: '', onInput: (event) => setCharacterField(activeCharacterIndex(), 'age', event.target.value) })])
              ]),
              h('h3', '性格'),
              h('div', { class: 'trait-list' }, ((activeCharacter().traits || splitList(activeCharacter().meta || '')).length ? (activeCharacter().traits || splitList(activeCharacter().meta || '')) : []).map((trait) => h('span', { title: '双击删除', onDblclick: () => removeCharacterTrait(trait) }, trait))),
              h('input', {
                class: 'tag-input',
                disabled: props.busy || activeAi.value === 'characters',
                value: characterTagDraft.value,
                placeholder: '空格添加标签，双击删除',
                onInput: (event) => { characterTagDraft.value = event.target.value },
                onKeydown: (event) => {
                  if (event.key === ' ' || event.key === 'Enter') {
                    event.preventDefault()
                    addCharacterTrait(event.target.value)
                  }
                }
              }),
              h('label', [h('h3', '背景'), h('textarea', { disabled: props.busy || activeAi.value === 'characters', value: activeCharacter().background || activeCharacter().bio || '', placeholder: '', onInput: (event) => setCharacterField(activeCharacterIndex(), 'background', event.target.value) })]),
              h('label', [h('h3', '核心动机'), h('textarea', { disabled: props.busy || activeAi.value === 'characters', value: activeCharacter().goal || '', placeholder: '', onInput: (event) => setCharacterField(activeCharacterIndex(), 'goal', event.target.value) })])
            ]),
            h('aside', { class: ['character-index', { generating: activeAi.value === 'characters' }] }, [
              ...(local.value.characters || []).map((item, index) => h('button', {
              class: { active: index === activeCharacterIndex(), creating: activeAi.value === 'characters' && index === activeCharacterIndex() },
              onClick: () => selectCharacter(index)
              }, [
                h('b', item.name || `角色${index + 1}`),
                h('span', item.role || item.meta || ''),
                activeAi.value !== 'characters' && h('i', {
                  class: 'character-delete',
                  title: '删除角色',
                  onClick: (event) => {
                    event.stopPropagation()
                    deleteCharacter(index)
                  }
                }, h(X, { size: 14 }))
              ])),
              activeAi.value === 'characters' && h('button', { class: 'add-role-card streaming-next', disabled: true }, [loadingIcon(22), h('span', '创建下一个')]),
              h('button', { class: 'add-role-card', title: '新增角色', 'aria-label': '新增角色', disabled: props.busy || activeAi.value === 'characters', onClick: addCharacter }, h(Plus, { size: 28 }))
            ])
          ])]
          : [h('p', local.value.source === 'original' ? ['先完成故事概梗，再由AI生成人物小传'] : ['点击右侧按钮创建新角色，自定义角色信息', h('br'), '或由AI一键生成所有角色']), h('div', { class: 'empty-actions' }, [
              local.value.source !== 'original' && h('button', { disabled: props.busy || activeAi.value === 'characters', onClick: addCharacter }, [h(Plus, { size: 18 }), '新增角色']),
              h('button', { class: 'primary-btn ai-empty-generate', disabled: props.busy || activeAi.value === 'characters', onClick: generateCharacters }, aiButtonLabel('AI生成人物', 50))
            ].filter(Boolean))]
      ]),
      editorTab.value === 'outline' && h('section', { class: 'outline-editor' }, [
        h('div', { class: 'outline-layout' }, [
          h('div', { class: 'outline-main' }, [
            h('article', { class: ['outline-block', 'phase-outline-block', { collapsed: outlineCollapsed.value }] }, [
              h('header', [h('b', '粗纲（选填）'), h('button', { type: 'button', 'aria-label': outlineCollapsed.value ? '展开粗纲' : '收起粗纲', onClick: () => { outlineCollapsed.value = !outlineCollapsed.value } }, outlineCollapsed.value ? '⌄' : '⌃')]),
              !outlineCollapsed.value && h('textarea', { value: outlineForPhase().content, placeholder: '请输入内容，3000字以内', maxlength: 3000, onInput: (event) => setPhaseOutlineContent(event.target.value) })
            ]),
            h('div', { class: 'episode-head' }, [h('h3', ['集纲 ', h('span', '*')]), h('div', { class: 'guide-anchor episode-guide-anchor' }, [
              h('button', { class: loadingClass(activeAi.value === 'episode', 'episode-generate-all'), title: '生成当前阶段集纲', disabled: props.busy || activeAi.value === 'episode', onClick: generateCurrentEpisodeOutline }, activeAi.value === 'episode' ? loadingIcon(14) : aiButtonLabel('生成本阶段集纲', 60)),
              guide.value === 'episode' && h('div', { class: 'guide-tip second' }, [h('h3', '点击AI，一键生成集纲'), h('p', '根据梗概&人物&粗纲生成当前所有集纲'), h('button', { onClick: () => guide.value = '' }, '知道了')])
            ])]),
            ...episodesForCurrentPhase().map((episode, index) => h('article', { class: 'episode-card' }, [
              h('h3', `第${episode.no || index + 1}集`),
              h('textarea', { value: episode.outline, placeholder: '填写本集核心事件、冲突推进、关键反转和结尾钩子', onInput: (event) => setEpisodeOutlineByNo(episode.no, event.target.value) })
            ]))
          ]),
          h('aside', { class: 'outline-phase-nav' }, phaseRanges().map((item, index) => h('button', { class: { active: selectedOutlinePhase.value === index }, onClick: () => { selectedOutlinePhase.value = index } }, [h('b', item.phase), h('em', phaseNavLabel(item, index))]))),
        ]),
      ]),
      editorTab.value === 'body' && h('section', { class: ['body-editor', { 'body-editor-rewrite': local.value.source === 'rewriting' }] }, [
        h('div', { class: 'body-layout' }, [
          h('div', { class: 'body-paper-stack' }, bodyEditorEpisodes().length ? bodyEditorEpisodes().map((episode, index) => h('div', { class: ['body-paper', { expanded: expandedBodyEpisode.value === episode.no }], 'data-body-episode': episode.no }, [
            h('header', [h('h3', `第${episode.no || index + 1}集`), h('div', { class: 'body-card-tools' }, [
              h('button', { title: '新增下一集', 'aria-label': '新增下一集', disabled: props.busy, onClick: () => addEpisode(episode) }, h(Plus, { size: 16 })),
              h('button', { class: { 'is-loading': activeAi.value === 'body' && isGeneratingBodyEpisode(episode) }, title: episode.body ? '重新生成本集' : '生成本集', 'aria-label': episode.body ? '重新生成本集' : '生成本集', disabled: props.busy || activeAi.value === 'body', onClick: () => generateSingleBodyEpisode(episode) }, activeAi.value === 'body' && isGeneratingBodyEpisode(episode) ? loadingIcon(16) : h(episode.body ? RotateCw : Sparkles, { size: 16 })),
              h('button', { title: expandedBodyEpisode.value === episode.no ? '收起' : '展开', 'aria-label': expandedBodyEpisode.value === episode.no ? '收起' : '展开', onClick: () => { selectedBodyEpisode.value = episode.no; expandedBodyEpisode.value = expandedBodyEpisode.value === episode.no ? null : episode.no } }, h(Maximize2, { size: 16 })),
              h('button', { title: '删除', 'aria-label': '删除', onClick: () => deleteBodyEpisode(episode.no) }, h(Trash2, { size: 16 }))
            ])]),
            h('textarea', { class: 'body-textarea', value: episode.body, placeholder: '可直接输入正文，或点击右侧按钮生成本集', onInput: (event) => setEpisodeBodyByNo(episode.no, event.target.value) })
          ])) : [
            h('article', { class: 'body-empty-state' }, [
              h('h3', '点击按钮创建新集，AI辅助您生成初稿'),
              h('p', '创建后将按集数自动关联对应集纲'),
              h('button', { class: 'primary-btn body-create-btn', disabled: props.busy, onClick: createFirstBodyEpisode }, [h(FilePlus2, { size: 18 }), '创建新集'])
            ])
          ]),
        ])
      ]),
      bodyRangeOpen.value && h('section', { class: 'modal-layer editor-layer' }, [
        h('article', { class: 'episode-count-modal body-range-modal' }, [
          h('div', { class: 'body-range-header' }, [
            h(Sparkles, { size: 20, class: 'body-range-icon' }),
            h('h3', '生成正文')
          ]),
          h('p', '选择本次生成的正文区间，将按对应集纲连续生成。'),
          h('div', { class: 'body-range-fields' }, [
            h('label', [h('span', '开始集数'), h('input', {
              type: 'number',
              min: 1,
              max: 500,
              value: bodyRangeStart.value,
              onInput: (event) => setBodyRange('start', event.target.value),
              onBlur: (event) => setBodyRange('start', event.target.value)
            })]),
            h('span', { class: 'body-range-arrow' }, '→'),
            h('label', [h('span', '结束集数'), h('input', {
              type: 'number',
              min: 1,
              max: 500,
              value: bodyRangeEnd.value,
              onInput: (event) => setBodyRange('end', event.target.value),
              onBlur: (event) => setBodyRange('end', event.target.value)
            })])
          ]),
          h('div', { class: 'body-range-hint' }, bodyRangeHint()),
          h('footer', [
            h('button', { class: 'body-range-cancel', onClick: () => { bodyRangeOpen.value = false } }, '取消'),
            h('button', { class: loadingClass(activeAi.value === 'body', 'primary-btn body-range-confirm'), disabled: props.busy || activeAi.value === 'body', onClick: confirmBodyGeneration }, activeAi.value === 'body' ? loadingIcon(14) : [h(Sparkles, { size: 14 }), '确认生成'])
          ])
        ])
      ]),
      outlineRangeOpen.value && h('section', { class: 'modal-layer editor-layer' }, [
        h('article', { class: 'episode-count-modal' }, [
          h('h3', '生成粗纲'),
          h('p', '填写本剧预计总集数，不能低于 10 集。'),
          h('label', { class: 'episode-count-field' }, [
            h('span', '一共生成'),
            h('input', {
              type: 'number',
              min: 10,
              value: selectedEpisodeCount.value,
              onInput: (event) => setSelectedEpisodeCount(event.target.value),
              onBlur: (event) => setSelectedEpisodeCount(event.target.value)
            }),
            h('span', '集')
          ]),
          h('footer', [
            h('button', { onClick: () => { outlineRangeOpen.value = false } }, '取消'),
            h('button', { class: loadingClass(activeAi.value === 'outline', 'primary-btn'), disabled: props.busy || activeAi.value === 'outline', onClick: confirmOutlineGeneration }, activeAi.value === 'outline' ? loadingIcon(14) : aiButtonLabel('确认生成', 80))
          ])
        ])
      ]),
      inspirationOpen.value && h('section', {
        class: 'modal-layer editor-layer inspiration-layer',
        onClick: () => {
          audienceOpen.value = false
          optionOpen.value = ''
        }
      }, [
        h('article', { class: ['inspiration-modal', { active: brainstormPlans().length || planningGenerating.value }] }, [
          h('header', [h('h1', planningButtonText()), h('div', { class: 'planning-top-actions' }, [h('button', { class: 'planning-skip', onClick: () => inspirationOpen.value = false }, '跳过')])]),
          !planningGenerating.value && !brainstormPlans().length && h('div', { class: 'planning-hero' }, [
            h('h3', `AI${planningButtonText()}`),
            h('h4', local.value.source === 'rewriting' ? '基于原剧核心信息策划多个改写方向' : '基于您对剧本的初步设定策划多个故事题材方向')
          ]),
          (planningGenerating.value || brainstormPlans().length > 0) && h('div', { class: 'planning-stream-head' }, [
            h('h3', planningGenerating.value ? '策划中' : '策划完成'),
            planningPlans.value.length > 0 ? h('p', '选择一个策划后，将写入故事概梗页') : null,
            planningGenerating.value && h('div', { class: 'planning-motion', 'aria-hidden': 'true' }, [
              h('span'), h('span'), h('span'), h('span')
            ]),
            h('div', { class: 'planning-steps' }, [
              h('span', { class: { done: brainstormPlans().length >= 1 } }, '策划 1'),
              h('span', { class: { done: brainstormPlans().length >= 2 } }, '策划 2'),
              h('span', { class: { done: brainstormPlans().length >= 3 || planningPlans.value.length >= 3 } }, '策划 3')
            ])
          ]),
          brainstormPlans().length > 0
            ? h('div', { class: 'plan-list', ref: planningListRef }, brainstormPlans().map((plan, index) => planCard(plan, index)))
            : planningGenerating.value && h('div', { class: 'plan-list planning-raw-stream', ref: planningListRef }, [
              h('article', { class: 'plan-card' }, [
                h('header', [h('span', '策划内容'), h('button', { class: 'is-loading', disabled: true }, loadingIcon(14))]),
                planningProgress.value && h('p', planningProgress.value)
              ])
            ]),
          h('div', { class: 'planning-compose', onClick: (event) => event.stopPropagation() }, [
            h('div', { class: 'planning-tags' }, [
              selectedAudience.value && h('button', { class: 'planning-tag audience', onClick: () => { selectedAudience.value = ''; setField('audience', '') } }, `#${selectedAudience.value}`),
              ...selectedEra.value.map((item) => h('button', { class: 'planning-tag era', onClick: () => togglePlanningOption(selectedEra, 'era', item, 3) }, `#${item}`)),
              ...selectedGenres.value.map((item) => h('button', { class: 'planning-tag genre', onClick: () => togglePlanningOption(selectedGenres, 'genres', item, 3) }, `#${item}`)),
              ...selectedCore.value.map((item) => h('button', { class: 'planning-tag core', onClick: () => togglePlanningOption(selectedCore, 'core', item, 3) }, `#${item}`))
            ].filter(Boolean)),
            h('textarea', { class: 'idea-box', disabled: planningGenerating.value, value: ideaPrompt.value, placeholder: '选择下方的题材设定或输入您的想法，生成三种剧本策划方案', style: { textAlign: 'left' }, onInput: (event) => { ideaPrompt.value = event.target.value } }),
            h('div', { class: 'planning-option-strip' }, [
              h('button', { class: ['wide-option', { selected: selectedAudience.value }], disabled: planningGenerating.value, onClick: () => { audienceOpen.value = !audienceOpen.value; optionOpen.value = '' } }, '目标受众'),
              h('button', { class: ['wide-option', { selected: selectedEra.value.length }], disabled: planningGenerating.value, onClick: () => { optionOpen.value = optionOpen.value === 'era' ? '' : 'era'; audienceOpen.value = false } }, '时代背景'),
              h('button', { class: ['wide-option', { selected: selectedGenres.value.length }], disabled: planningGenerating.value, onClick: () => { optionOpen.value = optionOpen.value === 'genres' ? '' : 'genres'; audienceOpen.value = false } }, '题材类型'),
              h('button', { class: ['wide-option', { selected: selectedCore.value.length }], disabled: planningGenerating.value, onClick: () => { optionOpen.value = optionOpen.value === 'core' ? '' : 'core'; audienceOpen.value = false } }, '核心设定')
            ]),
            audienceOpen.value && h('div', { class: 'option-pop planning-inline-pop audience-pop', onClick: (event) => event.stopPropagation() }, ['男频', '女频'].map((item) => h('button', { class: { selected: selectedAudience.value === item }, disabled: planningGenerating.value, onClick: () => { selectedAudience.value = item; setField('audience', item); audienceOpen.value = false } }, item))),
            optionOpen.value === 'era' && h('div', { class: 'option-pop option-pop-wrap planning-inline-pop grouped-pop era-pop', onClick: (event) => event.stopPropagation() }, planningGroupedOptions('era', eraOptions, selectedEra, 3)),
            optionOpen.value === 'genres' && h('div', { class: 'option-pop option-pop-wrap planning-inline-pop grouped-pop genres-pop', onClick: (event) => event.stopPropagation() }, planningGroupedOptions('genres', genreOptions, selectedGenres, 3)),
            optionOpen.value === 'core' && h('div', { class: 'option-pop option-pop-wrap planning-inline-pop grouped-pop core-pop', onClick: (event) => event.stopPropagation() }, planningGroupedOptions('core', coreOptions, selectedCore, 3)),
            h('button', { class: loadingClass(planningGenerating.value, 'primary-btn planning-submit'), disabled: props.busy || planningGenerating.value || !hasPlanningInput(), onClick: startPlanning }, planningGenerating.value ? loadingIcon(16) : aiButtonLabel('开始策划', 50))
          ]),
        ]),
        null
      ])
    ])
  }
})

function stringifyList(value) {
  return Array.isArray(value) ? value.join('、') : (value || '')
}

function splitList(value) {
  return String(value || '').split(/[、,，\s]+/).map((item) => item.trim()).filter(Boolean)
}

function initialEditorTab(project) {
  if (project?.source === 'rewriting') return 'infoflow'
  if (project?.source === 'adaptation') return 'novelOutline'
  return 'settings'
}
</script>
