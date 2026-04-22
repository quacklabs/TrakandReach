# FINAL COMPREHENSIVE SPECIFICATION: Trakand Reach Go Port (v1.0)

This document is the absolute, finalized technical blueprint for the Go 1.21+ port of Trakand Reach. It is designed to provide Evolution API-grade reliability using a lightweight Playwright orchestration engine.

---

## 1. Core Architecture & "Evolution-Grade" Concurrency

### 1.1 High-Level Design
- **Language**: Go 1.21+
- **Core Engine**: `playwright-go`
- **Session Manager**: A thread-safe registry (`sync.Map`) managing `Session` objects. Each session represents a unique WhatsApp account identified by its `ID` (Account Key).
- **Event-Driven Core**: Uses Go channels to pipe data from the JS Bridge to:
  - **WebSocket Emitters**: For real-time monitoring/streaming.
  - **Webhook Dispatchers**: For forwarding messages to external microservices.
  - **Persistence Workers**: For logging to SQLite.

### 1.2 Concurrency Primitives
- **Worker Pools**: Use a pool of workers to handle outgoing messages and webhook dispatches to prevent one slow external service from blocking the engine.
- **Session Isolation**: Each browser context is strictly isolated with its own temporary data directory and `context.Context`.

---

## 2. Persistence Layer (SQLite 3 + WAL)

### 2.1 Database Schema
```sql
PRAGMA journal_mode = WAL;

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY, -- The Account Key / Fingerprint
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    browser_type TEXT DEFAULT 'webkit',
    access_key TEXT NOT NULL,
    last_url TEXT,
    webhook_url TEXT, -- Forward incoming messages here
    connection_state TEXT DEFAULT 'created',
    owner_jid TEXT,
    profile_name TEXT
);

CREATE TABLE IF NOT EXISTS message_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT,
    msg_id TEXT UNIQUE,
    remote_jid TEXT,
    body TEXT,
    type TEXT,
    from_me BOOLEAN,
    timestamp INTEGER,
    FOREIGN KEY(session_id) REFERENCES sessions(id)
);
```

---

## 3. The "Deep-Intercept" Bridge (V2.5)

This bridge goes beyond DOM observation. It hooks into the internal WhatsApp Web "Store" to intercept messages at the data layer.

### 3.1 Bridge Source Code (Blueprint)
```javascript
(function() {
    const TrakandCore = {
        lastProcessed: new Set(),
        Store: null,

        init() {
            this.hookInternalStore();
            this.setupDeepInterceptor();
        },

        // Hook into WhatsApp's internal Webpack modules
        hookInternalStore() {
            const interval = setInterval(() => {
                try {
                    if (window.mR && window.mR.modules) {
                        const modules = window.mR.modules;
                        const store = Object.values(modules).find(m =>
                            m.exports && m.exports.default && m.exports.default.Chat && m.exports.default.Msg
                        );
                        if (store) {
                            this.Store = store.exports.default;
                            window.WWebStore = this.Store;
                            this.bindEvents();
                            clearInterval(interval);
                        }
                    }
                } catch (e) {}
            }, 500);
        },

        bindEvents() {
            // Intercept incoming messages directly from the Store
            this.Store.Msg.on('add', (msg) => {
                if (msg.isNewMsg && !this.lastProcessed.has(msg.id._serialized)) {
                    this.processIncoming(msg);
                }
            });
            console.log("Trakand Bridge: Deep Store Hooking Active ✅");
        },

        processIncoming(msg) {
            const data = {
                id: msg.id._serialized,
                body: msg.body || msg.caption || "",
                type: msg.type,
                t: msg.t,
                from: msg.from._serialized,
                to: msg.to._serialized,
                self: msg.id.fromMe ? "out" : "in",
                isGroup: msg.isGroupMsg,
                pushname: msg.sender?.pushname || ""
            };
            this.lastProcessed.add(data.id);
            window.trakand_emit('message_new', data);

            // Cleanup cache to prevent memory bloat
            if (this.lastProcessed.size > 1000) {
                const arr = Array.from(this.lastProcessed);
                this.lastProcessed = new Set(arr.slice(500));
            }
        },

        // Method for Go to call: Reliable Background Sending
        async sendMessage(to, text) {
            try {
                if (!this.Store || !this.Store.Chat) throw new Error("Store not ready");
                const jid = to.includes('@') ? to : to + '@c.us';
                let chat = this.Store.Chat.get(jid);
                if (!chat) {
                    chat = await this.Store.Chat.find(jid);
                }

                // Use the most reliable internal method
                await this.Store.SendTextMsgToChat(chat, text);
                return { success: true, jid: jid };
            } catch (e) {
                return { success: false, error: e.toString() };
            }
        },

        setupDeepInterceptor() {
            // Fallback: Mutation Observer for UI-only environments or if Store fails
            const observer = new MutationObserver((mutations) => {
                // ... DOM-based logic as a failover ...
            });
            observer.observe(document.body, { childList: true, subtree: true });
        }
    };

    TrakandCore.init();
    window.trakand_bridge = TrakandCore; // Expose to Go
})();
```

---

## 4. Message Interception & Forwarding Logic

### 4.1 Forwarding to Services (The "Microservice" Flow)
1. **Bridge** detects message -> Emits `message_new`.
2. **Go Engine** receives event -> Enriches with `session_id` (Account Key).
3. **Webhook Dispatcher** goroutine:
   - Retrieves `webhook_url` for the session from SQLite.
   - POSTs JSON payload with `Retry-Policy` (Exponential backoff).
   - Expected JSON format:
     ```json
     {
       "account": "marketing_1",
       "event": "message",
       "data": { "from": "12345@c.us", "body": "Hello", "id": "msg_001" }
     }
     ```

### 4.2 Replying via TrakandReach
1. **External Service** processes message -> Sends POST to `/reach/send`.
2. **Go Engine** identifies session -> Calls `trakand_bridge.sendMessage(to, text)` via `page.Evaluate`.
3. **Bridge** interacts with `WWebStore` for instant delivery.
4. **Result** is returned to the API caller immediately.

---

## 5. Optimized Binary WebP Streaming

### 5.1 The Binary Frame Protocol
- Each frame is sent as a **Binary Message** (Opcode 2).
- **Header (10 bytes)**:
  - `[0-5]`: `WREACH` (6 Magic bytes)
  - `[6-7]`: `0x0001` (2 bytes, Version)
  - `[8-9]`: Payload type (2 bytes, `0x0001` for WebP Image, `0x0002` for QR)
- **Payload**: Raw WebP buffer.
- **Client Side**: `URL.createObjectURL(new Blob([data.slice(10)], {type: 'image/webp'}))`.

---

## 6. Implementation & TDD Checklist (Low-Level Focus)

### Phase 1: The Core (Foundation)
- [x] Implement `Manager.SendMessage(sessionID, to, text)` with auto-retry.
- [x] Implement `Bridge.hookInternalStore()` with specific targeting for `mR` modules.
- [x] Implement `DB.LogMessage()` to ensure no message is lost.

### Phase 2: Interception
- [x] Verify `Msg.on('add')` captures both Group and Private messages.
- [x] Verify `sender_id` extraction works for international numbers.
- [x] Implement `WebhookForwarder` with a buffer channel.

### Phase 3: Performance
- [x] Benchmark WebP conversion latency using `chai2010/webp`.
- [x] Verify goroutine count remains stable under 100 concurrent sessions.

### Phase 4: API & CLI
- [x] Implement `/reach/session` to accept a `webhook_url` on init.
- [x] Port `setup` command to handle full Linux systemd installation.
