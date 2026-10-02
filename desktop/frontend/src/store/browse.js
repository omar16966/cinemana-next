// =============================================================================
// store/browse.js — صفوف الاستعراض وقوائم Top ولوحة الفلترة.
// =============================================================================

import * as api from '../services/api.js'
import { store, seq, timers, PAGE_SIZE, mergeById, errMsg, fail } from './state.js'

// =============================================================================
// صفوف الاستعراض وقوائم Top
// =============================================================================

// تعريف الصفوف الأفقية في الرئيسية (الترتيب هو ترتيب العرض).
export const HOME_ROWS = [
  { key: 'latest_movies_foreign', title: 'أحدث الأفلام الأجنبية' },
  { key: 'latest_series_foreign', title: 'أحدث المسلسلات الأجنبية' },
  { key: 'latest_movies_anime', title: 'أحدث أفلام الأنمي' },
  { key: 'latest_series_anime', title: 'أحدث مسلسلات الأنمي' },
  { key: 'latest_movies_arabic', title: 'أحدث الأفلام العربية' },
  { key: 'latest_series_arabic', title: 'أحدث المسلسلات العربية' },
  { key: 'latest_all', title: 'أحدث الإضافات' },
]

// TOP_TABS تبويبات قسم الأعلى تقييماً.
export const TOP_TABS = [
  { key: 'top_movies', label: 'أفلام' },
  { key: 'top_series', label: 'مسلسلات' },
  { key: 'top_anime', label: 'أنمي' },
]

// ensureRow يجلب عناصر صف عند حاجته (lazy — عند اقتراب ظهوره فقط).
export async function ensureRow(key) {
  const row = store.rows[key]
  if (row && (row.loaded || row.loading)) return
  store.rows[key] = { items: [], loading: true, loaded: false, error: '' }
  try {
    store.rows[key].items = await api.collection(key, 14)
    store.rows[key].loaded = true
  } catch (e) {
    store.rows[key].error = errMsg(e)
  } finally {
    store.rows[key].loading = false
  }
}

// ensureTop يجلب قائمة Top للتبويب النشط.
export async function ensureTop(tabKey) {
  store.topTab = tabKey
  const t = store.top[tabKey]
  if (t && (t.loaded || t.loading)) return
  store.top[tabKey] = { items: [], loading: true, loaded: false }
  try {
    store.top[tabKey].items = await api.collection(tabKey, 12)
    store.top[tabKey].loaded = true
  } catch (e) {
    // loaded تبقى false: النقر على التبويب مجدداً يعيد المحاولة (دون حلقة
    // جلب تلقائية لأن الاستدعاء يأتي من حدث المستخدم فقط).
    store.top[tabKey].error = errMsg(e)
    fail(e)
  } finally {
    store.top[tabKey].loading = false
  }
}

// setTopTab تبديل تبويب قسم Top (يجلب البيانات إن لم تكن محملة).
export function setTopTab(tabKey) {
  ensureTop(tabKey)
}

// =============================================================================
// لوحة الفلترة (يمين): تصفح بمعايير متعددة
// =============================================================================

// ensureCategories يجلب تصنيفات الخدمة مرة واحدة (للقائمة المنسدلة).
let categoriesPromise = null
export async function ensureCategories() {
  if (store.categories.length) return
  if (!categoriesPromise) {
    categoriesPromise = api
      .categories()
      .then((c) => {
        store.categories = Array.isArray(c) ? c : []
      })
      .catch(fail)
      .finally(() => {
        categoriesPromise = null
      })
  }
  await categoriesPromise
}

// applyFilters ينفذ التصفح بالفلاتر الحالية ويعرض الشبكة في الرئيسية.
export async function applyFilters(filters, reset = true) {
  clearTimeout(timers.search) // مؤقت بحث معلّق كان سيطغى على نتائج الفلترة
  const my = ++seq.browse
  const page = reset ? 1 : store.browse.page + 1
  store.view = 'home'
  store.browse.active = true
  store.browse.loading = true
  store.browse.filters = filters
  try {
    const items = await api.browse({ ...filters, page })
    if (my !== seq.browse) return // استجابة قديمة أو أُلغي التصفح
    store.browse.items = reset ? items : mergeById(store.browse.items, items)
    store.browse.page = page // يتقدم العدّاد عند النجاح فقط (الفشل لا يتخطى صفحة)
    store.browse.hasMore = items.length >= PAGE_SIZE // الصفحة ممتلئة غالباً = يوجد المزيد
    store.browse.title = browseTitle(filters)
    store.searched = false
    store.results = []
  } catch (e) {
    if (my === seq.browse) fail(e)
  } finally {
    if (my === seq.browse) store.browse.loading = false
  }
}

// browseTitle عنوان وصفي للشبكة حسب الفلاتر.
function browseTitle(f) {
  const parts = []
  if (f.query) parts.push(`«${f.query}»`)
  if (f.video_kind === '1') parts.push('أفلام')
  if (f.video_kind === '2') parts.push('مسلسلات')
  if (f.language_id === '9') parts.push('عربي')
  if (f.language_id === '7') parts.push('أجنبي')
  if (f.category_id) {
    const c = store.categories.find((x) => x.nb === f.category_id)
    if (c) parts.push(c.title)
  }
  if (f.min_star && f.min_star !== '0') parts.push(`تقييم ${f.min_star}+`)
  if (f.year_from || f.year_to) parts.push(`(${f.year_from || '...'} - ${f.year_to || '...'})`)
  return parts.length ? 'نتائج: ' + parts.join(' · ') : 'كل الأعمال'
}

// moreBrowse تحميل صفحة إضافية من نتائج التصفح.
export function moreBrowse() {
  if (!store.browse.loading && store.browse.active && store.browse.filters) {
    applyFilters(store.browse.filters, false)
  }
}

// clearBrowse إنهاء وضع التصفح والعودة للاستكشاف (Top + الصفوف).
export function clearBrowse() {
  seq.browse++
  store.browse = { active: false, title: '', filters: null, items: [], loading: false, page: 1, hasMore: false }
}
