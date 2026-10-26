(async () => {
  const pause = ms => new Promise(resolve => setTimeout(resolve, ms))
  const assert = (condition, message) => { if (!condition) throw new Error(message) }
  const $ = selector => document.querySelector(selector)
  const $$ = selector => [...document.querySelectorAll(selector)]
  async function waitFor(condition, label) {
    for (let attempt = 0; attempt < 240; attempt++) {
      if (condition()) return
      await pause(25)
    }
    throw new Error(`timed out: ${label}`)
  }
  const nativeRequests = new Map()
  window.workspaceFixtureReply = result => {
    nativeRequests.get(result.id)?.(result)
    nativeRequests.delete(result.id)
  }
  const fixture = (options = {}) => new Promise(resolve => {
    const id = crypto.randomUUID()
    nativeRequests.set(id, resolve)
    window.webkit.messageHandlers.fixture.postMessage({ id, ...options })
  })
  const resize = (width, zoom = 1) => new Promise(resolve => {
    window.layoutDone = resolve
    window.webkit.messageHandlers.layout.postMessage({ width: width * zoom, zoom })
  })
  const tables = () => $$('section.workspace-table[data-table-id]')
  const titleInput = table => table.querySelector('input.workspace-column-header')
  const rowElements = table => [...table.querySelectorAll('.note-row[data-row-id]')]
  async function ready() {
    await waitFor(() => !$('#days').inert, 'page not inert')
  }
  async function setMonth(month) {
    $('#workspace-month').value = month
    $('#workspace-month').dispatchEvent(new Event('change', { bubbles: true }))
    await pause(40)
    await ready()
    await waitFor(() => $('#workspace-month').value === month && tables().length > 0, `month ${month}`)
  }
  async function edit(row, text) {
    const cell = row.querySelector('.note-cell.single')
    assert(cell, 'single-column cell missing')
    cell.scrollIntoView({ block: 'center' })
    cell.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
    await waitFor(() => cell.classList.contains('mounted') && cell.querySelector('.ProseMirror'), 'editor mount')
    const editor = cell.querySelector('.ProseMirror')
    assert(!editor.classList.contains('virtual-cursor-enabled'), 'workspace retained the stale-coordinate virtual cursor')
    const caret = getComputedStyle(editor).caretColor
    assert(caret !== 'transparent' && caret !== 'rgba(0, 0, 0, 0)', 'native text caret is hidden')
    editor.focus()
    const selection = window.getSelection()
    const range = document.createRange()
    range.selectNodeContents(editor)
    selection.removeAllRanges()
    selection.addRange(range)
    assert(document.execCommand('insertText', false, text), 'contenteditable input failed')
    await pause(75)
    assert(editor.textContent.includes(text.slice(0, 30)), 'typed text missing from editor')
    return editor
  }
  function setTitle(table, text) {
    const input = titleInput(table)
    assert(input, 'editable column header missing')
    input.value = text
    input.dispatchEvent(new Event('input', { bubbles: true }))
  }
  function addRow(table) {
    const control = [...table.querySelectorAll('button')].find(button => button.textContent.trim() === '＋ 添加一行')
    assert(control, 'add-row button missing')
    control.click()
  }
  async function save() {
    await pause(50)
    await window.panelDualNotes.flush()
    return fixture()
  }
  function checkLayout(label) {
    const table = tables()[0]
    for (const row of rowElements(table)) {
      const bounds = row.getBoundingClientRect()
      const cell = row.querySelector('.note-cell.single').getBoundingClientRect()
      assert(Math.abs(cell.width - bounds.width) < 2, `${label}: single column does not fill row`)
      for (const paragraph of row.querySelectorAll('.ProseMirror > p, .cell-preview > p')) {
        const block = paragraph.getBoundingClientRect()
        const range = document.createRange()
        range.selectNodeContents(paragraph)
        const text = range.getBoundingClientRect()
        assert(block.bottom <= bounds.bottom + 2 && text.bottom <= bounds.bottom + 2, `${label}: row height does not contain text`)
      }
    }
    assert(document.documentElement.scrollWidth <= window.innerWidth + 2,
      `${label}: page overflows horizontally (${document.documentElement.scrollWidth}/${window.innerWidth}): ` +
      $$('body *').filter(item => item.getBoundingClientRect().right > window.innerWidth + 2)
        .slice(0, 8).map(item => `${item.tagName}.${item.className}:${Math.round(item.getBoundingClientRect().right)}`).join(', '))
    return rowElements(table)[0].getBoundingClientRect().height
  }
  try {
    await waitFor(() => window.panelDualNotes && $('.note-cell.mounted') && !$('#days').inert, 'initial focus mode')
    const now = new Date()
    const currentMonth = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
    const previousDate = new Date(now.getFullYear(), now.getMonth() - 1, 1, 12)
    const previousMonth = `${previousDate.getFullYear()}-${String(previousDate.getMonth() + 1).padStart(2, '0')}`
    $('#workspace-mode').click()
    await waitFor(() => tables().length === 1 && !$('#days').inert, 'workspace mode')
    assert($('#workspace-month').value === currentMonth, 'workspace did not open current month')
    assert($('#workspace-mode').getAttribute('aria-pressed') === 'true', 'workspace not selected')
    assert($('#column-ratio-controls').hidden, 'two-column controls visible in workspace')
    assert(rowElements(tables()[0]).length === 1, 'empty month needs one editable row')
    await pause(800)
    assert((await fixture()).workspaceSaves === 0, 'reading an empty month created a saved record')

    setTitle(tables()[0], '日常放松')
    const content = '月度检索词。' + '每天记录近期关注的事情，并在窗口宽度变化后保持完整排版。Mixed English words also wrap naturally. '.repeat(35)
    await edit(rowElements(tables()[0])[0], content)
    addRow(tables()[0])
    await waitFor(() => rowElements(tables()[0]).length === 2, 'first table second row')
    await edit(rowElements(tables()[0])[1], '第二行保持自己的内容。')
    $('#add-workspace-table').click()
    await waitFor(() => tables().length === 2, 'second table')
    setTitle(tables()[1], '晨间安排')
    await edit(rowElements(tables()[1])[0], '第二张表的单栏正文。')
    let snapshot = await save()
    const original = snapshot.months[currentMonth]
    assert(original.tables.length === 2, 'table count not saved')
    assert(original.tables[0].header === '日常放松' && original.tables[1].header === '晨间安排', 'headers not saved in order')
    assert(original.tables[0].rows.length === 2, 'new row not saved')
    assert(original.tables[0].rows[0].content.includes('月度检索词'), 'first row content not saved')
    assert(original.tables[0].rows[1].content.includes('第二行'), 'second row content not saved')
    assert(original.tables[1].rows[0].content.includes('第二张表'), 'new table content not saved')
    assert(new Set(original.tables.map(table => table.id)).size === 2, 'table IDs are not unique')

    $('#workspace-prev').click()
    await waitFor(() => $('#workspace-month').value === previousMonth && tables().length === 1 && !$('#days').inert, 'previous month')
    assert(!$('#days').textContent.includes('月度检索词'), 'previous month shows current month content')
    setTitle(tables()[0], '上个月的事项')
    await edit(rowElements(tables()[0])[0], '上个月独有正文。')
    await save()
    $('#workspace-next').click()
    await waitFor(() => $('#workspace-month').value === currentMonth && tables().length === 2 && !$('#days').inert, 'next month')
    assert(titleInput(tables()[0]).value === '日常放松', 'current month header not restored')
    assert($('#days').textContent.includes('月度检索词'), 'current month content not restored')
    assert(!$('#days').textContent.includes('上个月独有正文'), 'months share content')
    await setMonth(previousMonth)
    assert(titleInput(tables()[0]).value === '上个月的事项', 'month picker did not load saved month')
    $('#workspace-current').click()
    await waitFor(() => $('#workspace-month').value === currentMonth && tables().length === 2 && !$('#days').inert, 'return current month')

    $('#find-toggle').click()
    $('#find-input').value = '月度检索词'
    $('#find-input').dispatchEvent(new Event('input', { bubbles: true }))
    await waitFor(() => $('.find-hit') && $('#find-count').textContent.includes('1 / 1'), 'monthly content search')
    $('#find-close').click()
    const editor = await edit(rowElements(tables()[0])[0], content)
    await save()
    await document.fonts.ready
    let resizeCount = 0
    for (const zoom of [1, 1.8]) {
      const heights = []
      for (const width of [1293, 1000, 789, 1293, 380]) {
        await resize(width, zoom)
        assert(Math.abs(window.innerWidth - width) <= 1, 'native window did not reach requested width')
        heights.push(checkLayout(`${width}px at ${zoom}x`))
        assert(editor.isConnected, 'resizing recreated the active workspace editor')
        resizeCount++
        if (zoom === 1 && (width === 1293 || width === 380)) {
          window.scrollTo({ top: 0 })
          await pause(75)
          await fixture({ snapshot: width === 380 ? 'narrow' : 'desktop' })
        }
      }
      assert(heights[2] > heights[0] + 20, 'narrowing did not grow monthly row height')
      assert(heights[3] < heights[2] - 20, 'widening did not shrink monthly row height')
    }
    await resize(1100)
    $('#focus-mode').click()
    await waitFor(() => $('#focus-mode').getAttribute('aria-pressed') === 'true' && $('.note-cell.right') && !$('#days').inert, 'return focus')
    assert($('#days').textContent.includes('独立的每日记录'), 'daily content changed')
    assert(tables().length === 0, 'workspace tables remained in focus mode')
    $('#overview-mode').click()
    await waitFor(() => $('#overview-mode').getAttribute('aria-pressed') === 'true' && $$('.day-section').length === 10 && !$('#days').inert, 'overview')
    $('#workspace-mode').click()
    await waitFor(() => tables().length === 2 && !$('#days').inert, 'return workspace')
    assert(titleInput(tables()[1]).value === '晨间安排', 'mode switching lost table title')
    snapshot = await fixture()
    assert(snapshot.months[previousMonth].tables[0].rows[0].content.includes('上个月独有正文'), 'previous month save changed')
    assert(Object.keys(snapshot.days).length === 0, 'workspace editing unexpectedly saved day data')
    window.webkit.messageHandlers.result.postMessage(`PASS: workspace month isolation, multi-table editing, row append, save/restore, find, focus/overview, and ${resizeCount} native resizes`)
  } catch (error) {
    window.webkit.messageHandlers.result.postMessage(`FAIL: ${error.message}`)
  }
})()
