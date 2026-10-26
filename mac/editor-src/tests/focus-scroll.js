(async () => {
  const pause = ms => new Promise(resolve => setTimeout(resolve, ms))
  const assert = (condition, message) => { if (!condition) throw new Error(message) }
  const $ = selector => document.querySelector(selector)
  const $$ = selector => [...document.querySelectorAll(selector)]
  async function waitFor(condition, label) {
    const deadline = Date.now() + 6000
    while (Date.now() < deadline) {
      if (condition()) return
      await pause(25)
    }
    throw new Error(`timed out: ${label}`)
  }
  const distanceToEnd = () => document.documentElement.scrollHeight - window.innerHeight - window.scrollY
  const atEnd = () => distanceToEnd() <= 2
  const lastCells = () => [...$$('.note-row').pop().querySelectorAll('.note-cell')]
  const resize = width => new Promise(resolve => {
    window.layoutDone = resolve
    window.webkit.messageHandlers.layout.postMessage({ width, zoom: 1 })
  })
  try {
    await waitFor(() => window.panelDualNotes && !$('#days').inert && $$('.note-row').length === 4, 'initial focus load')
    await waitFor(atEnd, 'initial focus scrolls to page end')
    assert(!document.activeElement.closest('.ProseMirror'), 'initial reveal focused an editor')
    await waitFor(() => lastCells().every(cell => cell.classList.contains('mounted')), 'bottom editors mount')
    await document.fonts.ready
    await pause(150)
    assert(atEnd(), `bottom mounting moved the page away from the end: ${distanceToEnd()}`)

    window.dispatchEvent(new WheelEvent('wheel', { deltaY: -600, bubbles: true }))
    window.scrollTo({ top: 0 })
    await pause(100)
    await resize(789)
    await pause(150)
    assert(distanceToEnd() > 200 && window.scrollY < 50, 'layout change pulled the user back down after upward scrolling')

    $('#overview-mode').click()
    await waitFor(() => $('#overview-mode').getAttribute('aria-pressed') === 'true' && !$('#days').inert && $$('.day-section').length === 10, 'overview load')
    await pause(100)
    assert(window.scrollY <= 2, 'overview did not open at page top')
    $('#focus-mode').click()
    await waitFor(() => $('#focus-mode').getAttribute('aria-pressed') === 'true' && !$('#days').inert && $$('.note-row').length === 4, 'return focus')
    await waitFor(() => lastCells().every(cell => cell.classList.contains('mounted')) && atEnd(), 'return focus scrolls to mounted bottom')
    await pause(150)
    assert(atEnd(), 'returning focus did not remain at the loaded end')

    const editor = lastCells()[0].querySelector('.ProseMirror')
    const walker = document.createTreeWalker(editor, NodeFilter.SHOW_TEXT)
    let text = walker.nextNode()
    while (text && text.textContent.trim().length < 5) text = walker.nextNode()
    assert(text, 'fixture has no selectable text')
    editor.focus()
    const selection = window.getSelection()
    const range = document.createRange()
    range.setStart(text, 1)
    range.setEnd(text, 4)
    selection.removeAllRanges()
    selection.addRange(range)
    await pause(50)
    const before = { anchor: selection.anchorNode, start: selection.anchorOffset, focus: selection.focusNode, end: selection.focusOffset, active: document.activeElement }
    window.panelDualNotes.reveal()
    await waitFor(atEnd, 'explicit reveal scrolls to end')
    await pause(100)
    assert(selection.anchorNode === before.anchor && selection.anchorOffset === before.start && selection.focusNode === before.focus && selection.focusOffset === before.end, 'reveal changed the selected text')
    assert(document.activeElement === before.active, 'reveal moved keyboard focus')
    window.webkit.messageHandlers.result.postMessage('PASS: focus starts/returns at end, lazy bottom mounting stays anchored, user scroll cancels following, overview starts at top, reveal preserves selection and focus')
  } catch (error) {
    window.webkit.messageHandlers.result.postMessage(`FAIL: ${error.message}`)
  }
})()
