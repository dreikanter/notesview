/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './web/src/**/*.{html,js}',
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
    },
  },
  plugins: [require('@tailwindcss/typography')],
}
