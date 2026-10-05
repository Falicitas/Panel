import * as esbuild from 'esbuild'
import { copyFile, readFile, mkdir } from 'node:fs/promises'
import path from 'node:path'

const targets = [
  { entry: 'src/main.ts', html: 'src/index.html', directory: 'editor' },
  { entry: 'src/dual-notes.ts', html: 'src/dual-notes.html', directory: 'dual-notes' },
]

const watch = process.argv.includes('--watch')

// 把 dark 主题 css 包在 @media (prefers-color-scheme: dark) 里,
// 让浏览器根据系统 dark/light 自动切换,无需 JS 监听。
const wrapDarkInMediaQuery = {
  name: 'wrap-dark-theme',
  setup(build) {
    const darkThemes = /\/(crepe-dark|frame-dark|nord-dark)\/style\.css$/
    build.onLoad({ filter: darkThemes }, async (args) => {
      const css = await readFile(args.path, 'utf8')
      return {
        contents: `@media (prefers-color-scheme: dark) {\n${css}\n}`,
        loader: 'css',
      }
    })
  },
}

for (const target of targets) {
  const outDir = path.resolve('../Panel/Resources', target.directory)
  await mkdir(outDir, { recursive: true })
  const ctx = await esbuild.context({
    entryPoints: [target.entry],
    bundle: true,
    format: 'iife',
    target: 'es2020',
    outfile: path.join(outDir, 'bundle.js'),
    plugins: [wrapDarkInMediaQuery],
    loader: {
      '.css': 'css',
      '.svg': 'dataurl',
      '.woff': 'dataurl',
      '.woff2': 'dataurl',
      '.ttf': 'dataurl',
      '.otf': 'dataurl',
      '.eot': 'dataurl',
    },
    sourcemap: false,
    minify: !watch,
    logLevel: 'info',
  })

  await copyFile(target.html, path.join(outDir, 'index.html'))

  if (watch) {
    await ctx.watch()
    console.log('watching…')
  } else {
    await ctx.rebuild()
    await ctx.dispose()
    console.log('build complete →', outDir)
  }
}
