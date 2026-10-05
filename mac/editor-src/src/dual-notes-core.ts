/** Local-calendar arithmetic deliberately avoids UTC and 24-hour millisecond steps. */
export function dateKey(date = new Date()): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

export function shiftDate(key: string, offset: number): string {
  const [year, month, day] = key.split('-').map(Number)
  return dateKey(new Date(year, month - 1, day + offset, 12))
}

export function dateWindow(today: string, mode: 'focus' | 'overview', page = 0): string[] {
  const count = mode === 'focus' ? 1 : 10
  const end = shiftDate(today, mode === 'focus' ? 0 : -Math.max(0, page) * 10)
  return Array.from({ length: count }, (_, index) => shiftDate(end, index - count + 1))
}

export type Side = 'left' | 'right'
export interface NoteRow { id: string; left: string; right: string }
export interface NoteDay { date: string; revision: number; rows: NoteRow[] }
export interface SearchMatch { date: string; rowId: string; side: Side; from: number; to: number; excerpt: string }
export interface TextBlock { text: string; positions: number[] }

/** Positions come from the document model, so rich marks and code blocks are searchable. */
export function searchBlocks(blocks: TextBlock[], query: string): { from: number; to: number; excerpt: string }[] {
  if (!query) return []
  const pattern = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'giu')
  const matches = []
  for (const block of blocks) {
    for (const match of block.text.matchAll(pattern)) {
      const index = match.index!
      matches.push({
        from: block.positions[index], to: block.positions[index + match[0].length - 1] + 1,
        excerpt: block.text.slice(Math.max(0, index - 24), index + match[0].length + 48).replace(/\n/g, ' '),
      })
    }
  }
  return matches
}

export function sameRows(a: NoteRow[], b: NoteRow[]): boolean { return JSON.stringify(a) === JSON.stringify(b) }
export function cloneDay(day: NoteDay): NoteDay { return { ...day, rows: day.rows.map(row => ({ ...row })) } }

export interface DayState {
  day: NoteDay
  version: number
  dirty: boolean
  saving: boolean
  conflict: boolean
  error: string | null
}

/** One save in flight per date; edits made during that save keep their content and get the new revision. */
export class DayStore {
  readonly states = new Map<string, DayState>()
  private flights = new Map<string, Promise<void>>()
  constructor(private persist: (day: NoteDay) => Promise<NoteDay>, private changed: (state: DayState) => void) {}

  register(remote: NoteDay, draft?: NoteDay): DayState {
    const existing = this.states.get(remote.date)
    if (existing) {
      // Refresh clean pages when returning from another page; retain any unsaved edits.
      if (!existing.dirty && !existing.saving && remote.revision >= existing.day.revision) {
        existing.day = cloneDay(remote)
        existing.error = null
        existing.conflict = false
      }
      return existing
    }
    const differs = draft && !sameRows(remote.rows, draft.rows)
    const state: DayState = {
      day: cloneDay(differs ? draft : remote), version: 0, dirty: !!differs,
      saving: false, conflict: !!differs && draft.revision !== remote.revision,
      error: differs && draft.revision !== remote.revision ? '这份草稿对应的旧版本已被修改。草稿仍保留，请先查看版本冲突。' : null,
    }
    this.states.set(remote.date, state)
    this.changed(state)
    return state
  }

  edit(date: string, update: (day: NoteDay) => void): void {
    const state = this.states.get(date)
    if (!state) throw new Error(`尚未加载 ${date}`)
    update(state.day)
    state.version++
    state.dirty = true
    if (!state.conflict) state.error = null
    this.changed(state)
  }

  save(date: string): Promise<void> {
    const previous = this.flights.get(date)
    if (previous) return previous
    const state = this.states.get(date)
    if (!state || !state.dirty) return Promise.resolve()
    if (state.conflict) return Promise.reject(new Error(state.error ?? '版本冲突'))
    const run = async () => {
      while (state.dirty) {
        state.saving = true
        state.error = null
        const version = state.version
        const snapshot = cloneDay(state.day)
        this.changed(state)
        try {
          const saved = await this.persist(snapshot)
          state.day.revision = saved.revision
          state.dirty = version !== state.version
        } catch (error) {
          const failure = error as Error & { status?: number }
          state.conflict = failure.status === 409
          state.error = state.conflict
            ? '已保存版本有更新，本次未覆盖。你的修改已保留为草稿。'
            : `尚未保存：${failure.message || '无法连接本地服务'}。草稿已保留，可重试。`
          throw error
        } finally {
          state.saving = false
          this.changed(state)
        }
      }
    }
    const flight = run().finally(() => this.flights.delete(date))
    this.flights.set(date, flight)
    return flight
  }

  async flush(): Promise<void> {
    const results = await Promise.allSettled([...this.states.keys()].map(date => this.save(date)))
    if (results.some(result => result.status === 'rejected')) throw new Error('仍有草稿未保存，请查看页面中的提示。')
  }

  /** Caller must explicitly preserve the draft before accepting a conflicting saved version. */
  replaceWithRemote(remote: NoteDay): void {
    if (this.flights.has(remote.date)) throw new Error('请等待正在进行的保存完成。')
    this.states.delete(remote.date)
    this.register(remote)
  }
}

/** Keep recovery tied to its initial date and preserve any edits made while the load was pending. */
export async function recoverDay(
  store: DayStore,
  date: string,
  backup: (day: NoteDay) => void,
  load: (date: string) => Promise<NoteDay>,
  beforeReplace: () => Promise<void>,
): Promise<void> {
  const original = store.states.get(date)
  if (!original) throw new Error('找不到需要恢复的草稿。')
  const snapshot = cloneDay(original.day)
  backup(snapshot)
  const remote = await load(date)
  if (remote.date !== date) throw new Error('服务返回的日期不匹配，草稿未替换。')
  await beforeReplace()
  const latest = store.states.get(date)
  if (latest && !sameRows(latest.day.rows, snapshot.rows)) backup(cloneDay(latest.day))
  store.replaceWithRemote(remote)
}
