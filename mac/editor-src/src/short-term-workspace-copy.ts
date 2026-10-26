import { WorkspaceStore, validMonth, validWorkspace, type MonthWorkspace } from './short-term-workspace-core'

export interface WorkspaceCopyRequest {
  operationId: string; sourceMonth: string; targetMonth: string
  sourceRevision: number; targetRevision: number; tableIds: string[]
}
export interface WorkspaceCopyResult { operationId: string; target: MonthWorkspace; copiedTableIds: string[] }
export function validWorkspaceCopy(value: unknown): value is WorkspaceCopyRequest {
  const request = value as WorkspaceCopyRequest | undefined
  return !!request && typeof request.operationId === 'string' && request.operationId.length > 0 && request.operationId.length <= 128
    && typeof request.sourceMonth === 'string' && validMonth(request.sourceMonth)
    && typeof request.targetMonth === 'string' && validMonth(request.targetMonth) && request.sourceMonth !== request.targetMonth
    && Number.isSafeInteger(request.sourceRevision) && request.sourceRevision >= 0
    && Number.isSafeInteger(request.targetRevision) && request.targetRevision >= 0
    && Array.isArray(request.tableIds) && request.tableIds.length > 0 && request.tableIds.length <= 100
    && request.tableIds.every(id => typeof id === 'string' && id.length > 0 && id.length <= 128)
    && new Set(request.tableIds).size === request.tableIds.length
}

/** Persist the request before sending it; uncertain outcomes reuse its operation ID after a restart. */
export class WorkspaceCopier {
  pending: WorkspaceCopyRequest | null
  private flight: Promise<WorkspaceCopyResult> | null = null
  constructor(
    private store: WorkspaceStore,
    private load: (month: string) => Promise<MonthWorkspace>,
    private readDraft: (month: string) => MonthWorkspace | undefined,
    private send: (request: WorkspaceCopyRequest) => Promise<WorkspaceCopyResult>,
    private remember: (request: WorkspaceCopyRequest | null) => void,
    pending: WorkspaceCopyRequest | null = null,
  ) { this.pending = pending }

  copy(sourceMonth: string, targetMonth: string, tableIds: string[]): Promise<WorkspaceCopyResult> {
    if (this.flight) return this.flight
    const run = async () => {
      if (!this.pending) {
        if (!validMonth(sourceMonth) || !validMonth(targetMonth) || sourceMonth === targetMonth) throw new Error('请选择不同的来源月份和目标月份。')
        const source = this.store.states.get(sourceMonth)
        if (!source || !tableIds.length || new Set(tableIds).size !== tableIds.length || tableIds.some(id => !source.workspace.tables.some(table => table.id === id))) {
          throw new Error('所选表已变化，请重新选择。')
        }
        // Save the source's last keystroke, then merge and save any existing target draft.
        if (source.workspace.revision === 0 && !source.dirty) this.store.edit(sourceMonth, () => {})
        await this.store.save(sourceMonth)
        const remote = await this.load(targetMonth)
        if (!validWorkspace(remote, targetMonth)) throw new Error('目标月份数据无效。')
        const target = this.store.register(remote, this.readDraft(targetMonth))
        await this.store.save(targetMonth)
        const ids = new Set(tableIds)
        const selected = source.workspace.tables.filter(table => ids.has(table.id))
        if (selected.length !== ids.size) throw new Error('所选表已变化，请重新选择。')
        if (target.workspace.tables.length + selected.length > 100) throw new Error('迁移后当月会超过 100 张表，请减少选择。')
        if ([...target.workspace.tables, ...selected].reduce((sum, table) => sum + table.rows.length, 0) > 1000) throw new Error('迁移后当月会超过 1000 行，请减少选择。')
        const request: WorkspaceCopyRequest = {
          operationId: crypto.randomUUID(), sourceMonth, targetMonth,
          sourceRevision: source.workspace.revision, targetRevision: target.workspace.revision,
          tableIds: selected.map(table => table.id),
        }
        this.remember(request)
        this.pending = request
      }
      const request = this.pending
      let result: WorkspaceCopyResult
      try { result = await this.send({ ...request, tableIds: [...request.tableIds] }) }
      catch (error) {
        // These responses confirm that the transaction did not run. Timeouts and
        // server failures remain pending because a committed response may be lost.
        if ([400, 404, 409, 413].includes((error as Error & { status?: number }).status ?? 0)) {
          this.remember(null)
          this.pending = null
        }
        throw error
      }
      if (result.operationId !== request.operationId || !validWorkspace(result.target, request.targetMonth)
        || result.target.revision <= request.targetRevision
        || !Array.isArray(result.copiedTableIds) || result.copiedTableIds.length !== request.tableIds.length
        || result.copiedTableIds.some(id => typeof id !== 'string' || !id.length || id.length > 128)
        || new Set(result.copiedTableIds).size !== result.copiedTableIds.length
        || (result.target.revision === request.targetRevision + 1 && result.copiedTableIds.some(id => !result.target.tables.some(table => table.id === id)))) {
        throw new Error('服务返回的迁移结果无效，请重试确认。')
      }
      // register preserves a newer saved revision or any unexpected local draft.
      this.store.register(result.target, this.readDraft(request.targetMonth))
      this.remember(null)
      this.pending = null
      return result
    }
    this.flight = run().finally(() => { this.flight = null })
    return this.flight
  }
}
