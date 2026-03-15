import { render } from 'preact'
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { ImageAddon } from '@xterm/addon-image'
// AttachAddon removed — we wire onmessage/onData manually for reconnect support
import '@xterm/xterm/css/xterm.css'
import './styles.css'
import { attachKeyboardHandler } from './keyboard'
import { createReplayBuffer } from './replay'
import { createSidebarState } from './sidebar-state'

import type { Session, Folder } from './mock-data'
import { getMockFolders, groupByFolder } from './mock-data'
import type { Session as ProtocolSession } from '@gmux/protocol'

// ── Config ──

const USE_MOCK = import.meta.env.VITE_MOCK === '1' || location.search.includes('mock')

/** Map protocol session (partial fields) to UI session (all fields required) */
function toUISession(s: ProtocolSession): Session {
  return {
    id: s.id,
    created_at: s.created_at ?? new Date().toISOString(),
    command: s.command ?? [],
    cwd: s.cwd ?? '',
    kind: s.kind ?? 'shell',
    alive: s.alive,
    pid: s.pid ?? null,
    exit_code: s.exit_code ?? null,
    started_at: s.started_at ?? s.created_at ?? new Date().toISOString(),
    exited_at: s.exited_at ?? null,
    title: s.title ?? s.command?.[0] ?? 'session',
    subtitle: s.subtitle ?? '',
    status: s.status ?? null,
    unread: s.unread ?? false,
    resumable: (s as any).resumable ?? false,
    resume_key: (s as any).resume_key ?? '',
    close_action: s.close_action ?? (s.alive ? 'dismiss' : undefined),
    socket_path: s.socket_path ?? '',
  }
}

async function fetchSessions(): Promise<Session[]> {
  const resp = await fetch('/trpc/sessions.list')
  const json = await resp.json()
  // tRPC wraps in { result: { data: [...] } }
  const data: ProtocolSession[] = json?.result?.data ?? []
  return data.map(toUISession)
}

async function killSession(sessionId: string): Promise<void> {
  await fetch('/trpc/sessions.kill', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sessionId }),
  })
}

async function resumeSession(sessionId: string): Promise<void> {
  await fetch('/trpc/sessions.resume', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sessionId }),
  })
}

// ── Launcher types & config ──

interface LauncherDef {
  id: string
  label: string
  command: string[]
  description?: string
  available: boolean
}

interface LaunchConfig {
  default_launcher: string
  launchers: LauncherDef[]
}

let _configCache: LaunchConfig | null = null

async function fetchConfig(): Promise<LaunchConfig> {
  if (_configCache) return _configCache
  try {
    const resp = await fetch('/trpc/config')
    const json = await resp.json()
    _configCache = json.result?.data ?? json.data ?? json
    return _configCache!
  } catch {
    return { default_launcher: 'shell', launchers: [{ id: 'shell', label: 'Shell', command: [], available: true }] }
  }
}

async function launchSession(launcherId: string, cwd?: string): Promise<void> {
  await fetch('/trpc/sessions.launch', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ launcher_id: launcherId, cwd }),
  })
}

// ── LaunchButton — transforms into inline menu on click ──
//
// Idle:      [+]
// Open:      [+ button becomes default item] → other items appear below
// Launching: [spinner]
//
// Double-click works because the default item occupies the exact same
// position as the + button. First click opens, second click hits default.

// Track pending launches globally so App can auto-select new sessions
let _pendingLaunchAt = 0

function LaunchButton({ cwd, className }: { cwd?: string; className?: string }) {
  const [state, setState] = useState<'idle' | 'loading' | 'open' | 'launching'>('idle')
  const [config, setConfig] = useState<LaunchConfig | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Pre-fetch config on first hover so open is instant
  const handleMouseEnter = () => {
    if (!config) fetchConfig().then(setConfig)
  }

  const handleClick = (e: MouseEvent) => {
    e.stopPropagation()
    if (state === 'idle') {
      if (config) {
        setState('open')
      } else {
        setState('loading')
        fetchConfig().then(cfg => {
          setConfig(cfg)
          setState('open')
        })
      }
    } else if (state === 'open') {
      setState('idle')
    }
  }

  const handleLaunch = (id: string) => {
    setState('launching')
    _pendingLaunchAt = Date.now()
    launchSession(id, cwd).finally(() => {
      // Reset after a short delay to show spinner
      setTimeout(() => setState('idle'), 600)
    })
  }

  // Close on outside click
  useEffect(() => {
    if (state !== 'open') return
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setState('idle')
      }
    }
    const timer = setTimeout(() => document.addEventListener('mousedown', handler), 0)
    return () => {
      clearTimeout(timer)
      document.removeEventListener('mousedown', handler)
    }
  }, [state])

  // Close on Escape
  useEffect(() => {
    if (state !== 'open') return
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') setState('idle') }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [state])

  const isOpen = state === 'open' && config
  const isLoading = state === 'launching' || state === 'loading'

  let defaultLauncher: LauncherDef | undefined
  let others: LauncherDef[] = []
  if (isOpen && config) {
    defaultLauncher = config.launchers.find(l => l.id === config.default_launcher)
    others = config.launchers.filter(l => l.id !== config.default_launcher)
  }

  // Always render the + button for stable layout. Menu overlays on top.
  return (
    <div class={`launch-container ${className ?? ''}`} ref={containerRef} onMouseEnter={handleMouseEnter}>
      <button
        class={`launch-btn ${isLoading ? 'loading' : ''}`}
        title={cwd ? `New session in ${cwd}` : 'New session in ~'}
        onClick={handleClick}
      >
        {isLoading ? (
          <svg viewBox="0 0 16 16" width="14" height="14" class="spin">
            <circle cx="8" cy="8" r="6" fill="none" stroke="currentColor"
              stroke-width="2" stroke-dasharray="28" stroke-dashoffset="8" stroke-linecap="round" />
          </svg>
        ) : '+'}
      </button>
      {isOpen && (
        <div class="launch-inline-menu">
          {defaultLauncher && (
            <button
              class="launch-inline-item launch-inline-default"
              onClick={(e) => { e.stopPropagation(); handleLaunch(defaultLauncher!.id) }}
            >
              <span class="launch-inline-label">{defaultLauncher.label}</span>
              <span class="launch-inline-desc">{defaultLauncher.description ?? ''}</span>
            </button>
          )}
          {others.length > 0 && (
            <div class="launch-inline-divider" />
          )}
          {others.map((l, i) => (
            <button
              key={l.id}
              class="launch-inline-item"
              style={{ animationDelay: `${(i + 1) * 50}ms` }}
              onClick={(e) => { e.stopPropagation(); handleLaunch(l.id) }}
            >
              <span class="launch-inline-label">{l.label}</span>
              <span class="launch-inline-desc">{l.description ?? ''}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

const TERM_THEME = {
  background: '#0f141a',            // --bg-surface
  foreground: '#d3d8de',            // --text
  cursor: '#d3d8de',                // --text
  cursorAccent: '#0f141a',          // --bg-surface
  selectionBackground: '#141b24cc', // --bg-selected + alpha
  black: '#151b21',                 // --border
  red: '#c25d66',
  green: '#a3be8c',
  yellow: '#ebcb8b',
  blue: '#81a1c1',
  magenta: '#b48ead',
  cyan: '#49b8b8',                  // --accent
  white: '#d3d8de',                 // --text
  brightBlack: '#595e63',           // --text-muted
  brightRed: '#d06c75',
  brightGreen: '#b4d19a',
  brightYellow: '#f0d9a0',
  brightBlue: '#93b3d1',
  brightMagenta: '#c9a3c4',
  brightCyan: '#5fcece',
  brightWhite: '#eceff4',
}

// ── Utilities ──

function formatAge(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  const mins = Math.floor(ms / 60_000)
  if (mins < 1) return 'now'
  if (mins < 60) return `${mins}m`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h`
  const days = Math.floor(hrs / 24)
  return `${days}d`
}

function dotClass(session: Session): string {
  if (!session.alive && !session.resumable) return 'dead'
  if (!session.status) return 'paused'
  return session.status.state
}

function folderDotColor(folder: Folder): string | null {
  // Attention (needs input) takes priority — warm dot
  if (folder.sessions.some(s => s.alive && s.status?.state === 'attention'))
    return 'oklch(72% 0.17 55)'
  // Active (working) — accent cyan
  if (folder.sessions.some(s => s.alive && s.status?.state === 'active'))
    return 'var(--accent)'
  return null
}

// ── Components ──

/** Determine if a session should show a right-side indicator dot and which kind. */
function sessionIndicator(session: Session): 'working' | 'needs-input' | null {
  if (!session.alive || !session.status) return null
  if (session.status.state === 'attention') return 'needs-input'
  if (session.status.state === 'active') return 'working'
  return null
}

function SessionItem({
  session,
  selected,
  onClick,
  onClose,
}: {
  session: Session
  selected: boolean
  onClick: () => void
  onClose?: () => void
}) {
  const indicator = sessionIndicator(session)
  const closeAction = session.close_action

  return (
    <div
      class={`session-item ${selected ? 'selected' : ''} ${!session.alive && !session.resumable ? 'dead' : ''}`}
      onClick={onClick}
    >
      <div class="session-content">
        <div class={`session-title${session.unread ? ' unread' : ''}`}>{session.title}</div>
        <div class="session-meta">
          {session.status && (
            <>
              <span class="session-status-label">{session.status.label}</span>
              <span class="session-meta-sep">·</span>
            </>
          )}
          <span class="session-time">{formatAge(session.created_at)}</span>
          {session.kind !== 'shell' && (
            <>
              <span class="session-meta-sep">·</span>
              <span>{session.kind}</span>
            </>
          )}
        </div>
      </div>
      {indicator && (
        <div class="session-indicator">
          <span class={`session-indicator-dot ${indicator}`} />
        </div>
      )}
      {onClose && closeAction && (
        <button
          class={`session-close-btn ${closeAction}`}
          onClick={(e) => { e.stopPropagation(); onClose() }}
          title={closeAction === 'minimize' ? 'Suspend session' : 'Remove session'}
        >
          {closeAction === 'minimize' ? '−' : '×'}
        </button>
      )}
    </div>
  )
}

function FolderGroup({
  folder,
  selectedId,
  onSelect,
  onCloseSession,
  onHideFolder,
  isSessionVisible,
}: {
  folder: Folder
  selectedId: string | null
  onSelect: (id: string) => void
  onCloseSession: (session: Session) => void
  onHideFolder: (cwd: string) => void
  isSessionVisible: (session: Session) => boolean
}) {
  const [expanded, setExpanded] = useState(true)
  const [showMore, setShowMore] = useState(false)
  const dotColor = folderDotColor(folder)

  // Split sessions into visible and collapsed ("show more")
  const visible: Session[] = []
  const collapsed: Session[] = []
  for (const s of folder.sessions) {
    if (isSessionVisible(s)) visible.push(s)
    else collapsed.push(s)
  }

  const aliveCount = folder.sessions.filter(s => s.alive).length
  const displayCount = aliveCount > 0 ? aliveCount : visible.length

  return (
    <div class="folder">
      <div class="folder-header" onClick={() => setExpanded(e => !e)}>
        <div class={`folder-chevron ${expanded ? 'open' : ''}`}>▶</div>
        <div class="folder-name">{folder.name}</div>
        {dotColor && (
          <div class="folder-dot" style={{ background: dotColor }} />
        )}
        <div class="folder-count">{displayCount}</div>
        <LaunchButton cwd={folder.path} className="folder-launch-btn" />
      </div>
      {expanded && (
        <div class="folder-sessions">
          {visible.map(s => (
            <SessionItem
              key={s.id}
              session={s}
              selected={selectedId === s.id}
              onClick={() => onSelect(s.id)}
              onClose={() => onCloseSession(s)}
            />
          ))}
          {visible.length === 0 && collapsed.length > 0 && (
            <button
              class="folder-hide-btn"
              onClick={() => onHideFolder(folder.path)}
            >
              Hide folder
            </button>
          )}
          {collapsed.length > 0 && (
            <>
              <button
                class="show-more-btn"
                onClick={() => setShowMore(v => !v)}
              >
                {showMore ? 'Show less' : `Show ${collapsed.length} more`}
              </button>
              {showMore && collapsed.map(s => (
                <SessionItem
                  key={s.id}
                  session={s}
                  selected={selectedId === s.id}
                  onClick={() => onSelect(s.id)}
                  onClose={() => onCloseSession(s)}
                />
              ))}
            </>
          )}
        </div>
      )}
    </div>
  )
}

function Sidebar({
  folders,
  hiddenFolders,
  selectedId,
  onSelect,
  onCloseSession,
  onHideFolder,
  onShowFolder,
  isSessionVisible,
  open,
  onClose,
}: {
  folders: Folder[]
  hiddenFolders: Folder[]
  selectedId: string | null
  onSelect: (id: string) => void
  onCloseSession: (session: Session) => void
  onHideFolder: (cwd: string) => void
  onShowFolder: (cwd: string) => void
  isSessionVisible: (session: Session) => boolean
  open: boolean
  onClose: () => void
}) {
  const [showFolderPicker, setShowFolderPicker] = useState(false)

  return (
    <>
      <div class={`sidebar-overlay ${open ? 'visible' : ''}`} onClick={onClose} />
      <aside class={`sidebar ${open ? 'open' : ''}`}>
        <div class="sidebar-header">
          <div class="sidebar-logo">gmux</div>
          <div class="sidebar-badge">alpha</div>
          <LaunchButton className="sidebar-launch-btn" />
        </div>
        <div class="sidebar-scroll">
          {folders.map(f => (
            <FolderGroup
              key={f.path}
              folder={f}
              selectedId={selectedId}
              onSelect={(id) => {
                onSelect(id)
                onClose()
              }}
              onCloseSession={onCloseSession}
              onHideFolder={onHideFolder}
              isSessionVisible={isSessionVisible}
            />
          ))}
          {hiddenFolders.length > 0 && (
            <div class="sidebar-add-folder">
              <button
                class="add-folder-btn"
                onClick={() => setShowFolderPicker(v => !v)}
              >
                + Add folder
              </button>
              {showFolderPicker && (
                <div class="folder-picker">
                  {hiddenFolders.map(f => (
                    <button
                      key={f.path}
                      class="folder-picker-item"
                      onClick={() => {
                        onShowFolder(f.path)
                        setShowFolderPicker(false)
                      }}
                    >
                      <span class="folder-picker-name">{f.name}</span>
                      <span class="folder-picker-count">{f.sessions.length}</span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </aside>
    </>
  )
}



/**
 * Single xterm.js instance with reconnecting WebSocket.
 *
 * Architecture: one Terminal lives for the app lifetime. Switching sessions
 * closes the old WS, clears the terminal, and opens a new WS. The runner's
 * 128KB scrollback ring buffer replays on connect, so history is preserved
 * without keeping per-session xterm instances alive.
 *
 * Auto-reconnect on WS drop with exponential backoff.
 * No AttachAddon — we wire onmessage/onData manually so we can reconnect.
 */
/** Send current terminal dimensions over WebSocket (including pixel size for image protocols). */
function sendResize(ws: WebSocket | null, fit: FitAddon | null, term: Terminal | null) {
  if (!fit || !term || !ws || ws.readyState !== WebSocket.OPEN) return
  const dims = fit.proposeDimensions()
  if (!dims) return
  const msg: Record<string, unknown> = { type: 'resize', cols: dims.cols, rows: dims.rows }
  const el = term.element
  if (el) {
    msg.pixelWidth = el.clientWidth
    msg.pixelHeight = el.clientHeight
  }
  ws.send(JSON.stringify(msg))
}

function ctrlSequenceFor(data: string): string | null {
  if (data.length !== 1) return null

  const ch = data
  if (/[a-z]/i.test(ch)) {
    return String.fromCharCode(ch.toUpperCase().charCodeAt(0) - 64)
  }

  switch (ch) {
    case '@':
      return '\x00'
    case '[':
      return '\x1b'
    case '\\':
      return '\x1c'
    case ']':
      return '\x1d'
    case '^':
      return '\x1e'
    case '_':
      return '\x1f'
    case '?':
      return '\x7f'
    default:
      return null
  }
}

function TerminalView({
  sessionId,
  ctrlArmed,
  onCtrlConsumed,
  onInputReady,
}: {
  sessionId: string
  ctrlArmed: boolean
  onCtrlConsumed: () => void
  onInputReady?: (send: ((data: string) => void) | null) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const disposed = useRef(false)
  const currentSessionId = useRef(sessionId)
  const ctrlArmedRef = useRef(ctrlArmed)
  const [termLoading, setTermLoading] = useState(true)

  // Keep ref in sync so reconnect closure sees latest value
  currentSessionId.current = sessionId
  ctrlArmedRef.current = ctrlArmed

  // One-time terminal setup
  useEffect(() => {
    if (!containerRef.current || USE_MOCK) return
    disposed.current = false

    const term = new Terminal({
      theme: TERM_THEME,
      fontFamily: "'Fira Code', monospace",
      fontSize: 14,
      cursorBlink: true,
    })
    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(new ImageAddon())
    term.open(containerRef.current)
    fitAddon.fit()
    termRef.current = term
    fitRef.current = fitAddon

    // Send raw input to PTY — always uses current wsRef
    const sendRawInput = (data: string) => {
      const ws = wsRef.current
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
        term.focus()
      }
    }

    const sendInput = (data: string) => {
      if (ctrlArmedRef.current) {
        const ctrlData = ctrlSequenceFor(data)
        if (ctrlData) {
          ctrlArmedRef.current = false
          onCtrlConsumed()
          sendRawInput(ctrlData)
          return
        }
      }
      sendRawInput(data)
    }

    onInputReady?.(sendRawInput)

    // Terminal input → WS
    const dataDisposable = term.onData((data) => sendInput(data))

    // Keyboard handling
    attachKeyboardHandler(term, sendInput)

    // Auto-focus terminal on any keydown outside of it.
    // This ensures keyboard input always goes to the terminal
    // without requiring the user to click it first.
    const handleGlobalKeydown = (ev: KeyboardEvent) => {
      const tag = (ev.target as HTMLElement)?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
      if (containerRef.current?.contains(ev.target as Node)) return
      term.focus()
    }
    window.addEventListener('keydown', handleGlobalKeydown, true)

    // Window resize → fit + send dims (including pixel size for image protocols)
    const onResize = () => {
      fitAddon.fit()
      sendResize(wsRef.current, fitRef.current, termRef.current)
    }
    window.addEventListener('resize', onResize)

    return () => {
      disposed.current = true
      window.removeEventListener('keydown', handleGlobalKeydown, true)
      window.removeEventListener('resize', onResize)
      dataDisposable.dispose()
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      wsRef.current?.close()
      wsRef.current = null
      onInputReady?.(null)
      term.dispose()
      termRef.current = null
      fitRef.current = null
    }
  }, [onCtrlConsumed, onInputReady]) // terminal lives for component lifetime

  // WebSocket connection — reconnects on sessionId change or drop
  useEffect(() => {
    if (!termRef.current || USE_MOCK) return

    const term = termRef.current
    let attempt = 0
    let intentionalClose = false

    setTermLoading(true)

    function connect() {
      if (disposed.current) return

      // Close previous connection (reconnect case, not session switch)
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }

      // Replay buffer: detects synchronized scrollback replay.
      // The runner wraps the replay in BSU + reset sequences + scrollback + ESU,
      // so xterm handles the clear internally as part of the atomic render.
      // If BSU detected → buffer until ESU, write all at once (xterm renders atomically)
      // If no BSU → write immediately (old runner / no scrollback)
      // Frontend never calls term.clear()/term.reset() — all done via escape sequences.
      const replay = createReplayBuffer((chunks) => {
        // Check if replay has content beyond just BSU/ESU markers (16 bytes).
        // BSU (8 bytes) + ESU (8 bytes) = 16 bytes minimum for an empty replay.
        const totalBytes = chunks.reduce((n, c) => n + c.length, 0)
        const hasContent = totalBytes > 16

        for (const chunk of chunks) term.write(chunk)

        if (hasContent) setTermLoading(false)
      })

      const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
      const ws = new WebSocket(`${wsProtocol}//${location.host}/ws/${sessionId}`)
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onopen = () => {
        attempt = 0
        sendResize(ws, fitRef.current, termRef.current)
      }

      // WS data → terminal
      ws.onmessage = (ev) => {
        const data = ev.data instanceof ArrayBuffer
          ? new Uint8Array(ev.data)
          : new TextEncoder().encode(ev.data)

        // During replay: buffer feeds into replay detector which writes to term
        if (replay.state !== 'done') {
          replay.push(data)
          return
        }

        // Post-replay: any real output means we have something to show
        setTermLoading(false)
        term.write(data)
      }

      ws.onclose = () => {
        if (disposed.current || intentionalClose) return
        // Don't reconnect if session switched away
        if (currentSessionId.current !== sessionId) return

        // Exponential backoff: 500ms, 1s, 2s, 4s, max 8s
        const delay = Math.min(500 * Math.pow(2, attempt), 8000)
        attempt++
        reconnectTimer.current = setTimeout(connect, delay)
      }

      ws.onerror = () => {
        // onclose will fire after this, which handles reconnect
      }
    }

    connect()

    return () => {
      intentionalClose = true
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      reconnectTimer.current = null
      wsRef.current?.close()
      wsRef.current = null
    }
  }, [sessionId]) // reconnect when session changes

  if (USE_MOCK) {
    return (
      <div
        ref={containerRef}
        class="terminal-container"
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontFamily: "'JetBrains Mono', monospace",
          fontSize: '13px',
          color: 'var(--text-muted)',
        }}
      >
        Terminal: {sessionId}
      </div>
    )
  }

  return (
    <div style={{ flex: 1, position: 'relative', overflow: 'hidden' }}>
      <div ref={containerRef} class="terminal-container" />
      {termLoading && (
        <div class="terminal-loading">
          <span class="terminal-loading-dot" />
          Starting…
        </div>
      )}
    </div>
  )
}

function EmptyState({ launchers }: { launchers: LauncherDef[] }) {
  const [launching, setLaunching] = useState<string | null>(null)

  const handleLaunch = (id: string) => {
    setLaunching(id)
    _pendingLaunchAt = Date.now()
    launchSession(id).finally(() => setLaunching(null))
  }

  return (
    <div class="empty-state">
      <div class="empty-state-icon">⌘</div>
      <div class="empty-state-title">No session selected</div>
      {launchers.length > 0 && (
        <div class="empty-state-launchers">
          {launchers.map(l => (
            <button
              key={l.id}
              class={`empty-state-launch-btn ${launching === l.id ? 'launching' : ''}`}
              onClick={() => handleLaunch(l.id)}
              disabled={launching !== null}
            >
              {l.label}
              {l.description && <span class="launch-desc">{l.description}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function MainHeader({ session, onKill }: { session: Session | null; onKill?: (id: string) => void }) {
  if (!session) {
    return (
      <div class="main-header">
        <div class="main-header-title" style={{ color: 'var(--text-muted)' }}>
          gmux
        </div>
      </div>
    )
  }

  const shortCwd = session.cwd.replace(/^\/home\/[^/]+/, '~')

  return (
    <div class="main-header">
      <div class="main-header-left">
        <div class="main-header-title">{session.title}</div>
        <div class="main-header-meta">
          <span class="main-header-cwd">{shortCwd}</span>
          {session.kind !== 'shell' && (
            <>
              <span class="main-header-sep">·</span>
              <span class="main-header-kind">{session.kind}</span>
            </>
          )}
        </div>
      </div>
      <div class="main-header-right">
        {session.status && (
          <div class={`main-header-status ${session.status.state}`}>
            <span
              class={`session-dot ${session.status.state}`}
              style={{ width: 5, height: 5 }}
            />
            {session.status.label}
          </div>
        )}
        {session.alive && onKill && (
          <button
            class="header-kill-btn"
            onClick={() => onKill(session.id)}
            title="Kill session"
          >
            X
          </button>
        )}
      </div>
    </div>
  )
}

function MobileTerminalBar({
  canSend,
  ctrlArmed,
  onMenu,
  onSend,
  onToggleCtrl,
}: {
  canSend: boolean
  ctrlArmed: boolean
  onMenu: () => void
  onSend: (data: string) => void
  onToggleCtrl: () => void
}) {
  const actions = [
    { label: 'esc', sequence: '\x1b', title: 'Escape' },
    { label: 'tab', sequence: '\t', title: 'Tab' },
    { label: '↑', sequence: '\x1b[A', title: 'Up arrow' },
    { label: '↓', sequence: '\x1b[B', title: 'Down arrow' },
    { label: '↵', sequence: '\n', title: 'New line' },
  ]

  return (
    <div class="mobile-bottom-bar" aria-label="Mobile terminal controls">
      <button class="mobile-bottom-action mobile-bottom-menu" onClick={onMenu} title="Open sessions">
        <span class="mobile-bottom-icon">☰</span>
        <span>menu</span>
      </button>
      <div class="mobile-terminal-actions" role="toolbar" aria-label="Terminal keys">
        <button
          class={`mobile-bottom-action mobile-bottom-modifier ${ctrlArmed ? 'armed' : ''}`}
          disabled={!canSend}
          onClick={onToggleCtrl}
          title={ctrlArmed ? 'Ctrl armed for next typed key' : 'Arm Ctrl for next typed key'}
          aria-pressed={ctrlArmed}
        >
          ctrl
        </button>
        {actions.map((action) => (
          <button
            key={action.label}
            class={`mobile-bottom-action ${action.label === '↵' ? 'mobile-bottom-enter' : ''}`}
            disabled={!canSend}
            onClick={() => onSend(action.sequence)}
            title={action.title}
          >
            {action.label}
          </button>
        ))}
      </div>
    </div>
  )
}

// ── App ──

type ConnectionState = 'connecting' | 'connected' | 'error'

const sidebarState = createSidebarState()

function App() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [connState, setConnState] = useState<ConnectionState>('connecting')
  const [ctrlArmed, setCtrlArmed] = useState(false)
  const [launchers, setLaunchers] = useState<LauncherDef[]>([])
  const [, forceUpdate] = useState(0) // re-render on sidebar state change
  const terminalInputRef = useRef<((data: string) => void) | null>(null)
  const dismissedIds = useRef(new Set<string>())

  useEffect(() => { fetchConfig().then(cfg => setLaunchers(cfg.launchers)) }, [])

  // Subscribe to sidebar state changes for re-render
  useEffect(() => sidebarState.subscribe(() => forceUpdate(n => n + 1)), [])

  // Sync sidebar visibility whenever sessions change
  useEffect(() => { sidebarState.syncSessions(sessions) }, [sessions])

  // Load data
  useEffect(() => {
    if (USE_MOCK) {
      const mockFolders = getMockFolders()
      const allSessions = mockFolders.flatMap(f => f.sessions)
      setSessions(allSessions)
      setConnState('connected')
      // Auto-select: attention > active > any alive
      const attention = allSessions.find(s => s.alive && s.status?.state === 'attention')
      const active = allSessions.find(s => s.alive && s.status?.state === 'active')
      const first = attention ?? active ?? allSessions.find(s => s.alive)
      if (first) setSelectedId(first.id)
    } else {
      // Fetch initial session list
      fetchSessions().then(list => {
        setSessions(list)
        setConnState('connected')
        // Auto-select first alive session
        const attention = list.find(s => s.alive && s.status?.state === 'attention')
        const active = list.find(s => s.alive && s.status?.state === 'active')
        const first = attention ?? active ?? list.find(s => s.alive)
        if (first && !selectedId) setSelectedId(first.id)
      }).catch(err => {
        console.error('Failed to fetch sessions:', err)
        setConnState('error')
      })

      // Subscribe to SSE for live updates
      const source = new EventSource('/api/events')
      source.addEventListener('session-upsert', (e) => {
        try {
          const envelope = JSON.parse(e.data)
          const session = envelope.session ?? envelope
          const updated = toUISession(session)
          // Ignore updates for sessions the user has dismissed
          if (dismissedIds.current.has(updated.id)) return
          let isNew = false
          setSessions(prev => {
            const idx = prev.findIndex(s => s.id === updated.id)
            if (idx >= 0) {
              const next = [...prev]
              next[idx] = updated
              return next
            }
            isNew = true
            return [...prev, updated]
          })
          // Auto-select newly launched sessions
          if (isNew && _pendingLaunchAt && Date.now() - _pendingLaunchAt < 10_000) {
            _pendingLaunchAt = 0
            setSelectedId(updated.id)
          }
        } catch (err) {
          console.warn('session-upsert: bad event', err)
        }
      })
      source.addEventListener('session-remove', (e) => {
        try {
          const { id } = JSON.parse(e.data)
          setSessions(prev => prev.filter(s => s.id !== id))
        } catch (err) {
          console.warn('session-remove: bad event', err)
        }
      })
      return () => source.close()
    }
  }, [])

  // URL param filtering: ?project=name or ?cwd=/path
  const filteredSessions = useMemo(() => {
    const params = new URLSearchParams(location.search)
    const project = params.get('project')
    const cwdFilter = params.get('cwd')
    if (!project && !cwdFilter) return sessions
    return sessions.filter(s => {
      if (project && !s.cwd.toLowerCase().includes(project.toLowerCase())) return false
      if (cwdFilter && !s.cwd.startsWith(cwdFilter)) return false
      return true
    })
  }, [sessions])

  const allFolders = useMemo(() => groupByFolder(filteredSessions), [filteredSessions])
  const folders = useMemo(
    () => allFolders.filter(f => sidebarState.isFolderVisible(f.path)),
    [allFolders],
  )
  const hiddenFolders = useMemo(
    () => allFolders.filter(f => !sidebarState.isFolderVisible(f.path)),
    [allFolders],
  )
  const selected = useMemo(
    () => sessions.find(s => s.id === selectedId) ?? null,
    [sessions, selectedId],
  )

  // Auto-select only when nothing is selected (initial load).
  // When a selected session dies, we let the user decide — don't override their click.
  const hasAutoSelected = useRef(false)
  useEffect(() => {
    if (!selectedId && !hasAutoSelected.current && filteredSessions.length > 0) {
      hasAutoSelected.current = true
      const best =
        filteredSessions.find(s => s.alive && s.status?.state === 'attention') ??
        filteredSessions.find(s => s.alive && s.status?.state === 'active') ??
        filteredSessions.find(s => s.alive) ??
        filteredSessions[0]
      if (best) setSelectedId(best.id)
    }
  }, [filteredSessions, selectedId])

  const handleCloseSession = useCallback((session: Session) => {
    if (session.close_action === 'minimize') {
      // Kill but keep — will become resumable via SSE
      killSession(session.id)
    } else {
      // Dismiss: remove from sidebar state + sessions list immediately
      const cwd = session.cwd ?? '~'
      const key = session.resume_key ?? session.id
      sidebarState.dismissSession(cwd, key)
      dismissedIds.current.add(session.id)
      setSessions(prev => prev.filter(s => s.id !== session.id))
      if (session.alive) killSession(session.id)
      if (selectedId === session.id) setSelectedId(null)
    }
  }, [selectedId])

  const handleHideFolder = useCallback((cwd: string) => {
    sidebarState.hideFolder(cwd)
  }, [])

  const handleSelect = useCallback((id: string) => {
    setSelectedId(id)
    setCtrlArmed(false)
    // If resumable, resume it. The session will transition in-place
    // (alive: true, socket_path set) via SSE upsert, then the terminal opens.
    const sess = sessions.find(s => s.id === id)
    if (sess?.resumable) {
      resumeSession(id).catch(err => console.error('resume failed:', err))
    }
  }, [sessions])

  const canAttach = !!selected?.alive && !USE_MOCK

  useEffect(() => {
    if (!canAttach) setCtrlArmed(false)
  }, [canAttach])

  const handleTerminalInputReady = useCallback((send: ((data: string) => void) | null) => {
    terminalInputRef.current = send
  }, [])

  const handleMobileInput = useCallback((data: string) => {
    terminalInputRef.current?.(data)
  }, [])

  const handleToggleCtrl = useCallback(() => {
    if (!canAttach) return
    setCtrlArmed((armed) => !armed)
  }, [canAttach])

  const handleCtrlConsumed = useCallback(() => {
    setCtrlArmed(false)
  }, [])

  return (
    <div class="app-layout">
      <Sidebar
        folders={folders}
        hiddenFolders={hiddenFolders}
        selectedId={selectedId}
        onSelect={handleSelect}
        onCloseSession={handleCloseSession}
        onHideFolder={handleHideFolder}
        onShowFolder={(cwd) => sidebarState.showFolder(cwd)}
        isSessionVisible={(s) => sidebarState.isSessionVisible(s)}
        open={sidebarOpen}
        onClose={() => setSidebarOpen(false)}
      />

      <div class="main-panel">
        <MainHeader session={selected} onKill={killSession} />

        {connState === 'connecting' ? (
          <div class="state-message">
            <div class="state-icon">⋯</div>
            <div class="state-title">Connecting</div>
            <div class="state-subtitle">Reaching gmuxd…</div>
          </div>
        ) : connState === 'error' ? (
          <div class="state-message">
            <div class="state-icon" style={{ color: 'var(--status-error)' }}>⚠</div>
            <div class="state-title">Connection failed</div>
            <div class="state-subtitle">Could not reach gmuxd. Is it running?</div>
            <button class="btn btn-primary" style={{ marginTop: 12 }} onClick={() => location.reload()}>
              Retry
            </button>
          </div>
        ) : selected && canAttach ? (
          <TerminalView
            sessionId={selected.id}
            ctrlArmed={ctrlArmed}
            onCtrlConsumed={handleCtrlConsumed}
          />
        ) : (
          <EmptyState launchers={launchers} />
        )}

        <MobileTerminalBar
          canSend={canAttach}
          ctrlArmed={ctrlArmed}
          onMenu={() => setSidebarOpen(true)}
          onSend={handleMobileInput}
          onToggleCtrl={handleToggleCtrl}
        />
      </div>
    </div>
  )
}

render(<App />, document.getElementById('app')!)
