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
import { monthKey, shiftMonth, validMonth, validWorkspace, WorkspaceStore, recoverWorkspace, type MonthWorkspace, type WorkspaceTable, type WorkspaceState } from './short-term-workspace-core'
import { WorkspaceCopier, validWorkspaceCopy, type WorkspaceCopyRequest, type WorkspaceCopyResult } from './short-term-workspace-copy'
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
const workspaceDraftPrefix = 'panel.short-term-workspace.draft.'
const workspaceRecoveryPrefix = 'panel.short-term-workspace.recovery.'
let storageFailure = false
let loadFailure: string | null = null
let loading = false
let today = dateKey()
type Mode = 'focus' | 'overview' | 'workspace'
let mode: Mode = 'focus'
let selectedMonth = monthKey()
let page = 0
let visibleDates: string[] = []
let renderGeneration = 0
let parser: ((markdown: string) => ProseNode) | undefined
let conflictDate: string | null = null
let conflictMonth: string | null = null
let conflictBusy = false
let workspaceOrder: { month: string; ids: string[] } | null = null
let draggedWorkspaceTable: string | null = null
type MigrationChoice = { id: string; header: string }
let workspaceMigration: { sourceMonth: string; targetMonth: string; choices: MigrationChoice[]; selected: Set<string> } | null = null
let migrationBusy = false
const pendingCopyKey = 'panel.short-term-workspace.pending-copy'
let rememberedCopyHeaders: MigrationChoice[] = []
let copyRecoveryError: string | null = null
let workspaceCopier: WorkspaceCopier
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
  element('days').style.setProperty('--dual-left-share', `${left}%`)
  element('days').style.setProperty('--dual-right-share', `${100 - left}%`)
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

function readWorkspaceDraft(month: string): MonthWorkspace | undefined {
  try {
    const value = localStorage.getItem(workspaceDraftPrefix + month)
    if (!value) return undefined
    const parsed: unknown = JSON.parse(value)
    if (validWorkspace(parsed, month)) return parsed
    storageFailure = true
  } catch { storageFailure = true }
  return undefined
}
function saveWorkspaceDraft(state: WorkspaceState): void {
  try {
    const key = workspaceDraftPrefix + state.workspace.month
    if (state.dirty) localStorage.setItem(key, JSON.stringify(state.workspace))
    else localStorage.removeItem(key)
  } catch { storageFailure = true }
}
const workspaceStore = new WorkspaceStore(workspace => {
  if (workspaceCopier?.pending?.targetMonth === workspace.month) return Promise.reject(new Error('请先确认上次迁移结果，再编辑目标月份。'))
  return request<MonthWorkspace>('saveWorkspace', { workspace })
}, state => {
  saveWorkspaceDraft(state)
  updateErrors()
})
function readPendingCopy(): WorkspaceCopyRequest | null {
  try {
    const raw = localStorage.getItem(pendingCopyKey)
    if (!raw) return null
    const parsed = JSON.parse(raw)
    if (!validWorkspaceCopy(parsed.request)) throw new Error('无法读取上次迁移的请求。')
    rememberedCopyHeaders = Array.isArray(parsed.headers) ? parsed.headers.filter((item: MigrationChoice) => item && typeof item.id === 'string' && typeof item.header === 'string') : []
    return parsed.request
  } catch {
    copyRecoveryError = '无法读取上次迁移状态，请保留当前页面，暂勿再次迁移。'
    return null
  }
}
function rememberCopy(copy: WorkspaceCopyRequest | null): void {
  try {
    if (copy) {
      const source = workspaceStore.states.get(copy.sourceMonth)?.workspace
      const headers = copy.tableIds.map(id => ({ id, header: source?.tables.find(table => table.id === id)?.header ?? rememberedCopyHeaders.find(table => table.id === id)?.header ?? '' }))
      localStorage.setItem(pendingCopyKey, JSON.stringify({ request: copy, headers }))
      rememberedCopyHeaders = headers
    } else {
      localStorage.removeItem(pendingCopyKey)
      rememberedCopyHeaders = []
    }
  } catch { storageFailure = true; throw new Error('无法保留迁移状态，请恢复本机存储后重试。') }
}
workspaceCopier = new WorkspaceCopier(workspaceStore,
  month => request<MonthWorkspace>('loadWorkspace', { month }), readWorkspaceDraft,
  copy => request<WorkspaceCopyResult>('copyWorkspaceTables', { copy }), rememberCopy, readPendingCopy())
function scheduleWorkspaceSave(month: string): void {
  const key = 'month/' + month
  clearTimeout(timers.get(key))
  timers.set(key, setTimeout(() => {
    timers.delete(key)
    void workspaceStore.save(month).catch(() => {})
  }, 650))
}
async function flushStores(): Promise<void> {
  const results = await Promise.allSettled([store.flush(), workspaceStore.flush()])
  if (results.some(result => result.status === 'rejected')) throw new Error('仍有草稿未保存，请查看页面中的提示。')
}

function updateErrors(): void {
  const states = [...store.states.values()]
  const failed = states.filter(state => !!state.error)
  const errors = element('errors')
  errors.replaceChildren()
  if (copyRecoveryError) errors.append(node('div', 'error-line', copyRecoveryError))
  if (workspaceCopier?.pending) {
    const copy = workspaceCopier.pending
    const line = node('div', 'error-line')
    line.append(node('span', '', `从 ${copy.sourceMonth} 到 ${copy.targetMonth} 的迁移结果尚未确认，目标月份暂时不可编辑。`))
    const retry = button('重试确认', () => openWorkspaceMigration(copy.sourceMonth))
    retry.disabled = migrationBusy
    line.append(retry)
    errors.append(line)
  }
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
        conflictMonth = null
        element<HTMLDialogElement>('conflict-dialog').showModal()
      } else void store.save(state.day.date).catch(() => {})
    }))
    errors.append(line)
  }
  for (const state of workspaceStore.states.values()) {
    if (!state.error) continue
    const line = node('div', 'error-line')
    line.append(node('span', '', `${state.workspace.month}　${state.error}`))
    line.append(button(state.conflict ? '查看并重新加载' : '重试保存', () => {
      if (state.conflict) {
        conflictDate = null
        conflictMonth = state.workspace.month
        element<HTMLDialogElement>('conflict-dialog').showModal()
      } else void workspaceStore.save(state.workspace.month).catch(() => {})
    }))
    errors.append(line)
  }
  try {
    const months = Object.keys(localStorage).filter(key => key.startsWith(workspaceRecoveryPrefix)).map(key => key.slice(workspaceRecoveryPrefix.length)).sort().reverse()
    if (months.length) {
      const line = node('div', 'recovery-line')
      line.append(button(`保留的月度草稿（${months.length} 个月）`, () => showWorkspaceRecoveries(months)))
      errors.append(line)
    }
  } catch { /* Storage failure is reported above. */ }
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
  workspace?: { month: string; tableId: string }
  crepe?: Crepe; mounting?: Promise<void>; baseline?: string; alive: boolean
}
const cells = new Map<string, Cell>()
const documents = new Map<string, { markdown: string; doc: ProseNode }>()
const findKey = new PluginKey('dual-notes-find')
let matches: (SearchMatch & { cellKey: string; label: string })[] = []
let activeMatch = -1
let query = ''
let findTimer: ReturnType<typeof setTimeout> | undefined
let findJumpGeneration = 0
function cellKey(date: string, rowId: string, side: Side): string { return `${date}/${rowId}/${side}` }
function workspaceTableFor(cell: Cell): WorkspaceTable | undefined {
  return cell.workspace && workspaceStore.states.get(cell.workspace.month)?.workspace.tables.find(table => table.id === cell.workspace!.tableId)
}
function cellLabel(cell: Cell): string {
  return cell.workspace ? `${cell.workspace.month} · ${workspaceTableFor(cell)?.header || '未命名表'} · 第 ${Number(cell.root.parentElement?.dataset.rowIndex ?? 0) + 1} 行`
    : `${cell.date} · ${cell.side === 'left' ? '不用电脑' : '用电脑 or AI'}`
}
function markdownFor(cell: Cell): string {
  if (cell.workspace) return workspaceTableFor(cell)?.rows.find(row => row.id === cell.rowId)?.content ?? ''
  return store.states.get(cell.date)?.day.rows.find(row => row.id === cell.rowId)?.[cell.side] ?? ''
}

function changedCell(cell: Cell, markdown: string): void {
  if (!cell.alive || markdown === cell.baseline) return
  cell.baseline = markdown
  if (markdown === markdownFor(cell)) return
  if (cell.workspace) {
    workspaceStore.edit(cell.workspace.month, workspace => {
      const row = workspace.tables.find(table => table.id === cell.workspace!.tableId)?.rows.find(row => row.id === cell.rowId)
      if (row) row.content = markdown
    })
    scheduleWorkspaceSave(cell.workspace.month)
  } else {
    store.edit(cell.date, day => {
      const row = day.rows.find(row => row.id === cell.rowId)
      if (row) row[cell.side] = markdown
    })
    scheduleSave(cell.date)
  }
  documents.delete(cell.key)
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
          if (match.cellKey !== cell.key || match.to > state.doc.content.size) return []
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
      // Virtual cursors use the page-wide selection and can retain another
      // cell's coordinates after resizing. Keep the native caret in workspaces.
      featureConfigs: cell.workspace ? { [Crepe.Feature.Cursor]: { virtual: false } } : undefined,
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
    view.dom.setAttribute('aria-label', cellLabel(cell))
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

function newWorkspaceTable(): WorkspaceTable {
  return { id: crypto.randomUUID(), header: '', rows: [{ id: crypto.randomUUID(), content: '' }] }
}
function workspaceCellKey(month: string, tableId: string, rowId: string): string { return `month/${month}/${tableId}/${rowId}` }
function makeWorkspaceRow(month: string, tableId: string, row: WorkspaceTable['rows'][number], index: number): HTMLElement {
  const line = node('div', 'note-row')
  line.dataset.rowId = row.id
  line.dataset.rowIndex = String(index)
  line.setAttribute('role', 'group')
  line.setAttribute('aria-label', `第 ${index + 1} 行`)
  line.append(node('span', 'row-number', String(index + 1).padStart(2, '0')))
  const root = node('div', 'note-cell single')
  const key = workspaceCellKey(month, tableId, row.id)
  const cell: Cell = { key, date: month, rowId: row.id, side: 'left', workspace: { month, tableId }, root, alive: true }
  root.dataset.cellKey = key
  root.addEventListener('pointerdown', () => { if (!cell.crepe) void mountCell(cell, true) })
  cells.set(key, cell)
  line.append(root)
  root.setAttribute('aria-label', cellLabel(cell))
  drawPreview(cell)
  observer.observe(root)
  return line
}
function workspaceNotice(text: string): void {
  element('notice').textContent = text
  element('notice').hidden = false
}
function renderWorkspaceTable(workspace: MonthWorkspace, table: WorkspaceTable): HTMLElement {
  const section = node('section', 'workspace-table')
  section.dataset.tableId = table.id
  const grid = node('div', 'note-table')
  const columns = node('div', 'column-headings single-column')
  const header = node('input', 'workspace-column-header')
  header.type = 'text'; header.value = table.header; header.placeholder = '填写列标题'
  header.setAttribute('aria-label', `${workspace.month} 第 ${workspace.tables.indexOf(table) + 1} 张表的列标题`)
  header.addEventListener('input', () => {
    workspaceStore.edit(workspace.month, value => { value.tables.find(item => item.id === table.id)!.header = header.value })
    scheduleWorkspaceSave(workspace.month)
    for (const cell of cells.values()) {
      if (cell.workspace?.month !== workspace.month || cell.workspace.tableId !== table.id) continue
      cell.root.setAttribute('aria-label', cellLabel(cell))
      cell.root.querySelector('.ProseMirror')?.setAttribute('aria-label', cellLabel(cell))
    }
    if (query) rebuildFind(false)
  })
  columns.append(header)
  const rows = node('div', 'rows')
  table.rows.forEach((row, index) => rows.append(makeWorkspaceRow(workspace.month, table.id, row, index)))
  grid.append(columns, rows)
  const controls = node('div', 'add-rows')
  const count = node('input')
  count.type = 'number'; count.min = '1'; count.max = '50'; count.value = '3'
  count.setAttribute('aria-label', `${workspace.month} 第 ${workspace.tables.indexOf(table) + 1} 张表要添加的行数`)
  count.addEventListener('input', () => count.setCustomValidity(''))
  const append = (amount: number) => {
    if (!Number.isInteger(amount) || amount < 1 || amount > 50) { count.setCustomValidity('请输入 1 到 50 之间的整数'); count.reportValidity(); return }
    if (workspace.tables.reduce((sum, item) => sum + item.rows.length, 0) + amount > 1000) { workspaceNotice('每月最多添加 1000 行。'); return }
    captureAll()
    const start = table.rows.length
    workspaceStore.edit(workspace.month, () => {
      for (let i = 0; i < amount; i++) table.rows.push({ id: crypto.randomUUID(), content: '' })
    })
    table.rows.slice(start).forEach((row, index) => rows.append(makeWorkspaceRow(workspace.month, table.id, row, start + index)))
    scheduleWorkspaceSave(workspace.month)
    if (query) rebuildFind(false)
    const first = cells.get(workspaceCellKey(workspace.month, table.id, table.rows[start].id))
    if (first) { first.root.scrollIntoView({ block: 'nearest' }); void mountCell(first, true) }
  }
  controls.append(button('＋ 添加一行', () => append(1)), node('span', 'add-divider'), count, button('添加多行', () => append(Number(count.value))))
  section.append(grid, controls)
  return section
}
function renderWorkspace(workspace: MonthWorkspace): void {
  const main = element('days')
  const heading = node('div', 'day-heading workspace-heading')
  const [year, month] = workspace.month.split('-').map(Number)
  const title = node('h2', '', `${year} 年 ${month} 月`)
  if (workspace.month === monthKey()) title.append(node('span', 'today-tag', '本月'))
  const reorder = button('调整表顺序', () => openWorkspaceOrder(workspace.month))
  reorder.id = 'reorder-workspace-tables'
  heading.append(title, reorder)
  if (workspace.month !== monthKey()) {
    const migrate = button('迁移至最新月', () => openWorkspaceMigration(workspace.month))
    migrate.id = 'migrate-workspace-tables'
    heading.append(migrate)
  }
  const tables = node('div', 'workspace-tables')
  workspace.tables.forEach(table => tables.append(renderWorkspaceTable(workspace, table)))
  const add = button('＋ 添加表', () => {
    if (workspace.tables.length >= 100) { workspaceNotice('每月最多添加 100 张表。'); return }
    if (workspace.tables.reduce((sum, table) => sum + table.rows.length, 0) >= 1000) { workspaceNotice('每月最多添加 1000 行。'); return }
    captureAll()
    const table = newWorkspaceTable()
    workspaceStore.edit(workspace.month, value => value.tables.push(table))
    const section = renderWorkspaceTable(workspace, table)
    tables.append(section)
    scheduleWorkspaceSave(workspace.month)
    section.scrollIntoView({ block: 'nearest' })
    section.querySelector<HTMLInputElement>('.workspace-column-header')!.focus()
  })
  add.id = 'add-workspace-table'
  main.replaceChildren(heading, tables, add)
}

function updateWorkspaceLock(): void {
  element('days').inert = loading || migrationBusy || (mode === 'workspace' && workspaceCopier.pending?.targetMonth === selectedMonth)
}
function openWorkspaceMigration(sourceMonth: string): void {
  if (migrationBusy || loading) return
  if (copyRecoveryError) { workspaceNotice(copyRecoveryError); return }
  captureAll()
  closeWorkspaceOrder()
  const pending = workspaceCopier.pending
  if (pending) {
    workspaceMigration = {
      sourceMonth: pending.sourceMonth, targetMonth: pending.targetMonth,
      choices: pending.tableIds.map(id => rememberedCopyHeaders.find(table => table.id === id) ?? { id, header: '' }),
      selected: new Set(pending.tableIds),
    }
  } else {
    if (sourceMonth === monthKey()) return
    const source = workspaceStore.states.get(sourceMonth)?.workspace
    if (!source) return
    workspaceMigration = { sourceMonth, targetMonth: monthKey(), choices: source.tables.map(table => ({ id: table.id, header: table.header })), selected: new Set() }
  }
  const migration = workspaceMigration
  element('workspace-migrate-destination').textContent = `${migration.sourceMonth} → ${migration.targetMonth}，追加到末尾，原月份保留。`
  element('workspace-migrate-error').hidden = true
  const list = element('workspace-migrate-list')
  list.replaceChildren(...migration.choices.map(table => {
    const item = node('li')
    const label = node('label', 'workspace-migrate-item')
    const input = node('input')
    input.type = 'checkbox'; input.dataset.tableId = table.id
    input.checked = migration.selected.has(table.id)
    input.addEventListener('change', () => {
      if (input.checked) migration.selected.add(table.id)
      else migration.selected.delete(table.id)
      updateMigrationControls()
    })
    const title = node('span', 'workspace-migrate-title', table.header.trim() || '未命名表')
    title.title = table.header
    label.append(input, title); item.append(label)
    return item
  }))
  updateMigrationControls()
  element<HTMLDialogElement>('workspace-migrate-dialog').showModal()
}
function updateMigrationControls(): void {
  const frozen = migrationBusy || !!workspaceCopier.pending
  element('workspace-migrate-list').querySelectorAll<HTMLInputElement>('input').forEach(input => { input.disabled = frozen })
  element<HTMLButtonElement>('workspace-migrate-all').disabled = frozen
  element<HTMLButtonElement>('workspace-migrate-none').disabled = frozen
  const apply = element<HTMLButtonElement>('workspace-migrate-apply')
  apply.disabled = migrationBusy || !workspaceMigration?.selected.size
  apply.textContent = migrationBusy ? '迁移中…' : workspaceCopier.pending ? '重试确认' : `迁移所选表${workspaceMigration?.selected.size ? `（${workspaceMigration.selected.size}）` : ''}`
  const cancel = element<HTMLButtonElement>('workspace-migrate-cancel')
  cancel.disabled = migrationBusy
  cancel.textContent = workspaceCopier.pending ? '稍后重试' : '取消'
}
function closeWorkspaceMigration(): void {
  if (migrationBusy) return
  element<HTMLDialogElement>('workspace-migrate-dialog').close()
  workspaceMigration = null
}
function selectMigrationTables(all: boolean): void {
  if (!workspaceMigration || migrationBusy || workspaceCopier.pending) return
  workspaceMigration.selected = new Set(all ? workspaceMigration.choices.map(table => table.id) : [])
  element('workspace-migrate-list').querySelectorAll<HTMLInputElement>('input').forEach(input => { input.checked = all })
  updateMigrationControls()
}
async function applyWorkspaceMigration(): Promise<void> {
  const migration = workspaceMigration
  if (!migration || migrationBusy || !migration.selected.size) return
  if (!workspaceCopier.pending && migration.targetMonth !== monthKey()) {
    closeWorkspaceMigration()
    openWorkspaceMigration(migration.sourceMonth)
    return
  }
  captureAll()
  migrationBusy = true
  updateWorkspaceLock(); updateMigrationControls()
  element('workspace-migrate-error').hidden = true
  try {
    const result = await workspaceCopier.copy(migration.sourceMonth, migration.targetMonth, [...migration.selected])
    migrationBusy = false
    closeWorkspaceMigration()
    await navigate(mode, page)
    const notice = element('notice')
    notice.hidden = false
    notice.replaceChildren(node('span', '', `已将 ${result.copiedTableIds.length} 张表复制到 ${result.target.month} 末尾，原月份已保留。`))
    notice.append(button('查看本月', () => void navigate('workspace', 0, result.target.month)))
  } catch (error) {
    const failure = error as Error & { status?: number }
    const text = workspaceCopier.pending ? `结果尚未确认，请重试；同一次迁移不会重复追加。${failure.message}` : `未完成迁移：${failure.message}`
    element('workspace-migrate-error').textContent = text
    element('workspace-migrate-error').hidden = false
    if (failure.status === 409 && !workspaceCopier.pending) {
      migrationBusy = false
      closeWorkspaceMigration()
      await navigate(mode, page)
      workspaceNotice('月份内容已更新，请检查内容和保存提示后再迁移。')
    }
  } finally {
    migrationBusy = false
    updateMigrationControls(); updateWorkspaceLock(); updateErrors()
  }
}
element('workspace-migrate-apply').addEventListener('click', () => void applyWorkspaceMigration())
element('workspace-migrate-cancel').addEventListener('click', closeWorkspaceMigration)
element('workspace-migrate-all').addEventListener('click', () => selectMigrationTables(true))
element('workspace-migrate-none').addEventListener('click', () => selectMigrationTables(false))
element('workspace-migrate-dialog').addEventListener('cancel', event => { event.preventDefault(); closeWorkspaceMigration() })

function openWorkspaceOrder(month: string): void {
  captureAll()
  const workspace = workspaceStore.states.get(month)?.workspace
  if (!workspace || loading || mode !== 'workspace') return
  workspaceOrder = { month, ids: workspace.tables.map(table => table.id) }
  renderWorkspaceOrder()
  element<HTMLDialogElement>('workspace-order-dialog').showModal()
}
function closeWorkspaceOrder(): void {
  element<HTMLDialogElement>('workspace-order-dialog').close()
  workspaceOrder = null
  draggedWorkspaceTable = null
}
function moveWorkspaceTable(id: string, target: number, direction?: 'up' | 'down'): void {
  if (!workspaceOrder) return
  const from = workspaceOrder.ids.indexOf(id)
  if (from < 0 || target < 0 || target >= workspaceOrder.ids.length || from === target) return
  workspaceOrder.ids.splice(from, 1)
  workspaceOrder.ids.splice(target, 0, id)
  renderWorkspaceOrder()
  const item = [...element('workspace-order-list').children].find(row => (row as HTMLElement).dataset.tableId === id) as HTMLElement | undefined
  if (direction) {
    const preferred = item?.querySelector<HTMLButtonElement>(`button[data-direction="${direction}"]`)
    const fallback = item?.querySelector<HTMLButtonElement>('button:not(:disabled)')
    ;(preferred?.disabled ? fallback : preferred)?.focus({ preventScroll: true })
  }
  item?.scrollIntoView({ block: 'nearest' })
}
function clearWorkspaceDropTargets(): void {
  element('workspace-order-list').querySelectorAll('.drop-before, .drop-after').forEach(item => item.classList.remove('drop-before', 'drop-after'))
}
function renderWorkspaceOrder(): void {
  if (!workspaceOrder) return
  const tables = workspaceStore.states.get(workspaceOrder.month)?.workspace.tables ?? []
  const list = element('workspace-order-list')
  list.replaceChildren(...workspaceOrder.ids.map((id, index) => {
    const table = tables.find(table => table.id === id)!
    const label = table.header.trim() || '未命名表'
    const item = node('li', 'workspace-order-item')
    item.dataset.tableId = id
    item.draggable = true
    const grip = node('span', 'workspace-order-grip', '⠿')
    grip.setAttribute('aria-hidden', 'true')
    const title = node('span', 'workspace-order-title', label)
    title.title = label
    const up = button('↑', () => moveWorkspaceTable(id, index - 1, 'up'))
    const down = button('↓', () => moveWorkspaceTable(id, index + 1, 'down'))
    up.dataset.direction = 'up'; down.dataset.direction = 'down'
    up.disabled = index === 0; down.disabled = index === workspaceOrder!.ids.length - 1
    up.title = '上移'; down.title = '下移'
    up.setAttribute('aria-label', `上移 ${label}`); down.setAttribute('aria-label', `下移 ${label}`)
    item.append(grip, title, up, down)
    item.addEventListener('dragstart', event => {
      draggedWorkspaceTable = id
      if (event.dataTransfer) { event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('text/plain', id) }
      item.classList.add('dragging')
    })
    item.addEventListener('dragover', event => {
      if (!draggedWorkspaceTable || draggedWorkspaceTable === id) return
      event.preventDefault()
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
      clearWorkspaceDropTargets()
      const bounds = item.getBoundingClientRect()
      item.classList.add(event.clientY < bounds.top + bounds.height / 2 ? 'drop-before' : 'drop-after')
    })
    item.addEventListener('drop', event => {
      event.preventDefault()
      if (!workspaceOrder || !draggedWorkspaceTable || draggedWorkspaceTable === id) return
      const from = workspaceOrder.ids.indexOf(draggedWorkspaceTable)
      const bounds = item.getBoundingClientRect()
      const after = event.clientY >= bounds.top + bounds.height / 2
      const target = index + Number(after) - Number(from < index)
      moveWorkspaceTable(draggedWorkspaceTable, target)
      draggedWorkspaceTable = null
      clearWorkspaceDropTargets()
    })
    item.addEventListener('dragend', () => {
      draggedWorkspaceTable = null
      item.classList.remove('dragging')
      clearWorkspaceDropTargets()
    })
    return item
  }))
}
function applyWorkspaceOrder(): void {
  const order = workspaceOrder
  if (!order || mode !== 'workspace' || selectedMonth !== order.month) { closeWorkspaceOrder(); return }
  captureAll()
  const workspace = workspaceStore.states.get(order.month)?.workspace
  if (!workspace) { closeWorkspaceOrder(); return }
  const byId = new Map(workspace.tables.map(table => [table.id, table]))
  const container = document.querySelector<HTMLElement>('.workspace-tables')!
  const sections = new Map([...container.children].map(section => [(section as HTMLElement).dataset.tableId!, section]))
  if (order.ids.length !== byId.size || new Set(order.ids).size !== byId.size || order.ids.some(id => !byId.has(id) || !sections.has(id))) {
    closeWorkspaceOrder()
    workspaceNotice('表格列表已变化，请重新打开调整顺序。')
    return
  }
  if (order.ids.some((id, index) => workspace.tables[index].id !== id)) {
    workspaceStore.edit(order.month, value => { value.tables = order.ids.map(id => byId.get(id)!) })
    // Move the existing sections so long tables retain their mounted editors and input.
    for (const [index, table] of workspace.tables.entries()) {
      const section = sections.get(table.id)!
      container.append(section)
      section.querySelector('.workspace-column-header')?.setAttribute('aria-label', `${order.month} 第 ${index + 1} 张表的列标题`)
      section.querySelector('.add-rows input')?.setAttribute('aria-label', `${order.month} 第 ${index + 1} 张表要添加的行数`)
    }
    const orderedCells = workspace.tables.flatMap(table => table.rows.map(row => cells.get(workspaceCellKey(order.month, table.id, row.id))!))
    cells.clear()
    for (const cell of orderedCells) cells.set(cell.key, cell)
    scheduleWorkspaceSave(order.month)
    if (query) rebuildFind(false)
  }
  closeWorkspaceOrder()
}
element('workspace-order-apply').addEventListener('click', applyWorkspaceOrder)
element('workspace-order-cancel').addEventListener('click', closeWorkspaceOrder)
element('workspace-order-dialog').addEventListener('cancel', event => { event.preventDefault(); closeWorkspaceOrder() })

function captureAll(): void {
  for (const cell of cells.values()) if (cell.crepe && cell.root.classList.contains('mounted')) changedCell(cell, cell.crepe.getMarkdown())
}
async function flush(): Promise<void> {
  captureAll()
  for (const timer of timers.values()) clearTimeout(timer)
  timers.clear()
  await flushStores()
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
  const monthly = mode === 'workspace'
  element('workspace-mode').setAttribute('aria-pressed', String(monthly))
  element('rangebar').hidden = monthly
  element('workspace-range').hidden = !monthly
  element<HTMLInputElement>('workspace-month').value = selectedMonth
  element<HTMLInputElement>('workspace-month').disabled = loading
  element<HTMLButtonElement>('workspace-prev').disabled = loading || !validMonth(shiftMonth(selectedMonth, -1))
  element<HTMLButtonElement>('workspace-next').disabled = loading || !validMonth(shiftMonth(selectedMonth, 1))
  element<HTMLButtonElement>('workspace-current').disabled = loading || selectedMonth === monthKey()
  element('days').setAttribute('aria-label', monthly ? '按月保存的短期工作区' : '按日期排列的双栏记录')
  applyFocusColumnRatio()
  element('focus-mode').setAttribute('aria-pressed', String(mode === 'focus'))
  element('overview-mode').setAttribute('aria-pressed', String(mode === 'overview'))
  element('rangebar').classList.toggle('focus-range', mode === 'focus')
  element('range-label').textContent = mode === 'focus' ? `${today} · 只看今天` : `${visibleDates[visibleDates.length - 1]} — ${visibleDates[0]}`
  element<HTMLButtonElement>('newer-page').disabled = page === 0 || loading
  element<HTMLButtonElement>('today-page').disabled = page === 0 || loading
  element<HTMLButtonElement>('older-page').disabled = loading
  element<HTMLInputElement>('find-input').placeholder = monthly ? '查找当前月份' : mode === 'focus' ? '查找今天的双栏' : '查找当前 10 天'
}

async function navigate(nextMode: Mode, nextPage: number, nextMonth = selectedMonth): Promise<void> {
  if (migrationBusy) return
  closeWorkspaceMigration()
  closeWorkspaceOrder()
  captureAll()
  void flushStores().catch(() => {})
  const generation = ++renderGeneration
  mode = nextMode
  page = Math.max(0, nextPage)
  selectedMonth = nextMonth
  visibleDates = mode === 'workspace' ? [] : dateWindow(today, mode, page)
  const dates = [...visibleDates]
  loading = true
  element('days').inert = true
  loadFailure = null
  matches = []; activeMatch = -1
  findJumpGeneration++
  rangeControls(); updateErrors(); updateFindUI()
  try {
    if (mode === 'workspace') {
      const month = selectedMonth
      const remote = await request<MonthWorkspace>('loadWorkspace', { month })
      if (generation !== renderGeneration) return
      if (!validWorkspace(remote, month)) throw new Error('本地服务返回了无效的月份数据')
      const state = workspaceStore.register(remote, readWorkspaceDraft(month))
      if (!state.workspace.tables.length) state.workspace.tables = [newWorkspaceTable()]
      await disposeCells()
      if (generation !== renderGeneration) return
      renderWorkspace(state.workspace)
      const first = cells.values().next().value as Cell | undefined
      if (first) await mountCell(first)
      if (generation !== renderGeneration) return
      for (const cell of cells.values()) if (!cell.crepe) drawPreview(cell)
      loading = false
      updateWorkspaceLock()
      rangeControls(); updateErrors(); rebuildFind(false)
      window.scrollTo({ top: 0 })
      if (state.dirty && !state.conflict) scheduleWorkspaceSave(month)
      return
    }
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
    updateWorkspaceLock()
    rangeControls(); updateErrors(); rebuildFind(false)
    window.scrollTo({ top: 0 })
    for (const date of dates) {
      const state = store.states.get(date)!
      if (state.dirty && !state.conflict) scheduleSave(date)
    }
  } catch (error) {
    if (generation !== renderGeneration) return
    loading = false
    loadFailure = `未能加载${mode === 'workspace' ? selectedMonth : mode === 'focus' ? '今天' : '这 10 天'}的记录：${(error as Error).message}。`
    // A failed range must never show the previous range under a new date heading.
    await disposeCells()
    if (generation !== renderGeneration) return
    updateWorkspaceLock()
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
  element('find-location').textContent = current ? `${current.label}　${current.excerpt}` : mode === 'workspace' ? `范围：${selectedMonth}` : mode === 'focus' ? '范围：今天' : '范围：当前页的 10 天'
}
function rebuildFind(jump = true): void {
  findJumpGeneration++
  captureAll()
  const previous = matches[activeMatch]
  query = element<HTMLInputElement>('find-input').value
  matches = []
  if (query && !element('findbar').hidden && !loading) {
    for (const cell of cells.values()) {
      if (cell.workspace ? mode !== 'workspace' || cell.workspace.month !== selectedMonth : !visibleDates.includes(cell.date)) continue
      const doc = docFor(cell)
      if (doc) for (const found of searchBlocks(textBlocks(doc), query)) matches.push({ ...found, date: cell.date, rowId: cell.rowId, side: cell.side, cellKey: cell.key, label: cellLabel(cell) })
    }
  }
  const preserved = previous && matches.findIndex(match => match.cellKey === previous.cellKey && match.from === previous.from)
  activeMatch = matches.length ? Math.max(0, preserved ?? 0) : -1
  updateFindUI(); refreshDecorations()
  if (jump && matches.length) void jumpToMatch(activeMatch)
}
async function jumpToMatch(index: number): Promise<void> {
  if (!matches.length) return
  const generation = ++findJumpGeneration
  activeMatch = (index + matches.length) % matches.length
  const match = matches[activeMatch]
  const cell = cells.get(match.cellKey)
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
  if (workspaceOrder || workspaceMigration) return
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

function showWorkspaceRecoveries(months: string[]): void {
  const content = element('recovery-content')
  content.replaceChildren()
  for (const month of months) {
    try {
      const versions = JSON.parse(localStorage.getItem(workspaceRecoveryPrefix + month) || '[]') as { at: string; workspace: MonthWorkspace }[]
      for (const version of versions.slice().reverse()) {
        const section = node('section', 'recovery-day')
        section.append(node('h3', '', `${month} · ${version.at}`))
        version.workspace.tables.forEach((table, tableIndex) => {
          section.append(node('h4', '', `表 ${tableIndex + 1} · ${table.header || '未命名表'}`))
          const header = node('textarea')
          header.readOnly = true; header.value = table.header; header.rows = 1
          header.setAttribute('aria-label', '备份的列标题')
          section.append(header)
          table.rows.forEach((row, index) => {
            section.append(node('h4', '', `第 ${index + 1} 行`))
            const text = node('textarea')
            text.readOnly = true; text.value = row.content; text.rows = Math.min(12, Math.max(3, row.content.split('\n').length))
            section.append(text)
          })
        })
        content.append(section)
      }
    } catch { content.append(node('p', '', `${month} 的备份无法读取。`)) }
  }
  element<HTMLDialogElement>('recovery-dialog').showModal()
}
async function reloadWorkspaceConflict(month: string): Promise<void> {
  captureAll()
  conflictBusy = true
  element<HTMLButtonElement>('conflict-reload').disabled = true
  element<HTMLButtonElement>('conflict-cancel').disabled = true
  try {
    await recoverWorkspace(workspaceStore, month, workspace => {
      try {
        const key = workspaceRecoveryPrefix + month
        const old = JSON.parse(localStorage.getItem(key) || '[]')
        old.push({ at: new Date().toLocaleString('zh-CN'), workspace })
        localStorage.setItem(key, JSON.stringify(old))
      } catch (error) { storageFailure = true; throw error }
    }, month => request<MonthWorkspace>('loadWorkspace', { month }), captureAll)
    await disposeCells()
    element<HTMLDialogElement>('conflict-dialog').close()
    conflictMonth = null
    await navigate(mode, page)
  } catch (error) {
    workspaceNotice(`未能重新加载，当前草稿仍保留：${(error as Error).message}`)
    updateErrors()
  } finally {
    conflictBusy = false
    element<HTMLButtonElement>('conflict-reload').disabled = false
    element<HTMLButtonElement>('conflict-cancel').disabled = false
  }
}

element('conflict-reload').addEventListener('click', async () => {
  if (conflictBusy) return
  if (conflictMonth) { await reloadWorkspaceConflict(conflictMonth); return }
  if (!conflictDate) return
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
element('workspace-mode').addEventListener('click', () => { if (mode !== 'workspace') void navigate('workspace', 0, monthKey()) })
element('workspace-prev').addEventListener('click', () => void navigate('workspace', 0, shiftMonth(selectedMonth, -1)))
element('workspace-next').addEventListener('click', () => void navigate('workspace', 0, shiftMonth(selectedMonth, 1)))
element('workspace-current').addEventListener('click', () => void navigate('workspace', 0, monthKey()))
element<HTMLInputElement>('workspace-month').addEventListener('change', event => {
  const month = (event.target as HTMLInputElement).value
  if (validMonth(month)) void navigate('workspace', 0, month)
  else rangeControls()
})
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
  if (workspaceMigration) {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); closeWorkspaceMigration() }
    return
  }
  if (workspaceOrder) {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); closeWorkspaceOrder() }
    return
  }
  if (event.metaKey && !event.altKey && event.key.toLowerCase() === 'f') { event.preventDefault(); event.stopPropagation(); openFind() }
  if (event.key === 'Escape' && !element('findbar').hidden) { event.preventDefault(); closeFind() }
}, true)

function checkToday(): void {
  const current = dateKey()
  if (current === today) return
  const previousMonth = today.slice(0, 7)
  today = current
  if (mode === 'workspace') {
    if (previousMonth !== monthKey() && selectedMonth === previousMonth) {
      element('notice').textContent = '已经进入新月份，上月的修改已保留。'
      element('notice').hidden = false
      void navigate('workspace', 0, monthKey())
    }
    return
  }
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
window.addEventListener('pagehide', () => { captureAll(); void flushStores().catch(() => {}) })
window.panelDualNotes = { receive, openFind, flush }
void navigate('focus', 0)
