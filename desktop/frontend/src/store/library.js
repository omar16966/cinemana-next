// =============================================================================
// store/library.js — المفضلة والقوائم الخاصة والمشاهدات الأخيرة، مع تصدير/استيراد
// JSON. كل ما يُقرأ من التخزين أو ملف مستورد يُطبَّع ويُقيَّد قبل لمس الحالة.
// =============================================================================

import * as api from '../services/api.js'
import {
  saveFavorites,
  saveUserLists,
  safeSet,
  pickCardFields,
  normalizeCards,
  normalizeLists,
} from '../services/lists.js'
import { store, RECENT_KEY, fail } from './state.js'
import { refreshCollection, closeCollection } from './collections.js'

// pushRecent لا يرمي أبداً: فشل التخزين يجب ألا يوقف فتح التفاصيل.
export function pushRecent(item) {
  try {
    const entry = pickCardFields(item)
    store.recents = [entry, ...store.recents.filter((r) => r.id !== entry.id)].slice(0, 12)
    safeSet(RECENT_KEY, JSON.stringify(store.recents))
  } catch {
    /* المشاهدات الأخيرة ميزة ثانوية */
  }
}

// =============================================================================
// المفضلة والقوائم الخاصة (تُحفظ محلياً وتظهر في الشريط الجانبي)
// =============================================================================

export function isFavorite(id) {
  const key = String(id)
  return store.favorites.some((f) => f.id === key)
}

// persist يحفظ ويخبر المستخدم إن فشل (الحالة في الذاكرة تبقى سليمة).
function persist(ok) {
  if (!ok) store.error = 'تعذر الحفظ في التخزين المحلي — ستُفقد التغييرات عند إغلاق التطبيق'
  return ok
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
  persist(saveFavorites(store.favorites))
  refreshCollection()
}

export function createUserList(name) {
  const clean = (name || '').trim().slice(0, 80)
  if (!clean) return null
  const list = { id: String(Date.now()), name: clean, items: [] }
  store.userLists = [...store.userLists, list]
  persist(saveUserLists(store.userLists))
  return list
}

export function deleteUserList(id) {
  // حذف القائمة لا رجعة فيه: نطلب تأكيداً (إن توفر confirm في الـ WebView).
  const list = store.userLists.find((l) => l.id === id)
  if (!list) return
  if (typeof window.confirm === 'function' && !window.confirm(`حذف القائمة «${list.name}» (${list.items.length} عمل)؟`)) return
  store.userLists = store.userLists.filter((l) => l.id !== id)
  persist(saveUserLists(store.userLists))
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
  persist(saveUserLists(store.userLists))
  store.notice = `أُضيف إلى «${list.name}»`
  refreshCollection()
}

export function removeFromUserList(listId, itemId) {
  const list = store.userLists.find((l) => l.id === listId)
  if (!list) return
  list.items = list.items.filter((x) => x.id !== String(itemId))
  store.userLists = [...store.userLists]
  persist(saveUserLists(store.userLists))
  refreshCollection()
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
// (المستورد أولاً، وإزالة التكرار حسب المعرّف). المدخلات تُطبَّع وتُقيَّد قبل
// لمس الحالة، والقوائم ذات المعرّف نفسه تُدمج عناصرها بدل استبدالها،
// ولا تتغير الحالة في الذاكرة إلا بعد نجاح الحفظ.
export async function importUserData() {
  let raw
  try {
    raw = await api.importUserData()
  } catch (e) {
    fail(e)
    return
  }
  if (!raw) return
  let data
  try {
    data = JSON.parse(raw)
  } catch (e) {
    store.error = 'فشل الاستيراد: الملف ليس JSON صالحاً'
    return
  }
  if (!data || typeof data !== 'object' || Array.isArray(data)) {
    store.error = 'فشل الاستيراد: بنية الملف غير معروفة'
    return
  }
  const favs = normalizeCards(data.favorites)
  const lists = normalizeLists(data.user_lists)
  const recs = normalizeCards(data.recents, 12)
  if (!favs.length && !lists.length && !recs.length) {
    store.error = 'الملف لا يحتوي بيانات صالحة للاستيراد'
    return
  }
  const nextFavs = normalizeCards([...favs, ...store.favorites])
  const nextLists = normalizeLists([...store.userLists, ...lists])
  const nextRecs = normalizeCards([...recs, ...store.recents], 12)

  const ok =
    saveFavorites(nextFavs) &&
    saveUserLists(nextLists) &&
    safeSet(RECENT_KEY, JSON.stringify(nextRecs))
  store.favorites = nextFavs
  store.userLists = nextLists
  store.recents = nextRecs
  refreshCollection()
  if (!persist(ok)) return
  store.notice = `تم الاستيراد: ${favs.length} مفضلة و${lists.length} قائمة`
}

