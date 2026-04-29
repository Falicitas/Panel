import { Crepe } from '@milkdown/crepe'
import '@milkdown/crepe/theme/common/style.css'
import '@milkdown/crepe/theme/classic.css'
import '@milkdown/crepe/theme/classic-dark.css'
import './panel.css'

declare global {
  interface Window {
    panelEditor: {
      init(initialMarkdown: string): Promise<void>
      setRaw(markdown: string): Promise<void>
      getRaw(): string
      destroy(): Promise<void>
      scrollToText(text: string): void
    }
    webkit?: {
      messageHandlers: {
        markdown: { postMessage(payload: unknown): void }
      }
    }
  }
}

let crepe: Crepe | null = null
let suppressNext = false

function postToSwift(payload: unknown): void {
  window.webkit?.messageHandlers?.markdown?.postMessage(payload)
}

async function init(initialMarkdown: string): Promise<void> {
  const root = document.getElementById('editor')
  if (!root) throw new Error('#editor missing')
  crepe = new Crepe({ root, defaultValue: initialMarkdown })
  await crepe.create()
  crepe.on((listener) => {
    listener.markdownUpdated((_ctx, md, prev) => {
      if (suppressNext) {
        suppressNext = false
        return
      }
      if (md !== prev) postToSwift({ type: 'change', md })
    })
  })
  postToSwift({ type: 'ready' })
}

async function setRaw(markdown: string): Promise<void> {
  if (crepe) {
    await crepe.destroy()
    crepe = null
  }
  await init(markdown)
}

function getRaw(): string {
  return crepe?.getMarkdown() ?? ''
}

async function destroy(): Promise<void> {
  if (crepe) {
    await crepe.destroy()
    crepe = null
  }
}

/** 把后端给的 focus.text (任务原文,parser 已剥 markdown 标记) 在 webview
 * DOM 里 scrollIntoView + 短暂高亮。比从 raw md 行号反推干净很多。 */
function scrollToText(text: string): void {
  const probe = text.trim().slice(0, 40)
  if (!probe) return

  const root = document.querySelector('.ProseMirror') ?? document.querySelector('.milkdown')
  if (!root) return

  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  let node: Node | null
  while ((node = walker.nextNode())) {
    if ((node.textContent ?? '').includes(probe)) {
      const block = climbToBlock(node.parentElement)
      block?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      flashHighlight(block)
      return
    }
  }
}

function climbToBlock(el: HTMLElement | null): HTMLElement | null {
  let cur = el
  while (cur && cur !== document.body) {
    const display = getComputedStyle(cur).display
    if (display.startsWith('block') || display === 'list-item' || display === 'flex') return cur
    cur = cur.parentElement
  }
  return el
}

function flashHighlight(el: HTMLElement | null): void {
  if (!el) return
  const original = el.style.transition
  const originalBg = el.style.backgroundColor
  el.style.transition = 'background-color 250ms ease'
  el.style.backgroundColor = 'rgba(243, 166, 89, 0.28)'  // anchorAccent 暖橙
  setTimeout(() => {
    el.style.backgroundColor = originalBg
    setTimeout(() => { el.style.transition = original }, 300)
  }, 1200)
}

window.panelEditor = { init, setRaw, getRaw, destroy, scrollToText }
postToSwift({ type: 'loaded' })
