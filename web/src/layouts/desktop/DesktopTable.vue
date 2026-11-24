<script setup lang="ts">
import { computed } from 'vue'
import PlayerSeat from '../../components/PlayerSeat.vue'
import type { Player } from '../../types/protocol'

const props = defineProps<{ players: Player[]; active: string; passed: string[]; drawn: string[] }>()
const positions = computed(() => ({
  1: ['across'],
  2: ['top-right', 'top-left'],
  3: ['right', 'across', 'left'],
  4: ['right', 'top-right', 'top-left', 'left'],
})[props.players.length] ?? [])
</script>

<template>
  <section class="table-surface desktop-table" data-layout="desktop" aria-label="桌面牌桌">
    <div class="felt-line" aria-hidden="true"></div>
    <div class="felt-mark" aria-hidden="true">♣</div>
    <div v-for="(p, index) in players" :key="p.playerId" class="opponent-desktop" :class="positions[index]">
      <PlayerSeat :player="p" :active="active === p.playerId" :passed="passed.includes(p.playerId)" :drawn="drawn.includes(p.playerId)" />
    </div>
    <div class="board-meta"><slot name="meta" /></div>
    <div class="board-play"><slot name="board" /></div>
    <div class="board-bottom">
      <slot name="actions" />
      <slot name="hand" />
      <slot name="self" />
    </div>
  </section>
</template>
