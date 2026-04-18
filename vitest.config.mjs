import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    environment: 'happy-dom',
    include: ['web/src/**/*.test.js'],
    globals: false,
  },
})
