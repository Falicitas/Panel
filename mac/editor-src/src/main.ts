import { Editor, rootCtx, defaultValueCtx } from '@milkdown/kit/core'
import { commonmark } from '@milkdown/kit/preset/commonmark'
import { gfm } from '@milkdown/kit/preset/gfm'
import { history } from '@milkdown/kit/plugin/history'
import { clipboard } from '@milkdown/kit/plugin/clipboard'
import { listener, listenerCtx } from '@milkdown/kit/plugin/listener'
import { replaceAll, getMarkdown } from '@milkdown/kit/utils'
import { nord } from '@milkdown/theme-nord'
import '@milkdown/theme-nord/style.css'
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

let editor: Editor | null = null
let suppressNext = false

function postToSwift(payload: unknown): void {
  window.webkit?.messageHandlers?.markdown?.postMessage(payload)
}

async function init(initialMarkdown: string): Promise<void> {
  const root = document.getElementById('editor')
  if (!root) throw new Error('#editor missing')
  editor = await Editor
    .make()
    .config((ctx) => {
      ctx.set(rootCtx, root)
      ctx.set(defaultValueCtx, initialMarkdown)
      ctx.get(listenerCtx).markdownUpdated((_ctx, md, prev) => {
        if (suppressNext) {
          suppressNext = false
          return
        }
        if (md !== prev) postToSwift({ type: 'change', md })
      })
    })
    .config(nord)
    .use(commonmark)
    .use(gfm)
    .use(history)
    .use(clipboard)
    .use(listener)
    .create()
  postToSwift({ type: 'ready' })
}

async function setRaw(markdown: string): Promise<void> {
  if (!editor) {
    await init(markdown)
    return
  }
  suppressNext = true
  editor.action(replaceAll(markdown, true))
}

function getRaw(): string {
  if (!editor) return ''
  return editor.action(getMarkdown())
}

async function destroy(): Promise<void> {
  if (editor) {
    await editor.destroy()
    editor = null
  }
}

window.panelEditor = { init, setRaw, getRaw, destroy }
postToSwift({ type: 'loaded' })
