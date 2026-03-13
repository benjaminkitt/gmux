import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createGmuxdClient } from './client.js'

function mockFetch(responses: Record<string, unknown>) {
  return vi.fn(async (url: string) => {
    const path = new URL(url).pathname
    const body = responses[path]
    if (!body) {
      return { ok: false, status: 404, statusText: 'Not Found', json: async () => ({}) }
    }
    return { ok: true, json: async () => body }
  })
}

describe('gmuxd client', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('parses health response', async () => {
    global.fetch = mockFetch({
      '/v1/health': { ok: true, data: { service: 'gmuxd', node_id: 'n1' } },
    }) as any

    const client = createGmuxdClient('http://localhost:8790')
    const health = await client.health()
    expect(health.service).toBe('gmuxd')
  })

  it('parses session list', async () => {
    global.fetch = mockFetch({
      '/v1/sessions': {
        ok: true,
        data: [
          {
            session_id: 's1',
            abduco_name: 'pi:test:1',
            kind: 'pi',
            state: 'running',
            updated_at: 1000,
          },
        ],
      },
    }) as any

    const client = createGmuxdClient('http://localhost:8790')
    const sessions = await client.listSessions()
    expect(sessions).toHaveLength(1)
    expect(sessions[0].session_id).toBe('s1')
  })

  it('parses attach response', async () => {
    global.fetch = mockFetch({
      '/v1/sessions/s1/attach': {
        ok: true,
        data: { transport: 'ttyd', port: 7711, is_new: true },
      },
    }) as any

    const client = createGmuxdClient('http://localhost:8790')
    const attach = await client.attachSession('s1')
    expect(attach.transport).toBe('ttyd')
    expect(attach.port).toBe(7711)
  })
})
