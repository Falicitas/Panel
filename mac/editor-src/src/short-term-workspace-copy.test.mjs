// Copy control-flow tests use in-memory stores and deferred HTTP responses only.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { WorkspaceStore, cloneWorkspace } from './short-term-workspace-core'
import { WorkspaceCopier, validWorkspaceCopy } from './short-term-workspace-copy'

const sourceMonth = '2026-09'
const targetMonth = '2026-10'
const table = (id, content = `${id} content`) => ({ id, header: `${id} title`, rows: [{ id: `${id}-row`, content }] })
const source = (revision = 2) => ({ month: sourceMonth, revision, tables: [table('a'), table('b'), table('c')] })
const target = (revision = 5) => ({ month: targetMonth, revision, tables: [table('existing')] })
const pendingRequest = () => ({ operationId: 'copy-operation', sourceMonth, targetMonth, sourceRevision: 2, targetRevision: 5, tableIds: ['a', 'c'] })
const clone = value => structuredClone(value)
const resultFor = (request, existing = target().tables) => {
  const copied = request.tableIds.map(id => table(`copied-${id}`))
  return {
    operationId: request.operationId,
    target: { month: request.targetMonth, revision: request.targetRevision + 1, tables: [...clone(existing), ...copied] },
    copiedTableIds: copied.map(item => item.id),
  }
}
const deferred = () => {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
function setup(options = {}) {
  const events = [], saves = [], sends = [], remembered = []
  const store = new WorkspaceStore(async value => {
    events.push(`save:${value.month}`)
    saves.push(cloneWorkspace(value))
    return options.persist ? options.persist(value) : { ...value, revision: value.revision + 1 }
  }, () => {})
  if (options.initialSource !== null) store.register(options.initialSource ?? source())
  if (options.initialTarget) store.register(options.initialTarget)
  const copier = new WorkspaceCopier(store, async month => {
    events.push(`load:${month}`)
    return options.load ? options.load(month) : target()
  }, options.readDraft ?? (() => undefined), async request => {
    events.push('send')
    sends.push(clone(request))
    return options.send ? options.send(request) : resultFor(request)
  }, request => {
    events.push(request ? 'remember:pending' : 'remember:clear')
    options.remember?.(request)
    remembered.push(clone(request))
  }, options.pending ?? null)
  return { store, copier, events, saves, sends, remembered }
}

test('pending copy validation rejects malformed or ambiguous requests before they can be restored', () => {
  assert.equal(validWorkspaceCopy(pendingRequest()), true)
  const invalid = [null, {}, [], ...[
    { operationId: '' }, { operationId: 'x'.repeat(129) },
    { sourceMonth: '2026-9' }, { sourceMonth: '0000-01' }, { targetMonth: '2026-13' }, { targetMonth: sourceMonth },
    { sourceRevision: -1 }, { targetRevision: 0.5 }, { sourceRevision: Number.MAX_SAFE_INTEGER + 1 },
    { tableIds: [] }, { tableIds: ['a', 'a'] }, { tableIds: [''] }, { tableIds: [1] },
    { tableIds: ['x'.repeat(129)] }, { tableIds: Array.from({ length: 101 }, (_, i) => String(i)) },
  ].map(change => ({ ...pendingRequest(), ...change }))]
  for (const value of invalid) assert.equal(validWorkspaceCopy(value), false, JSON.stringify(value))
})

test('copy saves source edits and a recovered target draft before remembering and sending source-ordered selections', async () => {
  const draft = target()
  draft.tables[0].rows[0].content = 'unsaved target draft'
  let env
  env = setup({
    readDraft: month => month === targetMonth ? draft : undefined,
    send: async request => resultFor(request, env.store.states.get(targetMonth).workspace.tables),
  })
  env.store.edit(sourceMonth, value => { value.tables[0].rows[0].content = 'source last keystroke' })
  await env.copier.copy(sourceMonth, targetMonth, ['c', 'a'])
  assert.deepEqual(env.events, [
    `save:${sourceMonth}`, `load:${targetMonth}`, `save:${targetMonth}`,
    'remember:pending', 'send', 'remember:clear',
  ])
  assert.equal(env.saves[0].tables[0].rows[0].content, 'source last keystroke')
  assert.equal(env.saves[1].tables[0].rows[0].content, 'unsaved target draft')
  assert.equal(env.sends[0].sourceRevision, 3)
  assert.equal(env.sends[0].targetRevision, 6)
  assert.deepEqual(env.sends[0].tableIds, ['a', 'c'])
  assert.deepEqual(env.remembered, [env.sends[0], null])
  assert.equal(env.copier.pending, null)
  assert.equal(env.store.states.get(sourceMonth).workspace.tables[0].rows[0].content, 'source last keystroke')
  assert.equal(env.store.states.get(targetMonth).workspace.tables[0].rows[0].content, 'unsaved target draft')
})

test('target load cannot overwrite an already loaded dirty target before its draft is saved', async () => {
  let env
  env = setup({ initialTarget: target(), send: async request => resultFor(request, env.store.states.get(targetMonth).workspace.tables) })
  env.store.edit(targetMonth, value => { value.tables[0].header = 'local target title' })
  await env.copier.copy(sourceMonth, targetMonth, ['a'])
  assert.equal(env.saves.length, 1)
  assert.equal(env.saves[0].month, targetMonth)
  assert.equal(env.saves[0].tables[0].header, 'local target title')
  assert.equal(env.sends[0].targetRevision, 6)
  assert.equal(env.store.states.get(targetMonth).workspace.tables[0].header, 'local target title')
})

test('an explicit copy saves an unsaved empty default source table even if it was initially clean', async () => {
  const initialSource = { month: sourceMonth, revision: 0, tables: [{ id: 'empty', header: '', rows: [{ id: 'empty-row', content: '' }] }] }
  const env = setup({ initialSource })
  assert.equal(env.store.states.get(sourceMonth).dirty, false)
  await env.copier.copy(sourceMonth, targetMonth, ['empty'])
  assert.equal(env.events[0], `save:${sourceMonth}`)
  assert.equal(env.saves[0].tables[0].rows[0].content, '')
  assert.equal(env.sends[0].sourceRevision, 1)
})

test('reentrant calls share one flight and keep the first call selection', async () => {
  const loading = deferred()
  const env = setup({ load: () => loading.promise })
  const first = env.copier.copy(sourceMonth, targetMonth, ['c', 'a'])
  const second = env.copier.copy(sourceMonth, targetMonth, ['b'])
  assert.strictEqual(second, first)
  loading.resolve(target())
  const [left, right] = await Promise.all([first, second])
  assert.strictEqual(left, right)
  assert.equal(env.sends.length, 1)
  assert.equal(env.remembered.filter(Boolean).length, 1)
  assert.deepEqual(env.sends[0].tableIds, ['a', 'c'])
})

test('an uncertain outcome retains the durable request and retries it without loading or saving again', async () => {
  let fail = true
  const env = setup({ send: async request => {
    if (fail) {
      // The transport must not be able to mutate the durable request through aliases.
      request.tableIds.reverse()
      request.sourceMonth = '2025-01'
      throw new Error('response lost after commit')
    }
    return resultFor(request)
  } })
  await assert.rejects(env.copier.copy(sourceMonth, targetMonth, ['a', 'c']), /response lost/)
  const original = clone(env.sends[0])
  assert.deepEqual(env.copier.pending, original)
  assert.deepEqual(env.remembered, [original])
  const loads = env.events.filter(event => event.startsWith('load:')).length
  const saves = env.saves.length
  fail = false
  await env.copier.copy('2020-01', '2020-02', ['another-selection'])
  assert.deepEqual(env.sends[1], original)
  assert.equal(env.events.filter(event => event.startsWith('load:')).length, loads)
  assert.equal(env.saves.length, saves)
  assert.equal(env.copier.pending, null)
  assert.deepEqual(env.remembered, [original, null])
})

test('restored pending operations replay unchanged even when the source has since been edited', async () => {
  const pending = pendingRequest()
  const env = setup({
    pending: clone(pending), initialSource: source(9),
    load: () => assert.fail('Pending replay must not load a month'),
    persist: () => assert.fail('Pending replay must not save a month'),
  })
  env.store.edit(sourceMonth, value => { value.tables = [table('new-source-table', 'changed after original copy')] })
  await env.copier.copy(sourceMonth, targetMonth, ['new-source-table'])
  assert.deepEqual(env.sends, [pending])
  assert.deepEqual(env.events, ['send', 'remember:clear'])
  assert.deepEqual(env.remembered, [null])
  assert.equal(env.store.states.get(sourceMonth).dirty, true)
  assert.equal(env.store.states.get(sourceMonth).workspace.tables[0].id, 'new-source-table')
})

test('definite transaction failures clear pending, while server errors keep it for an idempotent retry', async () => {
  for (const status of [400, 404, 409, 413, 500, 503]) {
    const env = setup({ send: async () => { throw Object.assign(new Error(`HTTP ${status}`), { status }) } })
    await assert.rejects(env.copier.copy(sourceMonth, targetMonth, ['a']), new RegExp(`HTTP ${status}`))
    if (status < 500) {
      assert.equal(env.copier.pending, null)
      assert.deepEqual(env.remembered, [env.sends[0], null])
    } else {
      assert.deepEqual(env.copier.pending, env.sends[0])
      assert.deepEqual(env.remembered, [env.sends[0]])
    }
    assert.deepEqual(env.store.states.get(targetMonth).workspace, target())
  }
})

test('failure to remember a request prevents sending it', async () => {
  const env = setup({ remember: request => { if (request) throw new Error('storage quota') } })
  await assert.rejects(env.copier.copy(sourceMonth, targetMonth, ['a']), /storage quota/)
  assert.equal(env.copier.pending, null)
  assert.equal(env.sends.length, 0)
  assert.equal(env.remembered.length, 0)
})

test('invalid response ownership or structure retains pending and does not register a target', async () => {
  const invalid = [
    ['null response', () => null],
    ['wrong operation', value => ({ ...value, operationId: 'another-copy' })],
    ['wrong month', value => ({ ...value, target: { ...value.target, month: '2026-11' } })],
    ['invalid target', value => ({ ...value, target: { ...value.target, tables: null } })],
    ['missing copied IDs', value => ({ ...value, copiedTableIds: undefined })],
    ['wrong copied count', value => ({ ...value, copiedTableIds: ['one-copy'] })],
    ['duplicate copied IDs', value => ({ ...value, copiedTableIds: ['same', 'same'] })],
    ['non-string copied ID', value => ({ ...value, copiedTableIds: [1, 'second'] })],
    ['absent copied tables at first revision', value => ({ ...value, target: { ...value.target, tables: target().tables } })],
    ['unchanged revision', value => ({ ...value, target: { ...value.target, revision: 5 } })],
    ['older revision', value => ({ ...value, target: { ...value.target, revision: 4 } })],
  ]
  for (const [label, change] of invalid) {
    const pending = pendingRequest()
    const env = setup({ pending: clone(pending), initialTarget: target(), send: async request => change(resultFor(request)) })
    await assert.rejects(env.copier.copy(sourceMonth, targetMonth, ['a', 'c']), undefined, label)
    assert.deepEqual(env.copier.pending, pending, label)
    assert.deepEqual(env.remembered, [], label)
    assert.deepEqual(env.store.states.get(targetMonth).workspace, target(), label)
  }
})

test('a replay may confirm success after copied tables were removed in a newer target revision', async () => {
  const pending = pendingRequest()
  const current = { month: targetMonth, revision: 8, tables: [] }
  const env = setup({ pending, send: async request => ({ ...resultFor(request), target: current }) })
  await env.copier.copy(sourceMonth, targetMonth, ['a', 'c'])
  assert.equal(env.copier.pending, null)
  assert.deepEqual(env.store.states.get(targetMonth).workspace, current)
  assert.deepEqual(env.remembered, [null])
})

test('success does not overwrite a newer target loaded while the response was pending', async () => {
  const sending = deferred(), response = deferred()
  const env = setup({ initialTarget: target(), send: request => { sending.resolve(request); return response.promise } })
  const copying = env.copier.copy(sourceMonth, targetMonth, ['a'])
  const request = await sending.promise
  const newer = { month: targetMonth, revision: 9, tables: [table('newer', 'later server content')] }
  env.store.register(newer)
  response.resolve(resultFor(request))
  await copying
  assert.deepEqual(env.store.states.get(targetMonth).workspace, newer)
  assert.equal(env.copier.pending, null)
})

test('success does not overwrite a target draft typed while the response was pending', async () => {
  const sending = deferred(), response = deferred()
  const env = setup({ initialTarget: target(), send: request => { sending.resolve(request); return response.promise } })
  const copying = env.copier.copy(sourceMonth, targetMonth, ['a'])
  const request = await sending.promise
  env.store.edit(targetMonth, value => { value.tables[0].rows[0].content = 'new local draft' })
  response.resolve(resultFor(request))
  await copying
  const state = env.store.states.get(targetMonth)
  assert.equal(state.dirty, true)
  assert.equal(state.workspace.revision, 5)
  assert.equal(state.workspace.tables[0].rows[0].content, 'new local draft')
  assert.equal(env.copier.pending, null)
})

test('registered target tables and rows share no mutable objects with the response or source', async () => {
  const pending = { ...pendingRequest(), tableIds: ['a'] }
  let env, response
  env = setup({ pending, send: async request => {
    // Deliberately share a fixture object; register must still clone it recursively.
    const shared = env.store.states.get(sourceMonth).workspace.tables[0]
    response = { operationId: request.operationId, target: { month: targetMonth, revision: 6, tables: [shared] }, copiedTableIds: ['a'] }
    return response
  } })
  await env.copier.copy(sourceMonth, targetMonth, ['a'])
  const sourceTable = env.store.states.get(sourceMonth).workspace.tables[0]
  const targetTable = env.store.states.get(targetMonth).workspace.tables[0]
  assert.notStrictEqual(targetTable, response.target.tables[0])
  assert.notStrictEqual(targetTable.rows, sourceTable.rows)
  assert.notStrictEqual(targetTable.rows[0], sourceTable.rows[0])
  targetTable.header = 'target only'
  targetTable.rows[0].content = 'target edit'
  assert.equal(sourceTable.header, 'a title')
  assert.equal(sourceTable.rows[0].content, 'a content')
  response.target.tables[0].rows.push({ id: 'later-source-row', content: 'not a target row' })
  assert.equal(targetTable.rows.length, 1)
})

test('source save failure and conflicting target drafts stop copying before remembering a transaction', async () => {
  const failedSource = setup({ persist: async () => { throw new Error('source offline') } })
  failedSource.store.edit(sourceMonth, value => { value.tables[0].header = 'unsaved source' })
  await assert.rejects(failedSource.copier.copy(sourceMonth, targetMonth, ['a']), /source offline/)
  assert.deepEqual(failedSource.events, [`save:${sourceMonth}`])
  assert.equal(failedSource.store.states.get(sourceMonth).dirty, true)
  assert.equal(failedSource.copier.pending, null)

  const oldDraft = target(4)
  oldDraft.tables[0].rows[0].content = 'conflicting target draft'
  const conflictingTarget = setup({ readDraft: () => oldDraft })
  await assert.rejects(conflictingTarget.copier.copy(sourceMonth, targetMonth, ['a']))
  assert.equal(conflictingTarget.sends.length, 0)
  assert.equal(conflictingTarget.remembered.length, 0)
  assert.equal(conflictingTarget.copier.pending, null)
  assert.equal(conflictingTarget.store.states.get(targetMonth).conflict, true)
  assert.equal(conflictingTarget.store.states.get(targetMonth).workspace.tables[0].rows[0].content, 'conflicting target draft')
})
