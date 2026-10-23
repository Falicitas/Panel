import katex from 'katex'
import { remarkStringifyOptionsCtx } from '@milkdown/kit/core'
import type { Ctx } from '@milkdown/kit/ctx'
import remarkMath from 'remark-math'
import { $nodeSchema, $remark, $inputRule, $prose } from '@milkdown/kit/utils'
import { InputRule, textblockTypeInputRule } from '@milkdown/kit/prose/inputrules'
import { Plugin, TextSelection } from '@milkdown/kit/prose/state'
import { Decoration, DecorationSet } from '@milkdown/kit/prose/view'
import type { DOMOutputSpec, Node as ProseNode } from '@milkdown/kit/prose/model'

// Keep formulas as editable ProseMirror text, independent of the code-block editor.
export function formulaDOM(value: string, block: boolean): HTMLElement {
  const preview = document.createElement('span')
  preview.className = 'math-preview'
  preview.contentEditable = 'false'
  preview.innerHTML = katex.renderToString(value, { displayMode: block, throwOnError: false, trust: false })
  return preview
}

export function mathPlugins() {
  const math = $remark('panelMath', () => remarkMath)
  function schema(block: boolean) {
    const name = block ? 'math_block' : 'math_inline'
    return $nodeSchema(name, () => ({
      group: block ? 'block' : 'inline',
      inline: !block,
      content: 'text*',
      marks: '',
      code: true,
      defining: true,
      parseDOM: [{ tag: `[data-panel-math="${block ? 'block' : 'inline'}"]`, contentElement: '.math-source', preserveWhitespace: 'full' as const }],
      toDOM: (node: ProseNode) => [block ? 'div' : 'span', {
        'data-panel-math': block ? 'block' : 'inline', class: 'panel-math',
      }, ['span', { class: 'math-source', 'data-delimiter': block ? '$$' : '$' }, 0], formulaDOM(node.textContent, block)] as DOMOutputSpec,
      parseMarkdown: {
        match: (node: { type: string }) => node.type === (block ? 'math' : 'inlineMath'),
        runner: (state, node, type) => {
          state.openNode(type)
          if (node.value) state.addText(node.value)
          state.closeNode()
        },
      },
      toMarkdown: {
        match: (node: ProseNode) => node.type.name === name,
        runner: (state, node) => state.addNode(block ? 'math' : 'inlineMath', undefined, node.textContent),
      },
    }))
  }
  const inline = schema(false)
  const block = schema(true)
  function nodeView(node: ProseNode) {
    const block = node.type.name === 'math_block'
    const dom = document.createElement(block ? 'div' : 'span')
    dom.className = 'panel-math'
    dom.dataset.panelMath = block ? 'block' : 'inline'
    const contentDOM = document.createElement('span')
    contentDOM.className = 'math-source'
    contentDOM.dataset.delimiter = block ? '$$' : '$'
    let preview = formulaDOM(node.textContent, block)
    dom.append(contentDOM, preview)
    return {
      dom, contentDOM,
      update(next: ProseNode) {
        if (next.type !== node.type) return false
        const replacement = formulaDOM(next.textContent, block)
        preview.replaceWith(replacement)
        preview = replacement
        return true
      },
      ignoreMutation(mutation: MutationRecord | { type: 'selection'; target: Node }) {
        return mutation.type !== 'selection' && (mutation.target === dom || preview.contains(mutation.target))
      },
    }
  }
  return [math, inline, block,
    $inputRule(ctx => new InputRule(/(?<![\\$])\$([^$\n]+)\$$/, (state, match, start, end) =>
      state.tr.replaceWith(start, end, inline.type(ctx).create(null, state.schema.text(match[1]))))),
    $inputRule(ctx => textblockTypeInputRule(/^\$\$\s$/, block.type(ctx))),
    $prose(() => new Plugin({
      props: {
        nodeViews: { math_inline: nodeView, math_block: nodeView },
        decorations(state) {
          const decorations: Decoration[] = []
          state.doc.descendants((node, position) => {
            if (node.type.name !== 'math_inline' && node.type.name !== 'math_block') return true
            if (state.selection.from > position && state.selection.to < position + node.nodeSize) {
              decorations.push(Decoration.node(position, position + node.nodeSize, { class: 'math-editing' }))
            }
            return false
          })
          return DecorationSet.create(state.doc, decorations)
        },
        handleDOMEvents: {
          mousedown(view, event) {
            const target = event.target as HTMLElement
            const formula = target.closest<HTMLElement>('[data-panel-math]')
            if (!formula || !target.closest('.math-preview')) return false
            const source = formula.querySelector('.math-source')!
            const position = view.posAtDOM(source, 0)
            view.dispatch(view.state.tr.setSelection(TextSelection.near(view.state.doc.resolve(position))))
            view.focus()
            event.preventDefault()
            return true
          },
        },
      },
    })),
  ]
}

// Milkdown preserves trailing spaces via a text handler that otherwise skips
// escaping. Literal dollars must still be escaped when math parsing is enabled.
export function configureMath(ctx: Ctx) {
  ctx.update(remarkStringifyOptionsCtx, previous => ({
    ...previous,
    unsafe: [...(previous.unsafe ?? []), { character: '$' }],
    handlers: {
      ...previous.handlers,
      text(node, parent, state, info) {
        if (node.value.includes('$')) return state.safe(node.value, { ...info, encode: [] })
        return previous.handlers!.text!(node, parent, state, info)
      },
    },
  }))
}
