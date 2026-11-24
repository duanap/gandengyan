<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import CardFace from '../components/CardFace.vue'
import Modal from '../components/Modal.vue'
import DesktopTable from '../layouts/desktop/DesktopTable.vue'
import MobileTable from '../layouts/mobile/MobileTable.vue'
import { useTable } from '../stores/table'
import { usePlay } from '../composables/usePlay'
import { cardLabel, opponents, rankText, shapeName, shapeRanks, signed } from '../format'

const table = useTable()
const { selected, options, chooser, error, turn, canPass, canHint, canPlay, toggle, action, play, hint } = usePlay()
const game = computed(() => table.game!)
const self = computed(() => game.value.players.find(p => p.playerId === game.value.selfPlayerId)!)
const active = computed(() => game.value.players.find(p => p.playerId === game.value.currentPlayerId))
const enemySeats = computed(() => opponents(game.value.players, game.value.selfPlayerId, game.value.capacity))
const winner = computed(() => game.value.players.find(player => player.playerId === game.value.winnerId))
const finished = computed(() => game.value.phase === 'finished')
const settlement = ref(finished.value)
const mobileQuery = window.matchMedia('(max-width: 760px), (max-width: 1080px) and (max-height: 540px), (pointer: coarse) and (max-height: 640px)')
const mobile = ref(mobileQuery.matches)
const changeMedia = (event: MediaQueryListEvent) => { mobile.value = event.matches }
onMounted(() => mobileQuery.addEventListener('change', changeMedia))
onUnmounted(() => mobileQuery.removeEventListener('change', changeMedia))
watch(finished, value => { settlement.value = value })
const drawings = computed(() => table.motion?.events.filter(e => e.type === 'cards_drawn') ?? [])
const inDeal = computed(() => !!table.motion?.events.some(e => e.type === 'game_started'))
const played = computed(() => table.publicPlay)
const playedBy = computed(() => game.value.players.find(p => p.playerId === played.value?.playerId)?.nickname)
const tightHand = computed(() => game.value.selfHand.length > (mobile.value ? 8 : 11))
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
</script>

<template>
  <main class="game-main">
    <component :is="mobile ? MobileTable : DesktopTable" :players="enemySeats" :active="game.currentPlayerId"
      :passed="table.passed" :drawn="drawings.map(event => event.playerId ?? '')">
      <template #meta>
        <div class="stock-block">
          <div class="stock-icon" aria-hidden="true">♣</div>
          <div><small>牌堆</small><strong data-testid="deck-count">{{ game.deckCount }} <span>张</span></strong></div>
        </div>
      </template>
      <template #board>
        <div class="round-label">第 {{ game.roundNumber }} 轮</div>
        <div v-if="played" :key="played.key" :class="['public-play', { blast: played.pattern.type === 'bomb' }]">
          <div class="played-cards" :aria-label="shapeName(played.pattern)">
            <CardFace v-for="id in played.pattern.cardIds" :key="id" :id="id" :wild-rank="played.pattern.wildcards?.find(wild => wild.cardId === id)?.rank" />
          </div>
          <span class="pattern-caption">{{ playedBy }} · {{ shapeName(played.pattern) }} {{ shapeRanks(played.pattern).join(' ') }}</span>
        </div>
        <div v-else-if="finished" class="lead-label">本局已结束</div>
        <div v-else class="lead-label"><span aria-hidden="true">✧</span>{{ active?.nickname }} 领牌</div>
        <div v-if="drawings.length" :key="table.motion?.key" class="draw-caption" data-testid="draw-event">{{ game.mode === 'classic' && drawings.length === game.capacity ? '全员各摸 1 张' : drawings.map(event => game.players.find(p => p.playerId === event.playerId)?.nickname).join('、') + ' 各摸 1 张' }}</div>
      </template>
      <template #actions>
        <div class="action-zone">
          <p class="turn-copy" :class="{ 'my-turn': turn, 'play-error': error }" role="status" data-testid="turn-status">{{ status }}</p>
          <div class="play-actions">
            <template v-if="!finished">
              <button class="button light-outline" type="button" :disabled="!canHint" @click="hint">提示</button>
              <button v-if="game.phase === 'following'" class="button light-outline" type="button" :disabled="!canPass || !table.available" @click="action('pass')">不出</button>
              <button class="button gold" type="button" :disabled="!canPlay" @click="play()">出牌<span v-if="selected.length"> · {{ selected.length }}</span></button>
            </template>
            <button v-else class="button gold" type="button" @click="settlement = true">查看结算</button>
          </div>
        </div>
      </template>
      <template #hand>
        <div class="hand-scroll" :class="{ 'tight-hand': tightHand, 'initial-deal': inDeal }" aria-label="我的手牌">
          <div class="hand" data-testid="hand">
            <CardFace v-for="id in game.selfHand" :key="id" :id="id" as-button :selected="selected.includes(id)"
              :disabled="!turn || !table.available" :drawn="table.motion?.drawn.includes(id)" @toggle="toggle(id)" />
            <span v-if="!game.selfHand.length" class="empty-hand">手牌已出完</span>
          </div>
        </div>
      </template>
      <template #self>
        <div class="self-info">
          <span class="avatar" :class="'avatar-' + self.seat" aria-hidden="true">{{ Array.from(self.nickname)[0] }}</span>
          <strong>{{ self.nickname }}</strong><span>你 · {{ self.handCount }} 张 · {{ signed(self.score) }} 分</span>
          <button v-if="table.notice" class="private-notice" type="button" :title="table.notice.message" @click="table.notice = undefined">双王：需搭配普通牌 <span aria-hidden="true">×</span></button>
        </div>
      </template>
    </component>

    <Modal v-if="chooser" title="万能牌怎么用？" @close="chooser = false">
      <div class="wild-options">
        <button v-for="option in options" :key="[option.type, option.rank, option.length].join('-')" type="button" @click="play(option)">
          <strong>{{ shapeName(option) }} <span>{{ shapeRanks(option).join(' ') }}</span></strong>
          <small>{{ option.wildcards?.map(wild => cardLabel(wild.cardId) + ' → ' + rankText(wild.rank)).join(' · ') }}</small>
        </button>
      </div>
    </Modal>
    <Modal v-if="settlement && finished" title="本局结算" wide @close="settlement = false">
      <p class="winner"><span aria-hidden="true">✦</span> {{ winner?.nickname }} 获胜</p>
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
