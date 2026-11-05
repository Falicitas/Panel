(async () => {
  const waits = new Map()
  let waitToken = 0
  window.layoutTestResume = token => { const resume = waits.get(token); waits.delete(token); resume?.() }
  const pause = milliseconds => new Promise(resolve => {
    const token = ++waitToken
    waits.set(token, resolve)
    window.webkit.messageHandlers.testWait.postMessage({ token, milliseconds })
  })
  const assert = (condition, message) => { if (!condition) throw new Error(message) }
  const $ = selector => document.querySelector(selector)
  const $$ = selector => [...document.querySelectorAll(selector)]
  const checks = []
  let matchCells = []
  async function waitFor(condition, label) {
    const deadline = Date.now() + 6000
    while (Date.now() < deadline) {
      if (condition()) return
      await pause(25)
    }
    throw new Error(`timed out: ${label}; count=${$('#find-count')?.textContent}; hits=${$$('.find-hit-active').length}; rows=${$$('.note-row').length}`)
  }
  function geometry() {
    const hit = $('.find-hit-active')
    if (!hit) return null
    const bounds = hit.getBoundingClientRect()
    return {
      top: bounds.top, bottom: bounds.bottom,
      visibleTop: Math.max($('.topbar').getBoundingClientRect().bottom, $('#findbar').getBoundingClientRect().bottom),
      visibleBottom: window.innerHeight,
      scrollY: window.scrollY,
      documentHeight: document.documentElement.scrollHeight,
      cell: hit.closest('.note-cell')?.dataset.cellKey,
    }
  }
  async function settledHit(label, index) {
    await waitFor(() => $('#find-count').textContent === `${index} / ${matchCells.length}` && $('.find-hit-active'), `${label} highlight`)
    const start = Date.now()
    let previous = ''
    let stableSince = start
    while (Date.now() - start < 5000) {
      const position = geometry()
      const signature = JSON.stringify(position)
      if (signature !== previous) stableSince = Date.now()
      previous = signature
      if (Date.now() - start >= 900 && Date.now() - stableSince >= 300) {
        checks.push({ label, index, ...position })
        return position
      }
      await pause(50)
    }
    throw new Error(`${label}: scrolling/layout did not settle; ${JSON.stringify(geometry())}`)
  }
  async function jump(label, direction, index, times = 1) {
    for (let count = 0; count < times; count++) $('#find-' + direction).click()
    return settledHit(label, index)
  }
  try {
    await waitFor(() => window.panelDualNotes && !$('#days').inert && $$('.note-row').length === 4, 'fixture load')
    $('#overview-mode').click()
    await waitFor(() => $('#overview-mode').getAttribute('aria-pressed') === 'true' && !$('#days').inert && $$('.day-section').length === 11, 'overview load')
    await document.fonts.ready
    matchCells = $$('.note-cell').filter(cell => cell.textContent.includes('block'))
    assert(matchCells.length >= 4, `fixture has too few matches: ${matchCells.length}`)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', metaKey: true, bubbles: true, cancelable: true }))
    assert(!$('#findbar').hidden, 'Cmd+F did not open find')
    $('#find-input').value = 'block'
    $('#find-input').dispatchEvent(new Event('input', { bubbles: true }))
    const first = await settledHit('initial cold search', 1)
    const second = await jump('cold next', 'next', 2)
    const secondDistance = Math.abs(second.top + second.scrollY - first.top - first.scrollY)
    assert(secondDistance > window.innerHeight / 2, `fixture matches are too close: ${secondDistance}`)
    await jump('warm previous', 'prev', 1)
    await jump('warm next', 'next', 2)
    await jump('cold distant next', 'next', 3)
    await jump('cold last next', 'next', 4)
    if (matchCells.length > 4) await jump('rapid cold distant next', 'next', matchCells.length, matchCells.length - 4)
    await jump('wrap next', 'next', 1)
    await jump('wrap previous', 'prev', matchCells.length)
    await jump('rapid previous', 'prev', 1, matchCells.length - 1)
    await jump('rapid next', 'next', matchCells.length, matchCells.length - 1)
    // A deliberate upward scroll must release any temporary find anchoring.
    window.dispatchEvent(new WheelEvent('wheel', { deltaY: -600, bubbles: true }))
    window.scrollTo({ top: 0 })
    await pause(150)
    await new Promise(resolve => {
      window.layoutDone = resolve
      window.webkit.messageHandlers.layout.postMessage({ width: 789, zoom: 1 })
    })
    await pause(400)
    assert(window.scrollY < 50, `manual upward scroll was pulled back after resize: ${window.scrollY}`)
    const failures = checks.filter(position => position.top < position.visibleTop - 1 || position.bottom > position.visibleBottom + 1 || position.cell !== matchCells[position.index - 1].dataset.cellKey)
    assert(!failures.length, `find matches outside viewport or wrong active row: ${JSON.stringify(failures)}; all=${JSON.stringify(checks)}`)
    window.webkit.messageHandlers.result.postMessage(`PASS: overview Cmd+F, cold/warm next and previous, distant wrap and rapid jumps below sticky find bar, manual scroll cancellation; ${JSON.stringify(checks)}`)
  } catch (error) {
    window.webkit.messageHandlers.result.postMessage(`FAIL: ${error.message}`)
  }
})()
