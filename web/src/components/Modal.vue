<script setup lang="ts">
import { onMounted, ref, useId } from 'vue'
const props = defineProps<{ title: string; wide?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const element = ref<HTMLDialogElement>()
const titleId = useId()
onMounted(() => element.value?.showModal())
</script>

<template>
  <dialog ref="element" class="modal" :class="{ wide: props.wide }" :aria-labelledby="titleId"
    @cancel.prevent="emit('close')" @click.self="emit('close')">
    <div class="modal-body">
      <div class="modal-heading">
        <h2 :id="titleId" :class="{ 'visually-hidden': $slots.heading }">{{ title }}</h2>
        <slot name="heading" />
        <button class="icon-button" type="button" aria-label="关闭" @click="emit('close')">×</button>
      </div>
      <slot />
    </div>
  </dialog>
</template>
