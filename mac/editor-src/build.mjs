import * as esbuild from 'esbuild'
import { copyFile } from 'node:fs/promises'
import path from 'node:path'

const OUT_DIR = path.resolve('../Panel/Resources/editor')

const watch = process.argv.includes('--watch')

const ctx = await esbuild.context({
  entryPoints: ['src/main.ts'],
  bundle: true,
  format: 'iife',
  target: 'es2020',
  outfile: path.join(OUT_DIR, 'bundle.js'),
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
