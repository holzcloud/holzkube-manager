import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    // The bundle is compiled into the binary from here. go:embed reads the
    // directory at compile time, which is why the build order web-before-go is
    // a hard dependency rather than a convention.
    outDir: '../internal/httpapi/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      // In dev the UI runs on Vite and the API on holzkubed. secure:false
      // accepts the self-signed certificate generated on first run (D-04).
      '/api': {
        target: 'https://127.0.0.1:8443',
        changeOrigin: true,
        secure: false,
      },
    },
  },
})
