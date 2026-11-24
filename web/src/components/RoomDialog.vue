<script setup lang="ts">
import { computed, ref } from 'vue'
import Modal from './Modal.vue'
import { CommandError } from '../network/ws'
import { useTable } from '../stores/table'
import type { Mode } from '../types/protocol'

const props = defineProps<{ initial: 'create' | 'join' | 'quick' }>()
const emit = defineEmits<{ close: [] }>()
const table = useTable()
const kind = ref(props.initial)
const nickname = ref('')
const roomId = ref('')
const capacity = ref(4)
const mode = ref<Mode>('classic')
const busy = ref(false)
const error = ref('')
const title = computed(() => kind.value === 'quick' ? '快速开始' : kind.value === 'join' ? '加入房间' : '创建房间')

try {
  nickname.value = sessionStorage.getItem('gandengyan.nickname') ?? ''
  const invite = new URLSearchParams(location.search).get('room')
  if (invite && /^\d{6}$/.test(invite)) roomId.value = invite
} catch { /* Nickname storage is optional. */ }

function switchTo(next: 'join' | 'create') {
  kind.value = next
  error.value = ''
}
async function submit() {
  if (busy.value) return
  const name = nickname.value.trim()
  if (!name || [...name].length > 16) { error.value = '请输入 1–16 个字符的昵称。'; return }
  if (kind.value === 'join' && !/^\d{6}$/.test(roomId.value)) { error.value = '请输入 6 位数字房间号。'; return }
  busy.value = true
  error.value = ''
  try {
    await table.enter(kind.value, kind.value === 'join'
      ? { nickname: name, roomId: roomId.value }
      : { nickname: name, capacity: capacity.value, mode: mode.value })
    try { sessionStorage.setItem('gandengyan.nickname', name) } catch { /* Optional. */ }
    emit('close')
  } catch (err) { error.value = err instanceof CommandError ? err.message : '连接失败，请检查网络后重试。' }
  finally { busy.value = false }
}
</script>

<template>
  <Modal :title="title" @close="!busy && emit('close')">
    <div v-if="initial !== 'quick'" class="segmented tabs" role="group" aria-label="房间操作">
      <button type="button" :aria-pressed="kind === 'create'" :disabled="busy" @click="switchTo('create')">创建房间</button>
      <button type="button" :aria-pressed="kind === 'join'" :disabled="busy" @click="switchTo('join')">加入房间</button>
    </div>
    <form class="room-form" @submit.prevent="submit">
      <label>昵称<input v-model="nickname" autocomplete="nickname" maxlength="32" placeholder="输入你的昵称" required :disabled="busy"></label>
      <label v-if="kind === 'join'">房间号<input v-model="roomId" inputmode="numeric" autocomplete="off" maxlength="6" placeholder="6 位数字" required :disabled="busy"></label>
      <template v-else>
        <fieldset :disabled="busy">
          <legend>牌桌人数</legend>
          <div class="segmented seats">
            <label v-for="n in [2, 3, 4, 5]" :key="n">
              <input v-model="capacity" type="radio" name="capacity" :value="n"><span>{{ n }} 人</span>
            </label>
          </div>
        </fieldset>
        <fieldset :disabled="busy">
          <legend>玩法</legend>
          <div class="mode-options">
            <label><input v-model="mode" type="radio" name="mode" value="classic"><span><b>经典</b><small>每轮结束，全员各摸一张</small></span></label>
            <label><input v-model="mode" type="radio" name="mode" value="speed"><span><b>极速</b><small>每轮只有最后出牌者摸一张</small></span></label>
          </div>
        </fieldset>
      </template>
      <p v-if="error" class="form-error" role="alert">{{ error }}</p>
      <button class="button primary form-submit" type="submit" :disabled="busy">{{ busy ? '正在连接…' : kind === 'quick' ? '和 AI 开一局' : kind === 'create' ? '创建牌桌' : '进入牌桌' }} <span aria-hidden="true">→</span></button>
    </form>
  </Modal>
</template>
