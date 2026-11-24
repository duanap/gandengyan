<script setup lang="ts">
import PlayerSeat from '../../components/PlayerSeat.vue'
import type { Player } from '../../types/protocol'

defineProps<{ players: Player[]; active: string; passed: string[]; drawn: string[] }>()
</script>

<template>
  <section class="table-surface mobile-table" data-layout="mobile" aria-label="手机牌桌">
    <div class="felt-line" aria-hidden="true"></div>
    <div class="mobile-opponents">
      <PlayerSeat v-for="p in players" :key="p.playerId" :player="p" :active="active === p.playerId"
        :passed="passed.includes(p.playerId)" :drawn="drawn.includes(p.playerId)" />
    </div>
    <div class="mobile-main">
      <div class="board-meta"><slot name="meta" /></div>
      <div class="board-play"><slot name="board" /></div>
    </div>
    <div class="board-bottom">
      <slot name="actions" />
      <slot name="hand" />
      <slot name="self" />
    </div>
  </section>
</template>
