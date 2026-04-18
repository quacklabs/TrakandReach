import { chromium, firefox, webkit, Browser, BrowserContext, Page } from 'playwright-core';
import path from 'path';
import WebSocket, { WebSocketServer } from 'ws';
import {  BrowserWindow, ipcMain, IpcMain, app } from 'electron';
import fs from 'fs';
import sharp from 'sharp';
// import { parse } from 'querystring';
// import { dir } from 'console';

// Critical error types
export enum CriticalError {
  BROWSER_CRASH = 'BROWSER_CRASH',
  SESSION_TIMEOUT = 'SESSION_TIMEOUT',
  CONTEXT_ERROR = 'CONTEXT_ERROR',
  PAGE_CRASH = 'PAGE_CRASH'
}

interface DeviceType {
  type: string
  model: string
  brand: string
  os: string
}

interface DeviceInfo {
  os: string
  userAgent: string
  browser: string
  product: string
  manufacturer: string
  engine: string
  fingerprint: string
  width: number
  height: number
  device: DeviceType
  pixelRatio: number
  dark_mode: boolean
  language: string
}
interface Session {
  id: string;
  ws: WebSocket | null
  createdAt: number;
  browser: Browser | null;
  context: BrowserContext | null;
  page: Page | null;
  deviceInfo: DeviceInfo;
  isAlive: boolean;
  browserType: 'chromium' | 'firefox' | 'webkit';
  recoveryAttempts: number;
  screenshotInterval: NodeJS.Timeout | null | undefined
  access_key: string
}

interface ClientMessage {
  type: string;
  data: any;
}

class PlaywrightService {
  private static instance: PlaywrightService;
  public static isWebSocketRunning: boolean = false;
  private sessions: Map<string, Session> = new Map();
  private wss!: WebSocketServer;
  private MAX_RECOVERY_ATTEMPTS: number = 3;
  private wsRecoveryAttempts: number = 0;
  private readonly MAX_WS_RECOVERY_ATTEMPTS: number = 3;
  private readonly WS_RECOVERY_DELAY: number = 5000;
  private readonly userDataPath = app.getPath('userData')
  private readonly SESSIONS_BASE_DIR = path.join(this.userDataPath, 'browserSessions');

  private constructor() {
    this.sessions = new Map();
  }

  public static async getInstance(): Promise<PlaywrightService> {
    if (!PlaywrightService.instance) {
      const service = new PlaywrightService();
      PlaywrightService.instance = service;
    }
    return PlaywrightService.instance;
  }

  public async boot() {
    console.log("⏳ Initializing automation engine...")
    const port = 3000;
    try {
      this.wss = new WebSocketServer({ port });
      this.wss.on('connection', (ws) => this.onConnect(ws));
      this.wsRecoveryAttempts = 0;
      const windows = BrowserWindow.getAllWindows()
      windows.forEach(window => {
        if (!window.isDestroyed()) {
          window.webContents.send('engine:event', {
            type: 'booted'
          })
        }
      })
      console.log("Automation engine loaded! ✅");
    } catch(error: any) {
      console.error('Failed to initialize engine ❌:', error);
    }
  }

//
  // private async handleServerError(error: Error) {
    // console.error('WebSocket server error:', error);
    // PlaywrightService.isWebSocketRunning = false;
    // await this.handleServerFailure();
  // }
//
  // private async handleServerClose() {
    // console.log('WebSocket server closed');
    // PlaywrightService.isWebSocketRunning = false;
    // await this.handleServerFailure();
  // }
//
  private async handleServerFailure() {
    PlaywrightService.isWebSocketRunning = true;
    if (this.wsRecoveryAttempts >= this.MAX_WS_RECOVERY_ATTEMPTS) {
      console.error('Max WebSocket recovery attempts reached. Server will not restart.');
      return;
    }

    this.wsRecoveryAttempts++;
    console.log(`Attempting to recover WebSocket server (attempt ${this.wsRecoveryAttempts}/${this.MAX_WS_RECOVERY_ATTEMPTS})...`);

    await new Promise(resolve => setTimeout(resolve, this.WS_RECOVERY_DELAY));

    try {
      /* Close existing server if it's still around */
      if (this.wss) {
        this.wss.close();
      }
      await this.boot();
    } catch (error) {
      console.error('Failed to recover WebSocket server:', error);
    }
  }
//
  protected onConnect(ws: WebSocket & { sessionId?: string }) {
    const connectionId = Math.random().toString(36).substring(2, 15);
    console.log(`New connection established (id: ${connectionId})`);
    ws.on('message', (message: WebSocket.RawData) => this.handleMessage(ws, message));
    // ws.on('close', () => {
    //   if (ws.sessionId) {
    //     // this.destroySession(ws.sessionId);
    //     console.log(`Session ${ws.sessionId} closed`);
    //   }
    // });
    /* Send welcome message to client */
    const welcomeMsg = JSON.stringify({
      type: 'connection_established',
      data: { connectionId }
    });
    ws.send(welcomeMsg);
  }

  private parseBrowser(input: string): 'chromium' | 'firefox' | 'webkit' {
    switch(input.toLocaleLowerCase()) {
      case 'firefox':
        return 'firefox'
      case 'chrome':
        return 'chromium'
      case 'safari':
        return 'webkit'
      case 'unknown':
        return 'chromium'
      case 'chromium':
        return 'chromium'
      default:
        return 'chromium'
    }
  }

  protected handleMessage(ws: WebSocket & { sessionId?: string }, message: WebSocket.RawData): void {
    try {
      const parsedMessage = JSON.parse(message.toString()) as ClientMessage;
      const { type, data } = parsedMessage;
      if (type === 'init') {
        console.log("⏳ Initializing client...")
        if(this.sessions.has(data.deviceInfo.fingerprint)) {

          return
        }
        try {
          const browserType = this.parseBrowser(data.deviceInfo.browser)
          this.createSession(
            data.access_key,
            data!.deviceInfo,
            browserType
          ).then((session) => {
            return Promise.all([
              session,
              this.spinClient(data.deviceInfo.browser, ws, data)
            ])
          }).then(([session]) => {
            session.ws = ws
            this.sessions.set(session.id, session)
            this.startUpLink(session.id, data.url)
            return
          }).finally(() => {

            console.log("Client initialized ✅")
          });
        } catch (error) {
          console.error('Failed to initialize client:', error);
        }
      } else {
        // this.performAction(ws, data)
      }
    } catch (error) {
      console.error('Failed to handle message:', error);
    }
  }

  private startStream(session: Session) {
    console.log("⏳ Starting screenshot stream...");
    session.screenshotInterval = setInterval(async () => {
      console.log("Taking screenshot...");
      await this.sendScreenshot(session);
    }, 800); // 2000ms = 2 second (0.5 FPS)
    console.log("Screenshot stream started ✅");
  }

  public async sendScreenshot(session: Session) {
    try {
      if (session.page) {
        const screenshot = await session.page.screenshot({ fullPage: true});

        const compressed = await sharp(screenshot).jpeg({
		        quality: 85,
		      })
		      .toBuffer()
          const base64Screenshot = compressed.toString('base64');
          const message = JSON.stringify({
            type: 'screenshot',
            data: base64Screenshot
          });
          if (session.ws && session.ws.readyState === WebSocket.OPEN) {
            session.ws.send(message);
          } else {
            console.error("WebSocket is not open. Cannot send screenshot.");
          }
      } else {

      }
    } catch (err) {
	    console.error("Error taking screenshot:", err);
	    // this.stopScreenshotStream(session); // Stop streaming on error
	  }
  }

  private getOptimizedOptions(device: DeviceInfo, viewPort: {width: number, height: number} | null): any {
    const userDir = path.join(this.SESSIONS_BASE_DIR, `${device.fingerprint}`);

    try {
      // Create directory recursively if it doesn't exist
      if (!fs.existsSync(userDir)) {
        fs.mkdirSync(userDir, { recursive: true });
        console.log(`Successfully created user directory: ${userDir}`);
      } else {
        console.log(`User directory already exists: ${userDir}`);
      }

      return {
        options: {
          headless: true,
          args: [
            '--no-sandbox',
            '--disable-dev-shm-usage',
            '--disable-accelerated-2d-canvas',
            '--disable-gpu', // Only if having GPU issues
            '--disable-features=IsolateOrigins,site-per-process',
            '--flag-switches-begin',
            '--flag-switches-end'
          ],
          viewPort: viewPort,
          acceptDownloads: true,
          ignoreHTTPSErrors: true,
          userAgent: device.userAgent,
          bypassCSP: true,
          javaScriptEnabled: true,
          colorScheme: (device.dark_mode ==true) ? 'dark' : 'light',
          locale: device.language,
          deviceScaleFactor: device.pixelRatio,
          ...(userDir && { userDir })
        },
        directory: userDir
      }
    } catch (error) {
      console.error(`Failed to create directory ${userDir}:`, error);
      throw error; // Re-throw if you want calling code to handle it
    }

  }

  protected async spinClient(
    browserType: string,
    ws: WebSocket & { sessionId?: string },
    session: Session
  ): Promise<void> {
    return new Promise(async (resolve, reject) => {
      if (session.browser && !session.browser.isConnected()) {
        await session.browser.close();
        session.browser = null;
        session.context = null;
        this.sessions.set(session.id, session)
      }
      if (session.browser && session.context) {
        resolve()
        return
      }

      const type = this.parseBrowser(browserType);
      const { options, directory } = this.getOptimizedOptions(
        session.deviceInfo,
        { width: session.deviceInfo.width, height: session.deviceInfo.height }
      );
      if (!fs.existsSync(directory)) {
        fs.mkdirSync(directory, { recursive: true });
      }
      let browser: Browser;
      let context: BrowserContext;
      if(type == 'chromium') {
        context = await chromium.launchPersistentContext(directory, {
          ...options,

          args: [
            ...(options.args || [])
          ]
        });
        browser = context.browser()!;
      } else if(type === 'firefox') {
        context = await firefox.launchPersistentContext(directory, {
          ...options
        });
        browser = context.browser()!;
      }else if(type === 'webkit') {
        browser = await webkit.launch({ headless: false });
        context = await browser.newContext({
          viewport: options.viewport,
          userAgent: options.userAgent
        });
      } else {
        context = await chromium.launchPersistentContext(directory,
          {...options});
        browser = context.browser()!;
      }

      session.browser = browser;
      session.context = context;
      this.sessions.set(session.id, session)
      resolve()
      return
    })
  }

  public async startUpLink(sessionId: string, url: string) {
    console.log("⏳ Starting Uplink...");

    const session = this.sessions.get(sessionId);
    if (!session) {
      throw new Error(`Session ${sessionId} not found`);
    }

    try {
      // Ensure browser is initialized
      if (!session!.browser || !session!.browser.isConnected()) {
        await this.spinClient(session.browserType, null as any, session);
      }
      const loadPage = await session!.context!.newPage();
      session!.page = loadPage;
      this.sessions.set(sessionId, session);
      await loadPage.bringToFront();
      // const client = await session.page.target().createCDPSession();
      await loadPage.goto(url, {
        waitUntil: 'domcontentloaded',
        timeout: 30000
      });

      this.startStream(session);
      console.log("✅ Page loaded successfully");
    } catch (error) {
      console.error('Failed to load page:', error);
      if (session.page) {
        await session.page.close().catch(e => console.error('Error closing page:', e));
        session.page = null;
      }
      throw error;
    }
  }
//
   /* Self-healing mechanism for browser crashes */
  private async handleBrowserCrash(browserType: 'chromium' | 'firefox' | 'webkit'): Promise<Browser> {

    return new Promise(async (reject) => {
      console.error(`Browser ${browserType} crashed, attempting recovery...`);
      await this.closeBrowser(browserType);
    })

    // return this.initializeBrowser(browserType);
  }

  /* Self-healing mechanism for session errors */
  private async recoverSession(sessionId: string, error: CriticalError): Promise<Session | null> {
    const session = this.sessions.get(sessionId);
    if (!session) return null;

    if (session.recoveryAttempts >= this.MAX_RECOVERY_ATTEMPTS) {
      console.error(`Session ${sessionId} exceeded max recovery attempts, destroying...`);
      await this.destroySession(sessionId);
      return null;
    }

    session.recoveryAttempts++;
    console.log(`Attempting to recover session ${sessionId}, attempt ${session.recoveryAttempts}...`);

    try {
      switch (error) {
        case CriticalError.BROWSER_CRASH:
          session.browser = await this.handleBrowserCrash(session.browserType);
          session.context = await session.browser.newContext();
          session.page = await session.context.newPage();
          break;

        case CriticalError.CONTEXT_ERROR:
          if (session.browser) {
            session.context = await session.browser.newContext();
            session.page = await session.context.newPage();
          }
          break;

        case CriticalError.PAGE_CRASH:
          if (session.context) {
            session.page = await session.context.newPage();
          }
          break;

        default:
          throw new Error(`Unknown error type: ${error}`);
      }

      session.isAlive = true;
      return session;
    } catch (e) {
      console.error(`Failed to recover session ${sessionId}:`, e);
      return null;
    }
  }

  async createSession(access_key: string, deviceInfo: DeviceInfo, browserType: 'chromium' | 'firefox' | 'webkit' = 'chromium'): Promise<Session> {
    return new Promise<Session>((resolve, reject) => {
      if(!deviceInfo?.fingerprint || deviceInfo?.fingerprint == null) {
        reject({message: "No fingerprint for device. discarding..."})
        return
      }
      const sessionId = deviceInfo?.fingerprint || Date.now().toString();
      if (this.sessions.has(sessionId)) {
        resolve(this.sessions.get(sessionId)!);
      } else {
        const session: Session = {
          id: sessionId,
          createdAt: Date.now(),
          browser: null,
          context: null,
          page: null,
          deviceInfo: deviceInfo,
          isAlive: true,
          browserType: browserType,
          recoveryAttempts: 0,
          access_key: access_key,
          ws: null,
          screenshotInterval: null
        };
        this.sessions.set(sessionId, session);
        resolve(this.sessions.get(sessionId)!);
      }
    })

    // try {
    //   this.initializeBrowser(browserType).then((browser: Browser) => {
    //     return
    //   }).then();
    //   const context = await browser.newContext();
    //   const page = await context.newPage();

    //   /* Add error handlers */
    //   browser.on('disconnected', () => this.recoverSession(sessionId, CriticalError.BROWSER_CRASH));
    //   page.on('crash', () => this.recoverSession(sessionId, CriticalError.PAGE_CRASH));


    //   return session;
    // } catch (error) {
    //   console.error(`Failed to create session:`, error);
    //   throw error;
    // }
  }

  async resumeSession(sessionId: string): Promise<Session | null> {
    const session = this.sessions.get(sessionId);
    if (!session) return null;

    if (!session.isAlive) {
      try {
        /* Attempt recovery if session is not alive */
        return await this.recoverSession(sessionId, CriticalError.CONTEXT_ERROR);
      } catch (error) {
        console.error(`Failed to resume session ${sessionId}:`, error);
        await this.closeBrowser(session.browserType);
        this.sessions.delete(sessionId);
        return null;
      }
    }
    return session;
  }

  async closeBrowser(browserType: 'chromium' | 'firefox' | 'webkit') {
    // const browser = this.browserInstances.get(browserType);
    // if (browser) {
    //   try {
    //     await browser.close();
    //     // this.browserInstances.delete(browserType);

    //     /* Clean up any sessions using this browser */
    //     for (const [sessionId, session] of this.sessions) {
    //       if (session.browser === browser) {
    //         session.isAlive = false;
    //         this.sessions.delete(sessionId);
    //       }
    //     }
    //   } catch (error) {
    //     console.error(`Error closing ${browserType} browser:`, error);
    //   }
    // }
  }

  async closeAllBrowsers() {
    // const closePromises = Array.from(this.browserInstances.values()).map(browser => browser.close());
    // await Promise.all(closePromises);
    // this.browserInstances.clear();
    // this.sessions.clear();
  }

  async destroySession(sessionId: string) {
    const session = this.sessions.get(sessionId);
    if (session) {
      try {
        await session.page?.close();
        await session.context?.close();
        session.isAlive = false;
        this.sessions.delete(sessionId);
      } catch (error) {
        console.error(`Error destroying session ${sessionId}:`, error);
      }
    }
  }

  public isServerRunning(): boolean {
    return PlaywrightService.isWebSocketRunning;
  }
}
export const bindPlaywrightEvents = (ipcMain: IpcMain) => {
  // const auth = AuthService.getInstance(storage);

  // ipcMain.handle('auth:getCurrentUser', async () => {
  //   return auth.getCurrentUser();
  // });

  // ipcMain.handle('auth:logout', async () => {
  //   auth.logout();
  //   return { success: true };
  // });

  // ipcMain.handle('auth:login', async (_, credentials: LoginForm) => {
  //   return auth.login(credentials);
  // });
};
export { PlaywrightService };
