/** A workspace owns ordered single-column tables for one calendar month. */
export interface WorkspaceRow { id: string; content: string }
export interface WorkspaceTable { id: string; header: string; rows: WorkspaceRow[] }
export interface MonthWorkspace { month: string; revision: number; tables: WorkspaceTable[] }
export interface WorkspaceState {
  workspace: MonthWorkspace; version: number; dirty: boolean; saving: boolean; conflict: boolean; error: string | null
}
export function monthKey(date = new Date()): string {
  return `${String(date.getFullYear()).padStart(4, '0')}-${String(date.getMonth() + 1).padStart(2, '0')}`
}
export function validMonth(value: string): boolean { return /^(?!0000)\d{4}-(0[1-9]|1[0-2])$/.test(value) }
export function shiftMonth(value: string, offset: number): string {
  const [year, month] = value.split('-').map(Number)
  const index = year * 12 + month - 1 + offset
  return `${String(Math.floor(index / 12)).padStart(4, '0')}-${String(index % 12 + 1).padStart(2, '0')}`
}
export function validWorkspace(value: unknown, month: string): value is MonthWorkspace {
  const item = value as MonthWorkspace | undefined
  return !!item && item.month === month && validMonth(month) && Number.isSafeInteger(item.revision) && item.revision >= 0
    && Array.isArray(item.tables) && new Set(item.tables.map(table => table?.id)).size === item.tables.length
    && item.tables.every(table => table && typeof table.id === 'string' && !!table.id.trim()
      && typeof table.header === 'string' && Array.isArray(table.rows)
      && new Set(table.rows.map(row => row?.id)).size === table.rows.length
      && table.rows.every(row => row && typeof row.id === 'string' && !!row.id.trim() && typeof row.content === 'string'))
}
export function cloneWorkspace(workspace: MonthWorkspace): MonthWorkspace {
  return { ...workspace, tables: workspace.tables.map(table => ({ ...table, rows: table.rows.map(row => ({ ...row })) })) }
}
function sameTables(a: MonthWorkspace, b: MonthWorkspace): boolean { return JSON.stringify(a.tables) === JSON.stringify(b.tables) }

/** One request per month; a completed save never replaces edits typed while it was pending. */
export class WorkspaceStore {
  readonly states = new Map<string, WorkspaceState>()
  private flights = new Map<string, Promise<void>>()
  constructor(private persist: (workspace: MonthWorkspace) => Promise<MonthWorkspace>, private changed: (state: WorkspaceState) => void) {}
  register(remote: MonthWorkspace, draft?: MonthWorkspace): WorkspaceState {
    const existing = this.states.get(remote.month)
    if (existing) {
      if (!existing.dirty && !existing.saving && remote.revision >= existing.workspace.revision) {
        existing.workspace = cloneWorkspace(remote)
        existing.error = null
        existing.conflict = false
      }
      return existing
    }
    const differs = draft && !sameTables(remote, draft)
    const conflict = !!differs && draft.revision !== remote.revision
    const state: WorkspaceState = {
      workspace: cloneWorkspace(differs ? draft : remote), version: 0, dirty: !!differs, saving: false, conflict,
      error: conflict ? '这份月度草稿对应的旧版本已被修改。草稿仍保留，请先查看版本冲突。' : null,
    }
    this.states.set(remote.month, state)
    this.changed(state)
    return state
  }
  edit(month: string, update: (workspace: MonthWorkspace) => void): void {
    const state = this.states.get(month)
    if (!state) throw new Error(`尚未加载 ${month}`)
    update(state.workspace)
    state.version++
    state.dirty = true
    if (!state.conflict) state.error = null
    this.changed(state)
  }
  save(month: string): Promise<void> {
    const previous = this.flights.get(month)
    if (previous) return previous
    const state = this.states.get(month)
    if (!state || !state.dirty) return Promise.resolve()
    if (state.conflict) return Promise.reject(new Error(state.error ?? '版本冲突'))
    const run = async () => {
      while (state.dirty) {
        state.saving = true
        state.error = null
        const version = state.version
        const snapshot = cloneWorkspace(state.workspace)
        this.changed(state)
        try {
          const saved = await this.persist(snapshot)
          if (!validWorkspace(saved, month) || saved.revision <= snapshot.revision) throw new Error('服务返回了无效的月度保存结果')
          state.workspace.revision = saved.revision
          state.dirty = version !== state.version
        } catch (error) {
          const failure = error as Error & { status?: number }
          state.conflict = failure.status === 409
          state.error = state.conflict ? '已保存版本有更新，本次未覆盖。你的修改已保留为草稿。'
            : `尚未保存：${failure.message || '无法连接本地服务'}。草稿已保留，可重试。`
          throw error
        } finally { state.saving = false; this.changed(state) }
      }
    }
    const flight = run().finally(() => this.flights.delete(month))
    this.flights.set(month, flight)
    return flight
  }
  async flush(): Promise<void> {
    const results = await Promise.allSettled([...this.states.keys()].map(month => this.save(month)))
    if (results.some(result => result.status === 'rejected')) throw new Error('仍有月度草稿未保存，请查看页面中的提示。')
  }
  replaceWithRemote(remote: MonthWorkspace): void {
    if (this.flights.has(remote.month)) throw new Error('请等待正在进行的保存完成。')
    this.states.delete(remote.month)
    this.register(remote)
  }
}
export async function recoverWorkspace(
  store: WorkspaceStore, month: string, backup: (workspace: MonthWorkspace) => void,
  load: (month: string) => Promise<MonthWorkspace>, beforeReplace: () => void,
): Promise<void> {
  const original = store.states.get(month)
  if (!original) throw new Error('找不到需要恢复的月度草稿。')
  const snapshot = cloneWorkspace(original.workspace)
  backup(snapshot)
  const remote = await load(month)
  if (!validWorkspace(remote, month)) throw new Error('服务返回的月份不匹配或数据无效，草稿未替换。')
  beforeReplace()
  const latest = store.states.get(month)
  if (latest && !sameTables(latest.workspace, snapshot)) backup(cloneWorkspace(latest.workspace))
  store.replaceWithRemote(remote)
}
