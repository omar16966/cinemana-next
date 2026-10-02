// اختبارات منطق الحالة بلا متصفح: node tests/store.test.mjs (أو npm test)
import assert from 'node:assert/strict'
const mem = new Map()
globalThis.localStorage = { getItem: k => mem.has(k) ? mem.get(k) : null, setItem: (k,v)=>{ if (globalThis.__quota) throw new Error('quota'); mem.set(k,String(v)) }, removeItem: k=>mem.delete(k) }
mem.set('cinemana_recent', '{}')                 // تالف: ليس مصفوفة
mem.set('cinemana_favorites', '[null, {"x":1}, {"id":5,"en_title":"A"}]')
const delays = {}
let importPayload = ''
globalThis.window = { go: { main: { App: {
  Search: (q) => new Promise(r => setTimeout(() => r([{id: q, type:'movie'}]), delays[q] || 0)),
  GetDetails: async (id) => ({ id, type: 'movie', title: 'T' }),
  GetPlayback: async (id) => ({ id, qualities: [{local_url:'x'}], subtitles: null }),
  ImportUserData: async () => importPayload,
}}}}
const S = await import('../src/store.js')
const { store } = S

// 1) بيانات تخزين تالفة لا تكسر الإقلاع
assert.deepEqual(store.recents, [])
assert.equal(store.favorites.length, 1); assert.equal(store.favorites[0].id, '5')
assert.equal(S.isFavorite(5), true)

// 2) استجابة بحث قديمة لا تطغى على الأحدث
delays.slow = 60; delays.fast = 5
store.query = 'slow'; const p1 = S.doSearch(true)
store.query = 'fast'; const p2 = S.doSearch(true)
await Promise.all([p1, p2])
assert.equal(store.results[0].id, 'fast'); assert.equal(store.loading, false)

// 3) clearSearch أثناء بحث جارٍ لا يعيد ملء النتائج
store.query = 'slow'; const p3 = S.doSearch(true); S.clearSearch(); await p3
assert.equal(store.results.length, 0); assert.equal(store.searched, false)

// 4) فتح التفاصيل لا ينهار عند فشل التخزين، وتُطبَّع ترجمات null
globalThis.__quota = true
await S.openDetails({ id: '9', title: 'x', alts: ['10'] })
assert.equal(store.view, 'details'); assert.deepEqual(store.playback.subtitles, []); assert.equal(store.selectedSubtitle, -1)
globalThis.__quota = false

// 5) استيراد: عناصر فاسدة تُهمل، القوائم المتطابقة تدمج عناصرها
S.createUserList('mine'); const lid = store.userLists[0].id
S.addToUserList(lid, { id: 1, en_title: 'One' })
importPayload = JSON.stringify({ favorites: [null, 7, {id: 2, en_title:'Two'}], user_lists: [{ id: lid, name: 'mine', items: [{id: 3}] }, { id: 'bad', name: 5 }], recents: 'oops' })
await S.importUserData()
assert.equal(store.favorites.some(f => f.id === '2'), true)
assert.equal(store.userLists.length, 1)
assert.deepEqual(store.userLists[0].items.map(i=>i.id).sort(), ['1','3'])
importPayload = '{not json'; await S.importUserData(); assert.match(store.error, /JSON/)

// 6) fmtDuration لا ينتج 60 د
assert.equal(S.fmtDuration(3599), '1 س 00 د'); assert.equal(S.fmtDuration(55*60), '55 د')

// 7) goHome ينظف الحالة
S.openCollection('favorites'); S.goHome(); assert.equal(store.view,'home'); assert.equal(store.activeCollection, null)
console.log('store tests OK')
