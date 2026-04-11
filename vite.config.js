import { defineConfig } from 'vite'
import { resolve } from 'path'

export default defineConfig({
  root: 'web/src',
  base: '/static/',
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
