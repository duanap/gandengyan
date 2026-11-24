<script setup lang="ts">
import type { Player } from '../types/protocol'
defineProps<{ player: Player; active: boolean; passed: boolean; drawn: boolean }>()
</script>

<template>
  <div class="player-seat" :class="{ 'seat-active': active, 'seat-offline': !player.online }" :data-seat="player.seat">
    <span class="avatar" :class="'avatar-' + player.seat" aria-hidden="true">{{ player.bot ? '✦' : Array.from(player.nickname)[0] }}</span>
    <div class="seat-info">
      <strong>{{ player.nickname }}</strong>
      <small v-if="!player.online">离线</small>
      <small v-else-if="active">行动中</small>
      <small v-else-if="passed">不出</small>
      <small v-else>{{ player.bot ? 'AI 牌友' : '在线' }}</small>
    </div>
    <span class="seat-count" :class="{ 'count-drawn': drawn }" :key="String(drawn)" :aria-label="player.handCount + ' 张余牌'">{{ player.handCount }}</span>
  </div>
</template>
