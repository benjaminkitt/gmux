#!/usr/bin/env node
/**
 * Generate hero image for the landing page.
 *
 * Takes desktop + mobile screenshots of the mock UI, composites them
 * into a single image with the phone overlapping the desktop corner.
 *
 * Usage:
 *   # With mock server already running on :5199
 *   node scripts/gen-hero.cjs
 *
 *   # Auto-start vite dev server
 *   node scripts/gen-hero.cjs --serve
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

// ── Layout constants (tweak these) ──
const DESKTOP_DRAW_W = 1100
const MOBILE_DRAW_W = 220
const MOBILE_BEZEL = 6
const PAD = 50
const DESKTOP_RADIUS = 12
const MOBILE_RADIUS = 14
const MOBILE_BEZEL_RADIUS = 20

async function waitForServer(url, timeoutMs = 15000) {
  const start = Date.now()
  while (Date.now() - start < timeoutMs) {
    try { const r = await fetch(url); if (r.ok) return } catch {}
    await new Promise(r => setTimeout(r, 200))
  }
  throw new Error(`Server not ready at ${url} after ${timeoutMs}ms`)
}

async function takeScreenshots(browser) {
  // Desktop @2x
  console.log('Taking desktop screenshot...')
  const dPage = await browser.newPage({
    viewport: { width: 1280, height: 800 },
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

  // Mobile @2x with sidebar open
  console.log('Taking mobile screenshot...')
  const mPage = await browser.newPage({
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 2,
    isMobile: true,
    hasTouch: true,
  })
  await mPage.goto(URL, { timeout: 5000, waitUntil: 'load' })
  await mPage.waitForSelector('.mobile-bottom-bar', { timeout: 5000 })
  await mPage.waitForTimeout(500)

  // Open sidebar via hamburger button
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

        const dW = ${DESKTOP_DRAW_W}
        const dH = (desktop.height / desktop.width) * dW
        const mW = ${MOBILE_DRAW_W}
        const mH = (mobile.height / mobile.width) * mW
        const pad = ${PAD}
        const bezel = ${MOBILE_BEZEL}

        const canvasW = dW + mW * 0.55 + pad * 2
        const canvasH = dH + mH * 0.25 + pad * 2

        const c = document.getElementById('c')
        c.width = canvasW * 2
        c.height = canvasH * 2
        c.style.width = canvasW + 'px'
        c.style.height = canvasH + 'px'

        const ctx = c.getContext('2d')
        ctx.scale(2, 2)

        const dx = pad, dy = pad

        // Desktop shadow
        ctx.save()
        ctx.shadowColor = 'rgba(0,0,0,0.3)'
        ctx.shadowBlur = 50
        ctx.shadowOffsetY = 15
        roundRect(ctx, dx, dy, dW, dH, ${DESKTOP_RADIUS})
        ctx.fillStyle = '#0f141a'
        ctx.fill()
        ctx.restore()

        // Desktop image
        ctx.save()
        roundRect(ctx, dx, dy, dW, dH, ${DESKTOP_RADIUS})
        ctx.clip()
        ctx.drawImage(desktop, dx, dy, dW, dH)
        ctx.restore()

        // Desktop border
        ctx.save()
        roundRect(ctx, dx, dy, dW, dH, ${DESKTOP_RADIUS})
        ctx.strokeStyle = 'rgba(255,255,255,0.06)'
        ctx.lineWidth = 1
        ctx.stroke()
        ctx.restore()

        // Mobile position
        const mx = dx + dW - mW * 0.3
        const my = dy + dH - mH * 0.55

        // Phone shadow + bezel
        ctx.save()
        ctx.shadowColor = 'rgba(0,0,0,0.5)'
        ctx.shadowBlur = 35
        ctx.shadowOffsetX = -5
        ctx.shadowOffsetY = 10
        roundRect(ctx, mx - bezel, my - bezel, mW + bezel * 2, mH + bezel * 2, ${MOBILE_BEZEL_RADIUS})
        ctx.fillStyle = '#111'
        ctx.fill()
        ctx.restore()

        // Bezel border
        ctx.save()
        roundRect(ctx, mx - bezel, my - bezel, mW + bezel * 2, mH + bezel * 2, ${MOBILE_BEZEL_RADIUS})
        ctx.strokeStyle = 'rgba(255,255,255,0.08)'
        ctx.lineWidth = 1
        ctx.stroke()
        ctx.restore()

        // Phone screen
        ctx.save()
        roundRect(ctx, mx, my, mW, mH, ${MOBILE_RADIUS})
        ctx.clip()
        ctx.drawImage(mobile, mx, my, mW, mH)
        ctx.restore()

        document.title = 'done'
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

  await page.waitForFunction(() => document.title === 'done', { timeout: 10000 })

  const canvas = await page.$('#c')
  const box = await canvas.boundingBox()
  const outPath = path.join(ROOT, 'apps/website/public/hero.png')
  await page.screenshot({
    path: outPath,
    clip: { x: box.x, y: box.y, width: box.width, height: box.height },
    omitBackground: true,
  })
  console.log(`  → ${path.relative(ROOT, outPath)}`)
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
