<script setup lang="ts">
import PlayerAvatar from './PlayerAvatar.vue'
import { signed } from '../format'
import type { Player } from '../types/protocol'
defineProps<{ player: Player; active?: boolean; passed?: boolean; drawn?: boolean; self?: boolean; host?: boolean }>()
</script>

<template>
  <div class="player-seat" :class="{ 'seat-active': active, 'seat-offline': !player.online, 'seat-self': self }" :data-seat="player.seat">
    <PlayerAvatar :seat="player.seat" :bot="player.bot" />
    <div class="seat-info">
      <strong>{{ player.nickname }}</strong>
      <small v-if="self" class="seat-role">{{ host ? '你 · 房主' : '你 · 牌友' }}</small>
      <div class="seat-detail">
        <span :key="player.handCount" class="seat-count" :class="{ 'count-drawn': drawn }" :aria-label="player.handCount + ' 张余牌'">
          <i class="hand-count-icon" aria-hidden="true"></i>{{ player.handCount }} 张
        </span>
        <small v-if="self">{{ signed(player.score) }} 分</small>
        <small v-else-if="!player.online">离线</small>
        <small v-else-if="active">出牌中</small>
        <small v-else-if="passed">不出</small>
        <small v-else-if="drawn">摸牌</small>
        <small v-else>{{ player.bot ? 'AI' : '在线' }}</small>
      </div>
    </div>
  </div>
</template>
