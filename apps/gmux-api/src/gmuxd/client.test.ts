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

  it('parses session list (schema v2)', async () => {
    global.fetch = mockFetch({
      '/v1/sessions': {
        ok: true,
        data: [
          {
            id: 'sess-1',
            kind: 'pi',
            alive: true,
            pid: 12345,
            title: 'test session',
            status: { label: 'thinking', working: true },
          },
        ],
      },
    }) as any

    const client = createGmuxdClient('http://localhost:8790')
    const sessions = await client.listSessions()
    expect(sessions).toHaveLength(1)
    expect(sessions[0].id).toBe('sess-1')
    expect(sessions[0].alive).toBe(true)
  })

  it('parses attach response (websocket)', async () => {
    global.fetch = mockFetch({
      '/v1/sessions/sess-1/attach': {
        ok: true,
        data: { transport: 'websocket', ws_path: '/ws/sess-1', socket_path: '/tmp/gmux-sessions/sess-1.sock' },
      },
    }) as any

    const client = createGmuxdClient('http://localhost:8790')
    const attach = await client.attachSession('sess-1')
    expect(attach.transport).toBe('websocket')
    expect(attach.ws_path).toBe('/ws/sess-1')
  })

})
