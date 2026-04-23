package bridge

const JSBridge = `
(function() {
    const TrakandCore = {
        lastProcessed: new Set(),
        Store: null,
        lastQR: null,
        lastState: null,
        isReady: false,

        init() {
            console.log("Trakand Reach Bridge: Initializing...");
            this.hookInternalStore();
            this.setupDeepInterceptor();
            this.startStateMonitor();
        },

        hookInternalStore() {
            let attempts = 0;
            const interval = setInterval(() => {
                attempts++;
                try {
                    // Try multiple detection methods
                    if (window.require) {
                        try {
                            const modules = ["WAWebStore", "WAWebChat", "WAWebMsg", "WAWebConn"];
                            let found = {};
                            for (const m of modules) {
                                try { found[m] = window.require(m); } catch(e) {}
                            }
                            if (found.WAWebStore || (found.WAWebChat && found.WAWebMsg)) {
                                this.Store = found.WAWebStore || { Chat: found.WAWebChat, Msg: found.WAWebMsg, Conn: found.WAWebConn };
                                this.finalizeStore();
                                clearInterval(interval);
                                return;
                            }
                        } catch(e) {}
                    }

                    if (window.mR && window.mR.modules) {
                        const modules = window.mR.modules;
                        const store = Object.values(modules).find(m =>
                            m.exports && m.exports.default && m.exports.default.Chat && m.exports.default.Msg
                        );
                        if (store) {
                            this.Store = store.exports.default;
                            this.finalizeStore();
                            clearInterval(interval);
                            return;
                        }
                    }

                    // Backup: brute force search in webpack modules if available
                    if (window.webpackChunkwhatsapp_web_client) {
                        // This is more complex, but a common fallback
                    }

                } catch (e) {
                    if (attempts > 100) clearInterval(interval);
                }
            }, 500);
        },

        finalizeStore() {
            window.WWebStore = this.Store;
            this.bindEvents();
            this.isReady = true;
            console.log("Trakand Bridge: Internal Store Hooked ✅");
        },

        bindEvents() {
            if (!this.Store || !this.Store.Msg) return;

            // Evolution-style: listen for multiple events
            if (this.Store.Msg.on) {
                this.Store.Msg.on('add', (msg) => {
                    if (msg.isNewMsg && !this.lastProcessed.has(msg.id._serialized)) {
                        this.processIncoming(msg);
                    }
                });
            }
        },

        processIncoming(msg) {
            const data = {
                id: msg.id._serialized,
                body: msg.body || msg.caption || "",
                type: msg.type,
                t: msg.t,
                from: msg.from._serialized || msg.from,
                to: msg.to._serialized || msg.to,
                self: msg.id.fromMe ? "out" : "in",
                isGroup: msg.isGroupMsg,
                pushname: msg.sender?.pushname || msg.pushname || ""
            };
            this.lastProcessed.add(data.id);
            if (window.trakand_emit) {
                window.trakand_emit('message_new', data);
            }

            if (this.lastProcessed.size > 2000) {
                const arr = Array.from(this.lastProcessed);
                this.lastProcessed = new Set(arr.slice(1000));
            }
        },

        async sendMessage(to, text) {
            try {
                if (!this.Store) throw new Error("Store not ready");

                const jid = to.includes('@') ? to : to + '@c.us';

                // Advanced sending logic (Evolution style)
                let chat;
                if (this.Store.Chat.get) {
                    chat = this.Store.Chat.get(jid);
                }

                if (!chat && this.Store.Chat.find) {
                    chat = await this.Store.Chat.find(jid);
                }

                if (!chat) throw new Error("Chat not found and could not be created");

                // Try multiple sending methods for resilience
                if (this.Store.SendTextMsgToChat) {
                    await this.Store.SendTextMsgToChat(chat, text);
                } else if (chat.sendMessage) {
                    await chat.sendMessage(text);
                } else if (this.Store.Msg.sendMessage) {
                    // Create a dummy message object if needed or use internal msg utils
                    throw new Error("Generic send method not found");
                }

                return { success: true, jid: jid };
            } catch (e) {
                console.error("Bridge Send Error:", e);
                return { success: false, error: e.toString() };
            }
        },

        setupDeepInterceptor() {
            const observer = new MutationObserver((mutations) => {
                for (const mutation of mutations) {
                    for (const node of mutation.addedNodes) {
                        if (node.nodeType === 1) this.handleNewNode(node);
                    }
                }
            });
            observer.observe(document.body, { childList: true, subtree: true });
        },

        handleNewNode(node) {
            // Simplified UI interception as backup to Store hooks
            const selectors = ['.message-in', '[data-id]', '[data-testid="msg-container"]'];
            const isMsg = selectors.some(s => node.matches && (node.matches(s) || node.querySelector(s)));
            if (!isMsg) return;

            try {
                const textNode = node.querySelector('span.selectable-text, .copyable-text span, [data-testid="selectable-text"]');
                const msgId = node.getAttribute('data-id') || node.closest('[data-id]')?.getAttribute('data-id');

                if (!textNode || !msgId || this.lastMsgId === msgId) return;
                this.lastMsgId = msgId;

                if (window.trakand_emit) {
                    window.trakand_emit('message_new_ui', {
                        text: textNode.innerText,
                        msgId: msgId,
                        timestamp: Date.now()
                    });
                }
            } catch (e) {}
        },

        startStateMonitor() {
            setInterval(() => {
                try {
                    const qrDiv = document.querySelector('div[data-ref]');
                    const qr = qrDiv ? qrDiv.getAttribute('data-ref') : null;

                    if (qr && qr !== this.lastQR) {
                        this.lastQR = qr;
                        if (window.trakand_emit) window.trakand_emit('qr', qr);
                    }

                    const isLogged = !!document.querySelector('#pane-side, [data-testid="chat-list"]');
                    const state = qr ? 'connecting' : (isLogged ? 'open' : 'connecting');

                    if (state !== this.lastState) {
                        this.lastState = state;
                        const profile = {
                            state: state,
                            owner_jid: localStorage.getItem('last-wid-md') || localStorage.getItem('last-wid'),
                            profile_name: localStorage.getItem('last-pushname'),
                        };
                        if (window.trakand_emit) window.trakand_emit('connection_update', profile);
                    }
                } catch (e) {}
            }, 2000);
        }
    };

    if (document.readyState === 'loading') {
        window.addEventListener('DOMContentLoaded', () => TrakandCore.init());
    } else {
        TrakandCore.init();
    }
    window.trakand_bridge = TrakandCore;
})();
`
