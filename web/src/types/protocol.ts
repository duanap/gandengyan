export type Mode = 'classic' | 'speed'
export type Phase = 'waiting' | 'leading' | 'following' | 'finished'
export type PatternKind = 'single' | 'pair' | 'straight' | 'pair_run' | 'bomb'

export interface Credentials {
  roomId: string
  playerId: string
  sessionToken: string
  protocolVersion: number
}
export interface CreateRoom { nickname: string; capacity: number; mode: Mode }
export interface PatternShape { type: PatternKind; rank: number; length: number }
export interface Wildcard { cardId: number; rank: number }
export interface Pattern extends PatternShape {
  cardIds: number[]
  wildcards?: Wildcard[]
}
export interface Problem { code: string; message: string }
export interface Notice { noticeId: string; code: 'DOUBLE_JOKER'; message: string }
export interface Preview { cardIds: number[]; options: Pattern[]; problem?: Problem }
export interface Player {
  playerId: string
  seat: number
  nickname: string
  online: boolean
  bot: boolean
  score: number
  handCount: number
}
export interface RoomState {
  roomId: string
  mode: Mode
  capacity: number
  phase: Phase
  hostPlayerId: string
  selfPlayerId: string
  players: Player[]
  allowedActions: string[]
}
export interface TableEvent {
  type: 'game_started' | 'cards_played' | 'player_passed' | 'cards_drawn' | 'game_finished'
  eventIndex: number
  playerId?: string
  count?: number
  pattern?: Pattern
}
export interface GameState extends RoomState {
  currentPlayerId: string
  leaderPlayerId: string
  lastPlayPlayerId: string
  roundNumber: number
  passCount: number
  deckCount: number
  selfHand: number[]
  lastPattern: Pattern | null
  canPlay: boolean
  privateNotices: Notice[]
  winnerId?: string
  settlement?: { playerId: string; handCount: number; delta: number }[]
  events?: TableEvent[]
}
export interface Envelope<T = unknown> {
  v: number
  type: string
  requestId?: string
  gameId?: string
  revision: number
  payload: T
}
