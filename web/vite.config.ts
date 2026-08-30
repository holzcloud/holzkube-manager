import tailwindcss from '@tailwindcss/vite'
import { playwright } from '@vitest/browser-playwright'
import react from '@vitejs/plugin-react'
import { defaultExclude, defineConfig, type Plugin } from 'vitest/config'

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

/**
 * The one Node API this config needs, reached without depending on Node's types.
 *
 * `@types/node` is deliberately absent and `tsconfig.json` curates `types`
 * explicitly: this package's sources are browser code, and pulling Node's
 * globals into the same program would make `process`, `Buffer` and `__dirname`
 * type-check inside `src/`, where none of them exist at runtime. Holding the
 * specifier in a variable keeps TypeScript from resolving a module it has no
 * types for, while Node resolves it perfectly well when the config runs.
 */
const NODE_FS = 'node:fs'

async function fileExists(path: string): Promise<boolean> {
  const fs = (await import(/* @vite-ignore */ NODE_FS)) as {
    existsSync: (candidate: string) => boolean
  }
  return fs.existsSync(path)
}

/**
 * Fails the browser project with the command that fixes it.
 *
 * The ordinary way a developer meets this suite is by not having the browser
 * yet, and what Playwright says on its own is `npx playwright install` -- which
 * is the wrong command here, because playwright is a devDependency of `web/` and
 * not of the repository root, so run from where the developer is standing it
 * installs a *different* copy at a version nothing pins.
 *
 * Declared on the browser project rather than at the root: each project gets its
 * own Vite server, so this never loads for the jsdom run.
 */
function requireBrowserBinary(): Plugin {
  return {
    name: 'holzkube:require-browser-binary',
    async configResolved() {
      const { chromium } = await import('playwright')
      if (await fileExists(chromium.executablePath())) {
        return
      }
      throw new Error(
        'The browser test project needs a Chromium that is not installed yet.\n\n' +
          '    npm --prefix web exec -- playwright install chromium\n\n' +
          'That is the project\'s own runner, so the browser matches the pinned ' +
          'playwright version. It downloads roughly 150MB once and caches it. ' +
          'CI runs the same command. To skip this suite for one run: ' +
          'npm --prefix web run test -- --project jsdom',
      )
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
    // Two projects for the same reason: they are declared here so that both
    // inherit `resolve.alias` and `plugins` from the one config above, and the
    // Tailwind plugin in particular has to apply in BOTH -- the browser project
    // measures classes that plugin generates, and a project that did not run it
    // would measure an unstyled element and pass.
    //
    // `--project` selects one: `vitest run --project browser`. The bare command
    // runs both, which is what `task test:web` and CI's `Test web` step already
    // call, so neither of them had to learn a new command.
    projects: [
      {
        extends: true,
        test: {
          // Every setting this project has is the one the single `test` block
          // carried before the split; nothing about the jsdom suite changed.
          name: 'jsdom',
          environment: 'jsdom',
          globals: true,
          setupFiles: ['./src/test/setup.ts'],
          css: false,
          // The default include pattern matches `*.browser.test.tsx` too, so the
          // browser suite has to be excluded by name or it would run here as
          // well -- in an environment that lays nothing out, where every
          // measurement is zero and the sweep would pass vacuously. That failure
          // is silent, which is why the exclusion is explicit rather than a
          // narrower include.
          exclude: [...defaultExclude, '**/*.browser.test.?(c|m)[jt]s?(x)'],
        },
      },
      {
        extends: true,
        plugins: [requireBrowserBinary()],
        test: {
          name: 'browser',
          include: ['**/*.browser.test.?(c|m)[jt]s?(x)'],
          globals: true,
          // A layout measurement with the stylesheet switched off measures
          // nothing. This is the one setting that is deliberately the opposite
          // of the jsdom project's.
          css: true,
          // `./src/test/setup.ts` is deliberately NOT inherited here. Everything
          // it does is a jsdom prosthesis -- matchMedia, ResizeObserver,
          // hasPointerCapture, scrollIntoView -- and a real browser has all four.
          // Stubbing them would replace the engine's behaviour with the fake this
          // project exists to stop relying on. The browser suite does its own
          // cleanup in an afterEach.
          browser: {
            enabled: true,
            // A factory, not the string `'playwright'`: vitest 4 split each
            // provider into its own package and takes the imported factory, so
            // the provider and the runner cannot be at two different versions.
            // `@vitest/browser-playwright` is pinned at 4.1.11 alongside
            // `@vitest/browser` and `vitest` for that reason.
            provider: playwright(),
            headless: true,
            // 1200x900 is the viewport the UAT measured the 384px dialog at, so
            // the width assertion runs against the same page the finding came
            // from. It is also comfortably above the 640px `sm` breakpoint, which
            // is the branch the dialog's width rule lives in.
            instances: [{ browser: 'chromium' }],
            viewport: { width: 1200, height: 900 },
          },
        },
      },
    ],
  },
})
