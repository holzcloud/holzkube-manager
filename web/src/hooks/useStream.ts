import { useEffect, useMemo, useRef, useState } from 'react'

/**
 * One EventSource per tab, carrying every panel that tab has open.
 *
 * A connection per panel would be simpler and wrong in a way that only shows
 * up in use: browsers cap concurrent connections per origin at six, and a node
 * detail page with kubelet, etcd, apid and dmesg open is already four of them.
 * The seventh panel would silently never connect.
 *
 * Resumption is the browser's own. EventSource reconnects by itself and sends
 * back the last `id:` it saw; the server makes that id the *whole* cursor set
 * (`topic=id,topic=id`), so every topic resumes from where that topic actually
 * stood. Nothing here has to track it.
 *
 * What this hook does not do is buffer history. A panel that mounts gets what
 * the server's ring buffer still holds, which is the same thing a reconnect
 * gets, so there is one answer to "what do I see when I open this" rather than
 * two.
 */

/** The four causes of "no lines are arriving", which call for four different
 * reactions from the person reading. */
export type StreamState = 'live' | 'reconnecting' | 'rebooting' | 'disconnected'

export interface StreamLine {
  /** Monotonic within a topic; the key a list renders on. */
  id: number
  line?: string
  state?: StreamState
  reason?: string
  at: string
  /** Set on the synthetic entry a dropped range produces. */
  gap?: { missed: number; through: number }
}

/** The connection itself, as opposed to any one topic's state. */
export type ConnectionState = 'connecting' | 'open' | 'closed'

interface Envelope {
  topic: string
  payload: { line?: string; state?: StreamState; reason?: string; at: string }
}

/** How many lines a panel keeps. Beyond this the oldest go: a log panel is a
 * tail, and an unbounded one is a memory leak that grows with how long
 * somebody left the tab open. */
const MAX_LINES = 2000

export interface StreamResult {
  lines: Record<string, StreamLine[]>
  connection: ConnectionState
}

export function useStream(topics: string[]): StreamResult {
  // The topic list is stringified for the dependency, so that a caller passing
  // a fresh array literal on every render does not reopen the connection on
  // every render.
  const key = useMemo(() => [...topics].sort().join('|'), [topics])

  const [lines, setLines] = useState<Record<string, StreamLine[]>>({})
  const [connection, setConnection] = useState<ConnectionState>('closed')
  const nextGapID = useRef(-1)

  useEffect(() => {
    if (key === '') {
      setConnection('closed')
      return
    }

    const query = key
      .split('|')
      .map((t) => `topic=${encodeURIComponent(t)}`)
      .join('&')

    setConnection('connecting')
    const source = new EventSource(`/api/v1/stream?${query}`, { withCredentials: true })

    const append = (topic: string, entry: StreamLine, replayable = false) => {
      setLines((prev) => {
        const existing = prev[topic] ?? []
        // Toggling a panel reopens the connection, and the server answers a new
        // connection by replaying its ring buffer. What this tab already holds
        // must not be shown twice: (topic, id) is the identity of a line.
        if (replayable && existing.some((e) => e.id === entry.id)) {
          return prev
        }
        const next = [...existing, entry]
        return { ...prev, [topic]: next.length > MAX_LINES ? next.slice(-MAX_LINES) : next }
      })
    }

    source.onopen = () => setConnection('open')

    source.onmessage = (ev: MessageEvent<string>) => {
      const envelope = parse<Envelope>(ev.data)
      if (!envelope?.payload || typeof envelope.topic !== 'string') {
        return
      }
      // The frame's own id is the whole cursor set, so the per-line key is
      // taken from the position of the line within its topic instead. A frame
      // that carries none gets a unique negative id: it is rendered, but can
      // never collide with another entry's key or be mistaken for a replay.
      const known = idFor(ev.lastEventId, envelope.topic)
      append(
        envelope.topic,
        {
          id: known ?? nextGapID.current--,
          line: envelope.payload.line,
          state: envelope.payload.state,
          reason: envelope.payload.reason,
          at: envelope.payload.at,
        },
        known !== null,
      )
    }

    source.addEventListener('gap', (ev) => {
      const envelope = parse<{
        topic: string
        payload: { missed: number; through: number }
      }>((ev as MessageEvent<string>).data)
      if (!envelope?.payload || typeof envelope.topic !== 'string') {
        return
      }
      // A visible break, never a silent omission. Somebody reading a log to
      // diagnose an outage and not being told that lines are missing draws
      // confident wrong conclusions.
      append(envelope.topic, {
        id: nextGapID.current--,
        at: new Date().toISOString(),
        gap: envelope.payload,
      })
    })

    // EventSource reports every failure as an anonymous error and reconnects
    // by itself. 'connecting' is therefore the honest label: the connection is
    // not up and something is trying.
    source.onerror = () => {
      setConnection(source.readyState === EventSource.CLOSED ? 'closed' : 'connecting')
    }

    return () => {
      source.close()
      setConnection('closed')
    }
  }, [key])

  return { lines, connection }
}

/** A frame that is not JSON is dropped; it must not throw out of the handler. */
function parse<T>(data: string): T | null {
  try {
    return JSON.parse(data) as T
  } catch {
    return null
  }
}

/** The per-topic position out of the connection-wide cursor, or null. */
function idFor(lastEventID: string, topic: string): number | null {
  for (const part of lastEventID.split(',')) {
    const [name, value] = part.split('=')
    if (name === topic) {
      const n = Number(value)
      return Number.isFinite(n) && value !== '' ? n : null
    }
  }
  return null
}
