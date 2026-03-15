/**
 * Mock data for UI development and testing.
 * Import with ?mock query param or when VITE_MOCK=1.
 *
 * Provides realistic session data across multiple folders with
 * various states to exercise all UI paths.
 */

export interface SessionStatus {
  label: string
  working: boolean
}

export interface Session {
  id: string
  created_at: string
  command: string[]
  cwd: string
  kind: string
  alive: boolean
  pid: number | null
  exit_code: number | null
  started_at: string
  exited_at: string | null
  title: string
  subtitle: string
  status: SessionStatus | null
  unread: boolean
  resumable?: boolean
  resume_key?: string
  close_action?: 'minimize' | 'dismiss'
  socket_path: string
  terminal_cols?: number
  terminal_rows?: number
}

export interface Folder {
  name: string      // display name (basename of cwd)
  path: string      // full path
  sessions: Session[]
}

function ago(minutes: number): string {
  const d = new Date(Date.now() - minutes * 60_000)
  return d.toISOString()
}

const MOCK_SESSIONS: Session[] = [
  // --- gmux project: 3 sessions ---
  {
    id: 'sess-a1b2c3',
    created_at: ago(45),
    command: ['pi'],
    cwd: '/home/user/dev/gmux',
    kind: 'pi',
    alive: true,
    pid: 12345,
    exit_code: null,
    started_at: ago(45),
    exited_at: null,
    title: 'implement adapter system',
    subtitle: 'iteration 3/10',
    status: { label: 'thinking', working: true },
    unread: false,
    socket_path: '/tmp/gmux-sessions/sess-a1b2c3.sock',
  },
  {
    id: 'sess-d4e5f6',
    created_at: ago(120),
    command: ['pi'],
    cwd: '/home/user/dev/gmux',
    kind: 'pi',
    alive: true,
    pid: 12346,
    exit_code: null,
    started_at: ago(120),
    exited_at: null,
    title: 'fix websocket proxy',
    subtitle: 'waiting for approval',
    status: { label: 'waiting for input', working: false },
    unread: true,
    socket_path: '/tmp/gmux-sessions/sess-d4e5f6.sock',
  },
  {
    id: 'sess-g7h8i9',
    created_at: ago(360),
    command: ['pi'],
    cwd: '/home/user/dev/gmux',
    kind: 'pi',
    alive: false,
    pid: null,
    exit_code: 0,
    started_at: ago(360),
    exited_at: ago(300),
    title: 'setup monorepo',
    subtitle: 'completed',
    status: { label: 'done', working: false },
    unread: false,
    socket_path: '/tmp/gmux-sessions/sess-g7h8i9.sock',
  },

  // --- myapp project: 2 sessions ---
  {
    id: 'sess-j1k2l3',
    created_at: ago(15),
    command: ['pi'],
    cwd: '/home/user/dev/myapp',
    kind: 'pi',
    alive: true,
    pid: 23456,
    exit_code: null,
    started_at: ago(15),
    exited_at: null,
    title: 'fix auth bug',
    subtitle: 'running tests',
    status: { label: 'running tests', working: true },
    unread: true,
    socket_path: '/tmp/gmux-sessions/sess-j1k2l3.sock',
  },
  {
    id: 'sess-m4n5o6',
    created_at: ago(1440),
    command: ['pi'],
    cwd: '/home/user/dev/myapp',
    kind: 'pi',
    alive: false,
    pid: null,
    exit_code: 1,
    started_at: ago(1440),
    exited_at: ago(1430),
    title: 'refactor models',
    subtitle: 'exit 1',
    status: { label: 'failed', working: false },
    unread: false,
    socket_path: '/tmp/gmux-sessions/sess-m4n5o6.sock',
  },

  // --- docs project: 1 session ---
  {
    id: 'sess-p7q8r9',
    created_at: ago(30),
    command: ['pi'],
    cwd: '/home/user/dev/docs',
    kind: 'pi',
    alive: true,
    pid: 34567,
    exit_code: null,
    started_at: ago(30),
    exited_at: null,
    title: 'update API reference',
    subtitle: '',
    status: { label: 'idle', working: false },
    unread: false,
    socket_path: '/tmp/gmux-sessions/sess-p7q8r9.sock',
  },

  // --- api project: 2 sessions ---
  {
    id: 'sess-s1t2u3',
    created_at: ago(5),
    command: ['pytest', 'tests/', '-v'],
    cwd: '/home/user/dev/api',
    kind: 'generic',
    alive: true,
    pid: 45678,
    exit_code: null,
    started_at: ago(5),
    exited_at: null,
    title: 'pytest tests/ -v',
    subtitle: '42/50 passed',
    status: { label: '42/50 passed', working: true },
    unread: false,
    socket_path: '/tmp/gmux-sessions/sess-s1t2u3.sock',
  },
  {
    id: 'sess-v4w5x6',
    created_at: ago(60),
    command: ['pi'],
    cwd: '/home/user/dev/api',
    kind: 'pi',
    alive: true,
    pid: 56789,
    exit_code: null,
    started_at: ago(60),
    exited_at: null,
    title: 'add rate limiting',
    subtitle: 'reviewing changes',
    status: { label: 'info', working: false },
    unread: false,
    socket_path: '/tmp/gmux-sessions/sess-v4w5x6.sock',
  },

  // --- infra project: 1 resumable ---
  {
    id: 'sess-y7z8a9',
    created_at: ago(2880),
    command: ['pi'],
    cwd: '/home/user/dev/infra',
    kind: 'pi',
    alive: false,
    pid: null,
    exit_code: 0,
    started_at: ago(2880),
    exited_at: ago(2800),
    title: 'terraform migration',
    subtitle: '2 days ago',
    status: null,
    unread: false,
    socket_path: '/tmp/gmux-sessions/sess-y7z8a9.sock',
  },
]

/** Group sessions by cwd into folders, sorted by most recent activity. */
export function groupByFolder(sessions: Session[]): Folder[] {
  const map = new Map<string, Session[]>()
  for (const s of sessions) {
    const existing = map.get(s.cwd) || []
    existing.push(s)
    map.set(s.cwd, existing)
  }

  const folders: Folder[] = []
  for (const [path, sessions] of map) {
    const parts = path.split('/')
    folders.push({
      name: parts[parts.length - 1],
      path,
      sessions: sessions.sort((a, b) => {
        return new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
      }),
    })
  }

  // Sort: folders with working sessions first, then alive, then rest
  const statePriority = (f: Folder): number => {
    if (f.sessions.some(s => s.alive && s.status?.working)) return 0
    if (f.sessions.some(s => s.alive)) return 1
    return 2
  }
  return folders.sort((a, b) => {
    const pa = statePriority(a)
    const pb = statePriority(b)
    if (pa !== pb) return pa - pb
    const aMax = Math.max(...a.sessions.map(s => new Date(s.created_at).getTime()))
    const bMax = Math.max(...b.sessions.map(s => new Date(s.created_at).getTime()))
    return bMax - aMax
  })
}

export function getMockSessions(): Session[] {
  return MOCK_SESSIONS
}

export function getMockFolders(): Folder[] {
  return groupByFolder(MOCK_SESSIONS)
}
