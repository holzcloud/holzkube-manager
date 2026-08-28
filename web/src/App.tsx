import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { type AuditRecord, api, type Me, ProblemError } from './api'

/**
 * The minimal slice that proves store -> API -> UI on real data.
 *
 * Router, component library, theme toggle and the persistent app shell are
 * plan 05. Everything here is deliberately plain so that what is being proven
 * stays visible. All copy is English, with no i18n layer (D-09).
 */

type Phase = 'loading' | 'setup' | 'login' | 'ready'

function errorMessage(err: unknown): string {
  if (err instanceof ProblemError) {
    return err.message
  }
  if (err instanceof Error) {
    return err.message
  }
  return 'Something went wrong.'
}

export default function App() {
  const [phase, setPhase] = useState<Phase>('loading')
  const [chainOK, setChainOK] = useState(true)
  const [chainDetail, setChainDetail] = useState('')
  const [me, setMe] = useState<Me | null>(null)
  const [records, setRecords] = useState<AuditRecord[]>([])
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    try {
      const status = await api.status()
      setChainOK(status.audit_chain.ok)
      setChainDetail(
        status.audit_chain.ok
          ? ''
          : `Broken at line ${status.audit_chain.broken_at_line} of ${status.audit_chain.file}`,
      )

      if (status.setup_required) {
        setPhase('setup')
        setMe(null)
        return
      }

      try {
        const identity = await api.me()
        setMe(identity)
        const page = await api.audit()
        setRecords(page.items)
        setPhase('ready')
      } catch {
        setMe(null)
        setPhase('login')
      }
    } catch (err) {
      setError(errorMessage(err))
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  return (
    <main className="mx-auto max-w-4xl px-6 py-10">
      <header className="mb-8">
        <h1 className="text-2xl font-semibold tracking-tight">holzkube</h1>
        <p className="text-sm text-neutral-400">Talos cluster management</p>
      </header>

      {!chainOK && (
        <div className="mb-6 rounded border border-red-500/50 bg-red-500/10 px-4 py-3 text-sm text-red-200">
          <strong className="font-semibold">Audit chain does not verify.</strong> {chainDetail}
        </div>
      )}

      {error !== '' && (
        <div className="mb-6 rounded border border-amber-500/50 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">
          {error}
        </div>
      )}

      {phase === 'loading' && <p className="text-neutral-400">Loading…</p>}

      {phase === 'setup' && (
        <CredentialsForm
          title="Create the operator account"
          hint="This runs once. There is no second account and no recovery path."
          submitLabel="Create account"
          onSubmit={api.setup}
          onDone={refresh}
          setError={setError}
        />
      )}

      {phase === 'login' && (
        <CredentialsForm
          title="Sign in"
          hint=""
          submitLabel="Sign in"
          onSubmit={api.login}
          onDone={refresh}
          setError={setError}
        />
      )}

      {phase === 'ready' && me && (
        <section>
          <div className="mb-4 flex items-center justify-between">
            <p className="text-sm text-neutral-400">
              Signed in as <span className="text-neutral-100">{me.username}</span>
            </p>
            <button
              type="button"
              className="rounded border border-neutral-700 px-3 py-1 text-sm hover:bg-neutral-800"
              onClick={() => {
                void api
                  .logout()
                  .then(refresh)
                  .catch((err: unknown) => setError(errorMessage(err)))
              }}
            >
              Sign out
            </button>
          </div>
          <AuditTable records={records} />
        </section>
      )}
    </main>
  )
}

function CredentialsForm(props: {
  title: string
  hint: string
  submitLabel: string
  onSubmit: (username: string, password: string) => Promise<void>
  onDone: () => Promise<void>
  setError: (message: string) => void
}) {
  const { title, hint, submitLabel, onSubmit, onDone, setError } = props
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await onSubmit(username, password)
      setPassword('')
      await onDone()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="max-w-sm space-y-4">
      <div>
        <h2 className="text-lg font-medium">{title}</h2>
        {hint !== '' && <p className="mt-1 text-sm text-neutral-400">{hint}</p>}
      </div>
      <label className="block text-sm">
        <span className="mb-1 block text-neutral-300">Username</span>
        <input
          className="w-full rounded border border-neutral-700 bg-neutral-900 px-3 py-2"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoComplete="username"
          required
        />
      </label>
      <label className="block text-sm">
        <span className="mb-1 block text-neutral-300">Password</span>
        <input
          type="password"
          className="w-full rounded border border-neutral-700 bg-neutral-900 px-3 py-2"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="current-password"
          required
        />
      </label>
      <button
        type="submit"
        disabled={busy}
        className="rounded bg-neutral-100 px-4 py-2 text-sm font-medium text-neutral-900 disabled:opacity-50"
      >
        {busy ? 'Working…' : submitLabel}
      </button>
    </form>
  )
}

function AuditTable({ records }: { records: AuditRecord[] }) {
  if (records.length === 0) {
    return <p className="text-sm text-neutral-400">No audit records yet.</p>
  }

  return (
    <table className="w-full border-collapse text-left text-sm">
      <thead>
        <tr className="border-b border-neutral-800 text-neutral-400">
          <th className="py-2 pr-4 font-medium">Seq</th>
          <th className="py-2 pr-4 font-medium">Time</th>
          <th className="py-2 pr-4 font-medium">Actor</th>
          <th className="py-2 pr-4 font-medium">Action</th>
          <th className="py-2 font-medium">Outcome</th>
        </tr>
      </thead>
      <tbody>
        {records.map((record) => (
          <tr key={record.seq} className="border-b border-neutral-900">
            <td className="py-2 pr-4 tabular-nums text-neutral-500">{record.seq}</td>
            <td className="py-2 pr-4 tabular-nums">{record.ts}</td>
            <td className="py-2 pr-4">{record.actor}</td>
            <td className="py-2 pr-4 font-mono text-xs">{record.action}</td>
            <td className="py-2">{record.outcome}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
