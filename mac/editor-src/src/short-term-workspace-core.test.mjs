// Bundled into a temporary directory; all persistence is simulated in memory.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { monthKey, validMonth, shiftMonth, validWorkspace, cloneWorkspace, WorkspaceStore, recoverWorkspace } from './short-term-workspace-core'

const month = '2026-10'
const workspace = ({ key = month, revision = 0, content = 'initial', header = '事项' } = {}) => ({
  month: key, revision,
  tables: [{ id: 'table-a', header, rows: [{ id: 'row-a', content }] }],
})
const contentOf = state => state.workspace.tables[0].rows[0].content
const makeStore = (persist = async value => ({ ...value, revision: value.revision + 1 })) => new WorkspaceStore(persist, () => {})
const deferred = () => {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const modified = change => { const value = workspace(); change(value); return value }

test('month navigation uses local calendar months across years, leap years, and supported boundaries', () => {
  assert.equal(monthKey(new Date(2026, 11, 31, 23, 59)), '2026-12')
  assert.equal(monthKey(new Date(2027, 0, 1, 0, 1)), '2027-01')
  const early = new Date(2000, 0, 15, 12)
  early.setFullYear(1)
  assert.equal(monthKey(early), '0001-01')
  assert.equal(shiftMonth('2026-12', 1), '2027-01')
  assert.equal(shiftMonth('2027-01', -1), '2026-12')
  assert.equal(shiftMonth('2024-03', -1), '2024-02')
  assert.equal(shiftMonth('2024-02', 12), '2025-02')
  assert.equal(shiftMonth('2026-01', -25), '2023-12')
  assert.equal(shiftMonth('0001-02', -1), '0001-01')
  assert.equal(shiftMonth('9999-11', 1), '9999-12')
  assert.equal(validMonth(shiftMonth('0001-01', -1)), false)
  assert.equal(validMonth(shiftMonth('9999-12', 1)), false)
  for (const key of ['0001-01', '0099-12', '2024-02', '9999-12']) {
    assert.equal(shiftMonth(key, 0), key)
    assert.equal(validMonth(key), true)
  }
})

test('month keys reject malformed dates, unsupported years, whitespace, and path characters', () => {
  for (const key of ['', '0000-01', '10000-01', '2026-00', '2026-13', '2026-1', '026-01',
    '2026-01-01', ' 2026-01', '2026-01\n', '２０２６-０１', '2026/01', '../2026', '2026-01?x=1']) {
    assert.equal(validMonth(key), false, JSON.stringify(key))
  }
})

test('workspace schema accepts ordered single-column tables, empty headers, and independent row IDs', () => {
  const value = workspace({ revision: 4, content: '# 中文\n**Markdown**' })
  value.tables.push({ id: 'table-b', header: '', rows: [{ id: 'row-a', content: '' }] })
  assert.equal(validWorkspace(value, month), true)
  assert.equal(validWorkspace({ month, revision: 0, tables: [] }, month), true)
  assert.equal(validWorkspace({ month, revision: 0, tables: [{ id: 'empty', header: '', rows: [] }] }, month), true)
  const copied = cloneWorkspace(value)
  copied.tables[0].header = 'new title'
  copied.tables[0].rows[0].content = 'changed cell'
  copied.tables[1].rows.push({ id: 'new-row', content: 'new' })
  assert.equal(value.tables[0].header, '事项')
  assert.equal(value.tables[0].rows[0].content, '# 中文\n**Markdown**')
  assert.equal(value.tables[1].rows.length, 1)
})

test('workspace schema rejects wrong months, invalid revisions, malformed tables, and duplicate identities', () => {
  const invalid = [
    null, undefined, [], {}, 'not a workspace',
    workspace({ key: '2026-11' }), workspace({ key: '2026-1' }),
    ...[-1, 0.5, Number.MAX_SAFE_INTEGER + 1, NaN, Infinity, '1'].map(revision => workspace({ revision })),
    modified(value => { delete value.tables }),
    modified(value => { value.tables = null }),
    modified(value => { value.tables = [null] }),
    modified(value => { value.tables[0].id = '  ' }),
    modified(value => { value.tables[0].id = 1 }),
    modified(value => { value.tables.push(cloneWorkspace(value).tables[0]) }),
    modified(value => { value.tables[0].header = null }),
    modified(value => { delete value.tables[0].header }),
    modified(value => { value.tables[0].rows = null }),
    modified(value => { value.tables[0].rows = [null] }),
    modified(value => { value.tables[0].rows[0].id = '' }),
    modified(value => { value.tables[0].rows[0].id = 1 }),
    modified(value => { value.tables[0].rows.push({ id: 'row-a', content: 'duplicate' }) }),
    modified(value => { value.tables[0].rows[0].content = null }),
    modified(value => { delete value.tables[0].rows[0].content }),
  ]
  for (const value of invalid) assert.equal(validWorkspace(value, month), false, JSON.stringify(value))
})

test('each month keeps its own edits, revision, and in-flight save even when responses arrive out of order', async () => {
  const nextMonth = '2026-11'
  const pending = new Map([[month, deferred()], [nextMonth, deferred()]])
  const calls = []
  const store = makeStore(value => { calls.push(value); return pending.get(value.month).promise })
  const first = store.register(workspace({ revision: 2 }))
  const second = store.register(workspace({ key: nextMonth, revision: 6 }))
  store.edit(month, value => { value.tables[0].rows[0].content = 'October edit' })
  store.edit(nextMonth, value => { value.tables[0].rows[0].content = 'November edit' })
  const savingFirst = store.save(month)
  const savingSecond = store.save(nextMonth)
  assert.notStrictEqual(savingFirst, savingSecond)
  assert.deepEqual(calls.map(value => value.month), [month, nextMonth])
  pending.get(nextMonth).resolve({ ...calls[1], revision: 7 })
  await savingSecond
  assert.equal(first.dirty, true)
  assert.equal(first.saving, true)
  assert.equal(second.dirty, false)
  assert.equal(second.workspace.revision, 7)
  pending.get(month).resolve({ ...calls[0], revision: 3 })
  await savingFirst
  assert.equal(first.workspace.revision, 3)
  assert.equal(contentOf(first), 'October edit')
  assert.equal(contentOf(second), 'November edit')
})

test('edits made during a save are serialized with the returned revision without mutating the request snapshot', async () => {
  const firstSave = deferred()
  const calls = []
  const store = makeStore(snapshot => {
    calls.push(snapshot)
    return calls.length === 1 ? firstSave.promise : Promise.resolve({ ...snapshot, revision: snapshot.revision + 1 })
  })
  const state = store.register(workspace({ revision: 4 }))
  store.edit(month, value => { value.tables[0].header = 'first title' })
  const saving = store.save(month)
  store.edit(month, value => {
    value.tables[0].header = 'latest title'
    value.tables[0].rows[0].content = 'last keystroke'
    value.tables.push({ id: 'table-b', header: 'new table', rows: [{ id: 'row-b', content: 'new row' }] })
  })
  assert.strictEqual(store.save(month), saving)
  assert.equal(calls.length, 1)
  assert.equal(calls[0].tables[0].header, 'first title')
  assert.equal(calls[0].tables[0].rows[0].content, 'initial')
  assert.equal(calls[0].tables.length, 1)
  firstSave.resolve({ ...calls[0], revision: 5 })
  await saving
  assert.equal(calls.length, 2)
  assert.equal(calls[1].revision, 5)
  assert.equal(calls[1].tables[0].header, 'latest title')
  assert.equal(calls[1].tables.length, 2)
  assert.equal(contentOf(state), 'last keystroke')
  assert.equal(state.workspace.revision, 6)
  assert.equal(state.dirty, false)
  assert.equal(state.saving, false)
})

test('late loads cannot roll back saved revisions or replace unsaved edits', async () => {
  const store = makeStore()
  const state = store.register(workspace({ revision: 1 }))
  store.edit(month, value => { value.tables[0].rows[0].content = 'saved edit' })
  await store.save(month)
  assert.strictEqual(store.register(workspace({ revision: 1, content: 'old server' })), state)
  assert.equal(state.workspace.revision, 2)
  assert.equal(contentOf(state), 'saved edit')
  store.edit(month, value => { value.tables[0].rows[0].content = 'unsaved edit' })
  store.register(workspace({ revision: 3, content: 'new server' }))
  assert.equal(contentOf(state), 'unsaved edit')
  assert.equal(state.dirty, true)
})

test('a failed save preserves a reusable draft and retry clears its error after persistence succeeds', async () => {
  let offline = true
  const snapshots = []
  const store = new WorkspaceStore(async value => {
    if (offline) throw new Error('offline')
    return { ...value, revision: value.revision + 1 }
  }, state => { snapshots.push({ dirty: state.dirty, workspace: cloneWorkspace(state.workspace) }) })
  const state = store.register(workspace({ revision: 2 }))
  store.edit(month, value => { value.tables[0].rows[0].content = 'keep my draft' })
  await assert.rejects(store.flush(), /草稿未保存/)
  assert.equal(state.dirty, true)
  assert.equal(state.saving, false)
  assert.equal(state.conflict, false)
  assert.match(state.error, /offline/)
  const draft = snapshots.at(-1).workspace
  const restored = makeStore().register(workspace({ revision: 2 }), draft)
  assert.equal(restored.dirty, true)
  assert.equal(contentOf(restored), 'keep my draft')
  offline = false
  await store.flush()
  assert.equal(state.dirty, false)
  assert.equal(state.workspace.revision, 3)
  assert.equal(state.error, null)
})

test('409 preserves a conflicting draft and blocks further writes, including drafts restored after restart', async () => {
  let calls = 0
  const persist = async () => { calls++; throw Object.assign(new Error('conflict'), { status: 409 }) }
  const store = makeStore(persist)
  const state = store.register(workspace({ revision: 2 }))
  store.edit(month, value => { value.tables[0].header = 'unsaved title' })
  await assert.rejects(store.save(month))
  store.edit(month, value => { value.tables[0].rows[0].content = 'still editing' })
  await assert.rejects(store.save(month))
  assert.equal(calls, 1)
  assert.equal(state.dirty, true)
  assert.equal(state.conflict, true)
  assert.equal(state.workspace.tables[0].header, 'unsaved title')
  assert.equal(contentOf(state), 'still editing')
  const restarted = makeStore(persist)
  const restored = restarted.register(workspace({ revision: 3 }), cloneWorkspace(state.workspace))
  await assert.rejects(restarted.save(month))
  assert.equal(calls, 1)
  assert.equal(restored.conflict, true)
  assert.equal(contentOf(restored), 'still editing')
})

test('malformed or stale save responses never clear dirty state or advance the local revision', async () => {
  const responses = [
    null,
    workspace({ key: '2026-11', revision: 6 }),
    workspace({ revision: 4 }),
    workspace({ revision: 3 }),
    workspace({ revision: 4.5 }),
    workspace({ revision: Number.MAX_SAFE_INTEGER + 1 }),
    { month, revision: 5, tables: null },
    { month, revision: 5, tables: [{ id: 'table-a', header: 'missing rows' }] },
  ]
  for (const response of responses) {
    const store = makeStore(async () => response)
    const state = store.register(workspace({ revision: 4 }))
    store.edit(month, value => { value.tables[0].rows[0].content = 'keep me' })
    await assert.rejects(store.save(month), /无效的月度保存结果/)
    assert.equal(state.workspace.revision, 4, JSON.stringify(response))
    assert.equal(state.dirty, true)
    assert.equal(state.saving, false)
    assert.equal(state.conflict, false)
    assert.equal(contentOf(state), 'keep me')
  }
})

test('recovery preserves both the initial draft and final edits typed while the month load was pending', async () => {
  const store = makeStore()
  store.register(workspace({ revision: 3 }), workspace({ revision: 2, content: 'initial draft' }))
  const other = store.register(workspace({ key: '2026-11', revision: 5, content: 'other month' }))
  const load = deferred()
  const backups = []
  const recovery = recoverWorkspace(store, month, value => backups.push(value), key => {
    assert.equal(key, month)
    return load.promise
  }, () => {
    store.edit(month, value => { value.tables[0].header = 'last title at capture' })
  })
  store.edit(month, value => { value.tables[0].rows[0].content = 'typed during load' })
  load.resolve(workspace({ revision: 4, content: 'remote replacement' }))
  await recovery
  assert.equal(backups.length, 2)
  assert.equal(backups[0].tables[0].rows[0].content, 'initial draft')
  assert.equal(backups[0].tables[0].header, '事项')
  assert.equal(backups[1].tables[0].rows[0].content, 'typed during load')
  assert.equal(backups[1].tables[0].header, 'last title at capture')
  assert.equal(contentOf(store.states.get(month)), 'remote replacement')
  assert.equal(store.states.get(month).workspace.revision, 4)
  assert.equal(store.states.get(month).dirty, false)
  assert.equal(store.states.get(month).conflict, false)
  assert.strictEqual(store.states.get('2026-11'), other)
  assert.equal(contentOf(other), 'other month')
})

test('failed backup, failed load, or invalid recovery response leaves the live draft untouched', async () => {
  const failures = [
    { backup: () => { throw new Error('quota') }, load: async () => workspace({ revision: 4 }) },
    { backup: () => {}, load: async () => { throw new Error('offline') } },
    { backup: () => {}, load: async () => workspace({ key: '2026-11', revision: 4 }) },
    { backup: () => {}, load: async () => ({ month, revision: 4, tables: null }) },
  ]
  for (const { backup, load } of failures) {
    const store = makeStore()
    const state = store.register(workspace({ revision: 3 }), workspace({ revision: 2, content: 'local draft' }))
    await assert.rejects(recoverWorkspace(store, month, backup, load, () => {}))
    assert.strictEqual(store.states.get(month), state)
    assert.equal(contentOf(state), 'local draft')
    assert.equal(state.workspace.revision, 2)
    assert.equal(state.dirty, true)
  }
})

test('failure to back up final captured edits prevents replacement with the remote month', async () => {
  const store = makeStore()
  const state = store.register(workspace({ revision: 3 }), workspace({ revision: 2, content: 'draft' }))
  let backups = 0
  await assert.rejects(recoverWorkspace(store, month, () => {
    if (++backups === 2) throw new Error('quota on final backup')
  }, async () => workspace({ revision: 4 }), () => {
    store.edit(month, value => { value.tables[0].rows[0].content = 'last input' })
  }), /quota on final backup/)
  assert.equal(backups, 2)
  assert.strictEqual(store.states.get(month), state)
  assert.equal(contentOf(state), 'last input')
  assert.equal(state.workspace.revision, 2)
  assert.equal(state.dirty, true)
})

test('remote replacement is blocked while a save is in flight', async () => {
  const pending = deferred()
  const store = makeStore(() => pending.promise)
  const state = store.register(workspace({ revision: 1 }))
  store.edit(month, value => { value.tables[0].rows[0].content = 'pending edit' })
  const saving = store.save(month)
  assert.throws(() => store.replaceWithRemote(workspace({ revision: 9 })), /保存完成/)
  assert.strictEqual(store.states.get(month), state)
  pending.resolve(workspace({ revision: 2, content: 'pending edit' }))
  await saving
  assert.equal(contentOf(state), 'pending edit')
  assert.equal(state.workspace.revision, 2)
})
