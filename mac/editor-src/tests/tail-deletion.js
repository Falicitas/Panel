(async () => {
  const pageErrors = []
  window.addEventListener('error', event => pageErrors.push(event.message))
  window.addEventListener('unhandledrejection', event => pageErrors.push(String(event.reason)))
  const pause = ms => new Promise(resolve => setTimeout(resolve, ms))
  const assert = (condition, message) => { if (!condition) throw new Error(message) }
  const $ = selector => document.querySelector(selector)
  const $$ = selector => [...document.querySelectorAll(selector)]
  async function waitFor(condition, label) {
    window.workspaceTestStage = label
    const deadline = Date.now() + 6000
    while (Date.now() < deadline) {
      if (condition()) return
      await pause(25)
    }
    throw new Error(`timed out: ${label}`)
  }
  const requests = new Map()
  window.workspaceFixtureReply = result => {
    requests.get(result.id)?.(result)
    requests.delete(result.id)
  }
  const fixture = () => new Promise(resolve => {
    const id = crypto.randomUUID()
    requests.set(id, resolve)
    window.webkit.messageHandlers.fixture.postMessage({ id })
  })
  const daySections = () => $$('#days .day-section')
  const tables = () => $$('#days .workspace-table[data-table-id]')
  const rows = section => [...section.querySelectorAll('.note-row[data-row-id]')]
  const dialog = () => $('#tail-delete-dialog')
  async function edit(cell, text) {
    cell.scrollIntoView({ block: 'center' })
    cell.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
    await waitFor(() => cell.classList.contains('mounted') && cell.querySelector('.ProseMirror'), 'editor mount')
    const editor = cell.querySelector('.ProseMirror')
    editor.focus()
    const range = document.createRange()
    range.selectNodeContents(editor)
    const selection = window.getSelection()
    selection.removeAllRanges()
    selection.addRange(range)
    assert(document.execCommand(text ? 'insertText' : 'delete', false, text || null), 'editor input failed')
    await pause(75)
    assert(text ? editor.textContent.includes(text) : editor.textContent.trim() === '', 'editor content mismatch')
  }
  function addRow(section) {
    const control = [...section.querySelectorAll('button')].find(button => button.textContent.trim() === '＋ 添加一行')
    assert(control, 'add-row button missing')
    control.click()
  }
  async function save() {
    window.workspaceTestStage = 'save after ' + window.workspaceTestStage
    await pause(50)
    await window.panelDualNotes.flush()
    return fixture()
  }
  async function confirmDeletion() {
    await waitFor(() => dialog().open, 'deletion confirmation')
    assert($('#tail-delete-message').textContent.trim(), 'deletion warning is empty')
    $('#tail-delete-confirm').click()
    await waitFor(() => !dialog().open, 'confirmation closes')
  }
  async function selectMonth(month) {
    $('#workspace-month').value = month
    $('#workspace-month').dispatchEvent(new Event('change', { bubbles: true }))
    await pause(50)
    await waitFor(() => $('#workspace-month').value === month && !$('#days').inert, 'month load')
  }
  try {
    await waitFor(() => window.panelDualNotes && $('.note-cell.mounted') && !$('#days').inert, 'focus mode')
    let focus = daySections()[0]
    const today = focus.dataset.date
    const firstRowID = rows(focus)[0].dataset.rowId
    addRow(focus)
    await waitFor(() => rows(focus).length === 2, 'append focus tail row')
    await edit(rows(focus)[1].querySelector('.note-cell.right'), '只有右栏的尾行内容。')
    let snapshot = await save()
    const beforeCancel = snapshot.days[today]
    assert(beforeCancel.rows[1].left.trim() === '' && beforeCancel.rows[1].right.includes('只有右栏'), 'right-only fixture is incorrect')
    focus.querySelector('.delete-tail-row').click()
    await waitFor(() => dialog().open, 'right-column data confirmation')
    $('#tail-delete-cancel').click()
    await waitFor(() => !dialog().open, 'cancel deletion')
    snapshot = await save()
    assert(rows(focus).length === 2 && snapshot.days[today].revision === beforeCancel.revision && snapshot.days[today].rows[1].right === beforeCancel.rows[1].right, 'cancel deleted or saved the row')
    focus.querySelector('.delete-tail-row').click()
    await confirmDeletion()
    await waitFor(() => rows(focus).length === 1, 'delete focus tail row')
    assert(rows(focus)[0].dataset.rowId === firstRowID && rows(focus)[0].textContent.includes('独立的每日记录'), 'tail deletion changed the preceding row')
    focus.querySelector('.delete-tail-row').click()
    await confirmDeletion()
    await waitFor(() => rows(focus).length === 0, 'delete final focus row')
    snapshot = await save()
    assert(snapshot.days[today].rows.length === 0, 'empty day was not saved')

    $('#overview-mode').click()
    await waitFor(() => daySections().length === 10 && !$('#days').inert, 'overview mode')
    assert(rows(daySections().find(section => section.dataset.date === today)).length === 0, 'saved empty day regained a row')
    const target = daySections()[1]
    const neighbor = daySections()[2]
    const targetDate = target.dataset.date
    const neighborDate = neighbor.dataset.date
    const neighborRows = rows(neighbor).map(row => row.dataset.rowId)
    const bodyText = section => [...section.querySelectorAll('.note-cell')].map(cell => (cell.querySelector('.ProseMirror, .cell-preview')?.textContent || '').trim())
    const neighborText = bodyText(neighbor)
    target.querySelector('.delete-tail-row').click()
    await confirmDeletion()
    await waitFor(() => rows(target).length === 0, 'overview target tail deleted')
    snapshot = await save()
    assert(snapshot.days[targetDate].rows.length === 0, 'overview did not save the selected date')
    assert(!snapshot.days[neighborDate] && JSON.stringify(rows(neighbor).map(row => row.dataset.rowId)) === JSON.stringify(neighborRows) && JSON.stringify(bodyText(neighbor)) === JSON.stringify(neighborText), 'overview deletion changed another day')
    $('#focus-mode').click()
    await waitFor(() => daySections().length === 1 && !$('#days').inert, 'return focus')
    focus = daySections()[0]
    assert(rows(focus).length === 0, 'focus reload resurrected deleted row')
    addRow(focus)
    await waitFor(() => rows(focus).length === 1, 'add row after empty day')
    await save()

    $('#workspace-mode').click()
    await waitFor(() => tables().length === 1 && !$('#days').inert, 'workspace mode')
    const month = $('#workspace-month').value
    const [year, number] = month.split('-').map(Number)
    const priorDate = new Date(year, number - 2, 1, 12)
    const prior = `${priorDate.getFullYear()}-${String(priorDate.getMonth() + 1).padStart(2, '0')}`
    assert(rows(tables()[0]).length === 1, 'new unsaved month lacks its default blank row')
    tables()[0].querySelector('.delete-tail-row').click()
    await waitFor(() => rows(tables()[0]).length === 0, 'delete blank workspace row')
    assert(!dialog().open, 'blank row required confirmation')
    snapshot = await save()
    assert(snapshot.months[month].tables[0].rows.length === 0, 'zero-row table was not saved')
    await selectMonth(prior)
    await selectMonth(month)
    assert(tables().length === 1 && rows(tables()[0]).length === 0, 'month reload resurrected a row')
    addRow(tables()[0])
    await waitFor(() => rows(tables()[0]).length === 1, 'add workspace row')
    await edit(rows(tables()[0])[0].querySelector('.note-cell.single'), '工作区尾行需要确认。')
    tables()[0].querySelector('.delete-tail-row').click()
    await confirmDeletion()
    await waitFor(() => rows(tables()[0]).length === 0, 'delete nonempty workspace row')
    const header = tables()[0].querySelector('.workspace-column-header')
    header.value = '只有标题也要确认'
    header.dispatchEvent(new Event('input', { bubbles: true }))
    await save()
    $('#delete-workspace-table').click()
    await confirmDeletion()
    await waitFor(() => tables().length === 0, 'delete header-only table')
    snapshot = await save()
    assert(snapshot.months[month].tables.length === 0, 'empty month was not saved')
    await selectMonth(prior)
    await selectMonth(month)
    assert(tables().length === 0, 'saved empty month regained a table')
    $('#add-workspace-table').click()
    await waitFor(() => tables().length === 1 && rows(tables()[0]).length === 1, 'add table after empty month')
    $('#delete-workspace-table').click()
    await waitFor(() => tables().length === 0, 'delete empty table directly')
    assert(!dialog().open, 'empty table required confirmation')
    await save()
    $('#add-workspace-table').click()
    await waitFor(() => tables().length === 1, 'append body-only table')
    await edit(rows(tables()[0])[0].querySelector('.note-cell.single'), '没有标题，但正文仍然需要确认。')
    $('#delete-workspace-table').click()
    await confirmDeletion()
    await waitFor(() => tables().length === 0, 'delete body-only table')
    await save()
    $('#add-workspace-table').click()
    await waitFor(() => tables().length === 1, 're-add table')
    addRow(tables()[0])
    await waitFor(() => rows(tables()[0]).length === 2, 're-add rows after deletion')
    snapshot = await save()
    assert(snapshot.months[month].tables.length === 1 && snapshot.months[month].tables[0].rows.length === 2, 're-added content did not persist')
    window.webkit.messageHandlers.result.postMessage('PASS: focus/overview tail-row deletion, content confirmation and cancel, empty row/table direct deletion, header/body table protection, zero-state reload and re-add')
  } catch (error) {
    window.webkit.messageHandlers.result.postMessage(`FAIL: ${error.message}; page errors: ${pageErrors.join(' | ')}; tables=${tables().length}; inert=${$('#days').inert}; addDisabled=${$('#add-workspace-table')?.disabled}`)
  }
})()
