import type { Mode, PatternShape, Player } from './types/protocol'

export function rankText(rank: number) {
  return ({ 11: 'J', 12: 'Q', 13: 'K', 14: 'A', 15: '2' })[rank] ?? String(rank)
}

export function cardFace(id: number) {
  if (id >= 52) return { rank: id === 52 ? '小王' : '大王', suit: '✦', red: id === 53, joker: true }
  return { rank: rankText(Math.floor(id / 4) + 3), suit: ['♣', '♦', '♥', '♠'][id % 4], red: id % 4 === 1 || id % 4 === 2, joker: false }
}

export function cardLabel(id: number) {
  const face = cardFace(id)
  return face.joker ? face.rank : face.suit + face.rank
}

export function shapeName(p: PatternShape) {
  return { single: '单张', pair: '对子', straight: '顺子', pair_run: '连对', bomb: p.length + ' 张炸弹' }[p.type]
}

export function shapeRanks(p: PatternShape) {
  if (p.type === 'straight') return Array.from({ length: p.length }, (_, i) => rankText(p.rank + i))
  if (p.type === 'pair_run') return Array.from({ length: p.length }, (_, i) => rankText(p.rank + Math.floor(i / 2)))
  return Array.from({ length: p.length }, () => rankText(p.rank))
}

export const modeName = (mode: Mode) => mode === 'classic' ? '经典' : '极速'
export const signed = (n: number) => n > 0 ? '+' + n : String(n)

// Starting at the local seat, increasing backend seat numbers go counterclockwise.
export const tablePositions = {
  2: ['top-center'],
  3: ['top-right', 'top-left'],
  4: ['right', 'top-center', 'left'],
  5: ['right', 'top-right', 'top-left', 'left'],
} as const

export function opponents(players: Player[], selfId: string, capacity: number) {
  const self = players.find(player => player.playerId === selfId)
  if (!self) return []
  const offset = (seat: number) => (seat - self.seat + capacity) % capacity
  return players.filter(player => player.playerId !== selfId).sort((a, b) => offset(a.seat) - offset(b.seat))
}
