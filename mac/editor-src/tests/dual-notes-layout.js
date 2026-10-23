(async () => {
  const pause = ms => new Promise(resolve => setTimeout(resolve, ms))
  const assert = (condition, message) => { if (!condition) throw new Error(message) }
  async function waitFor(condition) {
    for (let attempt = 0; attempt < 200; attempt++) {
      if (condition()) return
      await pause(25)
    }
    throw new Error('page initialization timed out')
  }
  const resize = (width, zoom) => new Promise(resolve => {
    window.layoutDone = resolve
    window.webkit.messageHandlers.layout.postMessage({ width, zoom })
  })
  function checkLayout(label, ratio) {
    let mounted = 0
    let previews = 0
    for (const cell of document.querySelectorAll('.note-cell')) {
      const row = cell.closest('.note-row').getBoundingClientRect()
      const share = cell.classList.contains('left') ? ratio : 100 - ratio
      assert(Math.abs(cell.getBoundingClientRect().width / row.width * 100 - share) < 0.2,
        `${label}: incorrect column ratio`)
      if (cell.classList.contains('mounted')) mounted++
      if (cell.querySelector('.cell-preview')) previews++
      for (const block of cell.querySelectorAll('.ProseMirror > p, .ProseMirror > pre, .cell-preview > p, .cell-preview > pre')) {
        const bounds = block.getBoundingClientRect()
        const range = document.createRange()
        range.selectNodeContents(block)
        const text = range.getBoundingClientRect()
        assert(bounds.bottom <= row.bottom + 2 && text.bottom <= row.bottom + 2,
          `${label}: ${cell.classList.contains('mounted') ? 'editor' : 'preview'} text overflows its row`)
        assert(text.bottom <= bounds.bottom + 2, `${label}: text overflows its paragraph`)
      }
    }
    assert(mounted > 0 && previews > 0, `${label}: expected both mounted editors and previews`)
    return document.querySelector('.note-row').getBoundingClientRect().height
  }
  try {
    await waitFor(() => !document.getElementById('days').inert && document.querySelector('.note-cell.mounted'))
    await document.fonts.ready
    await pause(100)
    const editors = [...document.querySelectorAll('.ProseMirror')]
    assert(editors.length > 0, 'no mounted editor')
    let checks = 0
    for (const zoom of [1, 1.8]) {
      for (const ratio of [50, 20, 80]) {
        // Change the ratio once per scenario, never between width steps: a CSS
        // write would invalidate layout and conceal the original resize bug.
        const slider = document.getElementById('focus-ratio-slider')
        slider.value = String(ratio)
        slider.dispatchEvent(new Event('input', { bubbles: true }))
        const heights = []
        for (const width of [1293, 1000, 789, 1293]) {
          // All widths stay above the 720px media query, including at 1.8x zoom.
          await resize(width * zoom, zoom)
          assert(Math.abs(window.innerWidth - width) <= 1, 'native resize did not reach the requested CSS width')
          heights.push(checkLayout(`${zoom}x / ${ratio}% / ${width}px`, ratio))
          assert(editors.every(editor => editor.isConnected), 'resizing recreated an editor')
          checks++
        }
        assert(heights[2] > heights[0] + 20, 'narrowing did not increase the row height')
        assert(heights[3] < heights[2] - 20, 'widening did not reduce the row height')
      }
    }
    window.webkit.messageHandlers.result.postMessage(`PASS: ${checks} native resizes; editor and preview rows, column ratios, zoom, and editor identity`)
  } catch (error) {
    window.webkit.messageHandlers.result.postMessage(`FAIL: ${error.message}`)
  }
})()
