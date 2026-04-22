package bridge

const JSBridge = `
(function() {
    const TrakandCore = {
        lastProcessed: new Set(),
        Store: null,
        lastQR: null,
        lastState: null,

        init() {
            console.log("Trakand Reach Bridge: Initializing...");
            this.hookInternalStore();
            this.setupDeepInterceptor();
            this.startStateMonitor();
        },

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
            if (!this.Store || !this.Store.Msg) return;
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
            if (window.trakand_emit) {
                window.trakand_emit('message_new', data);
            }

            if (this.lastProcessed.size > 1000) {
                const arr = Array.from(this.lastProcessed);
                this.lastProcessed = new Set(arr.slice(500));
            }
        },

        async sendMessage(to, text) {
            try {
                if (!this.Store || !this.Store.Chat) throw new Error("Store not ready");
                const jid = to.includes('@') ? to : to + '@c.us';
                let chat = this.Store.Chat.get(jid);
                if (!chat) {
                    chat = await this.Store.Chat.find(jid);
                }
                await this.Store.SendTextMsgToChat(chat, text);
                return { success: true, jid: jid };
            } catch (e) {
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
            const selectors = ['.message-in', '[data-id]', '[data-testid="msg-container"]'];
            const isMsg = selectors.some(s => node.matches && (node.matches(s) || node.querySelector(s)));
            if (!isMsg) return;

            try {
                const textNode = node.querySelector('span.selectable-text, .copyable-text span, [data-testid="selectable-text"]');
                const msgId = node.getAttribute('data-id') || node.closest('[data-id]')?.getAttribute('data-id');

                if (!textNode || !msgId || this.lastMsgId === msgId) return;
                this.lastMsgId = msgId;

                const jidMatch = msgId.match(/_([0-9]+)@/);
                const sender_id = jidMatch ? jidMatch[1] : 'unknown';

                if (window.trakand_emit) {
                    window.trakand_emit('message_new_ui', {
                        text: textNode.innerText,
                        msgId: msgId,
                        sender_id: sender_id,
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
            }, 1500);
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
