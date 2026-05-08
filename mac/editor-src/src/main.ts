import { Crepe } from '@milkdown/crepe'
import { editorViewCtx } from '@milkdown/kit/core'
import { TextSelection } from '@milkdown/kit/prose/state'
import { replaceAll } from '@milkdown/kit/utils'
import '@milkdown/crepe/theme/common/style.css'
import '@milkdown/crepe/theme/classic.css'
import '@milkdown/crepe/theme/classic-dark.css'
import './panel.css'

interface FindResult {
  count: number
  current: number
}

declare global {
  interface Window {
    panelEditor: {
      init(initialMarkdown: string, readonly?: boolean): Promise<void>
      setRaw(markdown: string): Promise<void>
      getRaw(): string
      destroy(): Promise<void>
      scrollToText(text: string): void
      setReadonly(readonly: boolean): void
      findAll(query: string): FindResult
      findNext(): FindResult
      findPrev(): FindResult
      findClear(): FindResult
      commitFindCursor(): void
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

async function init(initialMarkdown: string, readonly = false): Promise<void> {
  const root = document.getElementById('editor')
  if (!root) throw new Error('#editor missing')
  crepe = new Crepe({ root, defaultValue: initialMarkdown })
  await crepe.create()
  if (readonly) {
    crepe.setReadonly(true)
    document.body.classList.add('readonly')
  }
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

function setReadonly(readonly: boolean): void {
  if (!crepe) return
  crepe.setReadonly(readonly)
  document.body.classList.toggle('readonly', readonly)
}

async function setRaw(markdown: string): Promise<void> {
  if (!crepe) {
    await init(markdown)
    return
  }
  // 用 replaceAll macro 替换内容,不销毁 DOM 容器,避免切文件时的 0.2s 空白闪烁
  suppressNext = true
  crepe.editor.action(replaceAll(markdown, true))
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

// ----- cmd+F find ----- (DOM TreeWalker 高亮, 跟 ProseMirror 解耦)
//
// 用 <span class="panel-find-mark"> 包文本, .panel-find-active 是当前选中那条。
// find bar 打开期间 mac DetailView 把 editor 切 readonly, ProseMirror 不 reconcile
// DOM, panel-find-mark span 才不会被异步 unwrap (否则 findNext 时 marks 已空 →
// '无匹配')。
//
// fenced code block (```...```) 跳过: Crepe 用 CodeMirror 6 渲染代码块, CM 内部
// MutationObserver 即使 readonly 下也会异步把外部插入的 mark span 剥掉, findAll
// 同步算的 marks.length 看着非零, 下一帧 findNext getMarks() 时已被吞 → '无匹配'。
// 整段 .cm-editor 子树 acceptNode REJECT, 行为变成 fenced code 不参与 cmd+F (比
// 旧的'虚假匹配数 → enter 后无匹配'稳定)。inline code (单 ` 包) 用 ProseMirror
// mark 渲染, 不在 .cm-editor 里, 仍能搜。

let findActiveIdx = -1

function findClearMarks(): void {
  const root = document.getElementById('editor')
  if (!root) return
  const marks = root.querySelectorAll('.panel-find-mark')
  marks.forEach((m) => {
    const parent = m.parentNode
    if (!parent) return
    while (m.firstChild) parent.insertBefore(m.firstChild, m)
    parent.removeChild(m)
    if (parent instanceof Element && (parent as Element & { normalize?: () => void }).normalize) {
      ;(parent as Element & { normalize: () => void }).normalize()
    }
  })
  findActiveIdx = -1
}

function findHighlightAll(query: string): HTMLElement[] {
  findClearMarks()
  if (!query) return []
  const root = document.getElementById('editor')
  if (!root) return []
  const lq = query.toLowerCase()
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode: (node) => {
      const p = node.parentNode as Element | null
      if (!p) return NodeFilter.FILTER_REJECT
      if (p.classList && p.classList.contains('panel-find-mark')) return NodeFilter.FILTER_REJECT
      const tag = (p.tagName || '').toLowerCase()
      if (tag === 'script' || tag === 'style') return NodeFilter.FILTER_REJECT
      if (typeof p.closest === 'function' && p.closest('.cm-editor')) return NodeFilter.FILTER_REJECT
      return NodeFilter.FILTER_ACCEPT
    }
  })
  const targets: Text[] = []
  let n: Node | null
  while ((n = walker.nextNode())) {
    const tn = n as Text
    if (tn.nodeValue && tn.nodeValue.toLowerCase().includes(lq)) {
      targets.push(tn)
    }
  }
  const marks: HTMLElement[] = []
  for (const node of targets) {
    const text = node.nodeValue ?? ''
    const lower = text.toLowerCase()
    const frag = document.createDocumentFragment()
    let idx = 0
    while (idx < text.length) {
      const found = lower.indexOf(lq, idx)
      if (found < 0) {
        frag.appendChild(document.createTextNode(text.slice(idx)))
        break
      }
      if (found > idx) {
        frag.appendChild(document.createTextNode(text.slice(idx, found)))
      }
      const mark = document.createElement('span')
      mark.className = 'panel-find-mark'
      mark.textContent = text.slice(found, found + lq.length)
      frag.appendChild(mark)
      marks.push(mark)
      idx = found + lq.length
    }
    node.parentNode?.replaceChild(frag, node)
  }
  return marks
}

function findGetMarks(): HTMLElement[] {
  const root = document.getElementById('editor')
  if (!root) return []
  return Array.from(root.querySelectorAll('.panel-find-mark')) as HTMLElement[]
}

function findSetActive(marks: HTMLElement[], idx: number): void {
  marks.forEach((m) => m.classList.remove('panel-find-active'))
  if (idx < 0 || idx >= marks.length) return
  const mark = marks[idx]
  mark.classList.add('panel-find-active')
  mark.scrollIntoView({ behavior: 'smooth', block: 'center' })
  // 不在这里调 DOM selection.collapse — ProseMirror 监听 selectionchange 会"修正"
  // DOM 把 mark span 剥掉, 后续 next/prev 立刻 count=0 '无匹配'。同步 cursor 走
  // commitFindCursor, 仅 findClear 时调一次。
}

function findAll(query: string): FindResult {
  const marks = findHighlightAll(query)
  if (marks.length === 0) {
    findActiveIdx = -1
    return { count: 0, current: 0 }
  }
  // 从当前 ProseMirror cursor 位置往下找第一个 match — 用 mark 屏幕 top 跟 cursor
  // 屏幕 top 比较 (跨多 block 也工作); 都在 cursor 之上时 wrap 到第一个 mark。
  // 行为对齐 vscode/sublime 等 cmd+F: 第二次进入不必从头, 续上之前的位置。
  let startIdx = 0
  if (crepe) {
    crepe.editor.action((ctx) => {
      const view = ctx.get(editorViewCtx)
      const cursorPos = view.state.selection.from
      let cursorTop: number | null = null
      try {
        const c = view.coordsAtPos(cursorPos)
        cursorTop = c.top
      } catch (_) { cursorTop = null }
      if (cursorTop == null) return
      for (let i = 0; i < marks.length; i++) {
        const r = marks[i].getBoundingClientRect()
        // mark.top 严格 > cursor.top 表示 mark 在 cursor 下面行;
        // 同行场景 mark.left > cursor.left 就算下一个。
        if (r.top > cursorTop + 0.5) {
          startIdx = i
          return
        }
        if (Math.abs(r.top - cursorTop) <= 0.5) {
          // 同行: 看 left 区分
          try {
            const cc = view.coordsAtPos(cursorPos)
            if (r.left > cc.left + 0.5) { startIdx = i; return }
          } catch (_) { /* fallback 保持上面逻辑 */ }
        }
      }
      // 全部 mark 都在 cursor 之上 → wrap 到第一个 (默认 startIdx=0)
    })
  }
  findActiveIdx = startIdx
  findSetActive(marks, startIdx)
  return { count: marks.length, current: startIdx + 1 }
}

function findNext(): FindResult {
  const marks = findGetMarks()
  if (marks.length === 0) return { count: 0, current: 0 }
  findActiveIdx = (findActiveIdx + 1) % marks.length
  findSetActive(marks, findActiveIdx)
  return { count: marks.length, current: findActiveIdx + 1 }
}

function findPrev(): FindResult {
  const marks = findGetMarks()
  if (marks.length === 0) return { count: 0, current: 0 }
  findActiveIdx = (findActiveIdx - 1 + marks.length) % marks.length
  findSetActive(marks, findActiveIdx)
  return { count: marks.length, current: findActiveIdx + 1 }
}

function findClear(): FindResult {
  // 关闭 find bar 时, 先把 ProseMirror cursor 同步到 active mark 位置
  // (走 EditorView dispatch 不动 DOM), 然后再 unwrap 所有 mark span。
  // 这样 ProseMirror 内部 selection 已是 cmd+F 跳到的位置, view sync
  // 不会滚回 cmd+F 之前的 focus 行。
  try { commitFindCursor() } catch (_) { /* 静默, 不影响 unwrap */ }
  findClearMarks()
  return { count: 0, current: 0 }
}

/** 把 ProseMirror 的 cursor 同步到 .panel-find-active mark 所在位置。
 * 关闭 find bar 后 ProseMirror view sync 到这个 cursor, 不会滚回 cmd+F 之前的
 * focus 行。
 *
 * 用 posAtCoords (屏幕坐标) 而不是 posAtDOM (DOM node) 找 doc position —
 * .panel-find-mark span 不在 ProseMirror schema 里, posAtDOM 在 readonly 模式
 * 下/对 unknown node 可能返回不准。posAtCoords 用 mark 中点屏幕坐标找最近的
 * ProseMirror text 位置, 更鲁棒。 */
function commitFindCursor(): void {
  if (!crepe) return
  const root = document.querySelector('.ProseMirror') as HTMLElement | null
  if (!root) return
  const active = root.querySelector('.panel-find-active') as HTMLElement | null
  if (!active) return
  const rect = active.getBoundingClientRect()
  if (rect.width === 0 && rect.height === 0) return
  // 取 mark 中点屏幕坐标 — 落到 mark 内部 text 节点上, posAtCoords 拿到对应 doc pos
  const coords = { left: rect.left + rect.width / 2, top: rect.top + rect.height / 2 }
  crepe.editor.action((ctx) => {
    const view = ctx.get(editorViewCtx)
    let pos: number | null = null
    try {
      const result = view.posAtCoords(coords)
      pos = result ? result.pos : null
    } catch (_) {
      pos = null
    }
    if (pos == null) return
    // 不调 tr.scrollIntoView() — setActive 已用 DOM scrollIntoView 滚到 mark 位置,
    // 这里只更新 ProseMirror state cursor, 不让 view 自己 sync 到 cursor 二次滚动
    // (有可能滚回老位置或抖动)。
    const tr = view.state.tr.setSelection(
      TextSelection.create(view.state.doc, pos)
    )
    view.dispatch(tr)
  })
}

window.panelEditor = {
  init, setRaw, getRaw, destroy, scrollToText, setReadonly,
  findAll, findNext, findPrev, findClear, commitFindCursor,
}
postToSwift({ type: 'loaded' })

// ----- 容器 resize 时 clamp scrollTop -----
//
// sidebar 隐藏 / 主窗口拖宽 / 全屏切换等让 WKWebView 变宽时, CSS reflow 让文
// 档变矮但 scroller 的 scrollTop 停在旧位置 → 落到没真实内容的空区出现一片
// 空白, 直到用户碰滚轮浏览器才修正。这里监听 resize 主动 clamp。不动 cursor
// / selection。
//
// 真 scroller 是 documentElement (body / #editor 都 height:100% 锁死 viewport,
// .milkdown 溢出顶大 HTML 让它接收滚动)。
//
// max 不能直接用 documentElement.scrollHeight — ProseMirror 末尾 phantom 元素
// (overflow:visible 子元素) 会撑大 scrollHeight 远超 .milkdown 实际渲染底
// (实测 sh 29865 vs .milkdown ch 25966 差 3899px), clamp 到 scrollHeight-
// clientHeight 反而把视口拉进空白。改用 .milkdown 实际底页面坐标减 innerHeight
// 算 max。
//
// 兜底: body / #editor / 子树里 scrollTop>0 的元素仍走 scrollHeight clamp,
// 实测都是 no-op, 留作 Crepe 升级换内部 scroller 时的兜底。
//
// 时序: rAF 一帧 + 80ms 延后双 clamp, 兜底 ProseMirror / Crepe 异步 layout。
let resizeClampInstalled = false

function clampEditorScrollers(): void {
  const clampTo = (el: HTMLElement, max: number): void => {
    if (max > 0 && el.scrollTop > max) {
      el.scrollTop = max
    } else if (max <= 0 && el.scrollTop > 0) {
      el.scrollTop = 0
    }
  }
  const clampToScrollHeight = (el: HTMLElement): void => {
    clampTo(el, el.scrollHeight - el.clientHeight)
  }

  // documentElement / body 的 scrollHeight 比真渲染高度大 — ProseMirror 末尾
  // 留 phantom space (实测 .milkdown clientHeight 25966 vs HTML scrollHeight
  // 29865, 差了 3899px), clamp 到 scrollHeight-clientHeight 反而把视口拉进
  // 这段空白。改用 .milkdown 的真实底 (页面绝对坐标) 减视口高算 max。
  const docScroller = (document.scrollingElement ?? document.documentElement) as HTMLElement
  const milk = document.querySelector<HTMLElement>('.milkdown')
  if (docScroller) {
    if (milk) {
      const milkDocBottom = milk.getBoundingClientRect().bottom + docScroller.scrollTop
      clampTo(docScroller, Math.max(0, Math.ceil(milkDocBottom - window.innerHeight)))
    } else {
      clampToScrollHeight(docScroller)
    }
  }
  if (document.body) clampToScrollHeight(document.body)
  const root = document.getElementById('editor')
  if (!root) return
  clampToScrollHeight(root)
  root.querySelectorAll<HTMLElement>('*').forEach((el) => {
    if (el.scrollTop > 0) clampToScrollHeight(el)
  })
}

function setupResizeClamp(): void {
  if (resizeClampInstalled) return
  resizeClampInstalled = true
  let raf: number | null = null
  const onResize = (): void => {
    if (raf != null) return
    raf = requestAnimationFrame(() => {
      raf = null
      clampEditorScrollers()
      window.setTimeout(clampEditorScrollers, 80)
    })
  }
  window.addEventListener('resize', onResize)
  new ResizeObserver(onResize).observe(document.body)
}

setupResizeClamp()
