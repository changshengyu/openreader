const bundledPlaywright = '/Users/yuchangsheng/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright/index.js'
const workspacePlaywright = new URL('../../node_modules/playwright/index.js', import.meta.url).href

export async function loadSmokePlaywright() {
  try {
    const module = await import('playwright')
    return module.chromium ? module : module.default
  } catch (rootError) {
    try {
      const module = await import(workspacePlaywright)
      return module.chromium ? module : module.default
    } catch (workspaceError) {
      try {
        const module = await import(bundledPlaywright)
        return module.chromium ? module : module.default
      } catch (bundledError) {
        throw new Error(
          `Playwright is required for browser smoke tests. Root: ${rootError.message}; workspace: ${workspaceError.message}; bundled: ${bundledError.message}`,
        )
      }
    }
  }
}

// Production builds register the offline-shell service worker on 127.0.0.1, a secure
// context. The existing contracts predate it and hold or mock /assets and /api requests
// with page.route, which a controlling worker would bypass, so new contexts block
// service workers unless a contract opts in with serviceWorkers: 'allow'
// (scripts/smoke/pwa-service-worker-contract.mjs).
function blockServiceWorkersByDefault(browser) {
  const newContext = browser.newContext.bind(browser)
  const newPage = browser.newPage.bind(browser)
  browser.newContext = (options = {}) => newContext({ serviceWorkers: 'block', ...options })
  browser.newPage = (options = {}) => newPage({ serviceWorkers: 'block', ...options })
  return browser
}

export async function openSmokeBrowser(launchOptions = {}) {
  const playwright = await loadSmokePlaywright()
  if (process.env.CDP_URL) {
    return blockServiceWorkersByDefault(await playwright.chromium.connectOverCDP(process.env.CDP_URL))
  }

  const options = {
    headless: true,
    ...launchOptions,
  }
  if (process.env.CHROME_PATH) options.executablePath = process.env.CHROME_PATH

  try {
    return blockServiceWorkersByDefault(await playwright.chromium.launch(options))
  } catch (error) {
    const detail = String(error?.message || error)
    if (detail.includes('Executable doesn\'t exist')) {
      throw new Error(
        `${detail}\nInstall the crash-safe headless browser once with: npm run smoke:install-browser`,
      )
    }
    throw error
  }
}
