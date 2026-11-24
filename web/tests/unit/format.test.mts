import assert from 'node:assert/strict'
import test from 'node:test'
import { cardFace, opponents, shapeRanks } from '../../src/format.ts'

test('牌面与协议物理编号一致', () => {
  assert.deepEqual(cardFace(0), { rank: '3', suit: '♣', red: false, joker: false })
  assert.equal(cardFace(47).rank, 'A')
  assert.equal(cardFace(49).suit, '♦')
  assert.equal(cardFace(51).rank, '2')
  assert.equal(cardFace(52).rank, '小王')
  assert.equal(cardFace(53).rank, '大王')
})

test('按逻辑点数展示顺子和连对', () => {
  assert.deepEqual(shapeRanks({ type: 'straight', rank: 11, length: 4 }), ['J', 'Q', 'K', 'A'])
  assert.deepEqual(shapeRanks({ type: 'pair_run', rank: 13, length: 4 }), ['K', 'K', 'A', 'A'])
})

test('从自己的下一席开始，按逆时针排列', () => {
  const players = Array.from({ length: 5 }, (_, seat) => ({
    playerId: 'p' + seat, seat, nickname: '玩家', online: true, bot: false, score: 0, handCount: 5,
  }))
  assert.deepEqual(opponents(players, 'p3', 5).map(player => player.seat), [4, 0, 1, 2])
})
