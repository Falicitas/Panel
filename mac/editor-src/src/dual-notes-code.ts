import { languages } from '@codemirror/language-data'
import { classHighlighter, highlightTree } from '@lezer/highlight'
import { schemaCtx } from '@milkdown/kit/core'
import { textblockTypeInputRule } from '@milkdown/kit/prose/inputrules'
import type { Node as ProseNode, NodeType } from '@milkdown/kit/prose/model'
import { Plugin, PluginKey } from '@milkdown/kit/prose/state'
import { Decoration, DecorationSet } from '@milkdown/kit/prose/view'
import { $inputRule, $prose } from '@milkdown/kit/utils'

type LanguageDescription = (typeof languages)[number]
type LanguageParser = Awaited<ReturnType<LanguageDescription['load']>>['language']['parser']
export interface CodeToken { from: number; to: number; classes: string }

const plainNames = new Set(['', 'text', 'txt', 'plain', 'plaintext', 'none'])
const aliases: Record<string, string> = {
  py: 'python', py3: 'python', python3: 'python', python2: 'python',
  cc: 'c++', cxx: 'c++', hpp: 'c++', golang: 'go',
  js: 'javascript', ts: 'typescript', yml: 'yaml', rs: 'rust',
  md: 'markdown', rb: 'ruby', objc: 'objective-c', 'objectivec': 'objective-c',
}

/** Preserve the saved language spelling; canonical names are only used for display/parser lookup. */
export function codeLanguage(rawLanguage: string): { label: string; description: LanguageDescription | null } {
  const raw = rawLanguage.trim()
  const lower = raw.toLowerCase()
  if (plainNames.has(lower)) return { label: '纯文本', description: null }
  const name = aliases[lower] ?? lower
  const description = languages.find(language => language.alias.includes(name))
    ?? languages.find(language => language.extensions.some(extension => extension.toLowerCase() === name))
    ?? null
  return { label: description?.name ?? raw, description }
}

const parsers = new Map<string, LanguageParser>()
const pendingParsers = new Map<string, Promise<LanguageParser | null>>()
const parserListeners = new Set<(language: string) => void>()

function loadParser(description: LanguageDescription): Promise<LanguageParser | null> {
  const loaded = parsers.get(description.name)
  if (loaded) return Promise.resolve(loaded)
  const pending = pendingParsers.get(description.name)
  if (pending) return pending
  const loading = description.load().then(support => {
    const parser = support.language.parser
    parsers.set(description.name, parser)
    for (const listener of parserListeners) listener(description.name)
    return parser
  }).catch(() => null)
  pendingParsers.set(description.name, loading)
  return loading
}

// Keep all editing/search paths usable for unusually large code blocks and bound memory across pages.
const maximumHighlightLength = 120_000
const tokenCache = new Map<string, readonly CodeToken[]>()
function parsedTokens(text: string, description: LanguageDescription, parser: LanguageParser): readonly CodeToken[] {
  if (!text || text.length > maximumHighlightLength) return []
  const cacheKey = `${description.name}\u0000${text}`
  const cached = tokenCache.get(cacheKey)
  if (cached) return cached
  const tokens: CodeToken[] = []
  try {
    const tree = parser.parse(text)
    highlightTree(tree, classHighlighter, (from, to, classes) => {
      if (from >= 0 && to <= text.length && to > from) tokens.push({ from, to, classes })
    })
  } catch { return [] }
  if (tokenCache.size >= 128) tokenCache.delete(tokenCache.keys().next().value!)
  tokenCache.set(cacheKey, tokens)
  return tokens
}

/** Shared by previews and native ProseMirror decorations; this never creates a CodeMirror editor. */
export async function highlightCodeTokens(text: string, rawLanguage: string): Promise<readonly CodeToken[]> {
  const { description } = codeLanguage(rawLanguage)
  if (!description || !text || text.length > maximumHighlightLength) return []
  const parser = await loadParser(description)
  return parser ? parsedTokens(text, description, parser) : []
}

function readyTokens(text: string, rawLanguage: string): readonly CodeToken[] {
  const { description } = codeLanguage(rawLanguage)
  if (!description || !text || text.length > maximumHighlightLength) return []
  const parser = parsers.get(description.name)
  if (parser) return parsedTokens(text, description, parser)
  void loadParser(description)
  return []
}

const syntaxKey = new PluginKey<DecorationSet>('panel-dual-code-syntax')
function decorationsFor(doc: ProseNode): DecorationSet {
  const decorations: Decoration[] = []
  doc.descendants((block, position) => {
    if (block.type.name !== 'code_block') return true
    const rawLanguage = String(block.attrs.language ?? '')
    decorations.push(Decoration.node(position, position + block.nodeSize, {
      'data-code-language': codeLanguage(rawLanguage).label,
    }))
    for (const token of readyTokens(block.textContent, rawLanguage)) {
      decorations.push(Decoration.inline(position + 1 + token.from, position + 1 + token.to, { class: token.classes }))
    }
    return false
  })
  return DecorationSet.create(doc, decorations)
}

/** Kept separate for testing: search metadata and selection changes reuse the existing decoration set. */
export function createCodeSyntaxPlugin(): Plugin<DecorationSet> {
  return new Plugin<DecorationSet>({
    key: syntaxKey,
    state: {
      init: (_, state) => decorationsFor(state.doc),
      apply: (transaction, decorations) => transaction.docChanged || transaction.getMeta(syntaxKey)
        ? decorationsFor(transaction.doc)
        : decorations,
    },
    props: { decorations: state => syntaxKey.getState(state) },
    view(view) {
      let alive = true
      let queued = false
      const refresh = () => {
        if (!alive || queued) return
        queued = true
        queueMicrotask(() => {
          queued = false
          if (alive && !view.isDestroyed) view.dispatch(view.state.tr.setMeta(syntaxKey, true))
        })
      }
      const loaded = (name: string) => {
        let relevant = false
        view.state.doc.descendants(block => {
          if (block.type.name !== 'code_block') return true
          if (codeLanguage(String(block.attrs.language ?? '')).description?.name === name) relevant = true
          return false
        })
        if (relevant) refresh()
      }
      parserListeners.add(loaded)
      // A parser may finish loading between EditorState construction and mounting this view.
      refresh()
      return { destroy() { alive = false; parserListeners.delete(loaded) } }
    },
  })
}

/** The bundled commonmark rule only accepts [a-z]*, excluding c++, c#, and uppercase labels. */
export const codeFencePattern = /^ {0,3}(?:`{3,}|~{3,})(?<language>[A-Za-z0-9_+#.-]*)[\t \n]$/
export function createCodeFenceInputRule(type: NodeType) {
  return textblockTypeInputRule(codeFencePattern, type, match => ({ language: match.groups?.language ?? '' }))
}

/** Use a fresh factory result for every cell so Milkdown's plugin cleanup closures stay independent. */
export function codeBlockPlugins() {
  return [
    $inputRule(ctx => createCodeFenceInputRule(ctx.get(schemaCtx).nodes.code_block)),
    $prose(() => createCodeSyntaxPlugin()),
  ]
}

/** Only call on preview DOM, never on a mounted ProseMirror editor. */
export async function decorateCodePreview(root: HTMLElement): Promise<void> {
  await Promise.all(Array.from(root.querySelectorAll<HTMLPreElement>('pre')).map(async pre => {
    const code = pre.querySelector<HTMLElement>(':scope > code')
    if (!code) return
    const rawLanguage = pre.dataset.language ?? ''
    const text = code.textContent ?? ''
    pre.dataset.codeLanguage = codeLanguage(rawLanguage).label
    const tokens = await highlightCodeTokens(text, rawLanguage)
    // A lazy editor may have replaced this preview, or another render may have changed its content.
    if (!root.contains(pre) || code.parentElement !== pre || code.textContent !== text || (pre.dataset.language ?? '') !== rawLanguage) return
    if (!tokens.length) return
    const fragment = document.createDocumentFragment()
    let cursor = 0
    for (const token of tokens) {
      if (token.from > cursor) fragment.append(document.createTextNode(text.slice(cursor, token.from)))
      const span = document.createElement('span')
      span.className = token.classes
      span.textContent = text.slice(token.from, token.to)
      fragment.append(span)
      cursor = token.to
    }
    if (cursor < text.length) fragment.append(document.createTextNode(text.slice(cursor)))
    code.replaceChildren(fragment)
  }))
}
