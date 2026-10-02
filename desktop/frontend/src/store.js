// =============================================================================
// store.js — واجهة موحدة للحالة المركزية: تعيد تصدير الوحدات حتى تبقى
// استيرادات المكوّنات كما هي (import { ... } from '../store.js').
//
// الوحدات في ./store/ (بلا اعتماد دائري: state ← collections ← library ← details):
//   state.js        الحالة المشتركة + seq/timers + أدوات صغيرة
//   utils.js        دوال تنسيق نقية
//   search.js       البحث السريع
//   browse.js       صفوف الرئيسية وTop ولوحة الفلترة
//   collections.js  شاشة القوائم وصفحة "المزيد"
//   library.js      المفضلة والقوائم والمشاهدات + استيراد/تصدير
//   details.js      التفاصيل والتشغيل
//   settings.js     الإعدادات وحجم البطاقات
// =============================================================================

export { store } from './store/state.js'
export * from './store/utils.js'
export * from './store/search.js'
export * from './store/browse.js'
export * from './store/collections.js'
export * from './store/library.js'
export * from './store/details.js'
export * from './store/settings.js'
