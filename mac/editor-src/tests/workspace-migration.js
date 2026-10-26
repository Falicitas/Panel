(async () => {
  const pause = ms => new Promise(resolve => setTimeout(resolve, ms))
  const assert = (condition, message) => { if (!condition) throw new Error(message) }
  const $ = selector => document.querySelector(selector)
  const $$ = selector => [...document.querySelectorAll(selector)]
  async function waitFor(condition, label, timeout = 6000) {
    const deadline = Date.now() + timeout
    while (Date.now() < deadline) {
      if (await condition()) return
      await pause(25)
    }
    throw new Error(`timed out: ${label}`)
  }
  const requests = new Map()
  window.workspaceFixtureReply = result => {
    requests.get(result.id)?.(result)
    requests.delete(result.id)
  }
  const fixture = (options = {}) => new Promise(resolve => {
    const id = crypto.randomUUID()
    requests.set(id, resolve)
    window.webkit.messageHandlers.fixture.postMessage({ id, ...options })
  })
  const tables = () => $$('section.workspace-table[data-table-id]')
  const rows = table => [...table.querySelectorAll('.note-row[data-row-id]')]
  const tableIDs = () => tables().map(table => table.dataset.tableId)
  const same = (a, b) => JSON.stringify(a) === JSON.stringify(b)
  const canonical = workspace => ({
    month: workspace.month, revision: workspace.revision,
    tables: workspace.tables.map(table => ({
      id: table.id, header: table.header,
      rows: table.rows.map(row => ({ id: row.id, content: row.content })),
    })),
  })
  const visible = element => element && !element.hidden && getComputedStyle(element).display !== 'none'
  async function selectMonth(month) {
    $('#workspace-month').value = month
    $('#workspace-month').dispatchEvent(new Event('change', { bubbles: true }))
    await pause(40)
    await waitFor(() => $('#workspace-month').value === month && !$('#days').inert && tables().length, `load ${month}`)
  }
  function title(table, text) {
    const input = table.querySelector('input.workspace-column-header')
    input.value = text
    input.dispatchEvent(new Event('input', { bubbles: true }))
  }
  async function edit(row, text) {
    const cell = row.querySelector('.note-cell.single')
    cell.scrollIntoView({ block: 'center' })
    cell.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
    await waitFor(() => cell.classList.contains('mounted') && cell.querySelector('.ProseMirror'), 'mount editor')
    const editor = cell.querySelector('.ProseMirror')
    editor.focus()
    const selection = window.getSelection()
    const range = document.createRange()
    range.selectNodeContents(editor)
    selection.removeAllRanges()
    selection.addRange(range)
    assert(document.execCommand('insertText', false, text), 'contenteditable input failed')
    await pause(75)
    assert(editor.textContent.includes(text), 'last keystroke was not entered')
  }
  async function save() {
    await pause(50)
    await window.panelDualNotes.flush()
    return fixture()
  }
  const choices = () => $$('#workspace-migrate-list input[type="checkbox"][data-table-id]')
  function choose(id) {
    const input = choices().find(input => input.dataset.tableId === id)
    assert(input && !input.disabled, 'source table cannot be selected')
    input.checked = true
    input.dispatchEvent(new Event('change', { bubbles: true }))
  }
  async function openMigration() {
    const control = $('#migrate-workspace-tables')
    assert(visible(control) && !control.disabled, 'history month migration button is unavailable')
    control.click()
    await waitFor(() => $('#workspace-migrate-dialog').open && choices().length === tables().length, 'migration dialog')
  }
  async function completed() {
    await waitFor(() => !$('#workspace-migrate-dialog').open, 'migration completion')
    assert(visible($('#notice')) && $('#notice').textContent.trim(), 'copy success notice missing')
  }
  async function retryable(label, timeout = 6000) {
    await waitFor(() => $('#workspace-migrate-dialog').open &&
      !$('#workspace-migrate-apply').disabled &&
      visible($('#workspace-migrate-error')) && $('#workspace-migrate-error').textContent.trim(), label, timeout)
  }
  try {
    window.workspaceTestStage = 'setup'
    await waitFor(() => window.panelDualNotes && $('.note-cell.mounted') && !$('#days').inert, 'initial page')
    const now = new Date()
    const current = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
    const date = new Date(now.getFullYear(), now.getMonth() - 1, 1, 12)
    const sourceMonth = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`
    $('#workspace-mode').click()
    await waitFor(() => tables().length === 1 && !$('#days').inert, 'current workspace')
    assert(!visible($('#migrate-workspace-tables')), 'migration control is visible in the current month')
    title(tables()[0], '本月已有表')
    await edit(rows(tables()[0])[0], '目标月原有正文。')
    let snapshot = await save()
    const targetOriginal = canonical(snapshot.months[current])

    await selectMonth(sourceMonth)
    assert(visible($('#migrate-workspace-tables')), 'migration control is hidden in a historical month')
    const sourceHeaders = ['源表甲', '源表乙', '源表丙']
    const sourceBodies = ['源甲正文，不进入标题列表。', '源乙正文，留待重试验证。', '源丙正文，保持源顺序。']
    for (let index = 0; index < 3; index++) {
      if (index) {
        $('#add-workspace-table').click()
        await waitFor(() => tables().length === index + 1, 'append source table')
      }
      title(tables()[index], sourceHeaders[index])
      await edit(rows(tables()[index])[0], sourceBodies[index])
    }
    snapshot = await save()
    const sourceIDs = tableIDs()
    const sourceBeforeCancel = canonical(snapshot.months[sourceMonth])
    const savesBeforeCancel = snapshot.workspaceSaves
    await openMigration()
    $('#workspace-migrate-none').click()
    assert(choices().every(input => !input.checked) && $('#workspace-migrate-apply').disabled, 'empty selection can be applied')
    assert(same($$('#workspace-migrate-list .workspace-migrate-title').map(item => item.textContent.trim()), sourceHeaders), 'selection dialog does not use table headers in source order')
    assert(!$('#workspace-migrate-list').textContent.includes('源甲正文') && !$('#workspace-migrate-list').querySelector('.ProseMirror, textarea'), 'selection dialog includes table bodies')
    $('#workspace-migrate-all').click()
    assert(choices().length === 3 && choices().every(input => input.checked), 'select all skipped a table')
    $('#workspace-migrate-cancel').click()
    await waitFor(() => !$('#workspace-migrate-dialog').open, 'cancel migration')
    await pause(750)
    snapshot = await fixture()
    assert(snapshot.copyRequests.length === 0 && snapshot.copyCommits === 0, 'cancel sent a copy request')
    assert(snapshot.workspaceSaves === savesBeforeCancel, 'selection/cancel wrote note data')
    assert(same(canonical(snapshot.months[sourceMonth]), sourceBeforeCancel), 'selection/cancel changed source month')
    assert(same(canonical(snapshot.months[current]), targetOriginal), 'selection/cancel changed current month')

    // Leave an unsaved final edit in the target while visiting the source. The
    // migration must flush that local draft before appending copied tables.
    await selectMonth(current)
    await fixture({ action: 'blockWorkspaceSaves', month: current })
    await edit(rows(tables()[0])[0], '目标月原有正文。目标月未保存末笔。')
    await selectMonth(sourceMonth)
    await pause(800)
    snapshot = await fixture()
    assert(!snapshot.months[current].tables[0].rows[0].content.includes('未保存末笔'), 'fixture did not retain an unsaved target draft')
    await edit(rows(tables()[0])[0], '源甲正文，不进入标题列表。源月未保存末笔。')
    await fixture({ action: 'unblockWorkspaceSaves', month: current })
    await openMigration()
    $('#workspace-migrate-none').click()
    // Deliberately choose in reverse order; copies must retain source order.
    choose(sourceIDs[2])
    choose(sourceIDs[0])
    assert(!$('#workspace-migrate-apply').disabled, 'multi-selection cannot be applied')
    await fixture({ snapshot: 'migration' })
    window.workspaceTestStage = 'before first copy'
    await fixture({ action: 'failNextCopy' })
    $('#workspace-migrate-apply').click()
    await retryable('retry after pre-commit service failure')
    window.workspaceTestStage = 'retrying first copy'
    snapshot = await fixture()
    assert(snapshot.copyRequests.length === 1 && snapshot.copyCommits === 0, 'pre-commit failure wrote copies')
    const firstOperation = snapshot.copyRequests[0].operationId
    $('#workspace-migrate-apply').click()
    await completed()
    snapshot = await fixture()
    assert(snapshot.copyRequests[1].operationId === firstOperation, 'failure retry generated a different operation ID')
    assert(snapshot.copyCommits === 1, 'first operation committed more than once')
    assert($('#workspace-month').value === sourceMonth && same(tableIDs(), sourceIDs), 'success left or changed the source month')
    window.workspaceTestStage = 'first copy succeeded'
    const sourceAtCopy = snapshot.copySources[firstOperation]
    const targetBeforeCopy = snapshot.copyTargetsBefore[firstOperation]
    assert(same(canonical(snapshot.months[sourceMonth]), canonical(sourceAtCopy)), 'copy changed source data or revision')
    assert(sourceAtCopy.tables[0].rows[0].content.includes('源月未保存末笔'), 'copy missed the final source edit')
    assert(targetBeforeCopy.tables[0].rows[0].content.includes('目标月未保存末笔'), 'copy overwrote the final target draft')
    const targetAfterCopy = snapshot.months[current]
    assert(targetAfterCopy.tables.length === 3, 'selected tables were not appended to target')
    assert(targetAfterCopy.tables[0].id === targetOriginal.tables[0].id && targetAfterCopy.tables[0].rows[0].id === targetOriginal.tables[0].rows[0].id, 'existing target IDs changed')
    assert(targetAfterCopy.tables[0].rows[0].content === targetBeforeCopy.tables[0].rows[0].content, 'existing target content was overwritten')
    assert(same(targetAfterCopy.tables.slice(1).map(table => table.header), [sourceHeaders[0], sourceHeaders[2]]), 'copies did not retain original source order')
    const sourceRowIDs = new Set(sourceAtCopy.tables.flatMap(table => table.rows.map(row => row.id)))
    for (const copied of targetAfterCopy.tables.slice(1)) {
      assert(!sourceIDs.includes(copied.id), 'copied table reused source ID')
      assert(copied.rows.every(row => !sourceRowIDs.has(row.id)), 'copied row reused source ID')
      const source = sourceAtCopy.tables.find(table => table.header === copied.header)
      assert(same(copied.rows.map(row => row.content), source.rows.map(row => row.content)), 'copied body changed')
    }
    assert(new Set(targetAfterCopy.tables.map(table => table.id)).size === 3, 'target has duplicate table IDs')
    assert(same(snapshot.copyResults[firstOperation].copiedTableIds, targetAfterCopy.tables.slice(1).map(table => table.id)), 'copy result does not identify new tables')

    // Commit a second copy but lose its successful response. The native
    // transport reports no HTTP status; a retry must recover without duplication.
    await openMigration()
    $('#workspace-migrate-none').click()
    choose(sourceIDs[1])
    window.workspaceTestStage = 'before dropped response'
    await fixture({ action: 'dropNextCopyResponse' })
    $('#workspace-migrate-apply').click()
    await waitFor(async () => (await fixture()).copyCommits === 2, 'commit with dropped response')
    snapshot = await fixture()
    const droppedRequest = snapshot.copyRequests[snapshot.copyRequests.length - 1]
    const targetAfterDropped = canonical(snapshot.months[current])
    assert(targetAfterDropped.tables.length === 4, 'dropped response fixture did not commit exactly one table')
    window.workspaceTestStage = 'waiting for lost response error'
    await retryable('retry after lost bridge response')
    window.workspaceTestStage = 'retrying lost response'
    $('#workspace-migrate-apply').click()
    await completed()
    snapshot = await fixture()
    assert(snapshot.copyRequests[snapshot.copyRequests.length - 1].operationId === droppedRequest.operationId, 'lost-response retry changed operation ID')
    assert(snapshot.copyCommits === 2 && same(canonical(snapshot.months[current]), targetAfterDropped), 'lost-response retry duplicated or rewrote copied tables')
    assert(same(canonical(snapshot.months[sourceMonth]), canonical(sourceAtCopy)), 'retry changed source month')
    assert($('#workspace-month').value === sourceMonth, 'retry success unexpectedly navigated away')
    const goToCurrent = $('#notice').querySelector('button')
    assert(goToCurrent, 'success notice lacks a current-month navigation button')
    goToCurrent.click()
    await waitFor(() => $('#workspace-month').value === current && tables().length === 4 && !$('#days').inert, 'open current month after copy')
    assert(!visible($('#migrate-workspace-tables')), 'migration remains available in target current month')
    assert(tables()[0].textContent.includes('目标月未保存末笔'), 'current-month view lost its pending edit')
    assert(same(tables().map(table => table.querySelector('.workspace-column-header').value), ['本月已有表', '源表甲', '源表丙', '源表乙']), 'current-month view shows incorrect copy order')
    assert(Object.keys(snapshot.days).length === 0, 'monthly copy wrote daily notes')
    window.webkit.messageHandlers.result.postMessage('PASS: historical-month multi-select copy, cancel/no writes, source retained, unsaved edits flushed, new IDs, source order, failed/lost-response retries without duplication')
  } catch (error) {
    window.webkit.messageHandlers.result.postMessage(`FAIL: ${error.message}`)
  }
})()
