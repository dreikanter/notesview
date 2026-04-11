/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './web/src/**/*.{html,js}',
    './web/templates/**/*.html',
  ],
  safelist: [
    // Emitted by Go server renderer — not in scanned content files
    'broken-link',
    'uid-link',
    'task-checked',
    'task-unchecked',
    'task-tag',
  ],
  theme: {
    extend: {
      screens: {
        sidebar: '900px',
      },
      // @tailwindcss/typography wraps inline <code> in literal backticks via
      // `code::before`/`code::after` content. Our markdown pipeline already
      // renders fenced code without the fences, so surface backticks are
      // noise. Override the typography theme to drop the pseudo-element
      // content — this is the documented extension point.
      // https://tailwindcss.com/docs/typography-plugin#customizing-the-css
      typography: {
        DEFAULT: {
          css: {
            'code::before': { content: 'none' },
            'code::after': { content: 'none' },
          },
        },
      },
    },
  },
  plugins: [require('@tailwindcss/typography')],
}
