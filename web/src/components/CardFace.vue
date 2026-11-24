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
    <span class="card-pip" aria-hidden="true">{{ face.suit }}</span>
    <span class="card-opposite" aria-hidden="true">{{ face.suit }}</span>
    <span v-if="wildRank" class="card-wild" aria-hidden="true">→ {{ rankText(wildRank) }}</span>
  </component>
</template>
