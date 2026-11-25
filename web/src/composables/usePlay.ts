import { computed, onScopeDispose, ref, watch } from 'vue'
import { useTable } from '../stores/table'
import type { Pattern, Preview } from '../types/protocol'

export function usePlay() {
  const table = useTable()
  const selected = ref<number[]>([])
  const options = ref<Pattern[]>([])
  const chooser = ref(false)
  const error = ref('')
  let pendingPreview: number | undefined
  let generation = 0
  const turn = computed(() => table.game?.currentPlayerId === table.game?.selfPlayerId && !!table.game)
  const canSelect = computed(() => table.available && !!table.game && table.game.phase !== 'finished')
  const canPass = computed(() => table.game?.allowedActions.includes('pass') === true)
  const canHint = computed(() => turn.value && table.game?.canPlay && table.available)
  const canPlay = computed(() => options.value.length > 0 && turn.value && table.available)

  function reset() {
    selected.value = []
    options.value = []
    error.value = ''
    chooser.value = false
  }
  watch(() => table.gameId, reset)
  watch(() => table.game?.selfHand, hand => {
    const valid = selected.value.filter(id => hand?.includes(id))
    if (valid.length !== selected.value.length) selected.value = valid
  })

  watch([selected, () => table.revision, () => table.connection], () => {
    clearTimeout(pendingPreview)
    const current = ++generation
    options.value = []
    chooser.value = false
    if (!turn.value) error.value = ''
    if (!turn.value || table.connection !== 'online' || !selected.value.length) return
    const selection = [...selected.value].sort((a, b) => a - b)
    pendingPreview = window.setTimeout(async () => {
      try {
        const reply = await table.query<Preview>('preview_play', { cardIds: selection })
        if (generation !== current || reply.revision !== table.revision || reply.gameId !== table.gameId) return
        options.value = reply.payload.options
        error.value = reply.payload.problem?.message ?? ''
      } catch (err) {
        if (generation === current && err instanceof Error) error.value = err.message
      }
    }, 105)
  })
  onScopeDispose(() => { clearTimeout(pendingPreview); generation++ })

  function toggle(id: number) {
    changeRange([id], !selected.value.includes(id))
  }

  function changeRange(ids: number[], select: boolean) {
    if (!canSelect.value) return
    const next = select ? [...new Set([...selected.value, ...ids])] : selected.value.filter(id => !ids.includes(id))
    if (next.length === selected.value.length) return
    error.value = ''
    selected.value = next
  }

  function clear() {
    if (canSelect.value && selected.value.length) reset()
  }

  async function action(type: string, payload: unknown = {}) {
    try {
      error.value = ''
      await table.command(type, payload)
      if (type === 'play_cards') reset()
    } catch (err) { error.value = err instanceof Error ? err.message : '操作失败，请重试。' }
  }

  function play(pattern?: Pattern) {
    if (!canPlay.value) return
    if (!pattern && options.value.length > 1) { chooser.value = true; return }
    const choice = pattern ?? options.value[0]
    chooser.value = false
    return action('play_cards', { cardIds: selected.value, pattern: { type: choice.type, length: choice.length, rank: choice.rank } })
  }

  async function hint() {
    if (!canHint.value) return
    const before = generation
    try {
      const reply = await table.query<Pattern | null>('hint')
      if (reply.revision !== table.revision || reply.gameId !== table.gameId || generation !== before) return
      selected.value = reply.payload?.cardIds ?? []
    } catch (err) { error.value = err instanceof Error ? err.message : '提示暂不可用。' }
  }
  return { selected, options, chooser, error, turn, canPass, canHint, canPlay, canSelect, toggle, changeRange, clear, action, play, hint }
}
