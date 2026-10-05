import { test } from 'node:test'
import assert from 'node:assert/strict'
import { formatDayForClipboard } from './dual-notes-export'

const day = rows => ({ date: '2026-10-06', revision: 7, rows: rows.map(([left, right], index) => ({ id: `row-${index}`, left, right })) })

test('single-day export keeps chronological row pairing and left-before-right order', () => {
  const output = formatDayForClipboard(day([
    ['第一行的推理', '第一行的 AI 对话'],
    ['第二行的结论', '第二行的 AI 对话'],
  ]))
  assert.ok(output.startsWith('# 双栏记录：2026-10-06\n'))
  const expectedOrder = ['## 第 1 行', '### 左栏：不用电脑', '第一行的推理', '### 右栏：用电脑 or AI', '第一行的 AI 对话', '## 第 2 行', '第二行的结论', '第二行的 AI 对话']
  let position = -1
  for (const item of expectedOrder) {
    position = output.indexOf(item, position + 1)
    assert.ok(position >= 0, item)
  }
  assert.ok(!output.includes('row-0'))
  assert.ok(!output.includes('revision'))
})

test('entirely blank rows are omitted without renumbering; one-sided rows explicitly retain the empty side', () => {
  const output = formatDayForClipboard(day([
    ['', ' \t\n'], ['只有左栏', ''], ['\n\t', '　'], ['', '只有右栏'],
  ]))
  assert.deepEqual([...output.matchAll(/^## 第 (\d+) 行$/gm)].map(match => Number(match[1])), [2, 4])
  assert.equal([...output.matchAll(/（空白）/g)].length, 2)
  assert.match(output, /### 右栏：用电脑 or AI\n\n（空白）/)
  assert.match(output, /### 左栏：不用电脑\n\n（空白）/)
})

test('original Chinese prose, Markdown languages, indentation, CRLF and trailing blank lines remain verbatim', () => {
  const original = '\n  # 原始标题  \r\n\r\n```c++\r\nint main() {\r\n\treturn 42; // 中文注释\r\n}\r\n```\r\n\r\n'
  const other = '   ```python\nprint("你好")\n```  '
  const output = formatDayForClipboard(day([[original, other]]))
  assert.ok(output.includes('````markdown\n' + original + '````'))
  assert.ok(output.includes('````markdown\n' + other + '\n````'))
  assert.ok(output.includes('\r\n\r\n'))
})

test('outer fences outlast arbitrary internal backtick runs; tilde fences and headings stay inside each cell', () => {
  const ticks = '`'.repeat(47)
  const tildes = '~'.repeat(83)
  const original = `${ticks}python\n# 并非导出的表格标题\nprint(1)\n${ticks}\n${tildes}js\nconst x = 2;\n${tildes}`
  const output = formatDayForClipboard(day([[original, '# 右栏标题\n\n```go\npackage main\n```']]))
  const outer = '`'.repeat(48)
  assert.ok(output.includes(`${outer}markdown\n${original}\n${outer}`))
  assert.equal(output.split(outer).length - 1, 2)
  assert.ok(output.includes(tildes))
  assert.ok(output.includes('````markdown\n# 右栏标题\n\n```go\npackage main\n```\n````'))
})

test('a day without content gets an explicit empty-day message and no empty row scaffold', () => {
  for (const rows of [[], [['', '']], [['\t\n', '  '], ['', '　']]]) {
    const output = formatDayForClipboard(day(rows))
    assert.ok(output.includes('暂无内容。'))
    assert.ok(!output.includes('## 第 '))
    assert.ok(!output.includes('（空白）'))
  }
})

test('formatting is deterministic and does not mutate the day, rows, metadata or original cell strings', () => {
  const original = day([['\n  保留前后空白  \n\n', '对应右栏']])
  const before = structuredClone(original)
  original.rows.forEach(Object.freeze)
  Object.freeze(original.rows)
  Object.freeze(original)
  assert.equal(formatDayForClipboard(original), formatDayForClipboard(original))
  assert.deepEqual(original, before)
})
