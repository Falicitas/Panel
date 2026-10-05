import { test } from 'node:test'
import assert from 'node:assert/strict'
import { Schema } from '@milkdown/kit/prose/model'
import { EditorState, TextSelection } from '@milkdown/kit/prose/state'
import { codeLanguage, codeFencePattern, createCodeFenceInputRule, highlightCodeTokens, createCodeSyntaxPlugin } from './dual-notes-code'

const schema = new Schema({ nodes: {
  doc: { content: 'block+' },
  paragraph: { content: 'inline*', group: 'block' },
  text: { group: 'inline' },
  code_block: { content: 'text*', group: 'block', code: true, attrs: { language: { default: '' } } },
} })

test('fence input converts c++, c#, uppercase and punctuated language labels without discarding their spelling', () => {
  for (const fence of ['```c++', '```cpp', '```C++', '```c#', '```Python', '```PYTHON', '```objective-c', '```custom.lang', '```', '~~~rust']) {
    const match = codeFencePattern.exec(`${fence}\n`)
    assert.ok(match, fence)
    const doc = schema.node('doc', null, [schema.node('paragraph', null, schema.text(fence))])
    const state = EditorState.create({ schema, doc, selection: TextSelection.create(doc, fence.length + 1) })
    const transaction = createCodeFenceInputRule(schema.nodes.code_block).handler(state, match, 1, fence.length + 1)
    assert.ok(transaction, fence)
    const block = state.apply(transaction).doc.firstChild
    assert.equal(block.type.name, 'code_block')
    assert.equal(block.attrs.language, match.groups.language)
    assert.equal(block.textContent, '')
  }
  for (const nonFence of ['prose ```c++\n', '```c++ code\n', '``c++\n', '```c++']) assert.equal(codeFencePattern.test(nonFence), false, nonFence)
})

test('language names, common aliases, plain text and unknown labels have predictable display names', () => {
  for (const name of ['cpp', 'c++', 'C++', 'cxx']) assert.equal(codeLanguage(name).label, 'C++')
  for (const name of ['Python', 'python', 'py', 'python3']) assert.equal(codeLanguage(name).label, 'Python')
  for (const name of ['c#', 'cs', 'csharp']) assert.equal(codeLanguage(name).label, 'C#')
  for (const name of ['', 'text', 'plaintext']) assert.equal(codeLanguage(name).label, '纯文本')
  assert.equal(codeLanguage('Unknown-Language').label, 'Unknown-Language')
  assert.equal(codeLanguage('Unknown-Language').description, null)
})

test('C++ and Python syntax are tokenized with offsets into the original code, including language aliases', async () => {
  const cpp = '// 中文注释\nint main() { return 42; }'
  const tokens = await highlightCodeTokens(cpp, 'C++')
  assert.ok(tokens.some(token => token.classes.includes('tok-keyword') && cpp.slice(token.from, token.to) === 'return'))
  assert.ok(tokens.some(token => token.classes.includes('tok-number') && cpp.slice(token.from, token.to) === '42'))
  assert.ok(tokens.some(token => token.classes.includes('tok-comment')))
  assert.strictEqual(await highlightCodeTokens(cpp, 'cpp'), tokens, 'aliases share the parser/token cache')
  const python = 'def hello(name):\n    return "你好😀"'
  const pyTokens = await highlightCodeTokens(python, 'py')
  assert.ok(pyTokens.some(token => token.classes.includes('tok-keyword') && python.slice(token.from, token.to) === 'def'))
  assert.ok(pyTokens.some(token => token.classes.includes('tok-string') && python.slice(token.from, token.to).includes('你好😀')))
  for (const list of [tokens, pyTokens]) for (let i = 1; i < list.length; i++) assert.ok(list[i].from >= list[i - 1].to)
})

test('unknown languages and very large code remain usable as plain text', async () => {
  assert.deepEqual(await highlightCodeTokens('<unsafe & text>', 'made-up-language'), [])
  assert.deepEqual(await highlightCodeTokens('int x = 1;', ''), [])
  assert.deepEqual(await highlightCodeTokens('x'.repeat(120_001), 'cpp'), [])
})

test('syntax decorations preserve code, language attributes and search positions; search-only metadata reuses tokens', async () => {
  const code = 'int main() { return 42; }'
  await highlightCodeTokens(code, 'cpp')
  const plugin = createCodeSyntaxPlugin()
  const doc = schema.node('doc', null, [schema.node('code_block', { language: 'cpp' }, schema.text(code))])
  let state = EditorState.create({ schema, doc, plugins: [plugin] })
  const decorations = plugin.getState(state)
  const all = decorations.find()
  assert.ok(all.some(item => item.type.attrs['data-code-language'] === 'C++'))
  const keyword = all.find(item => item.type.attrs.class?.includes('tok-keyword'))
  assert.ok(keyword)
  assert.equal(doc.textBetween(keyword.from, keyword.to), 'return')
  state = state.apply(state.tr.setMeta('dual-notes-find', true))
  assert.strictEqual(plugin.getState(state), decorations)
  assert.deepEqual(state.doc.toJSON(), doc.toJSON())
  state = state.apply(state.tr.insertText('0', code.length))
  assert.notStrictEqual(plugin.getState(state), decorations)
  assert.equal(state.doc.firstChild.attrs.language, 'cpp')
})
