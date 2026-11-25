<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BoardTable from '../components/BoardTable.vue'
import CardFace from '../components/CardFace.vue'
import HandRail from '../components/HandRail.vue'
import Modal from '../components/Modal.vue'
import PlayerSeat from '../components/PlayerSeat.vue'
import { useTable } from '../stores/table'
import { usePlay } from '../composables/usePlay'
import { opponents, shapeName, shapeRanks, signed, tablePositions } from '../format'
import type { Pattern } from '../types/protocol'

const table = useTable()
const { selected, options, chooser, error, turn, canPass, canHint, canPlay, canSelect, toggle, changeRange, clear, action, play, hint } = usePlay()
const game = computed(() => table.game!)
const self = computed(() => game.value.players.find(p => p.playerId === game.value.selfPlayerId)!)
const active = computed(() => game.value.players.find(p => p.playerId === game.value.currentPlayerId))
const enemySeats = computed(() => opponents(game.value.players, game.value.selfPlayerId, game.value.capacity))
const positions = computed(() => tablePositions[game.value.capacity as keyof typeof tablePositions])
const winner = computed(() => game.value.players.find(player => player.playerId === game.value.winnerId))
const finished = computed(() => game.value.phase === 'finished')
const settlement = ref(finished.value)
const roundEntered = ref(false)
watch(finished, value => { settlement.value = value })
watch(() => game.value.roundNumber, (round, previous) => {
  roundEntered.value = round > previous && table.motion?.key === table.revision
})
const drawings = computed(() => table.motion?.events.filter(e => e.type === 'cards_drawn') ?? [])
const inDeal = computed(() => !!table.motion?.events.some(e => e.type === 'game_started'))
const played = computed(() => table.publicPlay)
const playedBy = computed(() => game.value.players.find(p => p.playerId === played.value?.playerId)?.nickname)
const playOrigin = computed(() => played.value?.playerId === self.value.playerId ? 'self'
  : positions.value[enemySeats.value.findIndex(player => player.playerId === played.value?.playerId)])
const animatePlay = computed(() => played.value?.key === table.motion?.key
  && !!table.motion?.events.some(event => event.type === 'cards_played'))
const status = computed(() => {
  if (error.value) return error.value
  if (table.connection !== 'online') return '正在同步牌桌…'
  if (finished.value) return (winner.value?.nickname ?? '') + ' 获胜'
  if (!turn.value) return active.value?.online ? '等待 ' + (active.value?.nickname ?? '') + ' 出牌' : '等待 ' + (active.value?.nickname ?? '') + ' 重连'
  if (game.value.phase === 'leading') return '轮到你领牌'
  return game.value.canPlay ? '轮到你接牌' : '无牌可接，可以不出'
})
const settlementRows = computed(() => (game.value.settlement ?? []).map(result => ({
  ...result, player: game.value.players.find(p => p.playerId === result.playerId)!,
})))

function orderedCards(pattern: Pattern, cardIds: number[] = pattern.cardIds) {
  return cardIds.map(id => ({ id, wildRank: pattern.wildcards?.find(wild => wild.cardId === id)?.rank }))
    .sort((a, b) => (a.wildRank ?? Math.floor(a.id / 4) + 3) - (b.wildRank ?? Math.floor(b.id / 4) + 3))
}
</script>

<template>
  <main class="game-main">
    <BoardTable class="game-table" data-layout="shared" aria-label="干瞪眼牌桌">
      <div class="table-grid">
        <div v-for="(player, index) in enemySeats" :key="player.playerId" class="table-seat opponent"
          :data-position="positions[index]">
          <PlayerSeat :player="player" :active="game.currentPlayerId === player.playerId" :passed="table.passed.includes(player.playerId)"
            :drawn="drawings.some(event => event.playerId === player.playerId)" />
        </div>
        <div class="stock-block" :class="{ 'stock-empty': !game.deckCount }">
          <div class="stock-icon" aria-hidden="true"><span>♠</span></div>
          <small>牌堆</small><strong data-testid="deck-count">{{ game.deckCount }} <span>张</span></strong>
        </div>
        <div class="board-play">
          <div v-if="played" :key="played.key" class="public-play" :data-origin="playOrigin"
            :class="{ 'play-enter': animatePlay, blast: animatePlay && played.pattern.type === 'bomb' }"
            :data-bomb-size="played.pattern.type === 'bomb' ? played.pattern.length : undefined">
            <div class="played-cards" :aria-label="shapeName(played.pattern)" tabindex="0">
              <CardFace v-for="card in orderedCards(played.pattern)" :key="card.id" :id="card.id" :wild-rank="card.wildRank" />
            </div>
            <span class="pattern-caption" :title="playedBy">{{ shapeName(played.pattern) }} {{ shapeRanks(played.pattern).join(' ') }}</span>
          </div>
          <div v-else-if="finished" class="lead-label">本局已结束</div>
          <div v-else class="lead-label">{{ active?.nickname }} 领牌</div>
          <div v-if="drawings.length" :key="table.motion?.key" class="draw-caption" data-testid="draw-event">{{ game.mode === 'classic' && drawings.length === game.capacity ? '全员各摸 1 张' : drawings.map(event => game.players.find(p => p.playerId === event.playerId)?.nickname).join('、') + ' 各摸 1 张' }}</div>
        </div>
        <div class="round-wrap"><div :key="game.roundNumber" class="round-label" :class="{ 'round-enter': roundEntered }">第 {{ game.roundNumber }} 轮</div></div>
        <div class="action-zone">
          <p class="turn-copy" :class="{ 'my-turn': turn, 'play-error': error }" role="status" data-testid="turn-status">{{ status }}</p>
          <div class="play-actions">
            <template v-if="!finished">
              <button class="button secondary" type="button" :disabled="!canHint" @click="hint">提示</button>
              <button class="button secondary" :class="{ 'pass-emphasis': canPass && table.available && !game.canPlay }" type="button" :disabled="!canPass || !table.available" @click="action('pass')">不出</button>
              <button class="button gold" type="button" :disabled="!canPlay" @click="play()">出牌<span v-if="selected.length"> · {{ selected.length }}</span></button>
            </template>
            <button v-else class="button gold" type="button" @click="settlement = true">查看结算</button>
          </div>
        </div>
        <div class="table-seat own-seat" data-position="self">
          <PlayerSeat :player="self" :active="turn" :drawn="!!table.motion?.drawn.length" self :host="game.hostPlayerId === self.playerId" />
        </div>
        <div class="hand-zone">
          <button v-if="table.notice" class="private-notice" type="button" :title="table.notice.message" @click="table.notice = undefined">双王需搭配普通牌 <span aria-hidden="true">×</span></button>
          <HandRail :cards="game.selfHand" :selected="selected" :disabled="!canSelect" :drawn="table.motion?.drawn ?? []"
            :dealing="inDeal" @toggle="toggle" @change-range="changeRange" @clear="clear" />
        </div>
      </div>
    </BoardTable>

    <Modal v-if="chooser" title="万能牌怎么用？" @close="chooser = false">
      <div class="wild-options">
        <button v-for="option in options" :key="[option.type, option.rank, option.length].join('-')" type="button" @click="play(option)">
          <strong>{{ shapeName(option) }} {{ shapeRanks(option).join(' ') }}</strong>
          <span class="wild-hand">
            <CardFace v-for="card in orderedCards(option, selected)" :key="card.id" :id="card.id" :wild-rank="card.wildRank" />
          </span>
        </button>
      </div>
    </Modal>
    <Modal v-if="settlement && finished" title="本局结算" wide @close="settlement = false">
      <p class="winner" :class="{ 'winner-enter': table.motion?.events.some(event => event.type === 'game_finished') }"><span aria-hidden="true">✦</span> {{ winner?.nickname }} 获胜</p>
      <table class="score-table">
        <thead><tr><th>牌友</th><th>余牌</th><th>本局</th><th>累计</th></tr></thead>
        <tbody><tr v-for="row in settlementRows" :key="row.playerId">
          <td>{{ row.player.nickname }}{{ row.playerId === game.selfPlayerId ? '（你）' : '' }}</td>
          <td>{{ row.handCount }}</td><td :class="{ positive: row.delta > 0 }">{{ signed(row.delta) }}</td><td>{{ signed(row.player.score) }}</td>
        </tr></tbody>
      </table>
      <p v-if="error" class="form-error" role="alert">{{ error }}</p>
      <div class="settlement-actions">
        <button class="button secondary" type="button" :disabled="!table.available" @click="action('leave_room')">退出房间</button>
        <button v-if="game.allowedActions.includes('restart_game')" class="button primary" type="button" :disabled="!table.available" @click="action('restart_game')">再来一局 <span aria-hidden="true">→</span></button>
        <span v-else>{{ game.hostPlayerId === game.selfPlayerId ? '等待牌友重连' : '等待房主再开一局' }}</span>
      </div>
    </Modal>
  </main>
</template>
