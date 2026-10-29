/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,jsx}'],
  theme: {
    extend: {
      colors: {
        brand: { DEFAULT: '#1E6FD9', dark: '#0B4FA8', pale: '#E8F1FD' },
        wall: '#2E7DC4',
        floor: '#EAF4FC',
        desk: {
          available: '#66CC1F',
          booked: '#FDB813',
          selected: '#1E6FD9',
          unavailable: '#C5CCD3',
        },
        ink: { DEFAULT: '#1F2937', muted: '#6B7280' },
      },
      fontFamily: {
        // System fonts only: nothing is fetched from the internet.
        sans: ['Inter', 'Segoe UI', 'Roboto', 'Helvetica Neue', 'Arial', 'sans-serif'],
      },
      boxShadow: { card: '0 2px 10px rgba(15, 23, 42, 0.08)' },
    },
  },
  plugins: [],
};
