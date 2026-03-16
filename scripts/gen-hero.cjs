#!/usr/bin/env node
/**
 * Generate hero image for the landing page.
 *
 * Takes desktop + mobile screenshots of the mock UI, composites them
 * into a single image with the phone overlapping the desktop corner.
 *
 * Usage:
 *   node scripts/gen-hero.cjs           # mock server must be running on :5199
 *   node scripts/gen-hero.cjs --serve   # auto-start vite dev server
 *
 * Output:
 *   apps/website/src/assets/hero-desktop.png
 *   apps/website/src/assets/hero-mobile.png
 *   apps/website/public/hero.png  (composite)
 */

const { chromium } = require('playwright')
const { spawn } = require('child_process')
const fs = require('fs')
const path = require('path')

const ROOT = path.resolve(__dirname, '..')
const PORT = 5199
const URL = `http://localhost:${PORT}/?mock`

async function waitForServer(url, timeoutMs = 15000) {
  const start = Date.now()
  while (Date.now() - start < timeoutMs) {
    try { const r = await fetch(url); if (r.ok) return } catch {}
    await new Promise(r => setTimeout(r, 200))
  }
  throw new Error(`Server not ready at ${url} after ${timeoutMs}ms`)
}

async function takeScreenshots(browser) {
  // Desktop — tighter viewport to fill more of the frame
  console.log('Taking desktop screenshot...')
  const dPage = await browser.newPage({
    viewport: { width: 1100, height: 700 },
    deviceScaleFactor: 2,
  })
  await dPage.goto(URL, { timeout: 5000, waitUntil: 'load' })
  await dPage.waitForSelector('.session-item', { timeout: 5000 })
  await dPage.waitForTimeout(500)
  await dPage.click('.session-item')
  await dPage.waitForTimeout(800)

  const desktopPath = path.join(ROOT, 'apps/website/src/assets/hero-desktop.png')
  await dPage.screenshot({ path: desktopPath })
  console.log(`  → ${path.relative(ROOT, desktopPath)}`)

  // Mobile with sidebar open
  console.log('Taking mobile screenshot...')
  const mPage = await browser.newPage({
    viewport: { width: 390, height: 760 },
    deviceScaleFactor: 2,
    isMobile: true,
    hasTouch: true,
  })
  await mPage.goto(URL, { timeout: 5000, waitUntil: 'load' })
  await mPage.waitForSelector('.mobile-bottom-bar', { timeout: 5000 })
  await mPage.waitForTimeout(500)

  // Open sidebar
  const menuBtn = await mPage.$('.mobile-bottom-bar button:first-child')
  if (menuBtn) {
    await menuBtn.click()
    await mPage.waitForTimeout(600)
  }

  const mobilePath = path.join(ROOT, 'apps/website/src/assets/hero-mobile.png')
  await mPage.screenshot({ path: mobilePath })
  console.log(`  → ${path.relative(ROOT, mobilePath)}`)

  return { desktopPath, mobilePath }
}

async function composite(browser, desktopPath, mobilePath) {
  console.log('Compositing...')
  const desktopB64 = fs.readFileSync(desktopPath).toString('base64')
  const mobileB64 = fs.readFileSync(mobilePath).toString('base64')

  const page = await browser.newPage({
    viewport: { width: 1600, height: 1100 },
    deviceScaleFactor: 2,
  })

  await page.setContent(`<html><body style="margin:0;background:transparent">
    <canvas id="c"></canvas>
    <script>
      async function draw() {
        const desktop = new Image()
        desktop.src = 'data:image/png;base64,${desktopB64}'
        await desktop.decode()
        const mobile = new Image()
        mobile.src = 'data:image/png;base64,${mobileB64}'
        await mobile.decode()

        // Both images are @2x (2200x1400 desktop, 780x1520 mobile)
        // Scale down to ~75% to keep canvas manageable while sharp
        const scale = 0.75
        const dW = desktop.width * scale
        const dH = desktop.height * scale
        const mOrigW = mobile.width
        const mOrigH = mobile.height

        // Phone at ~30% of desktop width
        const phoneScale = 0.30
        const mDrawW = dW * phoneScale
        const mDrawH = (mOrigH / mOrigW) * mDrawW

        const pad = 50
        const canvasW = dW + mDrawW * 0.45 + pad * 2
        const canvasH = dH + mDrawH * 0.32 + pad * 2

        const c = document.getElementById('c')
        c.width = canvasW
        c.height = canvasH
        c.style.width = (canvasW / 2) + 'px'
        c.style.height = (canvasH / 2) + 'px'

        const ctx = c.getContext('2d')
        // No ctx.scale — we draw at native pixel resolution

        const dx = pad
        const dy = pad

        // Desktop shadow
        ctx.save()
        ctx.shadowColor = 'rgba(0,0,0,0.3)'
        ctx.shadowBlur = 80
        ctx.shadowOffsetY = 24
        roundRect(ctx, dx, dy, dW, dH, 20)
        ctx.fillStyle = '#0f141a'
        ctx.fill()
        ctx.restore()

        // Desktop image
        ctx.save()
        roundRect(ctx, dx, dy, dW, dH, 20)
        ctx.clip()
        ctx.drawImage(desktop, 0, 0, desktop.width, desktop.height, dx, dy, dW, dH)
        ctx.restore()

        // Desktop border
        ctx.save()
        roundRect(ctx, dx, dy, dW, dH, 20)
        ctx.strokeStyle = 'rgba(255,255,255,0.07)'
        ctx.lineWidth = 1.5
        ctx.stroke()
        ctx.restore()

        // Phone position — overlapping bottom-right, fully visible
        const bezel = 10
        const mx = dx + dW - mDrawW * 0.15
        const my = dy + dH - mDrawH * 0.75

        // Phone shadow
        ctx.save()
        ctx.shadowColor = 'rgba(0,0,0,0.45)'
        ctx.shadowBlur = 60
        ctx.shadowOffsetX = -8
        ctx.shadowOffsetY = 16
        roundRect(ctx, mx - bezel, my - bezel, mDrawW + bezel * 2, mDrawH + bezel * 2, 36)
        ctx.fillStyle = '#111'
        ctx.fill()
        ctx.restore()

        // Bezel highlight
        ctx.save()
        roundRect(ctx, mx - bezel, my - bezel, mDrawW + bezel * 2, mDrawH + bezel * 2, 36)
        ctx.strokeStyle = 'rgba(255,255,255,0.1)'
        ctx.lineWidth = 1.5
        ctx.stroke()
        ctx.restore()

        // Phone screen
        ctx.save()
        roundRect(ctx, mx, my, mDrawW, mDrawH, 26)
        ctx.clip()
        ctx.drawImage(mobile, 0, 0, mobile.width, mobile.height, mx, my, mDrawW, mDrawH)
        ctx.restore()

        document.title = 'done|' + canvasW + '|' + canvasH
      }

      function roundRect(ctx, x, y, w, h, r) {
        ctx.beginPath()
        ctx.moveTo(x + r, y)
        ctx.lineTo(x + w - r, y)
        ctx.arcTo(x + w, y, x + w, y + r, r)
        ctx.lineTo(x + w, y + h - r)
        ctx.arcTo(x + w, y + h, x + w - r, y + h, r)
        ctx.lineTo(x + r, y + h)
        ctx.arcTo(x, y + h, x, y + h - r, r)
        ctx.lineTo(x, y + r)
        ctx.arcTo(x, y, x + r, y, r)
        ctx.closePath()
      }

      draw()
    </script></body></html>`)

  await page.waitForFunction(() => document.title.startsWith('done'), { timeout: 10000 })

  // Read canvas dimensions from title
  const title = await page.title()
  const [, cw, ch] = title.split('|').map(Number)

  const canvas = await page.$('#c')
  const box = await canvas.boundingBox()
  const outPath = path.join(ROOT, 'apps/website/public/hero.png')
  await page.screenshot({
    path: outPath,
    clip: { x: box.x, y: box.y, width: box.width, height: box.height },
    omitBackground: true,
  })
  console.log(`  → ${path.relative(ROOT, outPath)}`)

  // Report sizes
  const stat = fs.statSync(outPath)
  console.log(`  ${(stat.size / 1024).toFixed(0)}KB, canvas ${cw}x${ch}px`)

  return outPath
}

;(async () => {
  const shouldServe = process.argv.includes('--serve')
  let server = null

  try {
    if (shouldServe) {
      console.log('Starting vite dev server...')
      server = spawn('npx', ['vite', '--port', String(PORT)], {
        cwd: path.join(ROOT, 'apps/gmux-web'),
        env: { ...process.env, VITE_MOCK: '1' },
        stdio: 'pipe',
      })
      await waitForServer(URL)
      console.log('Server ready.')
    }

    const browser = await chromium.launch()
    const { desktopPath, mobilePath } = await takeScreenshots(browser)
    await composite(browser, desktopPath, mobilePath)
    await browser.close()

    console.log('\n✓ Hero images generated.')
  } finally {
    if (server) server.kill('SIGTERM')
  }
})()
