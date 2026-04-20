import { defineConfig } from 'vite'
import { readFileSync } from 'fs'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'
import tailwindcss from '@tailwindcss/vite'

const __dirname = dirname(fileURLToPath(import.meta.url))

// Emit highlight.js's github + github-dark themes as standalone assets so
// layout.html can swap them at runtime by toggling `disabled` on <link>.
function hljsThemes() {
  return {
    name: 'notesview-hljs-themes',
    generateBundle() {
      const read = (p) => readFileSync(resolve(__dirname, 'node_modules/highlight.js/styles', p), 'utf-8').replace(/\n?$/, '\n')
      this.emitFile({ type: 'asset', fileName: 'hljs-light.css', source: read('github.css') })
      this.emitFile({ type: 'asset', fileName: 'hljs-dark.css',  source: read('github-dark.css') })
    },
  }
}

export default defineConfig({
  root: 'web/src',
  base: '/static/',
  plugins: [tailwindcss(), hljsThemes()],
  build: {
    outDir: '../static',
    emptyOutDir: true,
    minify: true,
    sourcemap: false,
    rollupOptions: {
      input: {
        app: resolve(__dirname, 'web/src/index.html'),
      },
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: '[name].js',
        assetFileNames: (assetInfo) => {
          if (assetInfo.name && assetInfo.name.endsWith('.css')) {
            return 'style[extname]'
          }
          return '[name][extname]'
        },
      },
    },
  },
})
