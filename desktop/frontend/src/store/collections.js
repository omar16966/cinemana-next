// =============================================================================
// store/collections.js — شاشة استعراض قائمة (مفضلة / مشاهدات أخيرة / قائمة
// مستخدم / صفحة "المزيد" لصف من الرئيسية).
// =============================================================================

import * as api from '../services/api.js'
import { store, seq, errMsg } from './state.js'

// refreshCollection يزامن القائمة المعروضة مع مصدرها بعد أي تعديل
// (كان العرض يحتفظ بنسخة قديمة فلا يتحدث عند إزالة ♥ داخل شاشة المفضلة).
export function refreshCollection() {
  const c = store.activeCollection
  if (!c) return
  if (c.kind === 'favorites') store.collectionItems = store.favorites
  else if (c.kind === 'recents') store.collectionItems = store.recents
  else if (c.kind === 'user') {
    store.collectionItems = store.userLists.find((l) => l.id === c.key)?.items || []
  }
}

// =============================================================================
// شاشة استعراض قائمة (مفضلة / مشاهدات أخيرة / قائمة مستخدم / صفحة "المزيد")
// =============================================================================

export function openCollection(kind, key = null, name = '') {
  seq.collection++ // يلغي أي صفحة "مزيد" جارية
  store.activeCollection = { kind, key, name }
  store.collectionError = ''
  store.collectionLoading = false
  refreshCollection()
  store.view = 'collection'
}

// openRowPage صفحة كاملة لصنف من الصفوف (زر "المزيد" في الرئيسية):
// تجلب دفعة كبيرة (60) من نفس مفتاح الصنف — والفرز بالتقييم متاح فيها.
export async function openRowPage(key, title) {
  const my = ++seq.collection
  store.activeCollection = { kind: 'row', key, name: title }
  store.rowSort = 'rating'
  store.collectionItems = []
  store.collectionError = ''
  store.collectionLoading = true
  store.view = 'collection'
  try {
    const items = await api.collection(key, 60)
    if (my !== seq.collection) return // انتقل المستخدم لشاشة أخرى
    store.collectionItems = items
  } catch (e) {
    if (my === seq.collection) store.collectionError = errMsg(e)
  } finally {
    if (my === seq.collection) store.collectionLoading = false
  }
}

// setRowSort تبديل فرز صفحة "المزيد" (rating | latest).
export function setRowSort(sort) {
  store.rowSort = sort
}

// closeCollection ينهي وضع التصفح ويعود للرئيسية.
export function closeCollection() {
  seq.collection++
  store.collectionLoading = false
  store.collectionError = ''
  store.view = 'home'
  store.activeCollection = null
  store.collectionItems = []
}
