<script setup lang="ts">
import Modal from './Modal.vue'
defineEmits<{ close: [] }>()
</script>

<template>
  <Modal title="玩法规则 · V1.3" wide @close="$emit('close')">
    <div class="rules-content">
      <section><h3>开局与回合</h3><p>2–5 人，一副 54 张牌。首家 6 张，其余各 5 张。逆时针行动，领牌必须出牌，跟牌可以不出。其他玩家连续都不出时，最后出牌者领出下一轮。</p></section>
      <section><h3>接牌只大一级</h3><p>同牌型、同张数，点数恰好大一级。单 2 和对 2 可以跨级压制对应的单张或对子。顺子至少三张，连对至少两对，两者都以 A 为上限。</p></section>
      <section><h3>万能牌与炸弹</h3><p>大小王规则等价，搭配普通牌可组成对子、顺子、连对或炸弹。王不能单独打出，双王不能直接打出。炸弹为三至六张同点数牌，能压制任意普通牌型。炸弹先比张数，再比点数。</p></section>
      <section><h3>余牌与收尾</h3><p>出牌后不得只剩单 2、对 2、单王、双王或 2 加王。单 2、对 2（含 2 加王）也不能收尾。三张及以上的 2 炸弹可以收尾。</p></section>
      <section><h3>摸牌与积分</h3><p>经典每轮全员依次各摸一张，极速仅最后出牌者摸一张。牌堆摸完后继续打。首个合法出完手牌的玩家立即获胜，其他玩家各扣余牌张数，赢家获得扣分总和。</p></section>
    </div>
  </Modal>
</template>
