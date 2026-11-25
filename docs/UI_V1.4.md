# V1.4 统一牌桌与交互

## 牌桌

桌面与手机横屏渲染同一组件树。顶栏显示游戏名、房间号、模式、连接状态和规则入口。牌桌依次排列牌堆、公共牌和轮次，操作区位于公共牌下方。自己的头像、身份、余牌和积分在左下，手牌在底部中央。

| 人数 | 从本人出发的逆时针顺序 |
| --- | --- |
| 2 人 | 顶部中央 |
| 3 人 | 右上 → 左上 |
| 4 人 | 右侧 → 顶部中央 → 左侧 |
| 5 人 | 右侧 → 右上 → 左上 → 左侧 |

有行动权的席位使用金色描边。头像、木框纹理、绒布纹理、牌背与牌面独立绘制。按钮位置固定，首出禁用「不出」，无牌可接时突出该按钮。

## 交互与动效

- 可以点击或拖动选择手牌，点击手牌区空白清空。触屏横滑滚动，先纵向移动再横扫则连续选牌。
- 对手回合可预选。轮到本人后按最新牌局向服务端申请预览，出牌仍由服务端校验。
- 多义万能牌显示实际牌组和王的代用点数。双王提示继续只对本人可见。
- 发牌、摸牌、出牌位置、万能牌标记、三至六炸、轮次和获胜采用短动效。视觉播放独立于权威状态，重连跳过旧动效，支持减少动态效果偏好。
- 首页突出 AI 快速开始、创建和加入。等待房间也使用围桌席位，创建与加入弹窗共用切换栏。功能弹窗统一暖色视觉。

## 验收

2026-11-25 在本地完成 Go 单测、`go vet`、`go test -race`、前端类型检查、3 项前端单测和嵌入构建。Chromium 端到端测试 19 项全部通过。

- 2～5 人的经典及极速牌局均连接实际服务。覆盖桌面 1280×720、1440×900、1920×1080，手机横屏 667×320、844×390、932×430，以及竖屏 390×844。
- 保持选牌跨视口变化，验证鼠标拖选、触屏连续选牌、触屏横向滚动、点击空白清除和回合外预选。短屏检查公共牌堆、牌组、轮次、控件和手牌的几何边界，包含 26 张手牌。
- 验证万能牌预览、非法余牌提示、双王私密提示、三至六炸、经典及极速补牌、牌堆用尽、结算、再开一局、刷新恢复和模拟弱网。
- 构建产物中 JS 约 42.75 KB gzip，CSS 约 9.82 KB gzip，头像 WebP 约 31.83 KB。动效使用 CSS，前端依赖清单沿用原版本。

Android 与 iOS 真机待验证。腾讯云线上版本继续运行 `b5d338e49337`。

## 实际运行截图

以下来自本地 Chromium 与实际 Go 服务。手机截图使用浏览器设备模拟。

| 人数 | PC 1440×900 | 手机横屏 932×430 |
| --- | --- | --- |
| 2 人 | [桌面](screenshots/v1.4/table-desktop-2p.jpg) | [横屏](screenshots/v1.4/table-mobile-2p.jpg) |
| 3 人 | [桌面](screenshots/v1.4/table-desktop-3p.jpg) | [横屏](screenshots/v1.4/table-mobile-3p.jpg) |
| 4 人 | [桌面](screenshots/v1.4/table-desktop-4p.jpg) | [横屏](screenshots/v1.4/table-mobile-4p.jpg) |
| 5 人 | [桌面](screenshots/v1.4/table-desktop-5p.jpg) | [横屏](screenshots/v1.4/table-mobile-5p.jpg) |

- [PC 首页](screenshots/v1.4/home-desktop.jpg) · [手机首页](screenshots/v1.4/home-mobile.jpg) · [PC 等待房间](screenshots/v1.4/lobby-desktop.jpg) · [手机等待房间](screenshots/v1.4/lobby-mobile.jpg)
- [创建](screenshots/v1.4/create.jpg) · [加入](screenshots/v1.4/join.jpg) · [AI 快速开始](screenshots/v1.4/quick.jpg) · [规则](screenshots/v1.4/rules.jpg)
- [万能牌](screenshots/v1.4/wildcard.jpg) · [结算](screenshots/v1.4/settlement-desktop.jpg) · [短屏 26 张手牌](screenshots/v1.4/long-hand-mobile.jpg) · [真实两人局公共牌组](screenshots/v1.4/table-desktop.jpg)

## 头像素材

使用内置 ImageGen 生成 [头像图册](../web/src/assets/table-avatars.webp)，然后压缩为 576×384 的 WebP，约 32 KB。组件以背景位置显示各角色。生成提示如下。

> Generate one rectangular 3-column by 2-row UI avatar sprite atlas for a cozy Chinese paper-card game. Six exactly equal square tiles, edge-to-edge, aligned to a clean mathematical 3x2 grid; no gutter, no text, no borders. Center each bust within its own tile, same scale with faces in the middle 60%, so the game can apply its own circular frame. Reference the attached game mockup ONLY for the little cartoon portrait art style, warm painted 2.5D lighting, and friendly expressions. Top row: young Chinese woman with brown hair and peach blossom hairpin on warm rose, young Chinese man in blue cap on dusky blue, young Chinese man with round glasses on sage. Bottom row: smiling Chinese man with short dark hair on muted lavender, smiling Chinese woman with short dark hair on soft golden sand, small round ivory robot with teal light eyes on muted cool blue. Original charming richly painted cartoon character portraits, tidy silhouettes. Only the 6 avatars. Do not render the screen, table, cards, UI, labels, gold circular frames, or phone shell. Landscape 3:2 aspect ratio.
