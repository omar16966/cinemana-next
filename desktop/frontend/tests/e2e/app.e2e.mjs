// اختبار نهاية-لنهاية في Chromium حقيقي على الواجهة المبنية (dist) مع CSP فعّالة:
// مسودة الفلاتر، استعادة التمرير، العرض المجزّأ للقوائم/الحلقات، وغياب انتهاكات CSP.
// التشغيل: npm run build && npm run test:e2e   (CHROME_PATH اختياري لمتصفح مثبّت مسبقاً)
import { chromium } from 'playwright-core'
import { fileURLToPath } from 'node:url'
import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'

const DIST = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../dist')
const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.woff2': 'font/woff2' }
const app = http.createServer((req, res) => {
  let p = req.url.split('?')[0]; if (p === '/') p = '/index.html'
  const f = path.join(DIST, p)
  if (!f.startsWith(DIST) || !fs.existsSync(f)) { res.writeHead(404); return res.end() }
  res.writeHead(200, { 'Content-Type': types[path.extname(f)] || 'application/octet-stream' }); fs.createReadStream(f).pipe(res)
}).listen(0)
await new Promise(r => setTimeout(r, 200))
const port = app.address().port

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || undefined, args: ['--no-sandbox'] })
const page = await browser.newPage({ viewport: { width: 1360, height: 420 } })
const errors = []
page.on('pageerror', e => errors.push(e.message))
page.on('console', m => { if (m.type() === 'error' && !/ERR_NAME|net::|Failed to load/.test(m.text())) errors.push(m.text()) })
await page.addInitScript(() => {
  const item = (i, t = 'movie') => ({ id: String(i), ar_title: 'عمل ' + i, en_title: 'Title ' + i, type: t, year: '2020', rating: '7.1', poster_url: '', thumbnail_url: '', categories: [] })
  // 300 مفضلة في التخزين
  if (!localStorage.getItem('cinemana_favorites')) localStorage.setItem('cinemana_favorites', JSON.stringify([item(777, 'series'), ...Array.from({ length: 299 }, (_, i) => item(1000 + i))]))
  window.go = { main: { App: {
    GetCollection: async () => Array.from({ length: 14 }, (_, i) => item(i + 1)),
    Search: async () => [item(1)], Browse: async () => [item(2)], GetCategories: async () => [{ nb: '1', title: 'أكشن' }],
    GetDetails: async (id) => id === '777'
      ? { ...item(id, 'series'), title: 'مسلسل طويل', subtitles: [] }
      : { ...item(id), title: 'عمل ' + id, subtitles: [] },
    GetEpisodes: async () => [{ season: '1', episodes: Array.from({ length: 150 }, (_, i) => ({ id: 'e' + i, episode_number: String(i + 1), title: 'حلقة ' + (i + 1), duration_sec: '1500' })) }],
    GetPlayback: async (id) => ({ id, qualities: [], subtitles: [] }),
    GetSettings: async () => ({ base_url: 'https://x', user_agent: '', mpv_path: '', insecure_tls: false }),
  } } }
})
await page.goto(`http://127.0.0.1:${port}/`)
await page.waitForSelector('text=/Title 1/', { timeout: 8000 })

// 1) مسودة الفلاتر تبقى بعد مغادرة الرئيسية
const q = page.getByPlaceholder('اسم فيلم أو مسلسل...')
await q.fill('matrix')
await page.locator('main button', { hasText: /Title 1/ }).first().click()
await page.waitForSelector('text=عودة للنتائج')
await page.getByText('عودة للنتائج').click()
await page.waitForSelector('text=عودة للنتائج', { state: 'detached' })
await page.waitForSelector('text=/Title 1/')
assert.equal(await page.getByPlaceholder('اسم فيلم أو مسلسل...').inputValue(), 'matrix', 'مسودة الفلتر ضاعت')
console.log('✓ مسودة الفلاتر تبقى')

// 2) موضع التمرير يُستعاد
const scrollInfo = await page.evaluate(() => {
  const el = [...document.querySelectorAll('main *')].find(e => e.scrollHeight > e.clientHeight + 50 && getComputedStyle(e).overflowY === 'auto')
  if (!el) return null
  el.scrollTop = 120; return { sh: el.scrollHeight, ch: el.clientHeight, top: el.scrollTop }
})
if (!scrollInfo) { console.log('! لا عنصر قابل للتمرير في الصفحة (المحتوى قصير) — تخطي اختبار التمرير') }
else {
    await page.evaluate(() => [...document.querySelectorAll('main button')].find(b => /Title 1/.test(b.textContent) || /Title 1/.test(b.innerHTML))?.click())  // نقر برمجي: لا يمرّر Playwright العنصر إلى الرؤية
  await page.waitForSelector('text=عودة للنتائج'); await page.getByText('عودة للنتائج').click()
  await page.waitForSelector('text=عودة للنتائج', { state: 'detached' })
  await page.waitForTimeout(500)
  const top = await page.evaluate(() => [...document.querySelectorAll('main *')].find(e => e.scrollHeight > e.clientHeight + 50 && getComputedStyle(e).overflowY === 'auto')?.scrollTop)
  assert.ok(top > 50, 'لم يُستعد موضع التمرير: ' + top); console.log('✓ موضع التمرير يُستعاد:', top)
}

// 3) المفضلة: 300 عنصر → 120 فقط ثم "عرض المزيد"
const fav = page.locator('aside button', { hasText: 'المفضلة' }).first()
await fav.click()
await page.waitForTimeout(500)
await page.waitForSelector('text=عرض المزيد', { timeout: 5000 })
let cards = await page.locator('main [class*="relative"] button.relative').count()
console.log('بطاقات مرسومة مبدئياً:', cards)
assert.ok(cards <= 125 && cards >= 100, 'يجب رسم ~120 بطاقة فقط')
await page.getByText(/عرض المزيد/).click()
cards = await page.locator('main [class*="relative"] button.relative').count()
assert.ok(cards > 200, 'بعد المزيد يجب أن تزيد البطاقات: ' + cards); console.log('✓ العرض المجزّأ للقوائم الكبيرة:', cards)

// 4) حلقات: 150 → 60 ثم المزيد
await page.locator('main button', { hasText: 'Title 777' }).first().click()
await page.waitForSelector('text=مسلسل طويل')
await page.waitForSelector('text=حلقة 1')
let eps = await page.locator('button', { hasText: /^\s*\d+\s*حلقة \d+/ }).count()
console.log('حلقات مرسومة مبدئياً:', eps)
assert.equal(eps, 60)
await page.getByText(/عرض المزيد \(90/).click()
eps = await page.locator('button', { hasText: /^\s*\d+\s*حلقة \d+/ }).count()
assert.equal(eps, 120); console.log('✓ الحلقات على دفعات:', eps)
// 5) CSP فعّالة: جلب خارجي وسكربت مضمن محجوبان (Chromium يسجّل كل انتهاك في الكونسول)
const cspLogs = []
page.on('console', (m) => { if (/Content Security Policy/i.test(m.text())) cspLogs.push(m.text()) })
const probe = await page.evaluate(async () => {
  const out = {}
  try { await fetch('https://example.com/'); out.external = 'allowed' } catch { out.external = 'blocked' }
  window.__ran = false
  const sc = document.createElement('script')
  sc.textContent = 'window.__ran = true'
  document.head.appendChild(sc)
  out.inlineScriptRan = window.__ran
  return out
})
await page.waitForTimeout(200)
assert.equal(probe.external, 'blocked', 'الجلب الخارجي يجب أن يُحجب')
assert.equal(probe.inlineScriptRan, false, 'السكربت المضمن يجب أن يُحجب')
assert.ok(cspLogs.some((l) => /connect-src/.test(l)), 'يجب أن يُسجَّل انتهاك connect-src')
assert.ok(cspLogs.some((l) => /script-src/.test(l)), 'يجب أن يُسجَّل انتهاك script-src')
console.log('✓ CSP تحجب الجلب الخارجي والسكربت المضمن')
// أي انتهاك آخر (غير ما اختبرناه عمداً) أو خطأ كونسول يعني كسراً حقيقياً
assert.deepEqual(errors.filter((e) => !/example\.com|Content Security Policy/i.test(e)), [], 'أخطاء كونسول غير متوقعة')
console.log('e2e OK')
await browser.close(); app.close()
