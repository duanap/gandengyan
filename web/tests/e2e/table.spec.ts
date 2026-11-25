import { expect, test, type Browser, type BrowserContext, type BrowserContextOptions, type Page } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import type { Envelope, GameState } from '../../src/types/protocol'

const fixture = 'http://127.0.0.1:18082'
const screenshots = fileURLToPath(new URL('../../../docs/screenshots/v1.4/', import.meta.url))
const seatOrder = [
  ['top-center'],
  ['top-right', 'top-left'],
  ['right', 'top-center', 'left'],
  ['right', 'top-right', 'top-left', 'left'],
]

async function capture(page: Page, file: string) {
  if (process.env.GDY_CAPTURE !== '1') return
  mkdirSync(screenshots, { recursive: true })
  await page.screenshot({ path: screenshots + file, type: 'jpeg', quality: 92, animations: 'disabled' })
}

function observe(page: Page) {
  let current: Envelope<GameState> | undefined
  const packets: Envelope[] = []
  const sent: Envelope[] = []
  page.on('websocket', socket => {
    if (!socket.url().endsWith('/ws')) return
    socket.on('framereceived', ({ payload }) => {
      const parsed = JSON.parse(payload.toString()) as Envelope
      packets.push(parsed)
      if (parsed.type === 'game_state') current = parsed as Envelope<GameState>
    })
    socket.on('framesent', ({ payload }) => sent.push(JSON.parse(payload.toString()) as Envelope))
  })
  return { state: () => current!, packets, sent }
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
async function fixturePair(browser: Browser, mode: 'classic' | 'speed', preparePeer?: (context: BrowserContext) => Promise<void>, peerOptions?: BrowserContextOptions) {
  const ac = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const bc = await browser.newContext(peerOptions ?? { viewport: { width: 1440, height: 900 } })
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
  if (mode === 'classic') await capture(a, 'lobby-desktop.jpg')
  await join(b, '小松', id)
  await a.getByRole('button', { name: '开始游戏' }).click()
  await expect(a.getByTestId('turn-status')).toHaveText('轮到你领牌')
  await expect(b.getByTestId('turn-status')).toHaveText('等待 栗子 出牌')
  return { a, b, ac, bc, watchedA, watchedB }
}
const card = (page: Page, id: number) => page.getByTestId('hand').locator('[data-card-id="' + id + '"]')
const pass = (page: Page) => page.getByRole('button', { name: '不出', exact: true })
const play = (page: Page) => page.getByRole('button', { name: /^出牌/ })
const table = (page: Page) => page.locator('[data-layout="shared"]')

async function assertTable(page: Page, width: number, height: number) {
  const sizes = await table(page).evaluate(node => {
    const rect = (selector: string) => node.querySelector(selector)!.getBoundingClientRect().toJSON()
    return {
      stock: rect('.stock-block'), round: rect('.round-wrap'), board: rect('.board-play'),
      controls: rect('.play-actions'), own: rect('.own-seat'), hand: rect('.hand-scroll'),
      lowCardEdge: Math.max(0, ...Array.from(node.querySelectorAll('.hand .playing-card'), card => card.getBoundingClientRect().bottom)),
      scrollWidth: document.documentElement.scrollWidth, scrollHeight: document.documentElement.scrollHeight,
    }
  })
  expect(sizes.scrollWidth).toBeLessThanOrEqual(width + 1)
  expect(sizes.scrollHeight, JSON.stringify(sizes)).toBeLessThanOrEqual(height + 1)
  expect(sizes.stock.right).toBeLessThanOrEqual(sizes.board.left + 1)
  expect(sizes.board.right).toBeLessThanOrEqual(sizes.round.left + 1)
  expect(sizes.controls.top).toBeGreaterThan(sizes.board.top)
  expect(sizes.controls.bottom).toBeLessThanOrEqual(sizes.hand.top + 1)
  expect(sizes.own.right).toBeLessThanOrEqual(sizes.hand.left + 1)
  expect(sizes.hand.bottom).toBeLessThanOrEqual(height + 1)
  expect(sizes.lowCardEdge).toBeLessThanOrEqual(sizes.hand.bottom + 1)
  expect(sizes.controls.height).toBeGreaterThanOrEqual(44)
}

async function touchStroke(page: Page, points: { x: number; y: number }[]) {
  const cdp = await page.context().newCDPSession(page)
  try {
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [points[0]] })
    for (const point of points.slice(1)) {
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [point] })
    }
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
  } finally { await cdp.detach() }
}

test('首页、规则、创建与加入切换', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: /^干瞪眼/ })).toBeVisible()
  await capture(page, 'home-desktop.jpg')
  await page.getByRole('button', { name: '玩法规则' }).click()
  await expect(page.getByRole('dialog')).toContainText('V1.3')
  await capture(page, 'rules.jpg')
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '创建房间', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: '加入房间' }).click()
  await capture(page, 'join.jpg')
  await dialog.getByLabel('昵称').fill('小松')
  await dialog.getByLabel('房间号').fill('12')
  await dialog.getByRole('button', { name: '进入牌桌' }).click()
  await expect(dialog.getByRole('alert')).toHaveText('请输入 6 位数字房间号。')
  await dialog.getByRole('button', { name: '创建房间' }).click()
  await expect(dialog.getByRole('group', { name: '牌桌人数' })).toBeVisible()
  await capture(page, 'create.jpg')
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '和 AI 快速开始' }).click()
  await capture(page, 'quick.jpg')
  await page.keyboard.press('Escape')
  await page.setViewportSize({ width: 390, height: 844 })
  await capture(page, 'home-mobile.jpg')
})

test('Vite 开发代理保持 API 与 WebSocket 的同源校验', async ({ page }) => {
  const watched = observe(page)
  await page.goto('http://127.0.0.1:18083/')
  await create(page, '开发联调', 2, 'speed', true)
  await expect(page.getByTestId('hand')).toBeVisible()
  expect(watched.state().payload.players.some(player => player.bot)).toBe(true)
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
    await a.setViewportSize({ width: 667, height: 320 })
    await assertTable(a, 667, 320)
    await expect(a.getByTestId('turn-status')).toContainText('不能仅剩')
    await a.setViewportSize({ width: 1440, height: 900 })
    await card(a, 12).click()
    await play(a).click()
    const dialog = a.getByRole('dialog', { name: '万能牌怎么用？' })
    await expect(dialog.getByRole('button', { name: /顺子/ })).toHaveCount(2)
    await expect(dialog.getByRole('img', { name: '大王 当作 3' })).toBeVisible()
    await capture(a, 'wildcard.jpg')
    await dialog.getByRole('button', { name: /顺子 4 5 6/ }).click()
    await expect(b.getByRole('img', { name: '大王 当作 6' })).toBeVisible()
    await expect(b.getByTestId('turn-status')).toHaveText('无牌可接，可以不出')
    await capture(b, 'table-desktop.jpg')
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
  const { a, b, ac, bc, watchedA } = await fixturePair(browser, 'classic', undefined,
    { viewport: { width: 1440, height: 900 }, hasTouch: true })
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
    await b.setViewportSize({ width: 667, height: 320 })
    await assertTable(b, 667, 320)
    const hand = b.locator('.hand-scroll')
    const firstId = await hand.getByRole('button').first().getAttribute('data-card-id')
    const lastId = await hand.getByRole('button').last().getAttribute('data-card-id')
    await expect(hand.getByRole('button')).toHaveCount(26)
    expect(await hand.evaluate(rail => rail.scrollWidth)).toBeGreaterThan(await hand.evaluate(rail => rail.clientWidth))
    const bounds = (await hand.boundingBox())!
    const y = bounds.y + bounds.height * .56
    await touchStroke(b, [{ x: bounds.x + 220, y }, { x: bounds.x + 176, y }, { x: bounds.x + 52, y }])
    expect(await hand.evaluate(rail => rail.scrollLeft)).toBeGreaterThan(100)
    await expect(b.getByTestId('hand').locator('[aria-pressed=true]')).toHaveCount(0)
    await hand.evaluate(rail => { rail.scrollLeft = rail.scrollWidth })
    await card(b, Number(lastId)).click()
    await expect(card(b, Number(lastId))).toHaveAttribute('aria-pressed', 'true')
    await capture(b, 'long-hand-mobile.jpg')
    await hand.evaluate(rail => { rail.scrollLeft = 0 })
    await card(b, Number(firstId)).click()
    await expect(card(b, Number(firstId))).toHaveAttribute('aria-pressed', 'true')
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
    await capture(a, 'settlement-desktop.jpg')
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

test('回合外预选、滑动选牌和空白清除使用同一操作区', async ({ browser }) => {
  const { a, b, ac, bc, watchedB } = await fixturePair(browser, 'speed', undefined,
    { viewport: { width: 844, height: 390 }, hasTouch: true, isMobile: true })
  try {
    await expect(pass(a)).toBeDisabled()
    await expect(pass(b)).toBeDisabled()
    const left = (await card(b, 16).boundingBox())!
    const right = (await card(b, 24).boundingBox())!
    const y = left.y + left.height * .55
    await touchStroke(b, [{ x: left.x + 15, y }, { x: left.x + 15, y: y - 16 }, { x: right.x + 15, y: y - 16 }])
    for (const id of [16, 20, 24]) await expect(card(b, id)).toHaveAttribute('aria-pressed', 'true')
    expect(watchedB.sent.some(message => message.type === 'preview_play')).toBe(false)
    await expect(play(b)).toBeDisabled()
    const rail = b.locator('.hand-scroll')
    await rail.tap({ position: { x: 5, y: 3 } })
    await expect(b.getByTestId('hand').locator('[aria-pressed=true]')).toHaveCount(0)

    await card(b, 16).tap()
    await expect(card(b, 16)).toHaveAttribute('aria-pressed', 'true')
    const first = (await card(a, 4).boundingBox())!
    const second = (await card(a, 8).boundingBox())!
    await a.mouse.move(first.x + 18, first.y + first.height / 2)
    await a.mouse.down()
    await a.mouse.move(second.x + 18, second.y + second.height / 2, { steps: 4 })
    await a.mouse.up()
    for (const id of [4, 8]) await expect(card(a, id)).toHaveAttribute('aria-pressed', 'true')
    await a.locator('.hand-scroll').click({ position: { x: 5, y: 3 } })
    await card(a, 12).click()
    await play(a).click()
    await expect(play(b)).toBeEnabled()
    expect(watchedB.sent.some(message => message.type === 'preview_play')).toBe(true)
    await play(b).tap()
    await expect(card(b, 16)).toHaveCount(0)
  } finally { await ac.close(); await bc.close() }
})

for (const n of [2, 3, 4, 5]) {
  test((n + 1) + ' 张炸弹由服务端结算并显示对应动效', async ({ page }) => {
    await page.goto(fixture)
    await create(page, '小竹', n, 'speed', true)
    await expect(page.getByTestId('turn-status')).toHaveText('轮到你领牌')
    const bomb = n === 2 ? [4, 52, 53] : [8, 9, 10, 11, 52, 53].slice(0, n + 1)
    for (const id of bomb) await card(page, id).click()
    await play(page).click()
    if (n === 2) {
      await page.getByRole('dialog', { name: '万能牌怎么用？' }).getByRole('button', { name: /3 张炸弹/ }).click()
    }
    const played = page.locator('.public-play[data-bomb-size="' + (n + 1) + '"]')
    await expect(played).toHaveClass(/blast/)
    await expect(played.locator('.playing-card')).toHaveCount(n + 1)
    if (n === 5) await expect(page.getByRole('dialog', { name: '本局结算' })).toContainText('小竹 获胜')
  })
}

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
          if (n === 5 && mode === 'classic') await capture(page, 'lobby-mobile.jpg')
          await page.getByRole('button', { name: '开始游戏' }).click()
        }
        const board = table(page)
        await expect(board).toBeVisible()
        await expect(board.locator('.opponent')).toHaveCount(n - 1)
        expect(await board.locator('.opponent').evaluateAll(nodes => nodes.map(node => node.getAttribute('data-position')))).toEqual(seatOrder[n - 2])
        await expect(page.getByTestId('hand').getByRole('button').first()).toBeVisible()
        await expect.poll(() => watch.state().payload.currentPlayerId).toBeTruthy()
        expect(watch.state().payload.mode).toBe(mode)
        expect(watch.state().payload.players.filter(p => p.bot)).toHaveLength(n - 1)
        await expect.poll(() => watch.state().payload.currentPlayerId === watch.state().payload.selfPlayerId).toBe(true)
        if (await page.getByRole('button', { name: '提示', exact: true }).isEnabled()) {
          await page.getByRole('button', { name: '提示', exact: true }).tap()
          await expect(play(page)).toBeEnabled()
        }
        await assertTable(page, width, height)
        const selected = await page.getByTestId('hand').locator('[aria-pressed=true]').count()
        if (mode === 'classic') await capture(page, 'table-mobile-' + n + 'p.jpg')
        for (const [wide, tall] of [[667, 320], [1280, 720], [1440, 900], [1920, 1080], [390, 844]]) {
          await page.setViewportSize({ width: wide, height: tall })
          await assertTable(page, wide, tall)
          await expect(page.getByTestId('hand').locator('[aria-pressed=true]')).toHaveCount(selected)
          if (mode === 'classic' && wide === 1440) await capture(page, 'table-desktop-' + n + 'p.jpg')
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
    await expect(table(a)).toBeVisible()
    await expect(table(b)).toBeVisible()
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
