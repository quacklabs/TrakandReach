import asyncio
import json
import logging
import os
import time
import base64
import io
from dataclasses import dataclass, field, asdict
from enum import Enum
from typing import Dict, Optional, Any, List
from pathlib import Path

from playwright.async_api import async_playwright, Browser, BrowserContext, Page
import websockets

# Setup logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("trakand_reach")

class CriticalError(Enum):
    BROWSER_CRASH = 'BROWSER_CRASH'
    SESSION_TIMEOUT = 'SESSION_TIMEOUT'
    CONTEXT_ERROR = 'CONTEXT_ERROR'
    PAGE_CRASH = 'PAGE_CRASH'

@dataclass
class DeviceType:
    type: str
    model: str
    brand: str
    os: str

@dataclass
class DeviceInfo:
    os: str
    userAgent: str
    browser: str
    product: str
    manufacturer: str
    engine: str
    fingerprint: str
    width: int
    height: int
    device: DeviceType
    pixelRatio: float
    dark_mode: bool
    language: str

@dataclass
class Session:
    id: str
    created_at: float
    device_info: DeviceInfo
    browser_type: str
    access_key: str
    last_url: Optional[str] = None
    ws: Optional[websockets.WebSocketServerProtocol] = None
    browser: Optional[Browser] = None
    context: Optional[BrowserContext] = None
    page: Optional[Page] = None
    is_alive: bool = True
    recovery_attempts: int = 0
    screenshot_task: Optional[asyncio.Task] = None
    event_listeners: Dict[str, List[Any]] = field(default_factory=lambda: {
        'qr': [],
        'connection_update': [],
        'message_new': [],
        'creds_update': []
    })

    def emit(self, event: str, *args, **kwargs):
        if event in self.event_listeners:
            for listener in self.event_listeners[event]:
                if asyncio.iscoroutinefunction(listener):
                    asyncio.create_task(listener(*args, **kwargs))
                else:
                    listener(*args, **kwargs)

    def to_dict(self):
        d = {
            "id": self.id,
            "created_at": self.created_at,
            "device_info": asdict(self.device_info),
            "browser_type": self.browser_type,
            "access_key": self.access_key,
            "last_url": self.last_url
        }
        return d

class PlaywrightService:
    _instance = None

    def __new__(cls, *args, **kwargs):
        if not cls._instance:
            cls._instance = super(PlaywrightService, cls).__new__(cls)
        return cls._instance

    def __init__(self, user_data_dir: Optional[str] = None):
        if hasattr(self, '_initialized'):
            return
        self._initialized = True
        self.sessions: Dict[str, Session] = {}
        self.max_recovery_attempts = 3
        self.user_data_path = user_data_dir or os.path.join(os.path.expanduser("~"), ".trakand_reach")
        self.sessions_base_dir = os.path.join(self.user_data_path, 'browserSessions')
        self.metadata_path = os.path.join(self.user_data_path, 'sessions.json')
        os.makedirs(self.sessions_base_dir, exist_ok=True)
        self.pw = None
        self.is_running = False

    async def start(self, auto_resume: bool = True):
        if self.is_running:
            return
        logger.info("⏳ Initializing automation engine...")
        try:
            self.pw = await async_playwright().start()
            self.load_sessions()
            self.is_running = True
            logger.info("Automation engine loaded! ✅")
            if auto_resume:
                asyncio.create_task(self.resume_all_sessions())
        except Exception as e:
            logger.error(f"Failed to initialize engine ❌: {e}")
            raise

    def load_sessions(self):
        if not os.path.exists(self.metadata_path):
            return
        try:
            with open(self.metadata_path, 'r') as f:
                data = json.load(f)
                for sid, sdata in data.items():
                    device_data = sdata['device_info']
                    device_type = DeviceType(**device_data.pop('device'))
                    info = DeviceInfo(**device_data, device=device_type)

                    session = Session(
                        id=sdata['id'],
                        created_at=sdata['created_at'],
                        device_info=info,
                        browser_type=sdata['browser_type'],
                        access_key=sdata['access_key'],
                        last_url=sdata.get('last_url'),
                        is_alive=False # Mark as not yet re-spun
                    )
                    self.sessions[sid] = session
            logger.info(f"Loaded {len(self.sessions)} sessions from metadata.")
        except Exception as e:
            logger.error(f"Failed to load sessions: {e}")

    def save_sessions(self):
        try:
            data = {sid: s.to_dict() for sid, s in self.sessions.items()}
            with open(self.metadata_path, 'w') as f:
                json.dump(data, f, indent=2)
        except Exception as e:
            logger.error(f"Failed to save sessions: {e}")

    async def resume_all_sessions(self):
        logger.info(f"Auto-resuming {len(self.sessions)} sessions...")
        for session_id, session in self.sessions.items():
            if session.last_url:
                try:
                    logger.info(f"Resuming session {session_id} -> {session.last_url}")
                    await self.start_up_link(session_id, session.last_url)
                except Exception as e:
                    logger.error(f"Failed to resume session {session_id}: {e}")

    async def stop(self):
        self.save_sessions()
        self.is_running = False
        for session_id in list(self.sessions.keys()):
            await self.destroy_session(session_id, remove_metadata=False)
        if self.pw:
            await self.pw.stop()

    def parse_browser(self, input_str: str) -> str:
        input_str = input_str.lower()
        if 'firefox' in input_str: return 'firefox'
        if 'chrome' in input_str or 'chromium' in input_str: return 'chromium'
        if 'safari' in input_str or 'webkit' in input_str: return 'webkit'
        return 'webkit'

    async def create_session(self, access_key: str, device_info: Dict[str, Any], browser_type: str = 'webkit') -> Session:
        info = DeviceInfo(
            os=device_info.get('os', 'unknown'),
            userAgent=device_info.get('userAgent', ''),
            browser=device_info.get('browser', 'webkit'),
            product=device_info.get('product', ''),
            manufacturer=device_info.get('manufacturer', ''),
            engine=device_info.get('engine', ''),
            fingerprint=device_info.get('fingerprint', str(time.time())),
            width=device_info.get('width', 1280),
            height=device_info.get('height', 720),
            device=DeviceType(**device_info.get('device', {'type': 'desktop', 'model': 'unknown', 'brand': 'unknown', 'os': 'unknown'})),
            pixelRatio=device_info.get('pixelRatio', 1.0),
            dark_mode=device_info.get('dark_mode', False),
            language=device_info.get('language', 'en-US')
        )

        session_id = info.fingerprint
        if session_id in self.sessions:
            return self.sessions[session_id]

        session = Session(
            id=session_id,
            created_at=time.time(),
            device_info=info,
            browser_type=self.parse_browser(browser_type),
            access_key=access_key
        )
        self.sessions[session_id] = session
        self.save_sessions()
        return session

    async def spin_client(self, session_id: str):
        session = self.sessions.get(session_id)
        if not session:
            raise ValueError(f"Session {session_id} not found")

        if session.browser and session.browser.is_connected():
            return

        user_dir = os.path.join(self.sessions_base_dir, session.device_info.fingerprint)
        os.makedirs(user_dir, exist_ok=True)

        browser_type_name = session.browser_type
        pw_browser_type = getattr(self.pw, browser_type_name)

        options = {
            "user_agent": session.device_info.userAgent,
            "viewport": {"width": session.device_info.width, "height": session.device_info.height},
            "device_scale_factor": session.device_info.pixelRatio,
            "is_mobile": session.device_info.device.type != 'desktop',
            "has_touch": session.device_info.device.type != 'desktop',
            "color_scheme": "dark" if session.device_info.dark_mode else "light",
            "locale": session.device_info.language,
            "bypass_csp": True,
            "ignore_https_errors": True,
        }

        try:
            context = await pw_browser_type.launch_persistent_context(
                user_data_dir=user_dir,
                headless=True,
                args=[
                    '--no-sandbox',
                    '--disable-dev-shm-usage',
                ] if browser_type_name == 'chromium' else [],
                **options
            )
            session.context = context
            session.browser = context.browser
        except Exception as e:
            logger.warning(f"Failed to launch persistent context: {e}. Falling back to non-persistent.")
            browser = await pw_browser_type.launch(headless=True)
            context = await browser.new_context(**options)
            session.browser = browser
            session.context = context

        session.is_alive = True

    async def start_up_link(self, session_id: str, url: str):
        session = self.sessions.get(session_id)
        if not session:
            raise ValueError(f"Session {session_id} not found")

        session.last_url = url
        self.save_sessions()

        await self.spin_client(session_id)

        if not session.page:
            session.page = await session.context.new_page()

        try:
            if "web.whatsapp.com" in url:
                # Inject a robust script to bridge into WhatsApp's internal modules
                await session.context.add_init_script("""
                    window.trakand_reach_bridge = {
                        init: function() {
                            console.log("Trakand Reach Bridge: Initializing...");
                            this.setupObserver();
                            this.tryInjectStore();
                        },
                        tryInjectStore: function() {
                            // Attempt to find WhatsApp internal Store modules
                            const interval = setInterval(() => {
                                if (window.mR) {
                                    const modules = window.mR.modules;
                                    const storeModule = Object.values(modules).find(m => m.exports && m.exports.default && m.exports.default.Chat);
                                    if (storeModule) {
                                        window.WWebStore = storeModule.exports.default;
                                        console.log("Trakand Reach Bridge: Store injected! ✅");
                                        clearInterval(interval);
                                    }
                                }
                            }, 1000);
                        },
                        setupObserver: function() {
                            const observer = new MutationObserver((mutations) => {
                                for (const mutation of mutations) {
                                    mutation.addedNodes.forEach(node => {
                                        if (node.nodeType === 1) {
                                            this.processNode(node);
                                        }
                                    });
                                }
                            });
                            observer.observe(document.body, { childList: true, subtree: true });
                        },
                        processNode: function(node) {
                            // Support multiple WhatsApp Web versions by checking common attributes
                            const isMsg = node.classList.contains('message-in') ||
                                         node.hasAttribute('data-id') ||
                                         node.querySelector('[data-pre-plain-text]');

                            if (isMsg) {
                                try {
                                    const textNode = node.querySelector('span.selectable-text, .copyable-text span');
                                    if (!textNode) return;

                                    const text = textNode.innerText;
                                    const msgId = node.getAttribute('data-id') ||
                                                 node.closest('[data-id]')?.getAttribute('data-id');

                                    if (!msgId) return;

                                    // Extract sender info
                                    let sender_id = 'unknown';
                                    if (msgId.includes('_')) {
                                        const parts = msgId.split('_');
                                        if (parts.length > 1) {
                                            sender_id = parts[1].split('@')[0];
                                        }
                                    }

                                    const meta = node.querySelector('.copyable-text');
                                    const sender = meta ? meta.getAttribute('data-pre-plain-text') : sender_id;

                                    // Prevent duplicate emissions for the same message ID
                                    if (window.last_msg_id === msgId) return;
                                    window.last_msg_id = msgId;

                                    window.trakand_emit('message_new', { text, sender, sender_id, msgId });
                                } catch (e) {
                                    console.error("Bridge Error processing node:", e);
                                }
                            }
                        }
                    };
                    window.addEventListener('load', () => window.trakand_reach_bridge.init());
                """)

                await session.page.expose_function("trakand_emit", lambda event, data: session.emit(event, data))

            await session.page.goto(url, wait_until='domcontentloaded', timeout=30000)

            # Special handling for WhatsApp Web QR code
            if "web.whatsapp.com" in url:
                logger.info("WhatsApp Web detected. Waiting for QR code...")
                try:
                    # Try to find the QR code data attribute if it exists in the DOM
                    await session.page.wait_for_selector("canvas", timeout=10000)
                    qr_data = await session.page.evaluate("""() => {
                        const div = document.querySelector('div[data-ref]');
                        return div ? div.getAttribute('data-ref') : null;
                    }""")
                    if qr_data:
                        session.emit('qr', qr_data)
                    logger.info("QR Code detected! ✅")
                except:
                    logger.warning("QR Code detection timeout. Starting stream anyway.")

            self.start_stream(session)
        except Exception as e:
            logger.error(f"Failed to load page: {e}")
            if session.page:
                await session.page.close()
                session.page = None
            raise

    def start_stream(self, session: Session):
        if session.screenshot_task and not session.screenshot_task.done():
            return

        async def stream_loop():
            logger.info(f"⏳ Starting screenshot stream for session {session.id}...")
            try:
                while session.is_alive and session.ws and not session.ws.closed:
                    await self.send_screenshot(session)
                    await asyncio.sleep(0.8)
            except Exception as e:
                logger.error(f"Stream error for session {session.id}: {e}")
            finally:
                logger.info(f"Screenshot stream stopped for session {session.id} ✅")

        session.screenshot_task = asyncio.create_task(stream_loop())

    async def send_screenshot(self, session: Session):
        if not session.page or session.page.is_closed():
            return

        try:
            # We take a standard screenshot instead of full_page to avoid memory issues
            # on long WhatsApp conversations, while still capturing the QR code/UI.
            screenshot_bytes = await session.page.screenshot(type='jpeg', quality=85)
            base64_screenshot = base64.b64encode(screenshot_bytes).decode('utf-8')
            message = json.dumps({
                'type': 'screenshot',
                'data': base64_screenshot
            })

            if session.ws and not session.ws.closed:
                await session.ws.send(message)
        except Exception as e:
            logger.error(f"Error taking/sending screenshot: {e}")

    async def destroy_session(self, session_id: str, remove_metadata: bool = True):
        session = self.sessions.get(session_id)
        if session:
            session.is_alive = False
            if session.screenshot_task:
                session.screenshot_task.cancel()
            try:
                if session.page: await session.page.close()
                if session.context: await session.context.close()
                if session.browser: await session.browser.close()
            except Exception as e:
                logger.error(f"Error destroying session {session_id}: {e}")

            if remove_metadata:
                self.sessions.pop(session_id, None)
                self.save_sessions()

    async def handle_websocket(self, ws: websockets.WebSocketServerProtocol, path: str = ""):
        connection_id = base64.b64encode(os.urandom(9)).decode('utf-8')
        logger.info(f"New connection established (id: {connection_id})")

        try:
            await ws.send(json.dumps({
                'type': 'connection_established',
                'data': {'connectionId': connection_id}
            }))

            async for message in ws:
                try:
                    parsed = json.loads(message)
                    msg_type = parsed.get('type')
                    data = parsed.get('data', {})

                    if msg_type == 'init':
                        logger.info("⏳ Initializing client...")
                        access_key = data.get('access_key')
                        device_info = data.get('deviceInfo', {})
                        url = data.get('url')

                        session = await self.create_session(access_key, device_info)
                        session.ws = ws

                        # Resume last URL if not provided
                        target_url = url or session.last_url
                        if target_url:
                            await self.start_up_link(session.id, target_url)
                            logger.info(f"Client initialized ✅ (Session: {session.id})")
                        else:
                            logger.warning(f"No URL provided for session {session.id}")
                except Exception as e:
                    logger.error(f"Failed to handle message: {e}")
        except websockets.exceptions.ConnectionClosed:
            logger.info(f"Connection closed (id: {connection_id})")
        finally:
            for s in self.sessions.values():
                if s.ws == ws:
                    s.ws = None

    async def send_whatsapp_message(self, session_id: str, to: str, text: str):
        """
        Send a WhatsApp message with 'tighter hooks'.
        Attempts to use internal Store modules first, falling back to UI automation.
        """
        session = self.sessions.get(session_id)
        if not session or not session.page:
            raise ValueError("Session or page not found")

        try:
            # Attempt internal module injection for sending
            # This bypasses the need for UI interaction entirely if successful
            result = await session.page.evaluate(f"""
                async (to, text) => {{
                    if (window.WWebStore && window.WWebStore.Chat && window.WWebStore.SendTextMsgToChat) {{
                        const jid = to.includes('@') ? to : to + '@c.us';
                        const chat = window.WWebStore.Chat.get(jid);
                        if (chat) {{
                            await window.WWebStore.SendTextMsgToChat(chat, text);
                            return {{ success: true, method: 'internal' }};
                        }}
                    }}
                    return {{ success: false }};
                }}
            """, to, text)

            if result.get('success'):
                logger.info(f"Message sent to {to} via internal bridge ✅")
                return

            # Fallback to UI Automation
            logger.info(f"Internal bridge unavailable. Falling back to UI automation for {to}...")

            # 1. Navigate to the contact
            is_phone = to.isdigit() and len(to) >= 10
            if is_phone:
                url = f"https://web.whatsapp.com/send?phone={to}"
                if f"phone={to}" not in session.page.url:
                    await session.page.goto(url, wait_until='domcontentloaded')
            else:
                search_selector = 'div[contenteditable="true"][data-tab="3"]'
                await session.page.fill(search_selector, to)
                await session.page.keyboard.press("Enter")
                await asyncio.sleep(1)

            # 2. Find and interact with input
            input_selectors = [
                'footer div[contenteditable="true"]',
                'div[contenteditable="true"][data-tab="10"]',
                '#main footer div.selectable-text'
            ]

            input_element = None
            for selector in input_selectors:
                try:
                    input_element = await session.page.wait_for_selector(selector, timeout=5000)
                    if input_element: break
                except: continue

            if input_element:
                await input_element.fill(text)
                await asyncio.sleep(0.5)
                await session.page.keyboard.press("Enter")
            else:
                # Last ditch effort: blind typing
                await session.page.keyboard.type(text)
                await session.page.keyboard.press("Enter")

            logger.info(f"Message sent to {to} via UI fallback ✅")
        except Exception as e:
            logger.error(f"Failed to send message to {to}: {e}")
            raise
