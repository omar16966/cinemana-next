// =============================================================================
// store/settings.js — الإعدادات (يحفظها الباك-اند) وحجم بطاقات البوسترات (محلي).
// =============================================================================

import * as api from '../services/api.js'
import { safeSet } from '../services/lists.js'
import { store, fail } from './state.js'

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
  if (!CARD_SIZES.some((s) => s.key === key)) return
  store.cardSize = key
  safeSet('cinemana_card_size', key)
  applyCardSizeVar()
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
    await api.saveSettings(s) // Go تعيد خطأً فقط (لا قيمة)
    store.settings = await api.getSettings() // القيم المطبَّعة كما حُفظت فعلاً
    store.settingsOpen = false
    store.notice = 'تم حفظ الإعدادات'
    return true
  } catch (e) {
    fail(e)
    return false
  }
}

// مسح رسالة الخطأ يدوياً (زر إغلاق في التنبيه).
export function dismissError() {
  store.error = ''
}
