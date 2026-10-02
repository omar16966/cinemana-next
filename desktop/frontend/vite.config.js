// vite.config.js
// إعدادات أداة البناء للواجهة الأمامية:
//   - vue(): تحليل ملفات .vue أحادية الملف.
//   - tailwindcss(): إضافة Tailwind v4 المدمجة مع Vite (بلا postcss.config).
//   - base: './' مسارات نسبية — تعمل سواء خُدمت الملفات من Wails أو فتحت محلياً.
//   - cspPlugin(): سياسة أمان المحتوى (CSP) تُحقن في الإصدار النهائي فقط.
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// سياسة CSP للتطبيق (دفاع في العمق: لا v-html في الكود، لكن الواجهة تعرض
// نصوصاً قادمة من الشبكة، وعنصر WebView قد يُحمّل ما لا نريد).
//   script-src 'self'      لا سكربتات مضمنة ولا eval؛ حزمة Vite وwails/ipc.js من نفس الأصل.
//   style-src +unsafe-inline  Vue يكتب style="..." على العناصر (ربط :style).
//   img-src https: data:   بوسترات المزود (روابط https موقّعة) + صور مضمنة صغيرة.
//   media-src/connect-src  وكيل البث المحلي على 127.0.0.1 (منفذ عشوائي) + blob: لـ hls.js.
//   worker-src blob:       hls.js يشغّل عاملاً من blob.
//   object/frame/base/form مغلقة تماماً.
export const CSP = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' https: data: blob:",
  "font-src 'self' data:",
  "media-src 'self' http://127.0.0.1:* blob:",
  "connect-src 'self' http://127.0.0.1:*",
  "worker-src 'self' blob:",
  "object-src 'none'",
  "frame-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
].join('; ')

// يحقن <meta http-equiv="Content-Security-Policy"> في البناء فقط: وضع التطوير
// (wails dev / vite) يحتاج websocket وسكربتات HMR مضمنة فلا يُقيَّد.
function cspPlugin() {
  return {
    name: 'inject-csp',
    apply: 'build',
    transformIndexHtml() {
      return [
        {
          tag: 'meta',
          attrs: { 'http-equiv': 'Content-Security-Policy', content: CSP },
          injectTo: 'head-prepend',
        },
      ]
    },
  }
}

export default defineConfig({
  plugins: [vue(), tailwindcss(), cspPlugin()],
  base: './',
})
