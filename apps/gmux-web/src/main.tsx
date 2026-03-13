import { createTRPCProxyClient, httpBatchLink } from '@trpc/client'
import type { SessionEvent, SessionSummary } from '@gmux/protocol'
import { SessionEventSchema } from '@gmux/protocol'
import { render } from 'preact'
import { useEffect, useMemo, useState } from 'preact/hooks'

const trpc: any = createTRPCProxyClient<any>({
  links: [
    httpBatchLink({
      url: '/trpc',
    }),
  ],
})

function sortSessions(items: SessionSummary[]) {
  return [...items].sort((a, b) => b.updated_at - a.updated_at)
}

function App() {
  const [sessions, setSessions] = useState<SessionSummary[]>([])
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let active = true

    trpc.sessions.list
      .query()
      .then((data: SessionSummary[]) => {
        if (!active) return
        setSessions(sortSessions(data))

        if (!selectedSessionId && data.length > 0) {
          setSelectedSessionId(data[0].session_id)
        }
      })
      .catch((err: unknown) => {
        if (!active) return
        setError(String(err))
      })

    return () => {
      active = false
    }
  }, [])

  useEffect(() => {
    const source = new EventSource('/api/events')

    source.onmessage = (message) => {
      let parsed: SessionEvent
      try {
        parsed = SessionEventSchema.parse(JSON.parse(message.data))
      } catch {
        return
      }

      if (parsed.type === 'session-upsert') {
        setSessions((current) => {
          const without = current.filter((it) => it.session_id !== parsed.session_id)
          return sortSessions([...without, parsed.session])
        })
        return
      }

      if (parsed.type === 'session-state') {
        setSessions((current) =>
          sortSessions(
            current.map((it) =>
              it.session_id === parsed.session_id
                ? { ...it, state: parsed.state, updated_at: parsed.updated_at }
                : it,
            ),
          ),
        )
        return
      }

      if (parsed.type === 'session-remove') {
        setSessions((current) => current.filter((it) => it.session_id !== parsed.session_id))
      }
    }

    source.onerror = () => {
      setError('event stream disconnected')
    }

    return () => {
      source.close()
    }
  }, [])

  const selected = useMemo(
    () => sessions.find((item) => item.session_id === selectedSessionId) ?? null,
    [sessions, selectedSessionId],
  )

  return (
    <main style={{ fontFamily: 'system-ui, sans-serif', padding: '1rem' }}>
      <h1>gmux</h1>
      <p>Vertical slice: typed session list + SSE updates + UI-owned selection.</p>
      {error ? <p style={{ color: '#bf616a' }}>Error: {error}</p> : null}

      <section style={{ display: 'grid', gap: '0.75rem', gridTemplateColumns: '320px 1fr' }}>
        <aside style={{ border: '1px solid #4c566a', borderRadius: '8px', padding: '0.5rem' }}>
          {sessions.length === 0 ? <p>No sessions</p> : null}
          {sessions.map((session) => {
            const selected = selectedSessionId === session.session_id
            return (
              <button
                key={session.session_id}
                onClick={() => setSelectedSessionId(session.session_id)}
                style={{
                  width: '100%',
                  textAlign: 'left',
                  marginBottom: '0.5rem',
                  border: selected ? '1px solid #88c0d0' : '1px solid #4c566a',
                  borderRadius: '6px',
                  background: selected ? '#3b4252' : '#2e3440',
                  color: '#eceff4',
                  padding: '0.5rem',
                }}
              >
                <div style={{ fontWeight: 600 }}>{session.title ?? session.abduco_name}</div>
                <div style={{ fontSize: '0.85rem', opacity: 0.9 }}>{session.state}</div>
              </button>
            )
          })}
        </aside>

        <section style={{ border: '1px solid #4c566a', borderRadius: '8px', padding: '0.75rem' }}>
          {selected ? (
            <>
              <h2 style={{ marginTop: 0 }}>{selected.title ?? selected.abduco_name}</h2>
              <p>session_id: {selected.session_id}</p>
              <p>state: {selected.state}</p>
            </>
          ) : (
            <p>Select a session.</p>
          )}
        </section>
      </section>
    </main>
  )
}

render(<App />, document.getElementById('app')!)
