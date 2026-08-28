import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, type Plugin } from 'vitest/config'

/**
 * `emptyOutDir` wipes internal/httpapi/dist on every build, including the
 * tracked `.gitkeep` that lets `go build` succeed on a fresh clone before the
 * web bundle has ever been produced -- go:embed fails outright on a missing
 * directory. Re-emitting it as part of the bundle keeps that guarantee whether
 * the build was started through the Taskfile or with a bare `npm run build`.
 */
function keepEmbedDirectoryTracked(): Plugin {
  return {
    name: 'holzkube:keep-embed-dir-tracked',
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: '.gitkeep', source: '' })
    },
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss(), keepEmbedDirectoryTracked()],
  resolve: {
    alias: {
      // The single path alias. It is declared here and in tsconfig.json only;
      // components.json points shadcn at the same one, so a copied component
      // resolves identically in the bundler, in tsc and in vitest.
      '@': new URL('./src', import.meta.url).pathname,
    },
  },
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
  test: {
    // vitest lives in the build config on purpose: a second config file would
    // duplicate the path alias above, and the two would drift apart silently.
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
})
