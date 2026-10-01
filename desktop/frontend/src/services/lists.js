// =============================================================================
// services/lists.js — تخزين محلي لقوائم المستخدم (بلا حساب ولا سيرفر):
//   - المفضلة (favorites)
//   - القوائم الخاصة التي ينشئها المستخدم (user lists)
// التخزين localStorage داخل WebView2 يبقى محفوظاً بين جلسات التطبيق.
//
// كل الوصول للتخزين يمر من safeGet/safeSet: قد يرمي localStorage (حصة ممتلئة،
// تخزين معطّل، SecurityError) ولا يجوز أن يمنع ذلك إقلاع التطبيق أو يترك
// الحالة في الذاكرة مختلفة عن المحفوظ دون إخبار المستخدم.
// =============================================================================

const FAV_KEY = 'cinemana_favorites'
const LISTS_KEY = 'cinemana_user_lists'

/** قراءة آمنة: نص أو null. */
export function safeGet(key) {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

/** كتابة آمنة: true عند النجاح، false عند أي فشل. */
export function safeSet(key, value) {
  try {
    localStorage.setItem(key, value)
    return true
  } catch {
    return false
  }
}

/** pickCardFields يستخرج الحقول الدنيا التي تحتاجها بطاقة البوستر/القوائم. */
export function pickCardFields(item) {
  const strs = (v) => (Array.isArray(v) ? v.filter((x) => typeof x === 'string') : [])
  return {
    id: String(item.id),
    ar_title: typeof item.ar_title === 'string' ? item.ar_title : '',
    en_title: typeof item.en_title === 'string' ? item.en_title : '',
    type: item.type === 'series' ? 'series' : 'movie',
    year: item.year ? String(item.year) : '',
    rating: item.rating ? String(item.rating) : '',
    poster_url: typeof item.poster_url === 'string' ? item.poster_url : '',
    thumbnail_url: typeof item.thumbnail_url === 'string' ? item.thumbnail_url : '',
    categories: strs(item.categories),
    alts: strs(item.alts), // نسخ مكررة يجربها الباك-اند إن كانت النسخة الأساسية ميتة
  }
}

/** عنصر بطاقة صالح: كائن له id غير فارغ. */
const isCard = (x) => x && typeof x === 'object' && x.id != null && String(x.id) !== ''

/** تطبيع قائمة بطاقات من مصدر غير موثوق (تخزين/ملف مستورد) مع حد أقصى للعدد. */
export function normalizeCards(arr, max = 5000) {
  if (!Array.isArray(arr)) return []
  const seen = new Set()
  const out = []
  for (const x of arr) {
    if (!isCard(x)) continue
    const card = pickCardFields(x)
    if (seen.has(card.id)) continue
    seen.add(card.id)
    out.push(card)
    if (out.length >= max) break
  }
  return out
}

/** تطبيع قوائم المستخدم: [{id, name, items}] — أسماء نصية وعناصر سليمة. */
export function normalizeLists(arr, max = 200) {
  if (!Array.isArray(arr)) return []
  const byId = new Map()
  for (const l of arr) {
    if (!l || typeof l !== 'object' || l.id == null) continue
    const id = String(l.id)
    const name = typeof l.name === 'string' ? l.name.trim().slice(0, 80) : ''
    if (!name) continue
    const items = normalizeCards(l.items)
    const prev = byId.get(id)
    if (prev) {
      // نفس المعرّف: ندمج العناصر بدل استبدال القائمة المحلية.
      prev.items = normalizeCards([...prev.items, ...items])
    } else {
      byId.set(id, { id, name, items })
    }
    if (byId.size >= max) break
  }
  return [...byId.values()]
}

function readJSON(key) {
  try {
    return JSON.parse(safeGet(key) || 'null')
  } catch {
    return null
  }
}

/** المفضلة: مصفوفة من عناصر MediaSummary المختصرة. */
export const loadFavorites = () => normalizeCards(readJSON(FAV_KEY))
export const saveFavorites = (list) => safeSet(FAV_KEY, JSON.stringify(list))

/**
 * قوائم المستخدم: [{ id, name, items: [MediaSummary...] }]
 * id نصي ثابت (timestamp) يكفي محلياً.
 */
export const loadUserLists = () => normalizeLists(readJSON(LISTS_KEY))
export const saveUserLists = (lists) => safeSet(LISTS_KEY, JSON.stringify(lists))
