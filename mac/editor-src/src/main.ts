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
 * DOM 里 scrollIntoView + 短暂高亮。
 * 走 block 级元素的整段 textContent — autolink / inline code / strong 等
 * 切分文本节点也能命中。 */
function scrollToText(text: string): void {
  const probe = normalize(text).slice(0, 40)
  if (!probe) return

  const root = document.querySelector('.ProseMirror') ?? document.querySelector('.milkdown')
  if (!root) return

  // 候选 block:任务项、普通 li、段落、标题、引用
  const candidates = root.querySelectorAll('li, p, h1, h2, h3, h4, h5, h6, blockquote')
  for (const el of Array.from(candidates) as HTMLElement[]) {
    if (normalize(el.textContent ?? '').includes(probe)) {
      el.scrollIntoView({ behavior: 'smooth', block: 'start' })
      flashHighlight(el)
      return
    }
  }
}

/** 容错:把 markdown 行内标记剥到跟 DOM textContent 同形态。
 * focus.text 是 raw md (后端 parser 只剥了 - [ ] 前缀);DOM 是渲染后,
 * 标记符已被 ProseMirror 消化。两边都跑这个,probe 才能命中。 */
function normalize(s: string): string {
  return s
    .replace(/<(https?:\/\/[^>\s]+)>/g, '$1')
    .replace(/`+/g, '')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/(?<!\*)\*(?!\*)([^*]+?)\*(?!\*)/g, '$1')
    .replace(/__([^_]+)__/g, '$1')
    .replace(/(?<!_)_(?!_)([^_]+?)_(?!_)/g, '$1')
    .replace(/~~([^~]+)~~/g, '$1')
    .replace(/\\([\\`*_{}\[\]()#+\-.!])/g, '$1')
    .replace(/\s+/g, ' ')
    .replace(/[，]/g, ',')
    .replace(/[。]/g, '.')
    .replace(/[：]/g, ':')
    .replace(/[；]/g, ';')
    .replace(/[（]/g, '(')
    .replace(/[）]/g, ')')
    .replace(/[""]/g, '"')
    .replace(/['']/g, "'")
    .trim()
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
