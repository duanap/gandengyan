<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import HomeView from './views/HomeView.vue'
import LobbyView from './views/LobbyView.vue'
import GameView from './views/GameView.vue'
import RoomDialog from './components/RoomDialog.vue'
import RulesDialog from './components/RulesDialog.vue'
import { modeName } from './format'
import { useTable } from './stores/table'

const table = useTable()
const dialog = ref<'create' | 'join' | 'quick'>()
const rules = ref(false)
const visibility = () => { if (!document.hidden) table.nudge() }
onMounted(() => {
  table.restore()
  if (!table.credentials && /^\d{6}$/.test(new URLSearchParams(location.search).get('room') ?? '')) dialog.value = 'join'
  document.addEventListener('visibilitychange', visibility)
  window.addEventListener('online', table.nudge)
})
onUnmounted(() => {
  document.removeEventListener('visibilitychange', visibility)
  window.removeEventListener('online', table.nudge)
})
</script>

<template>
  <div class="app-shell" :class="{ 'at-table': !!table.game }">
    <header class="site-header">
      <div class="brand">
        <span v-if="table.game" class="brand-suit" aria-hidden="true">♠</span>
        <img v-else src="/favicon.svg" alt="" width="36" height="36">
        <strong>干瞪眼</strong>
      </div>
      <div v-if="table.room" class="header-room"><span>房间 {{ table.room.roomId }}</span><span>{{ modeName(table.room.mode) }}模式</span></div>
      <div class="header-aside">
        <span v-if="table.credentials" class="connection" :class="{ connected: table.connection === 'online' }" role="status">
          {{ table.connection === 'online' ? '已连接' : table.connection === 'reconnecting' ? '重连中' : '连接中' }}
        </span>
        <button class="rules-button" type="button" @click="rules = true"><span aria-hidden="true">?</span> 玩法规则</button>
      </div>
    </header>
    <GameView v-if="table.game" />
    <LobbyView v-else-if="table.room" :room="table.room" />
    <main v-else-if="table.credentials" class="connecting-main">
      <p role="status">正在连接房间 {{ table.credentials.roomId }}…</p>
      <button class="button secondary" type="button" @click="table.back">返回首页</button>
    </main>
    <HomeView v-else :message="table.globalMessage" @open="dialog = $event" />
    <RoomDialog v-if="dialog" :initial="dialog" @close="dialog = undefined" />
    <RulesDialog v-if="rules" @close="rules = false" />
  </div>
</template>
