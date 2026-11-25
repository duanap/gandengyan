<script setup lang="ts">
import { watch } from 'vue'
import CardFace from './CardFace.vue'

const props = defineProps<{
  cards: number[]
  selected: number[]
  drawn: number[]
  disabled: boolean
  dealing: boolean
}>()
const emit = defineEmits<{
  toggle: [id: number]
  changeRange: [ids: number[], select: boolean]
  clear: []
}>()

let gesture: {
  pointer: number; index: number; x: number; y: number; select: boolean
  mode: 'pending' | 'select' | 'scroll'; scrollLeft: number
} | undefined
let suppressClick = false
watch(() => props.cards.join(','), () => { gesture = undefined })

function cardIndex(target: EventTarget | null) {
  const card = target instanceof Element ? target.closest('[data-card-id]') : null
  return card ? props.cards.indexOf(Number(card.getAttribute('data-card-id'))) : -1
}

function begin(event: PointerEvent) {
  suppressClick = false
  if (props.disabled || !event.isPrimary || event.button !== 0) return
  const index = cardIndex(event.target)
  gesture = {
    pointer: event.pointerId, index, x: event.clientX, y: event.clientY,
    select: index >= 0 && !props.selected.includes(props.cards[index]), mode: 'pending',
    scrollLeft: (event.currentTarget as HTMLElement).scrollLeft,
  }
}

function move(event: PointerEvent) {
  const current = gesture
  if (!current || current.pointer !== event.pointerId || props.disabled) return
  const rail = event.currentTarget as HTMLElement
  const dx = event.clientX - current.x
  if (current.mode === 'pending') {
    const dy = Math.abs(event.clientY - current.y)
    if (Math.hypot(dx, dy) < 10) return
    if (current.index < 0) {
      gesture = undefined  // Panning from the blank part of the rail is native.
      return
    }
    current.mode = event.pointerType === 'touch' && Math.abs(dx) > dy ? 'scroll' : 'select'
    suppressClick = true
    rail.setPointerCapture(event.pointerId)
    if (current.mode === 'select') emit('changeRange', [props.cards[current.index]], current.select)
  }
  if (current.mode === 'scroll') {
    rail.scrollLeft = current.scrollLeft - dx
    return
  }

  const target = document.elementFromPoint(event.clientX, event.clientY)?.closest('[data-card-id]')
  if (!target || !rail.contains(target)) return
  const next = cardIndex(target)
  if (next < 0 || next === current.index) return
  emit('changeRange', props.cards.slice(Math.min(next, current.index), Math.max(next, current.index) + 1), current.select)
  current.index = next
}

function stop(event: PointerEvent) {
  const current = gesture
  if (!current || current.pointer !== event.pointerId) return
  gesture = undefined
  if (current.index < 0 && event.type === 'pointerup'
    && Math.hypot(event.clientX - current.x, event.clientY - current.y) < 10) emit('clear')
}

function click(event: MouseEvent) {
  if (suppressClick && event.detail > 0) {
    suppressClick = false
    event.stopPropagation()
    return
  }
}

function fan(index: number) {
  const count = props.cards.length
  if (count < 2 || count > 8) return { '--deal-index': index }
  const offset = (index - (count - 1) / 2) / ((count - 1) / 2)
  return { '--deal-index': index, '--fan-angle': offset * 6 + 'deg', '--fan-depth': (Math.abs(offset) - 1) * 7 + 'px' }
}
</script>

<template>
  <div class="hand-scroll" :class="{ 'tight-hand': cards.length > 8, 'initial-deal': dealing }"
    aria-label="我的手牌" @pointerdown="begin" @pointermove="move" @pointerup="stop" @pointercancel="stop"
    @lostpointercapture.self="stop" @pointerleave="gesture?.mode === 'pending' && stop($event)" @click.capture="click">
    <div class="hand" data-testid="hand">
      <CardFace v-for="(id, index) in cards" :key="id" :id="id" as-button :selected="selected.includes(id)"
        :disabled="disabled" :drawn="drawn.includes(id)" :style="fan(index)" @toggle="emit('toggle', id)" />
      <span v-if="!cards.length" class="empty-hand">手牌已出完</span>
    </div>
  </div>
</template>
