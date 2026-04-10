##### openab-go 多平台支援實作計劃

---

##### 目前架構（重構後）

```
main.go                 ← 依 config 註冊 platform.Platform，啟動所有已啟用平台
platform/
  platform.go           ← Platform interface (Start / Stop) + SplitMessage 共用工具
discord/
  adapter.go            ← Discord 實作 Platform 介面
  handler.go            ← Discord 訊息處理、thread 管理、streaming
  reactions.go          ← Discord emoji reaction 狀態機
config/
  config.go             ← 多平台設定結構（DiscordConfig / TelegramConfig / TeamsConfig）
acp/
  connection.go         ← ACP JSON-RPC 連線管理（平台無關）
  pool.go               ← SessionPool（平台無關）
  protocol.go           ← JSON-RPC 訊息型別 + 事件分類（平台無關）
```

擴展新平台只需：
1. 建立 `telegram/` 或 `teams/` 套件，實作 `platform.Platform`
2. 在 `config/config.go` 新增對應設定區段（struct 已預留）
3. 在 `main.go` 加入 `if cfg.Telegram.Enabled { ... }` 註冊

`acp/` 和 `platform/` 完全不需要動。

---

##### Telegram 實作計劃

##### 1. 新增依賴

```
go get github.com/go-telegram-bot-api/telegram-bot-api/v5
```

##### 2. 設定格式

```toml
[telegram]
enabled = true
bot_token = "${TELEGRAM_BOT_TOKEN}"
allowed_chats = [123456789, -100987654321]   # chat / group ID (int64)
```

`config.TelegramConfig` 已預留。

##### 3. 建立 telegram/ 套件

```
telegram/
  adapter.go      ← NewAdapter() + Start() / Stop()
  handler.go      ← 訊息處理邏輯
```

##### adapter.go

- `NewAdapter(cfg config.TelegramConfig, pool *acp.SessionPool)` 建立 bot API 實例
- `Start()` 啟動 long-polling（`tgbotapi.NewUpdate(0)`），在 goroutine 裡跑 update loop
- `Stop()` 呼叫 `bot.StopReceivingUpdates()` 並等待 goroutine 結束

##### handler.go — 訊息處理流程

```
收到 Update
  → 檢查 update.Message != nil
  → 檢查 ChatID 在 allowed_chats 內
  → threadKey = fmt.Sprintf("tg_%d", chatID)  // 避免和 Discord thread ID 衝突
  → pool.GetOrCreate(threadKey)
  → pool.WithConnection(threadKey, func(conn) {
        notifyCh, reqID := conn.SessionPrompt(text)
        // 收集 AcpEventText 串流
        // 每 N 秒用 editMessageText 更新同一則訊息（Telegram 有 rate limit）
        // 完成後送出最終訊息
    })
```

##### 關鍵差異 vs Discord

| 面向 | Discord | Telegram |
|------|---------|----------|
| 訊息上限 | 2000 字元 | 4096 字元 |
| 串流更新 | editMessage，每 1.5 秒 | editMessageText，需注意 rate limit（同 chat 約 1 msg/sec） |
| thread 概念 | 原生 thread | reply_to_message_id 模擬 thread，或用 Telegram topic |
| 狀態指示 | emoji reaction | `sendChatAction("typing")`，每 5 秒重送 |
| 身份辨識 | mention `<@botID>` | 私訊直接觸發；群組用 `/command` 或 `@bot_username` |
| 格式 | Markdown | MarkdownV2（需跳脫特殊字元） |

##### 4. 串流策略

Telegram `editMessageText` 有 rate limit，建議：
- 先送一則 "..." placeholder（`sendMessage`）
- 用 ticker 每 2 秒 `editMessageText` 更新一次
- 完成後做最後一次 edit
- 超過 4096 字元時用 `platform.SplitMessage(text, 4096)` 拆分，後續 chunk 用 `sendMessage`

##### 5. 在 main.go 註冊

```go
if cfg.Telegram.Enabled {
    adapter, err := telegram.NewAdapter(cfg.Telegram, pool)
    if err != nil {
        slog.Error("failed to create telegram adapter", "error", err)
        os.Exit(1)
    }
    platforms = append(platforms, adapter)
}
```

---

##### Teams 實作計劃

##### 1. 新增依賴

Teams Bot 使用 Azure Bot Framework，Go 生態沒有官方 SDK，選項：
- **infobip/go-bot-framework** — 社群維護的 Bot Framework SDK
- **自行實作** — Teams 透過 webhook 推送 Activity JSON，用 HTTP server 接收即可

建議自行實作 HTTP handler，依賴輕量：

```
go get github.com/golang-jwt/jwt/v5   # 驗證 Bot Framework token
```

##### 2. 設定格式

```toml
[teams]
enabled = true
app_id = "${TEAMS_APP_ID}"
app_secret = "${TEAMS_APP_SECRET}"
tenant_id = "${TEAMS_TENANT_ID}"
listen_addr = ":3978"                  # Bot Framework messaging endpoint
```

需要擴展 `TeamsConfig`：

```go
type TeamsConfig struct {
    Enabled    bool   `toml:"enabled"`
    AppID      string `toml:"app_id"`
    AppSecret  string `toml:"app_secret"`
    TenantID   string `toml:"tenant_id"`
    ListenAddr string `toml:"listen_addr"`
}
```

##### 3. 建立 teams/ 套件

```
teams/
  adapter.go      ← NewAdapter() + Start() (HTTP server) / Stop()
  handler.go      ← Activity 處理邏輯
  auth.go         ← Bot Framework JWT token 驗證
```

##### adapter.go

- `Start()` 啟動 `http.Server`，監聽 `/api/messages`
- `Stop()` 呼叫 `server.Shutdown(ctx)`

##### handler.go — Activity 處理流程

```
POST /api/messages
  → 驗證 Authorization header（Bot Framework JWT）
  → 解析 Activity JSON
  → 檢查 activity.Type == "message"
  → threadKey = fmt.Sprintf("teams_%s", activity.Conversation.ID)
  → pool.GetOrCreate(threadKey)
  → pool.WithConnection(threadKey, func(conn) {
        notifyCh, reqID := conn.SessionPrompt(activity.Text)
        // 收集回覆
        // 用 Bot Framework REST API 回覆：
        //   POST {serviceUrl}/v3/conversations/{conversationId}/activities
    })
```

##### 關鍵差異 vs Discord

| 面向 | Discord | Teams |
|------|---------|-------|
| 傳輸方式 | WebSocket (gateway) | HTTP webhook (incoming) + REST API (outgoing) |
| 訊息上限 | 2000 字元 | ~28KB（Adaptive Card），純文字約 25000 字元 |
| 串流更新 | editMessage | updateActivity（支援，但有延遲） |
| 身份驗證 | Bot token | Azure AD + Bot Framework JWT |
| 格式 | Markdown | Adaptive Cards 或 HTML subset |
| 部署需求 | 只需出站連線 | 需要公開 HTTPS endpoint（或用 ngrok / Azure Bot Service） |

##### auth.go — Token 驗證

Bot Framework 送來的每個 request 都帶 `Authorization: Bearer <JWT>`，需要：
1. 從 `https://login.botframework.com/v1/.well-known/openidconfiguration` 取得 JWKS
2. 驗證 JWT 簽章、issuer、audience（= AppID）
3. 快取 JWKS（建議 24 小時 TTL）

##### 4. 串流策略

Teams 支援 `updateActivity` 但延遲較高，建議：
- 先回一則 "thinking..." placeholder
- 完成後一次性 `updateActivity` 替換為完整回覆
- 不做中間串流更新（避免 rate limit 和體驗不佳）
- 超長回覆可用 Adaptive Card 的 scrollable container

---

##### 實作優先順序建議

| 順序 | 項目 | 理由 |
|------|------|------|
| 1 | Telegram | 生態成熟（go-telegram-bot-api）、long-polling 不需公開 endpoint、與 Discord 模式最接近 |
| 2 | Teams | 需要 HTTP server + Azure AD 驗證，複雜度較高、需要公開 HTTPS endpoint |

##### 預估工作量

| 項目 | 核心程式碼 | 測試 | 合計 |
|------|-----------|------|------|
| Telegram adapter | ~200 行 | ~100 行 | ~300 行 |
| Teams adapter + auth | ~400 行 | ~150 行 | ~550 行 |
