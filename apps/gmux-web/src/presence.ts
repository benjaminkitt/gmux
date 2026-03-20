// Presence WebSocket — reports client state to gmuxd and receives notification
// commands. The daemon uses this to decide whether, when, and where to show
// OS notifications.

export interface NotifyMessage {
  type: 'notify'
  id: string
  session_id: string
  title: string
  body: string
  tag: string
}

export interface CancelMessage {
  type: 'cancel'
  id: string
}

export interface ClientState {
  visibility: string
  focused: boolean
  selected_session_id: string | null
  last_interaction: number // Unix seconds
}

export interface PresenceConnection {
  sendState(state: ClientState): void
  sendPermission(permission: string): void
  close(): void
}

/**
 * Connect to the presence WebSocket. Automatically sends a client-hello on
 * open and routes incoming notify/cancel messages to the provided callbacks.
 *
 * Reconnects automatically on disconnect with exponential backoff.
 */
export function connectPresence(options: {
  onNotify: (msg: NotifyMessage) => void
  onCancel: (msg: CancelMessage) => void
}): PresenceConnection {
  let ws: WebSocket | null = null
  let closed = false
  let backoff = 1000

  const deviceType = matchMedia('(pointer: coarse)').matches ? 'mobile' : 'desktop'

  // Queue state updates until the socket is ready.
  let pendingState: ClientState | null = null

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    ws = new WebSocket(`${proto}//${location.host}/v1/presence`)

    ws.onopen = () => {
      backoff = 1000
      // Read permission fresh on each connect — it may have changed since
      // the previous connection (e.g. user granted permission, then WS reconnected).
      const perm = 'Notification' in window ? Notification.permission : 'unavailable'
      ws!.send(JSON.stringify({
        type: 'client-hello',
        device_type: deviceType,
        notification_permission: perm,
      }))
      // Flush any pending state
      if (pendingState) {
        ws!.send(JSON.stringify({ type: 'client-state', ...pendingState }))
        pendingState = null
      }
    }

    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        if (msg.type === 'notify') options.onNotify(msg)
        if (msg.type === 'cancel') options.onCancel(msg)
      } catch { /* ignore malformed messages */ }
    }

    ws.onclose = () => {
      if (closed) return
      setTimeout(() => {
        if (!closed) connect()
      }, Math.min(backoff, 30000))
      backoff *= 2
    }

    ws.onerror = () => {
      // onclose will fire after this, triggering reconnect
    }
  }

  connect()

  return {
    sendState(state: ClientState) {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'client-state', ...state }))
      } else {
        pendingState = state
      }
    },
    sendPermission(permission: string) {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'notif-permission', permission }))
      }
    },
    close() {
      closed = true
      ws?.close()
    },
  }
}
