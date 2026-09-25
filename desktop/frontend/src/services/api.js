// =============================================================================
// services/api.js — طبقة الخدمة في الواجهة الأمامية
//
// القاعدة المعمارية: هذا الملف هو *الوحيد* في الواجهة الذي يعرف كيف
// نصل إلى منطق الشبكة. بقية المكونات تستدعي الدوال هنا فقط.
//
// داخل تطبيق Wails: يحقن Wails تلقائياً كائن window.go.main.App الذي
// يحتوي كل الدالة العامة في desktop/app.go — الاستدعاء Promise يعيد
// JSON جاهزاً، وأي خطأ Go يعاد كـ Promise مرفوض برسالة نصية.
//
// داخل متصفح عادي (npm run dev للتصميم): window.go غير موجود فنرجع
// إلى بيانات تجريبية من mock.js — يتيح تطوير/فحص الواجهة بلا تطبيق.
// =============================================================================

import * as mock from './mock.js'

// backend يعيد كائن الربط إن كنا داخل Wails، أو null في المتصفح.
function backend() {
  return typeof window !== 'undefined' && window.go && window.go.main.App
    ? window.go.main.App
    : null
}

// استدعاء موحد: يوجه للباك-اند الحقيقي أو للوضع التجريبي.
async function call(method, mockFn, ...args) {
  const b = backend()
  if (b) {
    return b[method](...args) // مثال: App.Search("فاست", "movie", 1)
  }
  return mockFn(...args)
}

/** بحث عن فيلم/مسلسل. يعيد: [ {id, ar_title, en_title, type, year, rating, poster_url, ...} ] */
export const search = (query, mediaType, page) =>
  call('Search', mock.search, query, mediaType, page)

/** تفاصيل عمل كاملة: عنوان/وصف/سنة/بوستر/ترجمات/تصنيفات. */
export const details = (nb) => call('GetDetails', mock.details, nb)

/** مواسم مسلسل وحلقاته: [ {season, episodes:[{id, episode_number, title, duration_sec}]} ] */
export const episodes = (nb) => call('GetEpisodes', mock.episodes, nb)

/** معلومات التشغيل: جودات (local_url للمشغل المدمج، remote_url للخارجي) + ترجمات VTT. */
export const playback = (nb, alts) => call('GetPlayback', mock.playback, nb, alts || [])

/** فتح الرابط في مشغل mpv الخارجي مع تمرير الترويسات (يجري في Go). */
export const openInMPV = (videoUrl, subtitleUrl, title) =>
  call('OpenInMPV', mock.openInMPV, videoUrl, subtitleUrl, title)

/** قراءة الإعدادات المحفوظة (مسار mpv، العنوان الأساسي، User-Agent). */
export const getSettings = () => call('GetSettings', mock.getSettings)

/** حفظ الإعدادات وتفعيلها فوراً في الباك-اند. */
export const saveSettings = (s) => call('SaveSettings', mock.saveSettings, s)

/** قائمة استعراض جاهزة (صفوف الرئيسية وقوائم Top): latest_movies_arabic، top_anime... */
export const collection = (key, limit) => call('GetCollection', mock.collection, key, limit)

/** تصفح بفلاتر لوحة اليمين: {query, video_kind, category_id, language_id, min_star, year_from, year_to, page} */
export const browse = (filters) => call('Browse', mock.browse, filters)

/** تصنيفات الخدمة (للقائمة المنسدلة في لوحة الفلترة). */
export const categories = () => call('GetCategories', mock.categories)

/** تصدير بيانات المستخدم إلى ملف JSON (حوار حفظ في Go). */
export const exportUserData = (dataJson) => call('ExportUserData', mock.exportUserData, dataJson)

/** استيراد بيانات المستخدم من ملف JSON (حوار اختيار في Go). */
export const importUserData = () => call('ImportUserData', mock.importUserData)
