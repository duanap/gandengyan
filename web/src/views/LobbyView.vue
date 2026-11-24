<script setup lang="ts">
import { ref } from 'vue'
import { modeName } from '../format'
import { useTable } from '../stores/table'
import type { RoomState } from '../types/protocol'

const props = defineProps<{ room: RoomState }>()
const table = useTable()
const info = ref('')
const error = ref('')
const share = ref(false)

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
    <section class="lobby" data-testid="lobby">
      <div class="lobby-heading">
        <div><p class="section-kicker">好友牌桌 · {{ modeName(room.mode) }}模式</p><h1>房间 {{ room.roomId }}</h1></div>
        <button class="button secondary" type="button" @click="copy">复制房间号</button>
      </div>
      <p v-if="info" class="lobby-info" role="status">{{ info }}</p>
      <input v-if="share" class="share-fallback" readonly aria-label="房间号" :value="room.roomId" @focus="$event.target && ($event.target as HTMLInputElement).select()">
      <div class="lobby-seats">
        <div v-for="seat in room.capacity" :key="seat" class="lobby-seat" :class="{ vacant: !room.players.find(p => p.seat === seat - 1) }">
          <template v-for="p in room.players.filter(p => p.seat === seat - 1)" :key="p.playerId">
            <span class="avatar" :class="'avatar-' + p.seat" aria-hidden="true">{{ p.bot ? '✦' : Array.from(p.nickname)[0] }}</span>
            <div class="lobby-seat-name">
              <strong>{{ p.nickname }}</strong>
              <small>{{ p.playerId === room.hostPlayerId ? '房主 · ' : '' }}{{ p.bot ? 'AI 牌友' : p.online ? '在线' : '离线' }}{{ p.playerId === room.selfPlayerId ? ' · 你' : '' }}</small>
            </div>
            <button v-if="p.bot && room.allowedActions.includes('remove_bot')" class="icon-button" :aria-label="'移除 ' + p.nickname"
              :disabled="!table.available" @click="action('remove_bot', { playerId: p.playerId })">×</button>
          </template>
          <template v-if="!room.players.some(p => p.seat === seat - 1)"><span class="avatar empty" aria-hidden="true">+</span><span>空席位</span></template>
        </div>
      </div>
      <p class="lobby-info" role="status">{{ error || (room.allowedActions.includes('start_game') ? '全员就绪' : room.players.length < room.capacity ? '等待牌友入座，也可以添加 AI 牌友。' : '等待房主开局或离线玩家返回。') }}</p>
      <div class="lobby-actions">
        <button class="button secondary" type="button" :disabled="!table.available" @click="action('leave_room')">退出房间</button>
        <button v-if="room.allowedActions.includes('add_bot')" class="button secondary" type="button" :disabled="!table.available" @click="action('add_bot')">添加 AI</button>
        <button v-if="room.hostPlayerId === room.selfPlayerId" class="button primary" type="button" :disabled="!table.available || !room.allowedActions.includes('start_game')"
          @click="action('start_game')">开始游戏 <span aria-hidden="true">→</span></button>
      </div>
    </section>
  </main>
</template>
