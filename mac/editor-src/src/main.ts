import { Crepe } from '@milkdown/crepe'
import '@milkdown/crepe/theme/common/style.css'
import '@milkdown/crepe/theme/classic.css'
import './panel.css'

declare global {
  interface Window {
    panelEditor: {
      init(initialMarkdown: string): Promise<void>
      setRaw(markdown: string): Promise<void>
      getRaw(): string
      destroy(): Promise<void>
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

window.panelEditor = { init, setRaw, getRaw, destroy }
postToSwift({ type: 'loaded' })
