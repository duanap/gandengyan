<script setup lang="ts">
import { computed } from 'vue'
import { cardFace, cardLabel, rankText } from '../format'

const props = defineProps<{ id: number; asButton?: boolean; selected?: boolean; disabled?: boolean; wildRank?: number; drawn?: boolean }>()
const emit = defineEmits<{ toggle: [] }>()
const face = computed(() => cardFace(props.id))
</script>

<template>
  <component :is="asButton ? 'button' : 'span'" class="playing-card"
    :class="{ red: face.red, joker: face.joker, 'card-selected': selected, 'card-drawn': drawn }"
    :data-card-id="id" :type="asButton ? 'button' : undefined" :disabled="asButton ? disabled : undefined"
    :role="asButton ? undefined : 'img'" :aria-label="cardLabel(id) + (wildRank ? ' 当作 ' + rankText(wildRank) : '')"
    :aria-pressed="asButton ? !!selected : undefined" @click="emit('toggle')">
    <span class="card-index" aria-hidden="true"><b>{{ face.rank }}</b><span v-if="!face.joker">{{ face.suit }}</span></span>
    <span class="card-pip" aria-hidden="true">
      <svg v-if="face.joker" class="joker-art" viewBox="0 0 42 36" fill="currentColor">
        <path d="M10 19C4 17 3 10 5 7c4 2 8 4 12 6 1-5 4-9 8-12 3 5 4 9 3 13 4-4 9-5 12-4-1 7-4 10-8 12 H11Z"/>
        <path d="M11 22h21c-.3 7-4.4 12-10.4 12S11.7 29.5 11 22Z" fill="none" stroke="currentColor" stroke-width="2.2"/>
        <circle cx="5" cy="7" r="3"/><circle cx="25" cy="3" r="3"/><circle cx="39" cy="10" r="3"/>
        <circle cx="18" cy="25" r="1.35"/><circle cx="25" cy="25" r="1.35"/>
        <path d="M18 29q3.5 3 7 0" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
      </svg>
      <template v-else>{{ face.suit }}</template>
    </span>
    <span class="card-opposite" aria-hidden="true">{{ face.suit }}</span>
    <span v-if="wildRank" class="card-wild" aria-hidden="true">→ {{ rankText(wildRank) }}</span>
  </component>
</template>
