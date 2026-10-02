// =============================================================================
// store/state.js — الحالة المركزية المشتركة (reactive) وأدوات مساعدة عامة.
// بدون مكتبات خارجية عمداً: reactive في Vue 3 يكفي تماماً لهذا الحجم.
//
// الحالة مقسومة بحسب الشاشات (والمنطق في وحدات مستقلة بجانب هذا الملف):
//   home       → search.js / browse.js   (query/results + صفوف + Top + الفلاتر)
//   details    → details.js              (currentItem/details/seasons/playback/player)
//   collection → collections.js          (المفضلة/المشاهدات/قوائمي/صفحة "المزيد")
//   library    → library.js              (مفضلة + قوائم + مشاهدات + استيراد/تصدير)
//   settings   → settings.js             (الإعدادات + حجم البطاقات)
// المكوّنات تستورد من ../store.js (واجهة موحدة تعيد تصدير كل هذه الوحدات).
// =============================================================================

import { reactive } from 'vue'
import { loadFavorites, loadUserLists, safeGet, normalizeCards } from '../services/lists.js'

export const RECENT_KEY = 'cinemana_recent'

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

  // مسودة الفلاتر وحالة طي اللوحة وموضع تمرير الرئيسية: تبقى عند التنقل بين
  // الشاشات (المكوّنات تُدمَّر عند مغادرة الرئيسية فلا تحتفظ بحالتها بنفسها).
  filterDraft: {
    query: '',
    video_kind: '',
    language_id: '',
    category_id: '',
    min_star: '',
    year_from: '',
    year_to: '',
  },
  filterOpen: true,
  homeScroll: 0,

  // ---------- شاشة التفاصيل ----------
  currentItem: null,
  detailsFrom: 'home', // الشاشة التي فُتحت منها التفاصيل (للعودة): home | collection
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
  collectionError: '',
  rowSort: 'rating', // فرز صفحات "المزيد": rating | latest

  // ---------- عام ----------
  error: '',
  notice: '',
  settingsOpen: false,
  settings: null,
  recents: loadRecents(),

  // ---------- حجم بطاقات البوسترات (إعداد محلي) ----------
  // sm/md/lg/xl — يضبط متغير CSS عام --card-w فتعاد شبكة العرض كلها.
  cardSize: loadCardSize(),
})

// loadCardSize يقرأ الحجم المحفوظ ويتحقق منه (قيمة تالفة = المتوسط).
function loadCardSize() {
  const v = safeGet('cinemana_card_size')
  return ['sm', 'md', 'lg', 'xl'].includes(v) ? v : 'md'
}

// أرقام تسلسلية لطلبات كل مورد: الاستجابة المتأخرة لطلب قديم تُهمل بدل أن
// تطغى على نتيجة أحدث (أو تعيد إحياء شاشة أُغلقت).
export const seq = { search: 0, details: 0, playback: 0, browse: 0, collection: 0, episodes: 0 }

// حجم صفحة التصفح/البحث كما يعيدها الباك-اند (desktop/app.go: Browse تقص إلى 30).
export const PAGE_SIZE = 30

// errMsg استخراج رسالة نصية من أي خطأ.
export function errMsg(err) {
  return typeof err === 'string' ? err : err?.message || String(err)
}

// mergeById يضيف عناصر جديدة إلى قائمة دون تكرار المعرّفات (مفاتيح v-for فريدة).
export function mergeById(base, extra) {
  const seen = new Set(base.map((x) => x.id))
  return [...base, ...extra.filter((x) => !seen.has(x.id))]
}

// مؤقتات مشتركة بين الوحدات (debounce البحث يُلغى من applyFilters أيضاً).
export const timers = { search: null }

export function fail(err) {
  store.error = errMsg(err)
}

function loadRecents() {
  try {
    return normalizeCards(JSON.parse(safeGet(RECENT_KEY) || 'null'), 12)
  } catch {
    return []
  }
}
