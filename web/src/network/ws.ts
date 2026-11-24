import type { Credentials, Envelope, Problem } from '../types/protocol'

export type Connection = 'connecting' | 'online' | 'reconnecting' | 'idle'
export class CommandError extends Error {
  constructor(public readonly code: string, message: string) { super(message) }
}

type Pending = {
  resolve: (message: Envelope) => void
  reject: (error: Error) => void
  timeout: number
}

function requestId() {
  return Array.from(crypto.getRandomValues(new Uint32Array(4)), part => part.toString(36)).join('-')
}

export class Wire {
  private socket?: WebSocket
  private credentials?: Credentials
  private pending = new Map<string, Pending>()
  private retry = 0
  private reconnect?: number
  private monitor?: number
  private handshake?: number
  private lastMessage = 0
  private synced = false

  constructor(
    private readonly onState: (state: Connection) => void,
    private readonly onMessage: (message: Envelope, recovering: boolean) => void,
    private readonly onEnd: (code: number) => void,
  ) {}

  open(credentials: Credentials) {
    this.close()
    this.credentials = credentials
    this.retry = 0
    this.connect()
  }

  close() {
    clearTimeout(this.reconnect)
    this.credentials = undefined
    const socket = this.socket
    this.socket = undefined
    this.synced = false
    socket?.close()
    this.clearPending()
    this.onState('idle')
  }

  nudge() {
    if (!this.credentials) return
    if (this.synced && this.socket?.readyState === WebSocket.OPEN) {
      if (Date.now() - this.lastMessage > 35_000) this.socket.close()
      else this.ping(this.socket)
    } else if (!this.socket) {
      clearTimeout(this.reconnect)
      this.connect()
    }
  }

  send(type: string, payload: unknown, revision: number, gameId?: string): Promise<Envelope> {
    const socket = this.socket
    if (!this.synced || socket?.readyState !== WebSocket.OPEN) {
      return Promise.reject(new CommandError('OFFLINE', '连接中，操作尚未发送。'))
    }
    const id = requestId()
    return new Promise((resolve, reject) => {
      const timeout = window.setTimeout(() => {
        this.pending.delete(id)
        reject(new CommandError('TIMEOUT', '响应超时，正在同步牌桌。'))
        // Reconnect to learn the authoritative result. Never resubmit a move.
        if (this.socket === socket) socket.close()
      }, 5000)
      this.pending.set(id, { resolve, reject, timeout })
      try {
        socket.send(JSON.stringify({ v: 1, type, requestId: id, payload, revision, gameId }))
      } catch {
        clearTimeout(timeout)
        this.pending.delete(id)
        reject(new CommandError('OFFLINE', '连接已断开，正在同步牌桌。'))
        socket.close()
      }
    })
  }

  private connect() {
    const credentials = this.credentials
    if (!credentials || this.socket) return
    this.onState(this.retry === 0 ? 'connecting' : 'reconnecting')
    const url = new URL('/ws', location.href)
    url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const socket = new WebSocket(url)
    this.socket = socket
    this.synced = false
    this.handshake = window.setTimeout(() => {
      if (this.socket === socket && !this.synced) socket.close()
    }, 7500)

    socket.onopen = () => {
      if (this.socket !== socket) return
      socket.send(JSON.stringify({
        v: 1, type: 'auth', requestId: requestId(),
        payload: { roomId: credentials.roomId, sessionToken: credentials.sessionToken },
      }))
      this.lastMessage = Date.now()
      this.monitor = window.setInterval(() => {
        if (this.socket !== socket) return
        if (Date.now() - this.lastMessage > 45_000) socket.close()
        else this.ping(socket)
      }, 20_000)
    }
    socket.onmessage = event => {
      if (this.socket !== socket) return
      let message: Envelope
      try { message = JSON.parse(event.data) as Envelope } catch { socket.close(); return }
      if (message.v !== 1) { socket.close(); return }
      this.lastMessage = Date.now()
      const snapshot = message.type === 'room_state' || message.type === 'game_state'
      this.onMessage(message, !this.synced)
      if (snapshot && !this.synced) {
        this.synced = true
        this.retry = 0
        clearTimeout(this.handshake)
        this.onState('online')
      }
      const pending = message.requestId && this.pending.get(message.requestId)
      if (!pending) return
      clearTimeout(pending.timeout)
      this.pending.delete(message.requestId!)
      if (message.type === 'action_rejected') {
        const problem = message.payload as Problem
        pending.reject(new CommandError(problem.code, problem.message))
      } else pending.resolve(message)
    }
    socket.onclose = event => {
      if (this.socket !== socket) return
      this.socket = undefined
      this.synced = false
      this.clearPending()
      if ([4001, 4003, 4004, 4006].includes(event.code)) {
        this.close()
        this.onEnd(event.code)
        return
      }
      this.onState('reconnecting')
      const delay = Math.min(10_000, 500 * 2 ** Math.min(this.retry++, 5))
      this.reconnect = window.setTimeout(() => this.connect(), delay * (0.8 + Math.random() * 0.4))
    }
  }

  private ping(socket: WebSocket) {
    socket.send(JSON.stringify({ v: 1, type: 'ping', requestId: requestId(), payload: {} }))
  }

  private clearPending() {
    clearInterval(this.monitor)
    clearTimeout(this.handshake)
    for (const pending of this.pending.values()) {
      clearTimeout(pending.timeout)
      pending.reject(new CommandError('OFFLINE', '正在重连，恢复后请确认牌桌状态。'))
    }
    this.pending.clear()
  }
}
