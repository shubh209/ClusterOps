/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // Dark terminal-inspired palette
        surface: {
          DEFAULT: '#0f1117',
          1: '#161b22',
          2: '#1c2128',
          3: '#22272e',
          4: '#2d333b',
        },
        border: '#373e47',
        accent: {
          DEFAULT: '#58a6ff',
          muted: '#1f4e8c',
        },
        success: '#3fb950',
        warning: '#d29922',
        danger: '#f85149',
        muted: '#768390',
      },
      fontFamily: {
        mono: ['JetBrains Mono', 'Fira Code', 'Consolas', 'monospace'],
      },
      animation: {
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        'fade-in': 'fadeIn 0.2s ease-out',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0', transform: 'translateY(4px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
      },
    },
  },
  plugins: [],
}
