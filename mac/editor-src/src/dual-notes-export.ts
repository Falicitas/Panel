import type { NoteDay } from './dual-notes-core'

/** A longer outer fence keeps headings and fenced code inside the original cell boundary. */
function markdownCell(content: string): string {
  if (!content.trim()) return '（空白）'
  let longestRun = 0
  for (const match of content.matchAll(/`+/g)) longestRun = Math.max(longestRun, match[0].length)
  const fence = '`'.repeat(Math.max(3, longestRun + 1))
  // The final separator belongs to the enclosure; never trim or normalize the original content.
  const separator = content.endsWith('\n') ? '' : '\n'
  return `${fence}markdown\n${content}${separator}${fence}`
}

/** A single day's original row pairing, in a format that can be pasted directly into an AI chat. */
export function formatDayForClipboard(day: NoteDay): string {
  const sections = [
    `# 双栏记录：${day.date}`,
    '按原表从上到下阅读，每行先左栏、后右栏；两栏内容一一对应。围栏内保留原始 Markdown。',
  ]
  let includedRows = 0
  day.rows.forEach((row, index) => {
    if (!row.left.trim() && !row.right.trim()) return
    includedRows++
    sections.push(
      `## 第 ${index + 1} 行`,
      '### 左栏：不用电脑',
      markdownCell(row.left),
      '### 右栏：用电脑 or AI',
      markdownCell(row.right),
    )
  })
  if (!includedRows) sections.push('暂无内容。')
  return sections.join('\n\n') + '\n'
}
