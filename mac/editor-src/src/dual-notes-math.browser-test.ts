import { Crepe } from '@milkdown/crepe'
import 'katex/dist/katex.min.css'
import './dual-notes.css'
import { editorViewCtx, parserCtx } from '@milkdown/kit/core'
import { DOMSerializer } from '@milkdown/kit/prose/model'
import { TextSelection } from '@milkdown/kit/prose/state'
import { mathPlugins, configureMath } from './dual-notes-math'
import { codeBlockPlugins } from './dual-notes-code'

function assert(value: unknown, message: string): asserts value { if (!value) throw new Error(message) }
async function run() {
  const root = document.createElement('div')
  document.body.append(root)
  const fixture = '行内 $E=mc^2$。\n\n$$\n\\frac{a}{b} + \\sqrt{x}\n$$\n\n价格 \\$20，代码 `$x$`。\n\n```python\nprint("$x$")\n```\n'
  const editor = new Crepe({ root, defaultValue: fixture, features: {
    [Crepe.Feature.CodeMirror]: false, [Crepe.Feature.Latex]: false,
  } })
  editor.editor.config(configureMath).use(codeBlockPlugins()).use(mathPlugins())
  await editor.create()
  const view = editor.editor.action(ctx => ctx.get(editorViewCtx))
  const parse = editor.editor.action(ctx => ctx.get(parserCtx))
  const types: string[] = []
  view.state.doc.descendants(node => { types.push(node.type.name) })
  assert(types.filter(type => type === 'math_inline').length === 1, 'inline parsing / escaped dollars')
  assert(types.filter(type => type === 'math_block').length === 1, 'block parsing')
  assert(types.includes('code_block'), 'existing code blocks preserved')
  assert(root.querySelectorAll('.panel-math .katex').length === 2, 'both formulas render')
  assert(getComputedStyle(root.querySelector('.math-source')!).display === 'none', 'formula source hidden while not editing')
  assert(root.querySelector('.katex .mfrac'), 'fraction rendered')
  const markdown = editor.getMarkdown()
  assert(markdown.includes('$$') && markdown.includes('$E=mc^2$'), 'formula delimiters saved')
  assert(markdown.includes('```python') && markdown.includes('`$x$`'), 'code retained')
  assert(parse(markdown).eq(parse(fixture)), 'save and reload preserve document: ' + JSON.stringify({ markdown, before: view.state.doc.toJSON(), after: parse(markdown).toJSON() }))
  const preview = document.createElement('div')
  preview.append(DOMSerializer.fromSchema(view.state.schema).serializeFragment(view.state.doc.content))
  assert(preview.querySelectorAll('.katex').length === 2, 'lazy previews render formulas')
  let inlinePos = -1
  view.state.doc.descendants((node, pos) => { if (node.type.name === 'math_inline') inlinePos = pos })
  const inlineNode = view.state.doc.nodeAt(inlinePos)!
  view.dispatch(view.state.tr.insertText('x^2', inlinePos + 1, inlinePos + inlineNode.nodeSize - 1))
  assert(root.querySelector('[data-panel-math="inline"] .math-source')?.textContent === 'x^2', 'source updates')
  assert(root.querySelector('[data-panel-math="inline"] annotation')?.textContent === 'x^2', 'preview updates after editing')
  view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, 1)))
  root.querySelector<HTMLElement>('[data-panel-math="inline"] .math-preview')!
    .dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true }))
  assert(view.state.selection.from === inlinePos + 1, 'click formula to edit source')
  assert(root.querySelector('[data-panel-math="inline"]')?.classList.contains('math-editing'), 'selected formula source shown')
  assert(getComputedStyle(root.querySelector('[data-panel-math="inline"] .math-source')!).display !== 'none', 'source visible while editing')
  assert(getComputedStyle(root.querySelector('[data-panel-math="inline"] .math-preview')!).display === 'none', 'preview hidden while editing')
  const end = view.state.doc.content.size - 1
  view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, end)))
  const handled = view.someProp('handleTextInput', handler => handler(view, end, end, '$z$'))
  assert(handled && editor.getMarkdown().includes('$z$'), 'typing inline formula creates math')
  view.dispatch(view.state.tr.insert(view.state.doc.content.size, view.state.schema.nodes.paragraph.create()))
  const blockEnd = view.state.doc.content.size - 1
  view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, blockEnd)))
  assert(view.someProp('handleTextInput', handler => handler(view, blockEnd, blockEnd, '$$ ')), 'typing block delimiter creates math')
  assert(view.state.selection.$from.parent.type.name === 'math_block', 'block source ready for editing')
  await editor.destroy()
}
run().then(() => (window as any).webkit.messageHandlers.result.postMessage('PASS: math parse/render/edit/save/reload, lazy previews, escaped dollars and code blocks'))
  .catch(error => (window as any).webkit.messageHandlers.result.postMessage('FAIL: ' + (String(error) + '\n' + (error.stack || ''))))
