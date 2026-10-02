// اختبار وحدة لسياسة CSP: يضمن ألا يُضعفها تعديل لاحق بالخطأ (بلا متصفح).
import assert from 'node:assert/strict'
import { CSP } from '../vite.config.js'

const dirs = Object.fromEntries(
  CSP.split(';').map((d) => d.trim().split(/\s+/)).map(([k, ...v]) => [k, v]),
)

assert.deepEqual(dirs['script-src'], ["'self'"], "script-src يجب أن تكون 'self' فقط (لا unsafe-inline/unsafe-eval)")
assert.ok(!CSP.includes('unsafe-eval'), 'لا unsafe-eval')
assert.deepEqual(dirs['object-src'], ["'none'"])
assert.deepEqual(dirs['frame-src'], ["'none'"])
assert.deepEqual(dirs['base-uri'], ["'none'"])
assert.deepEqual(dirs['form-action'], ["'none'"])
// الاتصالات والوسائط: الأصل نفسه + وكيل البث المحلي فقط (لا https: عام ولا *).
for (const k of ['connect-src', 'media-src']) {
  assert.ok(dirs[k].includes('http://127.0.0.1:*'), `${k} يجب أن تسمح بالوكيل المحلي`)
  assert.ok(!dirs[k].includes('https:') && !dirs[k].includes('*'), `${k} يجب ألا تسمح بشبكة عامة`)
}
assert.ok(dirs['img-src'].includes('https:'), 'img-src تحتاج https: لبوسترات المزود')
assert.ok(dirs['worker-src'].includes('blob:'), 'hls.js يحتاج worker من blob')
console.log('csp tests OK')
