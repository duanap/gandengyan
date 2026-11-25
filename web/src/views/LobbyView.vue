<script setup lang="ts">
import { computed, ref } from 'vue'
import BoardTable from '../components/BoardTable.vue'
import PlayerAvatar from '../components/PlayerAvatar.vue'
import { tablePositions } from '../format'
import { useTable } from '../stores/table'
import type { RoomState } from '../types/protocol'

const props = defineProps<{ room: RoomState }>()
const table = useTable()
const info = ref('')
const error = ref('')
const share = ref(false)
const seats = computed(() => {
  const { players, selfPlayerId, capacity } = props.room
  const first = players.find(player => player.playerId === selfPlayerId)?.seat ?? 0
  const positions = tablePositions[capacity as keyof typeof tablePositions]
  return Array.from({ length: capacity }, (_, offset) => {
    const seat = (first + offset) % capacity
    return { seat, position: offset === 0 ? 'self' : positions[offset - 1], player: players.find(p => p.seat === seat) }
  })
})

async function action(type: string, payload?: unknown) {
  error.value = ''
  try { await table.command(type, payload) }
  catch (err) { error.value = err instanceof Error ? err.message : '操作失败。' }
}
async function copy() {
  try {
    await navigator.clipboard.writeText(props.room.roomId)
    info.value = '已复制房间号'
  } catch {
    share.value = true
    info.value = '请选择下方房间号并复制'
  }
}
</script>

<template>
  <main class="lobby-main">
    <BoardTable class="lobby" data-testid="lobby" aria-label="等待牌桌">
      <div class="table-grid lobby-grid">
        <div v-for="{ seat, position, player } in seats" :key="seat" class="table-seat" :data-position="position">
          <div class="lobby-seat player-seat" :class="{ vacant: !player, 'seat-self': position === 'self', 'seat-offline': player && !player.online }">
            <template v-if="player">
              <PlayerAvatar :seat="player.seat" :bot="player.bot" />
              <div class="seat-info">
                <strong>{{ player.nickname }}</strong>
                <small>{{ player.playerId === room.hostPlayerId ? '房主 · ' : '' }}{{ player.bot ? 'AI 牌友' : player.online ? '在线' : '离线' }}{{ position === 'self' ? ' · 你' : '' }}</small>
              </div>
              <button v-if="player.bot && room.allowedActions.includes('remove_bot')" class="icon-button" :aria-label="'移除 ' + player.nickname"
                :disabled="!table.available" @click="action('remove_bot', { playerId: player.playerId })">×</button>
            </template>
            <template v-else><span class="table-avatar empty-seat" aria-hidden="true">+</span><small>空席位</small></template>
          </div>
        </div>
        <div class="lobby-center">
          <h1>房间 {{ room.roomId }}</h1>
          <button class="button light-outline" type="button" @click="copy">复制房间号</button>
          <p v-if="info" class="lobby-info" role="status">{{ info }}</p>
          <input v-if="share" class="share-fallback" readonly aria-label="房间号" :value="room.roomId" @focus="($event.target as HTMLInputElement).select()">
          <p class="lobby-info" role="status">{{ error || (room.allowedActions.includes('start_game') ? '全员就绪' : room.players.length < room.capacity ? '等待牌友入座' : '等待房主开局或离线玩家返回') }}</p>
        </div>
        <div class="lobby-actions">
          <button class="button secondary" type="button" :disabled="!table.available" @click="action('leave_room')">退出房间</button>
          <button v-if="room.allowedActions.includes('add_bot')" class="button secondary" type="button" :disabled="!table.available" @click="action('add_bot')">添加 AI</button>
          <button v-if="room.hostPlayerId === room.selfPlayerId" class="button gold" type="button" :disabled="!table.available || !room.allowedActions.includes('start_game')"
            @click="action('start_game')">开始游戏 <span aria-hidden="true">→</span></button>
        </div>
      </div>
    </BoardTable>
  </main>
</template>
