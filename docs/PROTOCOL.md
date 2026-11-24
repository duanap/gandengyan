# HTTP / WebSocket 通信协议

> 协议 v1，配套规则 V1.3。下方保留设计示例，当前实现补充见末节。

## 1. 原则

服务端权威；所有房间游戏命令串行化；使用 JSON；HTTPS/WSS；不发送全量内部状态给客户端；连接恢复依赖最新玩家专属快照。所有错误响应不得附带其他玩家的隐藏信息。

## 2. HTTP 接口

| 方法 | 路径 | 请求 | 返回 |
| --- | --- | --- | --- |
| GET | `/healthz` | 无 | `{"status":"ok"}` |
| POST | `/api/rooms` | 昵称、人数、模式 | roomId、playerId、sessionToken、协议版本 |
| POST | `/api/rooms/join` | 房间号、昵称 | 同上 |
| GET | `/api/version` | 无 | 构建版本、协议版本（不暴露内部配置） |

创建示例：

```json
{
  "nickname": "玩家A",
  "capacity": 4,
  "mode": "classic"
}
```

加入示例：

```json
{
  "roomId": "100001",
  "nickname": "玩家B"
}
```

成功示例（示例 token 非真实凭证）：

```json
{
  "roomId": "100001",
  "playerId": "p1",
  "sessionToken": "opaque-example-token",
  "protocolVersion": 1
}
```

`mode` 只接受 `classic`、`speed`。房间号为长度 6 的数字字符串，服务端分配并检查冲突。昵称去除首尾空白，限制长度（建议 1～16 个 Unicode 字符）并在 UI 使用文本插值而非 HTML；同房昵称可重复，身份以 playerId 区分。HTTP 请求体大小受限；创建/加入限速。错误可使用 400、404、409、429 等合适状态及统一 `{code,message}`。

## 3. WebSocket 连接与身份

地址 `/ws`，与 HTTP 同源部署。连接建立后首条应用消息必须为 `auth`；服务器在认证超时（建议 5 秒）或失败时关闭连接。凭证只在 TLS 保护的请求/认证消息中传输，不放 URL、公开日志或房间广播。生产验证 Origin。

```json
{
  "v": 1,
  "type": "auth",
  "requestId": "req-auth-001",
  "payload": {
    "roomId": "100001",
    "sessionToken": "opaque-example-token"
  }
}
```

认证成功返回 `auth_ok`，并发送最新专属 `room_state`/`game_state`。同一席位的新连接替换旧连接；旧连接不能再提交命令。token 应以安全随机方式生成（至少 128 位随机性），服务端校验其有效期及房间绑定；有效期应覆盖对局与正常重连窗口。前端可存 sessionStorage，刷新恢复；关闭浏览器后的长期恢复非默认承诺。

## 4. 消息信封

客户端命令示例：

```json
{
  "v": 1,
  "type": "play_cards",
  "requestId": "req-play-123",
  "gameId": "g-001",
  "revision": 15,
  "payload": {
    "cardIds": [4, 8, 53],
    "pattern": {
      "type": "straight",
      "rank": 4,
      "length": 3
    }
  }
}
```

ID 4、8 对应点数 4、5，53 为大王；此声明将大王解释为 6，组成 456。服务端依据实际选牌重新解析。

| 字段 | 含义 |
| --- | --- |
| `v` | 协议版本，当前 1 |
| `type` | 消息事件名称 |
| `requestId` | 客户端为命令生成唯一标识 |
| `gameId` | 当前对局 ID；大厅命令按需省略 |
| `revision` | 发出动作时所依据的房间版本 |
| `payload` | 按事件定义的对象 |

服务端消息同样携带 `v`、`type`、`revision`、适用的 `gameId`；响应携带对应的 `requestId`。客户端不得自报可信 playerId，服务端从已认证连接取得身份。对未支持协议版本明确拒绝。

## 5. 客户端事件

| type | payload | 约束 |
| --- | --- | --- |
| `auth` | roomId、sessionToken | 连接首条消息 |
| `start_game` | 空对象 | 房主、目标席位已满且在线、等待阶段 |
| `preview_play` | cardIds | 当前行动者；只读预览，校验 revision |
| `play_cards` | cardIds、pattern | 当前行动者，完整规则校验 |
| `pass` | 空对象 | 当前跟牌阶段且轮到自己 |
| `restart_game` | 空对象 | 房主、已结算且满足开始条件 |
| `leave_room` | 空对象 | 等待/结算可退房；对局中按断线处理 |
| `ping` | 客户端时间或空 | 心跳；不改变游戏状态 |
| `add_bot` | 空对象 | 仅 AI 已实现时开放，房主、等待阶段 |
| `remove_bot` | playerId | 仅 AI 已实现时开放，房主、等待阶段 |

未实现的 AI 或房间操作不得在前端呈现为可用功能；不存在未来胜负模式的协议枚举。

## 6. 牌型声明与预览

玩家选择物理牌后请求 `preview_play`。服务端校验当前行动者、牌 ID、牌型、待接牌、最后一手和剩余手牌限制，返回所有合法逻辑解释。此操作不消耗牌、不改变 revision。

选牌 `[4,8,53]` 在领出且剩余手牌合法时示例：

```json
{
  "v": 1,
  "type": "play_preview",
  "requestId": "req-preview-001",
  "gameId": "g-001",
  "revision": 15,
  "payload": {
    "cardIds": [4, 8, 53],
    "options": [
      {
        "type": "straight",
        "rank": 3,
        "length": 3,
        "wildcards": [{"cardId": 53, "rank": 3}]
      },
      {
        "type": "straight",
        "rank": 4,
        "length": 3,
        "wildcards": [{"cardId": 53, "rank": 6}]
      }
    ]
  }
}
```

若当前待接牌为 345，则仅返回 456 方案。无合法方案时返回明确的原因码及空选项。预览回复到达后若选牌已变或 revision 不匹配，客户端丢弃旧回复。

出牌声明 `type/rank/length` 足以唯一确定标准逻辑牌型；服务端构造规范化 wildcard 映射。类型仅支持 `single`、`pair`、`straight`、`pair_run`、`bomb`。顺子/连对 rank 为起点，length 为牌张数；炸弹 rank 为点数、length 为 3～6。客户端若附加 wildcard 映射，必须与合法声明一致，不能借此改写普通牌。预览并非出牌授权，正式提交仍完整校验。

## 7. 服务端事件

| type | 用途 | 可见性 |
| --- | --- | --- |
| `auth_ok` | 身份认证成功 | 本人 |
| `room_state` | 房间稳定状态快照 | 每连接独立生成 |
| `game_state` | 玩家专属牌局快照 | 每连接独立生成 |
| `play_preview` | 选牌合法解释 | 本人 |
| `action_accepted` | 请求确认 | 请求人 |
| `action_rejected` | 请求拒绝 | 请求人 |
| `cards_played` | 出牌展示事件 | 全房 |
| `player_passed` | 不出展示事件 | 全房 |
| `cards_drawn` | 摸牌展示事件 | 公共数量；具体牌只对本人 |
| `turn_changed` | 行动者变化（可选） | 全房 |
| `private_notice` | 双王提醒等私密事件 | 本人 |
| `game_finished` | 结算事件 | 全房 |
| `pong` | 心跳响应 | 请求人 |

稳定快照为权威；展示事件可按同一 revision 下的 `eventIndex` 排序，或作为快照中的 `events` 数组批量发送。客户端不应仅凭事件做最终扣牌、加牌或计分。展示事件不携带其他玩家手牌和摸到的具体牌。

## 8. 状态快照 DTO

### 8.1 房间状态

```json
{
  "v": 1,
  "type": "room_state",
  "revision": 7,
  "payload": {
    "roomId": "100001",
    "mode": "classic",
    "capacity": 4,
    "phase": "waiting",
    "hostPlayerId": "p1",
    "selfPlayerId": "p2",
    "players": [
      {"playerId": "p1", "seat": 0, "nickname": "玩家A", "online": true, "score": 0},
      {"playerId": "p2", "seat": 1, "nickname": "玩家B", "online": true, "score": 0}
    ],
    "allowedActions": ["leave_room"]
  }
}
```

### 8.2 游戏状态

以下是 2 人示意快照，牌 ID 与牌数仅用于展示字段。

```json
{
  "v": 1,
  "type": "game_state",
  "gameId": "g-001",
  "revision": 15,
  "payload": {
    "roomId": "100001",
    "mode": "speed",
    "phase": "following",
    "roundNumber": 2,
    "selfPlayerId": "p2",
    "currentPlayerId": "p2",
    "lastPlayPlayerId": "p1",
    "passCount": 0,
    "deckCount": 42,
    "players": [
      {"playerId": "p1", "seat": 0, "nickname": "玩家A", "online": true, "handCount": 3, "score": 0},
      {"playerId": "p2", "seat": 1, "nickname": "玩家B", "online": true, "handCount": 5, "score": 0}
    ],
    "selfHand": [4, 8, 16, 17, 53],
    "lastPattern": {
      "type": "straight",
      "rank": 3,
      "length": 3,
      "cardIds": [0, 5, 9],
      "wildcards": []
    },
    "allowedActions": ["play_cards", "preview_play", "pass"],
    "privateNotices": []
  }
}
```

`handCount`、`deckCount` 取真实数量；`selfHand` 仅当前席位的牌。`lastPattern` 仅在跟牌阶段作为待接牌型，新轮领出时为 null；上一轮展示牌可从已公开展示事件获取。结算时包含 `winnerId`、`settlement`，行动者置空、动作禁用；不默认公开失败者的具体手牌，除非未来明确增加结算展示规则。

## 9. 双王私密提醒

```json
{
  "v": 1,
  "type": "private_notice",
  "gameId": "g-001",
  "revision": 16,
  "payload": {
    "noticeId": "g-001-p2-double-joker",
    "code": "DOUBLE_JOKER",
    "message": "你同时持有大小王，请搭配其他手牌使用。双王不能直接出牌或组成王炸。"
  }
}
```

仅目标玩家的出站队列接收此事件；重连快照可带本人已产生提醒，客户端按 noticeId 去重（会话内存/本地会话记录）。其他玩家快照、通用日志、广播事件不包含提示存在与否。

## 10. revision、幂等和错误

- `revision` 是房间内单调递增的权威状态版本。一次成功游戏命令及其自动补牌/结算作为一次原子提交；只读预览、ping 不增加 revision。
- 房间成员或连接状态变化也可能更新 revision；客户端收到过期动作拒绝后刷新快照并让用户重新确认，不自动重放出牌。
- 请求去重键至少包含会话、gameId（大厅使用固定作用域）及 requestId；相同请求 ID 和内容重复到达时返回原确认/拒绝及最新状态，不重复修改牌局。相同 ID 搭配不同内容拒绝。
- 去重缓存设置有界容量与保留时间；过期的旧动作还必须被 gameId/revision/行动权校验拦截。
- 权威快照丢失时通过连接恢复获取最新状态；客户端忽略较旧 revision。对同版本的展示事件按 eventIndex 去重。

建议错误码：

| code | 含义 |
| --- | --- |
| `UNAUTHORIZED` | 会话无效或连接已被替换 |
| `ROOM_NOT_FOUND` / `ROOM_FULL` | 房间不存在或已满 |
| `INVALID_PHASE` | 当前阶段不允许操作 |
| `NOT_YOUR_TURN` | 非当前行动者 |
| `NOT_HOST` | 非房主 |
| `INVALID_CARD_IDS` | 牌 ID 重复、不存在或不属于本人 |
| `INVALID_PATTERN` | 非法牌型/声明 |
| `PATTERN_CHOICE_REQUIRED` | 多种解释且未明确选择 |
| `CANNOT_BEAT` | 不符合接牌规则 |
| `INVALID_REMAINING_SPECIALS` | 将仅剩单/对 2 或王等禁止余牌 |
| `INVALID_FINAL_PLAY` | 试图以单 2 或对 2 等非法牌型收尾 |
| `LEADING_CANNOT_PASS` | 领出阶段不允许不出 |
| `STALE_STATE` | gameId 或 revision 过期 |
| `GAME_FINISHED` | 对局已结束 |
| `DUPLICATE_REQUEST_CONFLICT` | requestId 被不同内容重复使用 |
| `RATE_LIMITED` / `SERVER_BUSY` | 请求频率或队列超限 |

错误示例：

```json
{
  "v": 1,
  "type": "action_rejected",
  "requestId": "req-play-123",
  "gameId": "g-001",
  "revision": 15,
  "payload": {
    "code": "INVALID_REMAINING_SPECIALS",
    "message": "不能仅剩单张2、对2或单独的王，请调整出牌组合。"
  }
}
```

## 11. 网络与安全默认值

应用心跳建议 20～30 秒，服务端超时与 Nginx 读取超时应大于合理的心跳间隔；网络中断用指数退避（带抖动，例如 0.5～10 秒），前端显示重连状态，成功后获取快照并恢复操作。普通 WebSocket Ping/Pong 与应用层 ping 可择一主用，不要求浏览器自行发送协议级 ping 帧。

限制请求体、消息长度（建议先设 32 KiB 并测试完整快照）、牌 ID 数量（最多 54）、连接数、房间数和每会话请求速率；对非法消息明确拒绝或关闭异常连接。TLS、可信 Origin、身份校验、文本转义、有界队列、读写超时和日志脱敏应在生产发布前验证。

## 12. 协议验收

使用至少两条独立连接验证相同对局的各自快照；检查所有消息类型而非仅检查 UI。重点测试跨玩家伪造、重复提交、过期 revision、错误声明、重连替换旧连接、补牌私密内容和双王提醒。任何一条通用广播泄露隐藏信息都视为发布阻断。

## 13. 当前实现字段

- `POST /api/rooms/quick` 与创建请求相同，补齐 AI，首位玩家认证后发牌。权威人数、玩法约束与普通建房相同。
- 等待状态发 `room_state`，进行中和结算发 `game_state`。后者也包含房主、容量和 `leaderPlayerId`。玩家 DTO 含 `bot`，本人行动时可读取 `canPlay`。
- `hint` 是只读私有请求，携带空对象。返回 `play_hint`，内容为一个可执行的牌型或 `null`。客户端仍须走预览及正式提交。预览失败在 `play_preview.payload.problem` 返回原因及空选项。
- 展示事件放在 `game_state.payload.events`，`eventIndex` 在批内单调递增。发牌为 `game_started`，出牌对象放在 `cards_played.pattern`。摸牌事件只含玩家 ID 与张数，摸到的牌通过各自 `selfHand` 更新。双王提醒在私有 `privateNotices` 中携带，同一提醒的 ID 在整局内稳定。
- 所有状态变更命令均校验 `revision`，有牌局时也要求当前 `gameId`。幂等缓存针对状态变更命令，每真人席位保留最近 256 条、最长 10 分钟。只读查询只校验当前版本，不改变状态。
- 原生 WebSocket Ping 每 25 秒发送，65 秒未收到 Pong 终止连接。另外支持应用层 `ping`/`pong`。认证失败或房间已回收使用关闭码 4003，替换旧连接用 4001，主动退房用 4004，不兼容版本用 4006。正常重新连接获取完整快照。
- 本地直连接受与当前请求同源的 WebSocket Origin。生产允许的 Origin 列表由配置指定，Nginx 终止 TLS。默认真人会话在 24 小时内可重连，每次认证刷新时限。房间与会话均在进程内存中。
