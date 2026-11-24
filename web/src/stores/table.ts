import { computed, ref, shallowRef } from 'vue'
import { defineStore } from 'pinia'
import { CommandError, Wire, type Connection } from '../network/ws'
import type { CreateRoom, Credentials, Envelope, GameState, Notice, Pattern, RoomState, TableEvent } from '../types/protocol'

const storageKey = 'gandengyan.session.v1'
const noticeKey = 'gandengyan.notices.v1'

function read<T>(key: string, fallback: T): T {
  try { return JSON.parse(sessionStorage.getItem(key) ?? 'null') ?? fallback } catch { return fallback }
}

function write(key: string, value: unknown) {
  try { sessionStorage.setItem(key, JSON.stringify(value)) } catch { /* Storage can be disabled. */ }
}

export const useTable = defineStore('table', () => {
  const connection = ref<Connection>('idle')
  const credentials = shallowRef<Credentials>()
  const room = shallowRef<RoomState>()
  const game = shallowRef<GameState>()
  const gameId = ref<string>()
  const revision = ref(-1)
  const busy = ref(false)
  const awaited = ref<number>()
  const globalMessage = ref('')
  const notice = shallowRef<Notice>()
  const shownNotices = new Set(read<string[]>(noticeKey, []))
  const motion = shallowRef<{ key: number; events: TableEvent[]; drawn: number[] }>()
  const publicPlay = shallowRef<{ playerId: string; pattern: Pattern; key: number }>()
  const passed = shallowRef<string[]>([])
  const available = computed(() => connection.value === 'online' && !busy.value && awaited.value === undefined)

  function receive(envelope: Envelope, recovering: boolean) {
    if (envelope.type !== 'room_state' && envelope.type !== 'game_state') return
    if (envelope.revision < revision.value) return
    const previous = game.value
    const isNew = envelope.gameId !== gameId.value
    const changed = envelope.revision > revision.value
    revision.value = envelope.revision
    gameId.value = envelope.gameId
    if (awaited.value !== undefined && envelope.revision >= awaited.value) awaited.value = undefined
    room.value = envelope.payload as RoomState
    const next = envelope.type === 'game_state' ? envelope.payload as GameState : undefined
    game.value = next
    if (!next || isNew) notice.value = undefined
    if (!next || isNew || recovering) {
      motion.value = undefined
      passed.value = []
      publicPlay.value = next?.lastPattern
        ? { playerId: next.lastPlayPlayerId, pattern: next.lastPattern, key: envelope.revision }
        : undefined
    }
    if (next && changed && (!recovering || next.events?.some(event => event.type === 'game_started'))) {
      const events = next.events ?? []
      if (events.length) {
        const drawsSelf = events.some(event => event.type === 'cards_drawn' && event.playerId === next.selfPlayerId)
        const oldHand = previous?.selfHand ?? []
        motion.value = { key: envelope.revision, events, drawn: drawsSelf ? next.selfHand.filter(id => !oldHand.includes(id)) : [] }
        for (const event of events) {
          if (event.type === 'cards_played' && event.pattern && event.playerId) {
            publicPlay.value = { playerId: event.playerId, pattern: event.pattern, key: envelope.revision }
            passed.value = []
          }
          if (event.type === 'player_passed' && event.playerId) passed.value = [...passed.value, event.playerId]
        }
        if (next.phase === 'leading') {
          passed.value = []
          publicPlay.value = undefined
        }
      }
    }
    for (const pending of next?.privateNotices ?? []) {
      if (shownNotices.has(pending.noticeId)) continue
      shownNotices.add(pending.noticeId)
      if (shownNotices.size > 128) shownNotices.delete(shownNotices.values().next().value!)
      write(noticeKey, [...shownNotices])
      notice.value = pending
    }
  }

  function reset(code: number) {
    credentials.value = undefined
    room.value = undefined
    game.value = undefined
    gameId.value = undefined
    revision.value = -1
    awaited.value = undefined
    motion.value = undefined
    publicPlay.value = undefined
    passed.value = []
    notice.value = undefined
    write(storageKey, null)
    const messages: Record<number, string> = {
      4001: '此席位已在另一个页面打开。',
      4003: '房间或会话已失效，无法恢复此席位。',
      4006: '游戏版本已更新，请刷新页面。',
    }
    globalMessage.value = messages[code] ?? ''
  }

  const wire = new Wire(state => {
    connection.value = state
    if (state !== 'online') awaited.value = undefined
  }, receive, reset)

  function restore() {
    const saved = read<Credentials | null>(storageKey, null)
    if (saved?.protocolVersion === 1 && typeof saved.sessionToken === 'string' && /^\d{6}$/.test(saved.roomId)) {
      credentials.value = saved
      wire.open(saved)
    }
  }

  async function enter(kind: 'create' | 'join' | 'quick', payload: CreateRoom | { nickname: string; roomId: string }) {
    const path = kind === 'create' ? '/api/rooms' : '/api/rooms/' + kind
    const response = await fetch(path, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
      signal: AbortSignal.timeout(8_000),
    })
    const data = await response.json()
    if (!response.ok) throw new CommandError(data.code, data.message || '房间操作失败，请重试。')
    const next = data as Credentials
    if (next.protocolVersion !== 1) throw new CommandError('UNSUPPORTED_VERSION', '游戏已更新，请刷新页面。')
    globalMessage.value = ''
    write(storageKey, next)
    credentials.value = next
    wire.open(next)
  }

  function back() {
    wire.close()
    reset(0)
  }

  async function command(type: string, payload: unknown = {}) {
    if (!available.value) throw new CommandError('OFFLINE', '牌桌正在同步。')
    busy.value = true
    try {
      const reply = await wire.send(type, payload, revision.value, gameId.value)
      if (reply.revision > revision.value) awaited.value = reply.revision
    } catch (err) {
      if (type !== 'leave_room' || credentials.value) throw err
    } finally { busy.value = false }
  }

  async function query<T>(type: string, payload: unknown = {}) {
    return await wire.send(type, payload, revision.value, gameId.value) as Envelope<T>
  }

  return { connection, credentials, room, game, gameId, revision, available, globalMessage, notice, motion, publicPlay, passed, restore, enter, back, command, query, nudge: () => wire.nudge() }
})
