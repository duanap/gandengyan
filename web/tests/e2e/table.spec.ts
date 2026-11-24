import { expect, test, type Browser, type BrowserContext, type Page } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import type { Envelope, GameState } from '../../src/types/protocol'

const fixture = 'http://127.0.0.1:18082'
const screenshots = fileURLToPath(new URL('../../../docs/screenshots/', import.meta.url))

async function capture(page: Page, file: string) {
  mkdirSync(screenshots, { recursive: true })
  await page.screenshot({ path: screenshots + file, animations: 'disabled' })
}

function observe(page: Page) {
  let current: Envelope<GameState> | undefined
  const packets: Envelope[] = []
  page.on('websocket', socket => {
    socket.on('framereceived', ({ payload }) => {
      const parsed = JSON.parse(payload.toString()) as Envelope
      packets.push(parsed)
      if (parsed.type === 'game_state') current = parsed as Envelope<GameState>
    })
  })
  return { state: () => current!, packets }
}
async function create(page: Page, nickname: string, capacity: number, mode: 'classic' | 'speed', quick = false) {
  await page.getByRole('button', { name: quick ? '和 AI 快速开始' : '创建房间', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('昵称').fill(nickname)
  await dialog.getByText(capacity + ' 人', { exact: true }).click()
  await dialog.getByText(mode === 'classic' ? '经典' : '极速', { exact: true }).click()
  await dialog.getByRole('button', { name: quick ? '和 AI 开一局' : '创建牌桌' }).click()
}
async function join(page: Page, nickname: string, code: string) {
  await page.getByRole('button', { name: '加入房间', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('昵称').fill(nickname)
  await dialog.getByLabel('房间号').fill(code)
  await dialog.getByRole('button', { name: '进入牌桌' }).click()
  await expect(page.getByTestId('lobby')).toBeVisible()
}
async function fixturePair(browser: Browser, mode: 'classic' | 'speed', preparePeer?: (context: BrowserContext) => Promise<void>) {
  const ac = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const bc = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  if (preparePeer) await preparePeer(bc)
  const a = await ac.newPage()
  const b = await bc.newPage()
  const watchedA = observe(a)
  const watchedB = observe(b)
  await Promise.all([a.goto(fixture), b.goto(fixture)])
  await create(a, '栗子', 2, mode)
  const title = a.getByRole('heading', { name: /^房间 \d{6}$/ })
  await expect(title).toBeVisible()
  const id = (await title.textContent())!.slice(-6)
  await join(b, '小松', id)
  await a.getByRole('button', { name: '开始游戏' }).click()
  await expect(a.getByTestId('turn-status')).toHaveText('轮到你领牌')
  await expect(b.getByTestId('turn-status')).toHaveText('等待 栗子 出牌')
  return { a, b, ac, bc, watchedA, watchedB }
}
const card = (page: Page, id: number) => page.getByTestId('hand').locator('[data-card-id="' + id + '"]')
const pass = (page: Page) => page.getByRole('button', { name: '不出', exact: true })
const play = (page: Page) => page.getByRole('button', { name: /^出牌/ })

test('首页、规则、创建与加入切换', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: /^干瞪眼/ })).toBeVisible()
  await capture(page, 'home-desktop.png')
  await page.getByRole('button', { name: '玩法规则' }).click()
  await expect(page.getByRole('dialog')).toContainText('V1.3')
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '创建房间', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: '加入房间' }).click()
  await dialog.getByLabel('昵称').fill('小松')
  await dialog.getByLabel('房间号').fill('12')
  await dialog.getByRole('button', { name: '进入牌桌' }).click()
  await expect(dialog.getByRole('alert')).toHaveText('请输入 6 位数字房间号。')
  await dialog.getByRole('button', { name: '创建房间' }).click()
  await expect(dialog.getByRole('group', { name: '牌桌人数' })).toBeVisible()
})

test('独立浏览器联机、非法余牌、多义王、公开出牌和经典补牌', async ({ browser }) => {
  const { a, b, ac, bc, watchedA, watchedB } = await fixturePair(browser, 'classic')
  try {
    await expect(a.getByRole('button', { name: /双王/ })).toBeVisible()
    await expect(b.getByRole('button', { name: /双王/ })).toHaveCount(0)
    for (const id of [4, 8, 53, 12]) await card(a, id).click()
    await expect(a.getByTestId('turn-status')).toContainText('不能仅剩')
    await expect(play(a)).toBeDisabled()
    await expect(card(a, 12)).toHaveAttribute('aria-pressed', 'true')
    await card(a, 12).click()
    await play(a).click()
    const dialog = a.getByRole('dialog', { name: '万能牌怎么用？' })
    await expect(dialog.getByRole('button', { name: /顺子/ })).toHaveCount(2)
    await dialog.getByRole('button', { name: /顺子 4 5 6/ }).click()
    await expect(b.getByRole('img', { name: '大王 当作 6' })).toBeVisible()
    await expect(b.getByTestId('turn-status')).toHaveText('无牌可接，可以不出')
    await capture(b, 'table-desktop.png')
    await pass(b).click()
    await expect(a.getByTestId('draw-event')).toHaveText('全员各摸 1 张')
    await expect(a.getByTestId('deck-count')).toHaveText('41 张')
    await expect(card(a, 13)).toBeVisible()
    await expect(card(b, 17)).toBeVisible()
    await expect(card(b, 13)).toHaveCount(0)
    const secret = watchedA.state().payload.privateNotices[0].noticeId
    expect(watchedB.packets.every(p => !JSON.stringify(p).includes(secret))).toBeTruthy()
    await a.reload()
    await expect(card(a, 13)).toBeVisible()
    await expect(a.getByRole('button', { name: /双王/ })).toHaveCount(0)
  } finally { await ac.close(); await bc.close() }
})

test('牌堆用完后继续、零和结算与赢家首出', async ({ browser }) => {
  const { a, b, ac, bc, watchedA } = await fixturePair(browser, 'classic')
  try {
    for (let i = 0; i < 23; i++) {
      // Repeated legal singles and passes make depletion deterministic.
      const next = watchedA.state().payload.selfHand.find(id => id < 52)!
      await card(a, next).click()
      await play(a).click()
      await pass(b).click()
      await expect(a.getByTestId('turn-status')).toHaveText('轮到你领牌')
      if (i < 22) await expect(a.getByTestId('deck-count')).toHaveText(Math.max(0, 43 - (i + 1) * 2) + ' 张')
    }
    await expect(a.getByTestId('deck-count')).toHaveText('0 张')
    await expect(a.getByTestId('draw-event')).toHaveCount(0)
    const gameId = watchedA.state().gameId
    let current = watchedA.state().payload
    while (current.phase !== 'finished') {
      const before = current.selfHand.length
      await a.getByRole('button', { name: '提示', exact: true }).click()
      await play(a).click()
      const choose = a.getByRole('dialog', { name: '万能牌怎么用？' })
      if (await choose.isVisible()) await choose.getByRole('button').nth(1).click()
      await expect.poll(() => watchedA.state().payload.selfHand.length).toBeLessThan(before)
      current = watchedA.state().payload
      if (current.phase !== 'finished') {
        await pass(b).click()
        await expect(a.getByTestId('turn-status')).toHaveText('轮到你领牌')
        current = watchedA.state().payload
      }
    }
    const dialog = a.getByRole('dialog', { name: '本局结算' })
    await expect(dialog).toContainText('栗子 获胜')
    expect(current.settlement!.reduce((sum, result) => sum + result.delta, 0)).toBe(0)
    await capture(a, 'settlement-desktop.png')
    const scores = current.players.map(player => player.score)
    await dialog.getByRole('button', { name: '再来一局' }).click()
    await expect(a.getByTestId('turn-status')).toHaveText('轮到你领牌')
    expect(watchedA.state().gameId).not.toBe(gameId)
    expect(watchedA.state().payload.players.map(player => player.score)).toEqual(scores)
  } finally { await ac.close(); await bc.close() }
})

test('延时连接、断网恢复和极速补牌', async ({ browser }) => {
  const { a, b, ac, bc, watchedB } = await fixturePair(browser, 'speed', async context => {
    await context.addInitScript(() => {
      const Native = window.WebSocket
      const active = new Set<WebSocket>()
      window.WebSocket = class extends Native {
        constructor(url: string | URL, protocols?: string | string[]) {
          super(url, protocols)
          const socket = this
          active.add(socket)
          socket.addEventListener('close', () => active.delete(socket), { once: true })
          Object.defineProperty(socket, 'onmessage', {
            set(handler: (this: WebSocket, message: MessageEvent) => void) {
              socket.addEventListener('message', message => {
                setTimeout(() => { if (socket.readyState === Native.OPEN) handler.call(socket, message) }, 135)
              })
            },
          })
        }
      }
      window.addEventListener('gdy-test-disconnect', () => {
        for (const socket of active) socket.close(4000)
      })
    })
  })
  try {
    const hand = watchedB.state().payload.selfHand
    const self = watchedB.state().payload.selfPlayerId
    await bc.setOffline(true)
    await b.evaluate(() => window.dispatchEvent(new Event('gdy-test-disconnect')))
    await expect(b.getByText('重连中', { exact: true })).toBeVisible()
    await card(a, 48).click()
    await play(a).click()
    await expect(a.getByTestId('turn-status')).toHaveText('等待 小松 重连')
    await bc.setOffline(false)
    await expect(pass(b)).toBeEnabled({ timeout: 8000 })
    expect(watchedB.state().payload.selfPlayerId).toBe(self)
    expect(watchedB.state().payload.selfHand).toEqual(hand)
    await pass(b).click()
    await expect(a.getByTestId('draw-event')).toHaveText('栗子 各摸 1 张')
    await expect(a.getByTestId('deck-count')).toHaveText('42 张')
    await expect.poll(() => watchedB.state().payload.deckCount).toBe(42)
    expect(watchedB.state().payload.selfHand).toEqual(hand)
  } finally { await ac.close(); await bc.close() }
})

for (const mode of ['classic', 'speed'] as const) {
  for (const n of [2, 3, 4, 5]) {
    test('实际二进制开桌与移动布局 ' + mode + ' / ' + n, async ({ browser }) => {
      const width = mode === 'classic' ? 932 : 844
      const height = mode === 'classic' ? 430 : 390
      const context = await browser.newContext({ viewport: { width, height }, isMobile: true, hasTouch: true, deviceScaleFactor: 1 })
      const page = await context.newPage()
      const watch = observe(page)
      try {
        await page.goto('http://127.0.0.1:18081/')
        if (n === 2 && mode === 'speed') {
          await create(page, '晓风', n, mode, true)
        } else {
          await create(page, '晓风', n, mode)
          const add = page.getByRole('button', { name: '添加 AI' })
          for (let i = 0; i < n - 1; i++) {
            await add.click()
            await expect(page.locator('.lobby-seat:not(.vacant)')).toHaveCount(i + 2)
          }
          if (n === 4 && mode === 'speed') {
            await page.getByRole('button', { name: /移除/ }).first().click()
            await expect(page.locator('.lobby-seat.vacant')).toHaveCount(1)
            await add.click()
          }
          await page.getByRole('button', { name: '开始游戏' }).click()
        }
        const mobile = page.locator('[data-layout="mobile"]')
        await expect(mobile).toBeVisible()
        await expect(mobile.locator('.player-seat')).toHaveCount(n - 1)
        await expect(page.getByTestId('hand').getByRole('button').first()).toBeVisible()
        await expect.poll(() => watch.state().payload.currentPlayerId).toBeTruthy()
        expect(watch.state().payload.mode).toBe(mode)
        expect(watch.state().payload.players.filter(p => p.bot)).toHaveLength(n - 1)
        const box = await play(page).boundingBox()
        expect(box!.y + box!.height).toBeLessThanOrEqual(height)
        if (n === 5) {
          await expect.poll(() => watch.state().payload.currentPlayerId === watch.state().payload.selfPlayerId).toBe(true)
          if (await page.getByRole('button', { name: '提示', exact: true }).isEnabled()) {
            await page.getByRole('button', { name: '提示', exact: true }).tap()
            await expect(play(page)).toBeEnabled()
          }
          await capture(page, 'table-mobile-' + width + '.png')
          await page.setViewportSize({ width: 667, height: 320 })
          await expect(play(page)).toBeInViewport()
          expect(await page.evaluate(() => document.documentElement.scrollHeight)).toBeLessThanOrEqual(321)
          await page.setViewportSize({ width: 390, height: 844 })
          await expect(play(page)).toBeInViewport()
          expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
        }
      } finally { await context.close() }
    })
  }
}

test('两位真人与 AI 共桌，机器人走同一行动流', async ({ browser }) => {
  const contexts = await Promise.all([browser.newContext(), browser.newContext()])
  const [a, b] = await Promise.all(contexts.map(context => context.newPage()))
  const watched = observe(a)
  const peer = observe(b)
  try {
    await Promise.all([a.goto('http://127.0.0.1:18081/'), b.goto('http://127.0.0.1:18081/')])
    await create(a, '栗子', 3, 'speed')
    const title = a.getByRole('heading', { name: /^房间 \d{6}$/ })
    await expect(title).toBeVisible()
    await join(b, '小松', (await title.textContent())!.slice(-6))
    await a.getByRole('button', { name: '添加 AI' }).click()
    await a.getByRole('button', { name: '开始游戏' }).click()
    await expect(a.locator('[data-layout="desktop"]')).toBeVisible()
    await expect(b.locator('[data-layout="desktop"]')).toBeVisible()
    const humans = new Map([[watched.state().payload.selfPlayerId, a], [peer.state().payload.selfPlayerId, b]])
    const bot = watched.state().payload.players.find(p => p.bot)!
    const botMoved = () => watched.packets.some(packet => packet.type === 'game_state' &&
      (packet.payload as GameState).events?.some(event => event.playerId === bot.playerId && ['cards_played', 'player_passed'].includes(event.type)))
    for (let turns = 0; turns < 4 && !botMoved(); turns++) {
      const state = watched.state()
      const actor = humans.get(state.payload.currentPlayerId)
      if (actor) {
        const hint = actor.getByRole('button', { name: '提示', exact: true })
        if (await hint.isEnabled()) {
          await hint.click()
          await play(actor).click()
          const chooser = actor.getByRole('dialog', { name: '万能牌怎么用？' })
          if (await chooser.isVisible()) await chooser.getByRole('button').nth(1).click()
        } else {
          await pass(actor).click()
        }
      }
      await expect.poll(() => watched.state().revision).toBeGreaterThan(state.revision)
    }
    expect(botMoved()).toBeTruthy()
    expect(watched.state().gameId).toBe(peer.state().gameId)
  } finally { await Promise.all(contexts.map(context => context.close())) }
})
