<script setup lang="ts">
import CardFace from '../components/CardFace.vue'
defineProps<{ message: string }>()
const emit = defineEmits<{ open: [mode: 'create' | 'join' | 'quick'] }>()
</script>

<template>
  <main class="home-main">
    <p v-if="message" class="home-alert" role="status">{{ message }}</p>
    <section class="hero">
      <div class="hero-copy">
        <span class="hero-tag"><span aria-hidden="true">♣</span> 2–5 人休闲纸牌</span>
        <h1>干瞪眼<span>.</span></h1>
        <p>差一级，就能接。<br>先出完手牌的人赢。</p>
        <div class="hero-actions">
          <button class="button gold" type="button" @click="emit('open', 'create')">创建房间 <span aria-hidden="true">↗</span></button>
          <button class="button light-outline" type="button" @click="emit('open', 'join')">加入房间</button>
        </div>
        <button class="quick-link" type="button" @click="emit('open', 'quick')"><span aria-hidden="true">✦</span> 和 AI 快速开始 <span aria-hidden="true">→</span></button>
      </div>
      <div class="hero-art" aria-hidden="true">
        <div class="art-mat"></div>
        <div class="art-orbit"></div>
        <span class="art-spark art-spark-top">✦</span>
        <span class="art-spark art-spark-bottom">✧</span>
        <span class="art-spark art-spark-right">✦</span>
        <div class="art-card art-back"></div>
        <div class="art-card art-ace"><CardFace :id="47" /></div>
        <div class="art-card art-five"><CardFace :id="10" /></div>
        <div class="art-card art-joker"><CardFace :id="53" /></div>
        <div class="art-token">♣</div>
      </div>
    </section>
    <div class="mode-strip" aria-label="两种玩法">
      <section><span class="mode-icon classic-icon" aria-hidden="true">♣</span><div><h2>经典</h2><p>每轮结束，全员各摸一张</p></div></section>
      <section><span class="mode-icon speed-icon" aria-hidden="true">ϟ</span><div><h2>极速</h2><p>仅最后出牌者摸一张</p></div></section>
    </div>
  </main>
</template>
