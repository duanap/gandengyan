# 干瞪眼

2–5 人的实时纸牌游戏。和好友建房，或直接与 AI 开一局。经典模式每轮全员补一张，极速模式每轮只有最后成功出牌者补一张。第一位合法出完手牌的玩家获胜。

[在线游戏](https://gdy.duanap.cn/) · [腾讯云首发记录](docs/TENCENT_RELEASE_2026-11-24.md) · [V1.4 开发记录与截图](docs/UI_V1.4.md)

## 本地开发

需要 Go 1.24+、Node.js 22.12+ 和 npm。两个终端分别执行：

```sh
cd server
go run ./cmd/server
```

```sh
cd web
npm ci
npm run dev
```

浏览器打开 `http://127.0.0.1:5173`。Vite 将 `/api` 和 `/ws` 转发至本机 Go 服务。验证两位真人联机时，使用独立浏览器窗口或浏览器配置，确保每位玩家有独立会话。生产单文件构建后可直接访问 Go 服务端口。

## 构建与运行

```sh
node scripts/build.mjs
```

Windows 输出 `dist/gandengyan.exe`，Linux 输出 `dist/gandengyan`。加 `--linux` 可生成 Linux amd64 二进制。构建同时输出 SHA-256 文件和版本元数据。运行示例：

```powershell
.\dist\gandengyan.exe -listen 127.0.0.1:8080
```

访问 `http://127.0.0.1:8080`。运行参数包括 `-listen`、`-config`、`-version`。Linux 反向代理、TLS、systemd、维护窗口和回滚步骤见 [部署手册](docs/DEPLOYMENT.md)。

## 开发与验收

```sh
make test
make simulate
make build
cd web
npx playwright install chromium
npm run e2e
```

Windows 可直接在对应目录运行验收报告中的 Go 和 npm 命令。全量模拟默认每个人数与模式组合 1,250 局，`dist/simulation.jsonl` 含逐局种子和结果。浏览器 E2E 拉起实际嵌入构建、确定性测试牌局服务和 Vite 开发代理。

| 目录 | 内容 |
| --- | --- |
| `server/cmd/` | HTTP/WebSocket 服务入口、无网络模拟工具 |
| `server/internal/card, rules, game` | 物理牌、纯规则、权威状态机和结算 |
| `server/internal/room, session, gateway, protocol` | 串行房间、会话、网络入口、私有与公共 DTO |
| `server/internal/ai` | 可见信息视图、手牌规划和有限推演 |
| `web/src/` | Vue 3 + Pinia、通信、桌面与移动布局 |
| `web/tests/` | 前端单测与 Chromium 端到端验收 |
| `scripts/`、`deploy/` | 构建脚本、Linux 配置模板 |

[正式规则 V1.3](docs/RULES.md) · [架构](docs/ARCHITECTURE.md) · [通信协议](docs/PROTOCOL.md) · [验收报告与截图](docs/ACCEPTANCE.md)

房间、牌局和累计积分存于服务进程内存，重启后会丢失。刷新和短时断网可凭会话恢复原席位。轮到掉线玩家时，牌局等待该玩家重连。
