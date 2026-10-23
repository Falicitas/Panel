import { Crepe } from '@milkdown/crepe'
import { mathPlugins, configureMath } from './dual-notes-math'
import 'katex/dist/katex.min.css'
import { formatDayForClipboard } from './dual-notes-export'
import { codeBlockPlugins, decorateCodePreview } from './dual-notes-code'
import { editorViewCtx, parserCtx } from '@milkdown/kit/core'
import { DOMSerializer, type Node as ProseNode } from '@milkdown/kit/prose/model'
import { Plugin, PluginKey, TextSelection } from '@milkdown/kit/prose/state'
import { Decoration, DecorationSet } from '@milkdown/kit/prose/view'
import { $prose } from '@milkdown/kit/utils'
import { dateKey, dateWindow, DayStore, recoverDay, searchBlocks, type NoteDay, type DayState, type Side, type SearchMatch, type TextBlock } from './dual-notes-core'
import '@milkdown/crepe/theme/common/style.css'
import '@milkdown/crepe/theme/classic.css'
import '@milkdown/crepe/theme/classic-dark.css'
import './dual-notes.css'

type BridgeResponse = { requestId: string; ok: boolean; data?: unknown; error?: string; status?: number }
const native = window as unknown as { webkit?: { messageHandlers?: { dualNotes?: { postMessage(payload: unknown): void } } } }
declare global {
  interface Window {
    panelDualNotes: { receive(message: BridgeResponse): void; openFind(): void; flush(): Promise<void> }
  }
}

const pending = new Map<string, { resolve: (data: unknown) => void; reject: (error: Error) => void; timer: ReturnType<typeof setTimeout> }>()
function request<T>(method: string, payload: Record<string, unknown>): Promise<T> {
  return new Promise((resolve, reject) => {
    const bridge = native.webkit?.messageHandlers?.dualNotes
    if (!bridge) { reject(new Error('尚未连接本地双栏服务')); return }
    const requestId = crypto.randomUUID()
    const timer = setTimeout(() => { pending.delete(requestId); reject(new Error('本地服务响应超时')) }, 20000)
    pending.set(requestId, { resolve: data => resolve(data as T), reject, timer })
    try { bridge.postMessage({ type: 'request', requestId, method, ...payload }) }
    catch (error) { clearTimeout(timer); pending.delete(requestId); reject(error) }
  })
}
function receive(message: BridgeResponse): void {
  const waiter = pending.get(message.requestId)
  if (!waiter) return
  clearTimeout(waiter.timer)
  pending.delete(message.requestId)
  if (message.ok) waiter.resolve(message.data)
  else waiter.reject(Object.assign(new Error(message.error || '本地服务未完成请求'), { status: message.status }))
}

function element<T extends HTMLElement>(id: string): T { return document.getElementById(id) as T }

// The toolbar can wrap in a narrow window or when the native web view is zoomed.
// Keep the sticky find bar directly below its measured height rather than assuming one row.
const topbar = document.querySelector<HTMLElement>('.topbar')!
function updateTopbarOffset(): void {
  document.documentElement.style.setProperty('--dual-topbar-height', `${topbar.getBoundingClientRect().height}px`)
}
new ResizeObserver(updateTopbarOffset).observe(topbar)
updateTopbarOffset()

function node<K extends keyof HTMLElementTagNameMap>(tag: K, className = '', text = ''): HTMLElementTagNameMap[K] {
  const result = document.createElement(tag)
  result.className = className
  if (text) result.textContent = text
  return result
}
function button(text: string, action: () => void): HTMLButtonElement {
  const result = node('button', '', text)
  result.type = 'button'
  result.addEventListener('click', action)
  return result
}
function newRow() { return { id: crypto.randomUUID(), left: '', right: '' } }
const draftPrefix = 'panel.dual-notes.draft.'
const recoveryPrefix = 'panel.dual-notes.recovery.'
let storageFailure = false
let loadFailure: string | null = null
let loading = false
let today = dateKey()
let mode: 'focus' | 'overview' = 'focus'
let page = 0
let visibleDates: string[] = []
let renderGeneration = 0
let parser: ((markdown: string) => ProseNode) | undefined
let conflictDate: string | null = null
let conflictBusy = false
const timers = new Map<string, ReturnType<typeof setTimeout>>()

const focusRatioKey = 'panel.dual-notes.focus-left-ratio'
function normalizedRatio(raw: string | null, fallback = 50): number {
  if (raw === null || !raw.trim()) return fallback
  const value = Number(raw)
  return Number.isFinite(value) ? Math.max(20, Math.min(80, Math.round(value))) : fallback
}
let focusLeftRatio = (() => {
  try { return normalizedRatio(localStorage.getItem(focusRatioKey)) }
  catch { return 50 }
})()

// Column width is presentation state: update CSS only, leaving editor instances and note saves alone.
function applyFocusColumnRatio(syncNumber = false): void {
  const focused = mode === 'focus'
  element('column-ratio-controls').hidden = !focused
  const left = focused ? focusLeftRatio : 50
  element('days').style.setProperty('--dual-left-share', `${left}fr`)
  element('days').style.setProperty('--dual-right-share', `${100 - left}fr`)
  const number = element<HTMLInputElement>('focus-left-ratio')
  if (syncNumber || document.activeElement !== number) number.value = String(focusLeftRatio)
  const slider = element<HTMLInputElement>('focus-ratio-slider')
  slider.value = String(focusLeftRatio)
  slider.setAttribute('aria-valuetext', `左栏 ${focusLeftRatio}％，右栏 ${100 - focusLeftRatio}％`)
  element('focus-right-ratio').textContent = `右栏 ${100 - focusLeftRatio}％`
  element<HTMLButtonElement>('reset-column-ratio').disabled = focusLeftRatio === 50
}
function setFocusColumnRatio(value: number, syncNumber: boolean): void {
  const changed = focusLeftRatio !== value
  focusLeftRatio = value
  applyFocusColumnRatio(syncNumber)
  if (changed) {
    // A display preference failing to persist must not interrupt note editing or draft storage.
    try { localStorage.setItem(focusRatioKey, String(value)) } catch { /* Keep the ratio for this session. */ }
  }
}

function readDraft(date: string): NoteDay | undefined {
  try {
    const value = localStorage.getItem(draftPrefix + date)
    if (!value) return undefined
    const parsed = JSON.parse(value) as NoteDay
    if (parsed.date === date && Number.isInteger(parsed.revision) && Array.isArray(parsed.rows)
      && parsed.rows.every(row => typeof row.id === 'string' && typeof row.left === 'string' && typeof row.right === 'string')) return parsed
    storageFailure = true
  } catch { storageFailure = true }
  return undefined
}
function saveDraft(state: DayState): void {
  try {
    if (state.dirty) localStorage.setItem(draftPrefix + state.day.date, JSON.stringify(state.day))
    else localStorage.removeItem(draftPrefix + state.day.date)
  } catch { storageFailure = true }
}
const store = new DayStore(day => request<NoteDay>('save', { day }), state => {
  saveDraft(state)
  updateErrors()
})

function updateErrors(): void {
  const states = [...store.states.values()]
  const failed = states.filter(state => !!state.error)
  const errors = element('errors')
  errors.replaceChildren()
  if (storageFailure) errors.append(node('div', 'error-line', '本机草稿备份不可用。尚未保存的内容仍在当前页面，请在关闭前重试保存。'))
  if (loadFailure) {
    const line = node('div', 'error-line', loadFailure)
    line.append(button('重新加载', () => void navigate(mode, page)))
    errors.append(line)
  }
  for (const state of failed) {
    const line = node('div', 'error-line')
    line.append(node('span', '', `${state.day.date}　${state.error}`))
    line.append(button(state.conflict ? '查看并重新加载' : '重试保存', () => {
      if (state.conflict) {
        conflictDate = state.day.date
        element<HTMLDialogElement>('conflict-dialog').showModal()
      } else void store.save(state.day.date).catch(() => {})
    }))
    errors.append(line)
  }
  try {
    const backupDates = Object.keys(localStorage).filter(key => key.startsWith(recoveryPrefix)).map(key => key.slice(recoveryPrefix.length)).sort().reverse()
    if (backupDates.length) {
      const line = node('div', 'recovery-line')
      line.append(button(`保留的草稿（${backupDates.length} 天）`, () => showRecoveries(backupDates)))
      errors.append(line)
    }
  } catch { /* The storage failure banner above covers unavailable local storage. */ }
}

function scheduleSave(date: string): void {
  clearTimeout(timers.get(date))
  timers.set(date, setTimeout(() => {
    timers.delete(date)
    void store.save(date).catch(() => {})
  }, 650))
}

interface Cell {
  key: string; date: string; rowId: string; side: Side; root: HTMLElement
  crepe?: Crepe; mounting?: Promise<void>; baseline?: string; alive: boolean
}
const cells = new Map<string, Cell>()
const documents = new Map<string, { markdown: string; doc: ProseNode }>()
const findKey = new PluginKey('dual-notes-find')
let matches: SearchMatch[] = []
let activeMatch = -1
let query = ''
let findTimer: ReturnType<typeof setTimeout> | undefined
let findJumpGeneration = 0
function cellKey(date: string, rowId: string, side: Side): string { return `${date}/${rowId}/${side}` }
function markdownFor(cell: Cell): string { return store.states.get(cell.date)?.day.rows.find(row => row.id === cell.rowId)?.[cell.side] ?? '' }

function changedCell(cell: Cell, markdown: string): void {
  if (!cell.alive || markdown === cell.baseline) return
  cell.baseline = markdown
  if (markdown === markdownFor(cell)) return
  store.edit(cell.date, day => {
    const row = day.rows.find(row => row.id === cell.rowId)
    if (row) row[cell.side] = markdown
  })
  documents.delete(cell.key)
  scheduleSave(cell.date)
  if (query) {
    clearTimeout(findTimer)
    findTimer = setTimeout(() => rebuildFind(false), 150)
  }
}

function searchPlugin(cell: Cell) {
  return $prose(() => new Plugin({
    key: findKey,
    props: {
      decorations(state) {
        if (!query || element('findbar').hidden) return null
        const decorations = matches.flatMap((match, index) => {
          if (cellKey(match.date, match.rowId, match.side) !== cell.key || match.to > state.doc.content.size) return []
          return [Decoration.inline(match.from, match.to, { class: index === activeMatch ? 'find-hit find-hit-active' : 'find-hit' })]
        })
        return DecorationSet.create(state.doc, decorations)
      },
    },
  }))
}

async function mountCell(cell: Cell, focus = false): Promise<void> {
  if (!cell.alive) return
  if (cell.crepe) {
    if (cell.mounting) await cell.mounting
    if (focus && cell.alive) cell.crepe.editor.action(ctx => ctx.get(editorViewCtx).focus())
    return
  }
  const create = async () => {
    const markdown = markdownFor(cell)
    cell.root.replaceChildren()
    cell.root.classList.add('mounting')
    const crepe = new Crepe({
      root: cell.root, defaultValue: markdown,
      features: {
        [Crepe.Feature.CodeMirror]: false,
        // Formula support below preserves the existing native code-block editor.
        [Crepe.Feature.Latex]: false,
        [Crepe.Feature.TopBar]: false,
        [Crepe.Feature.Placeholder]: false,
      },
    })
    cell.crepe = crepe
    crepe.editor.config(configureMath).use(searchPlugin(cell)).use(codeBlockPlugins()).use(mathPlugins())
    await crepe.create()
    if (!cell.alive) { await crepe.destroy(); return }
    cell.baseline = crepe.getMarkdown()
    parser = crepe.editor.action(ctx => ctx.get(parserCtx))
    crepe.on(listener => listener.markdownUpdated((_ctx, next, previous) => {
      if (next !== previous) changedCell(cell, next)
    }))
    const view = crepe.editor.action(ctx => ctx.get(editorViewCtx))
    view.dom.setAttribute('aria-label', `${cell.date} ${cell.side === 'left' ? '不用电脑' : '用电脑 or AI'}`)
    view.dom.setAttribute('spellcheck', 'false')
    cell.root.classList.remove('mounting')
    cell.root.classList.add('mounted')
    if (focus) view.focus()
  }
  cell.mounting = create().catch(error => {
    if (cell.alive) {
      cell.root.replaceChildren(node('p', 'cell-failure', `编辑器未能加载：${String(error)}`), button('重试', () => {
        cell.crepe = undefined
        cell.mounting = undefined
        void mountCell(cell, true)
      }))
    }
  })
  await cell.mounting
}

function docFor(cell: Cell): ProseNode | null {
  if (cell.crepe && cell.root.classList.contains('mounted')) return cell.crepe.editor.action(ctx => ctx.get(editorViewCtx).state.doc)
  const markdown = markdownFor(cell)
  const cached = documents.get(cell.key)
  if (cached?.markdown === markdown) return cached.doc
  if (!parser) return null
  const doc = parser(markdown)
  documents.set(cell.key, { markdown, doc })
  return doc
}

function drawPreview(cell: Cell): void {
  const doc = docFor(cell)
  const preview = node('div', 'cell-preview')
  if (doc) preview.append(DOMSerializer.fromSchema(doc.type.schema).serializeFragment(doc.content))
  else preview.textContent = markdownFor(cell)
  cell.root.replaceChildren(preview)
  void decorateCodePreview(preview)
}
const observer = new IntersectionObserver(entries => {
  for (const entry of entries) {
    if (!entry.isIntersecting) continue
    const cell = cells.get((entry.target as HTMLElement).dataset.cellKey ?? '')
    if (cell) void mountCell(cell)
    observer.unobserve(entry.target)
  }
}, { rootMargin: '120px' })

function makeRow(day: NoteDay, row: NoteDay['rows'][number], index: number): HTMLElement {
  const line = node('div', 'note-row')
  line.dataset.rowId = row.id
  line.setAttribute('role', 'group')
  line.setAttribute('aria-label', `第 ${index + 1} 行`)
  line.append(node('span', 'row-number', String(index + 1).padStart(2, '0')))
  for (const side of ['left', 'right'] as const) {
    const root = node('div', `note-cell ${side}`)
    const key = cellKey(day.date, row.id, side)
    const cell: Cell = { key, date: day.date, rowId: row.id, side, root, alive: true }
    root.dataset.cellKey = key
    root.setAttribute('aria-label', `${day.date} ${side === 'left' ? '不用电脑' : '用电脑 or AI'}`)
    root.addEventListener('pointerdown', () => { if (!cell.crepe) void mountCell(cell, true) })
    cells.set(key, cell)
    drawPreview(cell)
    observer.observe(root)
    line.append(root)
  }
  return line
}

function readableDate(date: string): string {
  const [year, month, day] = date.split('-').map(Number)
  const week = new Date(year, month - 1, day, 12).toLocaleDateString('zh-CN', { weekday: 'long' })
  return `${month} 月 ${day} 日 · ${week}`
}

function dayCopyControl(date: string): { control: HTMLButtonElement; feedback: HTMLElement } {
  const feedback = node('p', 'day-copy-feedback')
  feedback.setAttribute('role', 'status')
  feedback.setAttribute('aria-live', 'polite')
  let reset: ReturnType<typeof setTimeout> | undefined
  const control = button('复制当天', async () => {
    clearTimeout(reset)
    control.disabled = true
    control.textContent = '复制中…'
    feedback.textContent = ''
    try {
      // Capture the last keystroke before formatting; copying works even while saving is offline.
      captureAll()
      const current = store.states.get(date)?.day
      if (!current) throw new Error('这一天的记录尚未加载，请重试。')
      const text = formatDayForClipboard(current)
      await request<{ copied: boolean }>('copy', { text })
      control.textContent = '已复制'
      reset = setTimeout(() => { control.textContent = '复制当天' }, 2000)
    } catch (error) {
      control.textContent = '重新复制'
      feedback.textContent = `复制失败：${(error as Error).message}`
    } finally { control.disabled = false }
  })
  control.classList.add('day-copy')
  control.setAttribute('aria-label', `复制 ${date} 的双栏内容`)
  control.setAttribute('aria-live', 'polite')
  control.title = '复制这一天的全部双栏，按行整理，适合粘贴给 AI。'
  return { control, feedback }
}

function renderDay(day: NoteDay): HTMLElement {
  const section = node('section', 'day-section')
  section.dataset.date = day.date
  const header = node('div', 'day-heading')
  const title = node('h2', '', readableDate(day.date))
  if (day.date === today) title.append(node('span', 'today-tag', '今天'))
  header.append(title, node('span', 'date-year', day.date))
  const copy = dayCopyControl(day.date)
  header.append(copy.control)
  section.append(header, copy.feedback)
  const table = node('div', 'note-table')
  const columns = node('div', 'column-headings')
  columns.append(node('span', '', '不用电脑'), node('span', '', '用电脑 or AI'))
  table.append(columns)
  const rows = node('div', 'rows')
  day.rows.forEach((row, index) => rows.append(makeRow(day, row, index)))
  table.append(rows)
  section.append(table)
  const controls = node('div', 'add-rows')
  const count = node('input')
  count.type = 'number'; count.min = '1'; count.max = '50'; count.value = '3'
  count.setAttribute('aria-label', `${day.date} 要添加的行数`)
  const append = (amount: number) => {
    if (!Number.isInteger(amount) || amount < 1 || amount > 50) { count.setCustomValidity('请输入 1 到 50 之间的整数'); count.reportValidity(); return }
    captureAll()
    const start = day.rows.length
    store.edit(day.date, value => { for (let i = 0; i < amount; i++) value.rows.push(newRow()) })
    day.rows.slice(start).forEach((row, index) => rows.append(makeRow(day, row, start + index)))
    scheduleSave(day.date)
    if (query) rebuildFind(false)
    const first = cells.get(cellKey(day.date, day.rows[start].id, 'left'))
    if (first) { first.root.scrollIntoView({ block: 'nearest' }); void mountCell(first, true) }
  }
  count.addEventListener('input', () => count.setCustomValidity(''))
  controls.append(button('＋ 添加一行', () => append(1)), node('span', 'add-divider'), count, button('添加多行', () => append(Number(count.value))))
  section.append(controls)
  return section
}

function captureAll(): void {
  for (const cell of cells.values()) if (cell.crepe && cell.root.classList.contains('mounted')) changedCell(cell, cell.crepe.getMarkdown())
}
async function flush(): Promise<void> {
  captureAll()
  for (const timer of timers.values()) clearTimeout(timer)
  timers.clear()
  await store.flush()
}

async function disposeCells(): Promise<void> {
  observer.disconnect()
  const old = [...cells.values()]
  for (const cell of old) cell.alive = false
  cells.clear()
  documents.clear()
  parser = undefined
  await Promise.allSettled(old.map(async cell => {
    if (cell.mounting) await cell.mounting
    if (cell.crepe && cell.root.classList.contains('mounted')) await cell.crepe.destroy()
  }))
}

function rangeControls(): void {
  applyFocusColumnRatio()
  element('focus-mode').setAttribute('aria-pressed', String(mode === 'focus'))
  element('overview-mode').setAttribute('aria-pressed', String(mode === 'overview'))
  element('rangebar').classList.toggle('focus-range', mode === 'focus')
  element('range-label').textContent = mode === 'focus' ? `${today} · 只看今天` : `${visibleDates[visibleDates.length - 1]} — ${visibleDates[0]}`
  element<HTMLButtonElement>('newer-page').disabled = page === 0 || loading
  element<HTMLButtonElement>('today-page').disabled = page === 0 || loading
  element<HTMLButtonElement>('older-page').disabled = loading
  element<HTMLInputElement>('find-input').placeholder = mode === 'focus' ? '查找今天的双栏' : '查找当前 10 天'
}

async function navigate(nextMode: 'focus' | 'overview', nextPage: number): Promise<void> {
  captureAll()
  void store.flush().catch(() => {})
  const generation = ++renderGeneration
  mode = nextMode
  page = Math.max(0, nextPage)
  visibleDates = dateWindow(today, mode, page)
  const dates = [...visibleDates]
  loading = true
  element('days').inert = true
  loadFailure = null
  matches = []; activeMatch = -1
  findJumpGeneration++
  rangeControls(); updateErrors(); updateFindUI()
  try {
    const result = await request<{ days: NoteDay[] }>('load', { end: dates[0], days: dates.length })
    if (generation !== renderGeneration) return
    if (!Array.isArray(result.days)) throw new Error('本地服务返回了无效的日期数据')
    const records = dates.map(date => {
      const remote = result.days.find(day => day.date === date) ?? { date, revision: 0, rows: [] }
      const state = store.register(remote, readDraft(date))
      if (!state.day.rows.length) state.day.rows = [newRow()]
      return state.day
    })
    await disposeCells()
    if (generation !== renderGeneration) return
    const main = element('days')
    main.replaceChildren(...records.map(renderDay))
    const first = cells.values().next().value as Cell | undefined
    if (first) await mountCell(first)
    if (generation !== renderGeneration) return
    for (const cell of cells.values()) if (!cell.crepe) drawPreview(cell)
    loading = false
    element('days').inert = false
    rangeControls(); updateErrors(); rebuildFind(false)
    window.scrollTo({ top: 0 })
    for (const date of dates) {
      const state = store.states.get(date)!
      if (state.dirty && !state.conflict) scheduleSave(date)
    }
  } catch (error) {
    if (generation !== renderGeneration) return
    loading = false
    loadFailure = `未能加载${mode === 'focus' ? '今天' : '这 10 天'}的记录：${(error as Error).message}。`
    // A failed range must never show the previous range under a new date heading.
    await disposeCells()
    if (generation !== renderGeneration) return
    element('days').inert = false
    element('days').replaceChildren(node('p', 'empty-message', '记录尚未加载，请重试。已有修改仍保留。'))
    rangeControls(); updateErrors()
  }
}

function textBlocks(doc: ProseNode): TextBlock[] {
  const blocks: TextBlock[] = []
  doc.descendants((block, blockPosition) => {
    if (!block.isTextblock) return true
    let text = ''
    const positions: number[] = []
    block.descendants((child, position) => {
      if (child.isText) {
        text += child.text!
        for (let i = 0; i < child.text!.length; i++) positions.push(blockPosition + 1 + position + i)
      } else if (child.isLeaf) {
        text += child.type.name === 'hardbreak' ? '\n' : '\uFFFC'
        positions.push(blockPosition + 1 + position)
      }
    })
    blocks.push({ text, positions })
    return false
  })
  return blocks
}

function refreshDecorations(): void {
  for (const cell of cells.values()) {
    if (!cell.crepe || !cell.root.classList.contains('mounted')) continue
    cell.crepe.editor.action(ctx => {
      const view = ctx.get(editorViewCtx)
      view.dispatch(view.state.tr.setMeta(findKey, true))
    })
  }
}
function updateFindUI(): void {
  element('find-count').textContent = loading ? '正在加载范围…' : !query ? '输入要查找的内容' : !matches.length ? '没有匹配' : `${activeMatch + 1} / ${matches.length}`
  element<HTMLButtonElement>('find-prev').disabled = matches.length === 0
  element<HTMLButtonElement>('find-next').disabled = matches.length === 0
  const current = matches[activeMatch]
  element('find-location').textContent = current ? `${current.date} · ${current.side === 'left' ? '不用电脑' : '用电脑 or AI'}　${current.excerpt}` : mode === 'focus' ? '范围：今天' : '范围：当前页的 10 天'
}
function rebuildFind(jump = true): void {
  findJumpGeneration++
  captureAll()
  const previous = matches[activeMatch]
  query = element<HTMLInputElement>('find-input').value
  matches = []
  if (query && !element('findbar').hidden && !loading) {
    for (const cell of cells.values()) {
      if (!visibleDates.includes(cell.date)) continue
      const doc = docFor(cell)
      if (doc) for (const found of searchBlocks(textBlocks(doc), query)) matches.push({ ...found, date: cell.date, rowId: cell.rowId, side: cell.side })
    }
  }
  const preserved = previous && matches.findIndex(match => match.date === previous.date && match.rowId === previous.rowId && match.side === previous.side && match.from === previous.from)
  activeMatch = matches.length ? Math.max(0, preserved ?? 0) : -1
  updateFindUI(); refreshDecorations()
  if (jump && matches.length) void jumpToMatch(activeMatch)
}
async function jumpToMatch(index: number): Promise<void> {
  if (!matches.length) return
  const generation = ++findJumpGeneration
  activeMatch = (index + matches.length) % matches.length
  const match = matches[activeMatch]
  const cell = cells.get(cellKey(match.date, match.rowId, match.side))
  updateFindUI(); refreshDecorations()
  if (!cell) return
  await mountCell(cell)
  if (generation !== findJumpGeneration || !cell.alive || !cell.root.classList.contains('mounted')) return
  refreshDecorations()
  cell.crepe!.editor.action(ctx => {
    const view = ctx.get(editorViewCtx)
    if (match.to > view.state.doc.content.size) return
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, match.from, match.to)))
    const hit = cell.root.querySelector('.find-hit-active')
    ;(hit ?? cell.root).scrollIntoView({ block: 'center', behavior: 'smooth' })
  })
}
function openFind(): void {
  element('findbar').hidden = false
  const input = element<HTMLInputElement>('find-input')
  input.focus(); input.select()
  rebuildFind(false)
}
function closeFind(): void {
  element('findbar').hidden = true
  query = ''
  matches = []; activeMatch = -1
  findJumpGeneration++
  refreshDecorations()
}

function showRecoveries(dates: string[]): void {
  const content = element('recovery-content')
  content.replaceChildren()
  for (const date of dates) {
    try {
      const versions = JSON.parse(localStorage.getItem(recoveryPrefix + date) || '[]') as { at: string; day: NoteDay }[]
      for (const version of versions.slice().reverse()) {
        const section = node('section', 'recovery-day')
        section.append(node('h3', '', `${date} · ${version.at}`))
        version.day.rows.forEach((row, index) => {
          for (const side of ['left', 'right'] as const) {
            section.append(node('h4', '', `第 ${index + 1} 行 · ${side === 'left' ? '不用电脑' : '用电脑 or AI'}`))
            const text = node('textarea')
            text.readOnly = true; text.value = row[side]; text.rows = Math.min(12, Math.max(3, row[side].split('\n').length))
            section.append(text)
          }
        })
        content.append(section)
      }
    } catch { content.append(node('p', '', `${date} 的备份无法读取。`)) }
  }
  element<HTMLDialogElement>('recovery-dialog').showModal()
}

element('conflict-reload').addEventListener('click', async () => {
  if (!conflictDate || conflictBusy) return
  const targetDate = conflictDate
  captureAll()
  const state = store.states.get(targetDate)
  if (!state) return
  const control = element<HTMLButtonElement>('conflict-reload')
  control.disabled = true
  conflictBusy = true
  element<HTMLButtonElement>('conflict-cancel').disabled = true
  try {
    await recoverDay(store, targetDate, draft => {
      try {
        const key = recoveryPrefix + targetDate
        const old = JSON.parse(localStorage.getItem(key) || '[]')
        old.push({ at: new Date().toLocaleString('zh-CN'), day: draft })
        localStorage.setItem(key, JSON.stringify(old))
      } catch (error) { storageFailure = true; throw error }
    }, async date => {
      const result = await request<{ days: NoteDay[] }>('load', { end: date, days: 1 })
      return result.days.find(day => day.date === date) ?? { date, revision: 0, rows: [] }
    }, async () => {
      // Keep the live editors available if preserving the final input fails.
      captureAll()
    })
    // Replacement is complete. Do not capture the old editors again into the new state.
    await disposeCells()
    element<HTMLDialogElement>('conflict-dialog').close()
    conflictDate = null
    await navigate(mode, page)
  } catch (error) {
    element('notice').hidden = false
    element('notice').textContent = `未能重新加载，当前草稿仍保留：${(error as Error).message}`
    updateErrors()
  } finally {
    control.disabled = false
    conflictBusy = false
    element<HTMLButtonElement>('conflict-cancel').disabled = false
  }
})
element('conflict-cancel').addEventListener('click', () => { if (!conflictBusy) element<HTMLDialogElement>('conflict-dialog').close() })
element('conflict-dialog').addEventListener('cancel', event => { if (conflictBusy) event.preventDefault() })
element('recovery-close').addEventListener('click', () => element<HTMLDialogElement>('recovery-dialog').close())
element('focus-mode').addEventListener('click', () => { if (mode !== 'focus') void navigate('focus', 0) })
const ratioNumber = element<HTMLInputElement>('focus-left-ratio')
ratioNumber.addEventListener('input', () => {
  const value = Number(ratioNumber.value)
  // Let users clear the field or type an intermediate value before correcting on blur/Enter.
  if (ratioNumber.value.trim() && Number.isInteger(value) && value >= 20 && value <= 80) setFocusColumnRatio(value, false)
})
const commitRatioNumber = () => setFocusColumnRatio(normalizedRatio(ratioNumber.value, focusLeftRatio), true)
ratioNumber.addEventListener('blur', commitRatioNumber)
ratioNumber.addEventListener('keydown', event => {
  if (event.key === 'Enter') { event.preventDefault(); commitRatioNumber() }
})
element<HTMLInputElement>('focus-ratio-slider').addEventListener('input', event => {
  setFocusColumnRatio(normalizedRatio((event.target as HTMLInputElement).value, focusLeftRatio), true)
})
element('reset-column-ratio').addEventListener('click', () => setFocusColumnRatio(50, true))
element('overview-mode').addEventListener('click', () => { if (mode !== 'overview') void navigate('overview', 0) })
element('older-page').addEventListener('click', () => void navigate('overview', page + 1))
element('newer-page').addEventListener('click', () => void navigate('overview', page - 1))
element('today-page').addEventListener('click', () => void navigate('overview', 0))
element('find-toggle').addEventListener('click', openFind)
element('find-close').addEventListener('click', closeFind)
element('find-prev').addEventListener('click', () => void jumpToMatch(activeMatch - 1))
element('find-next').addEventListener('click', () => void jumpToMatch(activeMatch + 1))
element<HTMLInputElement>('find-input').addEventListener('input', () => rebuildFind())
element<HTMLInputElement>('find-input').addEventListener('keydown', event => {
  if (event.key === 'Enter') { event.preventDefault(); void jumpToMatch(activeMatch + (event.shiftKey ? -1 : 1)) }
})
document.addEventListener('keydown', event => {
  if (event.metaKey && !event.altKey && event.key.toLowerCase() === 'f') { event.preventDefault(); event.stopPropagation(); openFind() }
  if (event.key === 'Escape' && !element('findbar').hidden) { event.preventDefault(); closeFind() }
}, true)

function checkToday(): void {
  const current = dateKey()
  if (current === today) return
  today = current
  element('notice').textContent = '已经是新的一天。昨天的修改已保留，日期范围已更新。'
  element('notice').hidden = false
  void navigate(mode, page)
}
setInterval(checkToday, 30000)
window.addEventListener('focus', checkToday)
document.addEventListener('visibilitychange', () => {
  if (document.hidden) void flush().catch(() => {})
  else checkToday()
})
window.addEventListener('pagehide', () => { captureAll(); void store.flush().catch(() => {}) })
window.panelDualNotes = { receive, openFind, flush }
void navigate('focus', 0)
