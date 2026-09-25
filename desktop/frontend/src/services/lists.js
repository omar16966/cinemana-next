// =============================================================================
// services/lists.js — تخزين محلي لقوائم المستخدم (بلا حساب ولا سيرفر):
//   - المفضلة (favorites)
//   - القوائم الخاصة التي ينشئها المستخدم (user lists)
// التخزين localStorage داخل WebView2 يبقى محفوظاً بين جلسات التطبيق.
// =============================================================================

const FAV_KEY = 'cinemana_favorites'
const LISTS_KEY = 'cinemana_user_lists'

function readJSON(key, fallback) {
  try {
    const v = JSON.parse(localStorage.getItem(key) || 'null')
    return Array.isArray(v) ? v : fallback
  } catch {
    return fallback
  }
}

function writeJSON(key, value) {
  localStorage.setItem(key, JSON.stringify(value))
}

/** المفضلة: مصفوفة من عناصر MediaSummary المختصرة. */
export const loadFavorites = () => readJSON(FAV_KEY, [])
export const saveFavorites = (list) => writeJSON(FAV_KEY, list)

/**
 * قوائم المستخدم: [{ id, name, items: [MediaSummary...] }]
 * id نصي ثابت (timestamp) يكفي محلياً.
 */
export const loadUserLists = () => readJSON(LISTS_KEY, [])
export const saveUserLists = (lists) => writeJSON(LISTS_KEY, lists)
