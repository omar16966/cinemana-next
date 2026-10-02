// =============================================================================
// store/search.js — البحث السريع (شريط الأعلى) مع debounce وحماية من الاستجابات القديمة.
// =============================================================================

import * as api from '../services/api.js'
import { store, seq, timers, mergeById, fail } from './state.js'
import { clearBrowse } from './browse.js'

// debounce بسيط لشريط البحث السريع: ننتظر توقف الكتابة 350ms ثم نبحث.
// ملاحظة إصلاح مهم: القيمة المكتوبة يجب أن تُخزن في store.query —
// بدون هذا السطر كان البحث يقرأ نصاً فارغاً ولا يعمل أبداً.
export function onQueryInput(value) {
  store.query = value
  clearTimeout(timers.search)
  if (!value.trim()) {
    clearSearch()
    return
  }
  timers.search = setTimeout(() => doSearch(true), 350)
}

// clearSearch إلغاء البحث والعودة لصفحة الاستكشاف (Top + الصفوف).
export function clearSearch() {
  clearTimeout(timers.search)
  seq.search++ // يلغي أي بحث جارٍ فلا يعيد ملء النتائج بعد المسح
  store.query = ''
  store.results = []
  store.searched = false
  store.hasMore = false
  store.loading = false
  clearBrowse()
}

// =============================================================================
// البحث
// =============================================================================

export async function doSearch(reset = true) {
  clearTimeout(timers.search) // الضغط على Enter لا يترك مؤقت الكتابة يطلق طلباً مكرراً
  const q = store.query.trim()
  if (!q) {
    seq.search++
    store.results = []
    store.searched = false
    store.hasMore = false
    return
  }
  const my = ++seq.search
  const type = store.type
  store.view = 'home' // نتائج البحث تظهر في الرئيسية مهما كانت الشاشة الحالية
  store.loading = true
  store.error = ''
  store.browse.active = false // البحث له الأولوية على نتائج التصفح
  try {
    const page = reset ? 1 : store.page + 1
    const items = await api.search(q, type, page)
    if (my !== seq.search) return // استجابة قديمة
    store.results = reset ? items : mergeById(store.results, items)
    store.page = page
    store.searched = true
    // الخدمة ترجع صفحات كاملة عادة؛ الصفحة الفارغة تعني نهاية النتائج.
    store.hasMore = items.length > 0
  } catch (e) {
    if (my === seq.search) fail(e)
  } finally {
    if (my === seq.search) store.loading = false
  }
}

export function setType(t) {
  if (store.type === t) return
  store.type = t
  seq.search++ // أي بحث جارٍ بالنوع القديم يُهمل
  if (store.query.trim()) doSearch(true)
}

export function loadMore() {
  if (!store.loading && store.hasMore) doSearch(false)
}
