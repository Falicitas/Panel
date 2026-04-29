import * as esbuild from 'esbuild'
import { copyFile, readFile } from 'node:fs/promises'
import path from 'node:path'

const OUT_DIR = path.resolve('../Panel/Resources/editor')

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

const ctx = await esbuild.context({
  entryPoints: ['src/main.ts'],
  bundle: true,
  format: 'iife',
  target: 'es2020',
  outfile: path.join(OUT_DIR, 'bundle.js'),
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

await copyFile('src/index.html', path.join(OUT_DIR, 'index.html'))

if (watch) {
  await ctx.watch()
  console.log('watching…')
} else {
  await ctx.rebuild()
  await ctx.dispose()
  console.log('build complete →', OUT_DIR)
}
