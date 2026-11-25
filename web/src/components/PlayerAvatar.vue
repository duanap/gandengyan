<script setup lang="ts">
defineProps<{ seat: number; bot?: boolean }>()
</script>

<template>
  <span class="table-avatar" :data-avatar="bot ? 5 : seat % 5" aria-hidden="true"></span>
</template>
