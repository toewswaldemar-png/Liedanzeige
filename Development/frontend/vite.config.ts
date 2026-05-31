import path from 'path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/ws': {
        target: 'ws://localhost:1980',
        ws: true,
      },
    },
  },
  build: {
    outDir: '../server/static',
    emptyOutDir: true,
  },
})
