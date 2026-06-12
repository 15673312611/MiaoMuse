import fs from 'node:fs/promises'
import path from 'node:path'

const root = process.cwd()
const apiFile = path.join(root, 'api.example.txt')
const outputFile = path.join(root, 'docs', 'reference-four-tabs.html')
const rawFile = path.join(root, 'docs', 'reference-four-tabs.raw.txt')
const screenshots = [
  path.join(root, 'screenshots', 'real-four-tabs-01-settings-small.jpeg'),
  path.join(root, 'screenshots', 'real-four-tabs-02-characters-small.jpeg'),
  path.join(root, 'screenshots', 'real-four-tabs-03-outline-small.jpeg'),
  path.join(root, 'screenshots', 'real-four-tabs-04-body-small.jpeg')
]

function stripFences(value) {
  const text = String(value || '').trim()
  return text
    .replace(/^```html\s*/i, '')
    .replace(/^```\s*/i, '')
    .replace(/```$/i, '')
    .trim()
}

async function readConfig() {
  const lines = (await fs.readFile(apiFile, 'utf8'))
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
  if (lines.length < 3) {
    throw new Error('API config file must contain base URL, model, and API key on non-empty lines')
  }
  return {
    baseURL: lines[0].replace(/\/+$/, ''),
    model: lines[1],
    apiKey: lines[2]
  }
}

async function imagePart(filePath) {
  const data = await fs.readFile(filePath)
  const ext = path.extname(filePath).toLowerCase()
  const mime = ext === '.jpg' || ext === '.jpeg' ? 'image/jpeg' : 'image/png'
  return {
    type: 'image_url',
    image_url: {
      url: `data:${mime};base64,${data.toString('base64')}`,
      detail: 'low'
    }
  }
}

async function main() {
  const cfg = await readConfig()
  const imageParts = []
  for (const file of screenshots) {
    await fs.access(file)
    imageParts.push(await imagePart(file))
  }

  const prompt = [
    '下面四张截图来自同一个网站编辑器的四个 tap，截图顺序是：故事梗概、人物小传、分集大纲、剧本正文。',
    '帮我一比一复刻图中网站  用html生成 保留全部细节 要完全一模一样 直接返回完整html给我',
    '必须生成单文件 HTML，CSS 写在 style 标签内，不要依赖外部资源，不要输出 Markdown。',
    '重点还原：左侧深色导航、顶部标题和 tap、右上操作按钮、暗色斜纹背景、表单尺寸、输入框间距、按钮位置、人物标签和角色索引、大纲长文本编辑块、正文纸张编辑器。'
  ].join('\n')

  const body = {
    model: cfg.model,
    stream: true,
    messages: [
      {
        role: 'user',
        content: [
          { type: 'text', text: prompt },
          ...imageParts
        ]
      }
    ],
    temperature: 0.2
  }

  let res
  try {
    res = await fetch(`${cfg.baseURL}/v1/chat/completions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${cfg.apiKey}`
      },
      body: JSON.stringify(body)
    })
  } catch (err) {
    const cause = err.cause ? `; cause: ${err.cause.message || String(err.cause)}` : ''
    throw new Error(`${err.message}${cause}`)
  }
  if (!res.ok) {
    const text = await res.text()
    await fs.writeFile(rawFile, text, 'utf8')
    throw new Error(`provider returned ${res.status}; raw response saved to ${rawFile}`)
  }
  const decoder = new TextDecoder()
  let buffer = ''
  let html = ''
  for await (const chunk of res.body) {
    buffer += decoder.decode(chunk, { stream: true })
    const lines = buffer.split(/\r?\n/)
    buffer = lines.pop() || ''
    for (const line of lines) {
      const trimmed = line.trim()
      if (!trimmed || !trimmed.startsWith('data:')) continue
      const data = trimmed.slice(5).trim()
      if (data === '[DONE]') continue
      try {
        const json = JSON.parse(data)
        const delta = json?.choices?.[0]?.delta?.content
        if (delta) html += delta
      } catch {
        // Keep the raw stream for debugging malformed chunks below.
      }
    }
  }
  await fs.writeFile(rawFile, html, 'utf8')
  const cleanHTML = stripFences(html)
  if (!cleanHTML.includes('<html') && !cleanHTML.includes('<!doctype')) {
    throw new Error(`model response did not look like HTML; raw response saved to ${rawFile}`)
  }
  await fs.writeFile(outputFile, cleanHTML, 'utf8')
  console.log(`reference HTML saved to ${outputFile}`)
}

main().catch((err) => {
  console.error(err.message)
  process.exitCode = 1
})
