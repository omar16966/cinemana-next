// =============================================================================
// store.js — الحالة المركزية للواجهة (بدون مكتبات خارجية عمداً:
// reactive في Vue 3 يكفي تماماً لهذا الحجم، وأخف على الإقلاع من Pinia).
//
// الحالة مقسومة بحسب الشاشات:
//   home      → query/type/results/page/loading + صفوف الاستعراض وTop
//   details   → currentItem/details/seasons/activeEpisode/playback
//   player    → player
//   collection → activeCollection/collectionItems (المفضلة/المشاهدات/قوائمي)
// =============================================================================

import { reactive } from 'vue'
import * as api from './services/api.js'
import {
  loadFavorites,
  saveFavorites,
  loadUserLists,
  saveUserLists,
} from './services/lists.js'

const RECENT_KEY = 'cinemana_recent'

export const store = reactive({
  // ---------- الشاشة الحالية ----------
  view: 'home', // home | details | player | collection

  // ---------- البحث ----------
  query: '',
  type: 'all', // all | movie | series
  results: [],
  page: 1,
  loading: false,
  searched: false, // هل بدأ المستخدم بالبحث؟ (بدونه نعرض الصفوف والـ Top)
  hasMore: false,

  // ---------- صفوف الاستعراض وTop (الصفحة الرئيسية) ----------
  // كل صف: { items, loading, loaded, error } — تُجلب عند اقتراب ظهورها.
  rows: {},
  top: {}, // tabKey -> { items, loading, loaded }
  topTab: 'top_movies',

  // ---------- لوحة الفلترة (يمين) ----------
  categories: [], // تصنيفات الخدمة (تُجلب مرة عند فتح اللوحة)
  browse: {
    active: false,
    title: '',
    filters: null,
    items: [],
    loading: false,
    page: 1,
    hasMore: false,
  },

  // ---------- شاشة التفاصيل ----------
  currentItem: null,
  details: null,
  detailsLoading: false,
  seasons: [],
  seasonsLoading: false,
  activeSeason: null,
  activeEpisode: null, // {id, label} عند اختيار حلقة

  // ---------- التشغيل ----------
  playback: null,
  playbackLoading: false,
  selectedQuality: 0, // فهرس الجودة المختارة (مرتبة تنازلياً من الباك-اند)
  selectedSubtitle: -1, // فهرس في playback.subtitles؛ -1 = بدون ترجمة
  player: null, // {id, src, sub, subLabel, subLang, title, label} — id لحفظ موضع المشاهدة

  // ---------- المفضلة والقوائم الخاصة ----------
  favorites: loadFavorites(), // [MediaSummary]
  userLists: loadUserLists(), // [{id, name, items:[MediaSummary]}]

  // ---------- شاشة استعراض قائمة ----------
  activeCollection: null, // {kind:'favorites'|'recents'|'user'|'row', key?, name}
  collectionItems: [],
  collectionLoading: false,
  rowSort: 'rating', // فرز صفحات "المزيد": rating | latest

  // ---------- عام ----------
  error: '',
  notice: '',
  settingsOpen: false,
  settings: null,
  recents: loadRecents(),

  // ---------- حجم بطاقات البوسترات (إعداد محلي) ----------
  // sm/md/lg/xl — يضبط متغير CSS عام --card-w فتعاد شبكة العرض كلها.
  cardSize: localStorage.getItem('cinemana_card_size') || 'md',
})

// CARD_SIZES خيارات حجم البطاقة بالبكسل (عرض البوستر).
export const CARD_SIZES = [
  { key: 'sm', label: 'صغير', px: 115 },
  { key: 'md', label: 'متوسط', px: 150 },
  { key: 'lg', label: 'كبير', px: 190 },
  { key: 'xl', label: 'ضخم', px: 235 },
]

// applyCardSizeVar تطبيق الحجم الحالي على متغير CSS الجذري —
// كل الشبكات والصفوف تستخدم minmax(var(--card-w),1fr) فتعاد تلقائياً.
export function applyCardSizeVar() {
  const hit = CARD_SIZES.find((s) => s.key === store.cardSize) || CARD_SIZES[1]
  document.documentElement.style.setProperty('--card-w', hit.px + 'px')
}

// setCardSize تغيير الحجم وحفظه محلياً.
export function setCardSize(key) {
  store.cardSize = key
  localStorage.setItem('cinemana_card_size', key)
  applyCardSizeVar()
}

// =============================================================================
// أدوات مساعدة
// =============================================================================

// debounce بسيط لشريط البحث السريع: ننتظر توقف الكتابة 350ms ثم نبحث.
// ملاحظة إصلاح مهم: القيمة المكتوبة يجب أن تُخزن في store.query —
// بدون هذا السطر كان البحث يقرأ نصاً فارغاً ولا يعمل أبداً.
let searchTimer = null
export function onQueryInput(value) {
  store.query = value
  clearTimeout(searchTimer)
  if (!value.trim()) {
    clearSearch()
    return
  }
  searchTimer = setTimeout(() => doSearch(true), 350)
}

// clearSearch إلغاء البحث والعودة لصفحة الاستكشاف (Top + الصفوف).
export function clearSearch() {
  clearTimeout(searchTimer)
  store.query = ''
  store.results = []
  store.searched = false
  store.hasMore = false
  store.loading = false
  clearBrowse()
}

function fail(err) {
  store.error = typeof err === 'string' ? err : err?.message || String(err)
}

// تحويل ثوانٍ إلى مدة مقروءة: "1 س 47 د" أو "55 د".
export function fmtDuration(sec) {
  const n = Math.round(parseFloat(sec) || 0)
  if (n <= 0) return ''
  const h = Math.floor(n / 3600)
  const m = Math.round((n % 3600) / 60)
  return h > 0 ? `${h} س ${String(m).padStart(2, '0')} د` : `${m} د`
}

function loadRecents() {
  try {
    return JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')
  } catch {
    return []
  }
}

function pushRecent(item) {
  const entry = pickCardFields(item)
  store.recents = [entry, ...store.recents.filter((r) => r.id !== entry.id)].slice(0, 12)
  localStorage.setItem(RECENT_KEY, JSON.stringify(store.recents))
}

// pickCardFields يستخرج الحقول الدنيا التي تحتاجها بطاقة البوستر/القوائم.
function pickCardFields(item) {
  return {
    id: String(item.id),
    ar_title: item.ar_title || '',
    en_title: item.en_title || '',
    type: item.type || 'movie',
    year: item.year || '',
    rating: item.rating || '',
    poster_url: item.poster_url || '',
    thumbnail_url: item.thumbnail_url || '',
    categories: item.categories || [],
  }
}

// =============================================================================
// البحث
// =============================================================================

export async function doSearch(reset = true) {
  const q = store.query.trim()
  if (!q) {
    store.results = []
    store.searched = false
    store.hasMore = false
    return
  }
  store.loading = true
  store.error = ''
  store.browse.active = false // البحث له الأولوية على نتائج التصفح
  try {
    const page = reset ? 1 : store.page + 1
    const items = await api.search(q, store.type, page)
    store.results = reset ? items : [...store.results, ...items]
    store.page = page
    store.searched = true
    // الخدمة ترجع صفحات كاملة عادة؛ الصفحة الفارغة تعني نهاية النتائج.
    store.hasMore = items.length > 0
  } catch (e) {
    fail(e)
  } finally {
    store.loading = false
  }
}

export function setType(t) {
  if (store.type === t) return
  store.type = t
  if (store.query.trim()) doSearch(true)
}

export function loadMore() {
  if (!store.loading && store.hasMore) doSearch(false)
}

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
    store.rows[key].error = typeof e === 'string' ? e : e?.message || String(e)
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
    fail(e)
    store.top[tabKey].loaded = true // لا نعيد الجلب اللانهائي عند الخطأ
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
export async function ensureCategories() {
  if (store.categories.length) return
  try {
    store.categories = await api.categories()
  } catch (e) {
    fail(e)
  }
}

// applyFilters ينفذ التصفح بالفلاتر الحالية ويعرض الشبكة في الرئيسية.
export async function applyFilters(filters, reset = true) {
  const page = reset ? 1 : store.browse.page + 1
  store.browse.active = true
  store.browse.loading = true
  store.browse.filters = filters
  store.browse.page = page
  try {
    const items = await api.browse({ ...filters, page })
    store.browse.items = reset ? items : [...store.browse.items, ...items]
    store.browse.hasMore = items.length >= 30 // الصفحة ممتلئة غالباً = يوجد المزيد
    store.browse.title = browseTitle(filters)
    store.searched = false
    store.results = []
  } catch (e) {
    fail(e)
  } finally {
    store.browse.loading = false
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
  store.browse = { active: false, title: '', filters: null, items: [], loading: false, page: 1, hasMore: false }
}

// =============================================================================
// المفضلة والقوائم الخاصة (تُحفظ محلياً وتظهر في الشريط الجانبي)
// =============================================================================

export function isFavorite(id) {
  return store.favorites.some((f) => f.id === String(id))
}

export function toggleFavorite(item) {
  const card = pickCardFields(item)
  if (isFavorite(card.id)) {
    store.favorites = store.favorites.filter((f) => f.id !== card.id)
    store.notice = 'أُزيل من المفضلة'
  } else {
    store.favorites = [card, ...store.favorites]
    store.notice = 'أُضيف إلى المفضلة'
  }
  saveFavorites(store.favorites)
}

export function createUserList(name) {
  const clean = (name || '').trim()
  if (!clean) return null
  const list = { id: String(Date.now()), name: clean, items: [] }
  store.userLists = [...store.userLists, list]
  saveUserLists(store.userLists)
  return list
}

export function deleteUserList(id) {
  store.userLists = store.userLists.filter((l) => l.id !== id)
  saveUserLists(store.userLists)
  // إن كنا نعرض هذه القائمة حالياً نعود للرئيسية.
  if (store.activeCollection?.kind === 'user' && store.activeCollection?.key === id) {
    closeCollection()
  }
}

export function addToUserList(listId, item) {
  const list = store.userLists.find((l) => l.id === listId)
  if (!list) return
  const card = pickCardFields(item)
  if (list.items.some((x) => x.id === card.id)) {
    store.notice = 'موجود مسبقاً في القائمة'
    return
  }
  list.items = [card, ...list.items]
  store.userLists = [...store.userLists]
  saveUserLists(store.userLists)
  store.notice = `أُضيف إلى «${list.name}»`
}

export function removeFromUserList(listId, itemId) {
  const list = store.userLists.find((l) => l.id === listId)
  if (!list) return
  list.items = list.items.filter((x) => x.id !== String(itemId))
  store.userLists = [...store.userLists]
  saveUserLists(store.userLists)
  // تحديث العرض إن كنا نتصفح هذه القائمة.
  if (store.activeCollection?.kind === 'user' && store.activeCollection?.key === listId) {
    store.collectionItems = list.items
  }
}

// =============================================================================
// شاشة استعراض قائمة (مفضلة / مشاهدات أخيرة / قائمة مستخدم / صفحة "المزيد")
// =============================================================================

export function openCollection(kind, key = null, name = '') {
  store.activeCollection = { kind, key, name }
  if (kind === 'favorites') {
    store.collectionItems = store.favorites
  } else if (kind === 'recents') {
    store.collectionItems = store.recents
  } else {
    const list = store.userLists.find((l) => l.id === key)
    store.collectionItems = list ? list.items : []
  }
  store.view = 'collection'
}

// openRowPage صفحة كاملة لصنف من الصفوف (زر "المزيد" في الرئيسية):
// تجلب دفعة كبيرة (60) من نفس مفتاح الصنف — والفرز بالتقييم متاح فيها.
export async function openRowPage(key, title) {
  store.activeCollection = { kind: 'row', key, name: title }
  store.rowSort = 'rating'
  store.collectionItems = []
  store.collectionLoading = true
  store.view = 'collection'
  try {
    store.collectionItems = await api.collection(key, 60)
  } catch (e) {
    fail(e)
  } finally {
    store.collectionLoading = false
  }
}

// setRowSort تبديل فرز صفحة "المزيد" (rating | latest).
export function setRowSort(sort) {
  store.rowSort = sort
}

// closeCollection ينهي وضع التصفح ويعود للرئيسية.
export function closeCollection() {
  store.view = 'home'
  store.activeCollection = null
  store.collectionItems = []
}

// =============================================================================
// شاشة التفاصيل
// =============================================================================

export async function openDetails(item) {
  store.currentItem = item
  store.view = 'details'
  store.details = null
  store.playback = null
  store.seasons = []
  store.activeEpisode = null
  store.activeSeason = null
  pushRecent(item)

  store.detailsLoading = true
  try {
    store.details = await api.details(item.id)
    if (store.details.type === 'series') {
      store.seasonsLoading = true
      store.seasons = await api.episodes(item.id)
      // نفتح الموسم الأول تلقائياً ليظهر المحتوى فوراً.
      store.activeSeason = store.seasons[0]?.season ?? null
    } else {
      // الأفلام: نجلب معلومات التشغيل فوراً مع تمرير معرفات النسخ
      // المكررة — إن كانت النسخة الأساسية ميتة جُرب البديل تلقائياً.
      await preparePlayback(item.id, item.alts || [])
    }
  } catch (e) {
    fail(e)
  } finally {
    store.detailsLoading = false
    store.seasonsLoading = false
  }
}

export function closeDetails() {
  // العودة: للقائمة إن أتينا منها، وإلا للرئيسية.
  if (store.activeCollection && store.view === 'details') {
    store.view = 'collection'
  } else {
    store.view = 'home'
  }
  store.currentItem = null
  store.details = null
  store.playback = null
}

export function selectSeason(num) {
  store.activeSeason = num
}

// اختيار حلقة: نجلب معلومات التشغيل الخاصة بها (لكل حلقة معرّفها الخاص).
// شريط التشغيل ثابت أعلى صفحة التفاصيل فيظهر فور الاختيار دون تمرير.
export async function selectEpisode(ep) {
  store.activeEpisode = { id: ep.id, label: `موسم ${store.activeSeason} — حلقة ${ep.episode_number}` }
  await preparePlayback(ep.id)
}

// =============================================================================
// التشغيل
// =============================================================================

// preparePlayback يجلب الجودات والترجمات (فيلم أو حلقة حسب المعرف).
// alts: معرفات النسخ المكررة لنفس العمل — يجرّبها الباك-اند تلقائياً
// إذا كانت نسخة المعرف الأساسي ميتة (ملفاتها محذوفة من الخدمة).
export async function preparePlayback(nb, alts = []) {
  store.playback = null
  store.playbackLoading = true
  store.selectedQuality = 0
  try {
    store.playback = await api.playback(nb, alts)
    // افتراضياً: أعلى جودة (القائمة مرتبة تنازلياً من الباك-اند)
    // وأول ترجمة عربية إن وُجدت.
    const arIdx = store.playback.subtitles.findIndex((s) => s.lang_code === 'ar')
    store.selectedSubtitle = store.playback.subtitles.length ? (arIdx >= 0 ? arIdx : 0) : -1
  } catch (e) {
    fail(e)
  } finally {
    store.playbackLoading = false
  }
}

// playbackTitle عنوان كامل للاستخدام في المشغل (فيلم أو حلقة).
export function playbackTitle() {
  if (!store.details) return ''
  if (store.activeEpisode) return `${store.details.title} — ${store.activeEpisode.label}`
  return store.details.title
}

// play يبدأ التشغيل المدمج: مصدر الفيديو عبر وكيل البث المحلي
// (local_url) + ملف الترجمة المحول VTT (local_url أيضاً).
export function play() {
  const p = store.playback
  if (!p || !p.qualities.length) return
  const q = p.qualities[store.selectedQuality]
  const sub = store.selectedSubtitle >= 0 ? p.subtitles[store.selectedSubtitle] : null
  store.player = {
    id: p.id, // معرف الفيلم/الحلقة — مفتاح حفظ موضع المشاهدة
    src: q.local_url,
    sub: sub?.local_url || '',
    subLabel: sub?.language || '',
    subLang: sub?.lang_code || 'ar',
    title: playbackTitle(),
    label: q.resolution,
  }
  store.view = 'player'
}

// playMpv يمرر الرابط الأصلي (remote_url) لمشغل mpv الخارجي — الترويسات
// يضيفها الباك-اند كوسيطات mpv.
export async function playMpv() {
  const p = store.playback
  if (!p || !p.qualities.length) return
  const q = p.qualities[store.selectedQuality]
  const sub = store.selectedSubtitle >= 0 ? p.subtitles[store.selectedSubtitle] : null
  try {
    await api.openInMPV(q.remote_url, sub?.remote_url || '', playbackTitle())
    store.notice = 'تم إرسال الفيديو إلى مشغل mpv'
  } catch (e) {
    fail(e)
  }
}

export function closePlayer() {
  // إزالة الكائن تلغي عنصر <video> (v-if) فيتوقف البث فوراً.
  store.player = null
  store.view = 'details'
}

// =============================================================================
// الإعدادات
// =============================================================================

export async function openSettings() {
  try {
    store.settings = await api.getSettings()
    store.settingsOpen = true
  } catch (e) {
    fail(e)
  }
}

export function closeSettings() {
  store.settingsOpen = false
}

export async function saveSettings(s) {
  try {
    store.settings = await api.saveSettings(s)
    store.settingsOpen = false
    store.notice = 'تم حفظ الإعدادات'
  } catch (e) {
    fail(e)
  }
}

// مسح رسالة الخطأ يدوياً (زر إغلاق في التنبيه).
export function dismissError() {
  store.error = ''
}

// =============================================================================
// تصدير/استيراد بيانات المستخدم (JSON) — عبر حوارات الملفات في Go
// =============================================================================

// exportUserData يصدر المفضلة والقوائم الخاصة والمشاهدات الأخيرة.
export async function exportUserData() {
  try {
    const payload = JSON.stringify(
      {
        version: 1,
        app: 'CinemaNa Next',
        exported_at: new Date().toISOString(),
        favorites: store.favorites,
        user_lists: store.userLists,
        recents: store.recents,
      },
      null,
      2,
    )
    const path = await api.exportUserData(payload)
    if (path) store.notice = 'تم التصدير إلى: ' + path
  } catch (e) {
    fail(e)
  }
}

// importUserData يستعيد بيانات من ملف JSON مع دمجها بالحالية
// (المستورد أولاً، وإزالة التكرار حسب المعرّف).
export async function importUserData() {
  try {
    const raw = await api.importUserData()
    if (!raw) return
    const data = JSON.parse(raw)
    const favs = Array.isArray(data.favorites) ? data.favorites : []
    const lists = Array.isArray(data.user_lists) ? data.user_lists : []
    const recs = Array.isArray(data.recents) ? data.recents : []
    if (!favs.length && !lists.length && !recs.length) {
      store.error = 'الملف لا يحتوي بيانات صالحة للاستيراد'
      return
    }
    const uniq = (arr) => {
      const seen = new Set()
      return arr.filter((x) => !seen.has(String(x.id)) && seen.add(String(x.id)))
    }
    store.favorites = uniq([...favs, ...store.favorites])
    const byId = new Map(store.userLists.map((l) => [l.id, l]))
    for (const l of lists) byId.set(String(l.id), { ...l, id: String(l.id), items: l.items || [] })
    store.userLists = [...byId.values()]
    store.recents = uniq([...recs, ...store.recents]).slice(0, 12)
    saveFavorites(store.favorites)
    saveUserLists(store.userLists)
    localStorage.setItem(RECENT_KEY, JSON.stringify(store.recents))
    store.notice = `تم الاستيراد: ${favs.length} مفضلة و${lists.length} قائمة`
  } catch (e) {
    store.error = 'فشل الاستيراد: الملف غير صالح — ' + (typeof e === 'string' ? e : e?.message || e)
  }
}
