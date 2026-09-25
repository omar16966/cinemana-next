// vite.config.js
// إعدادات أداة البناء للواجهة الأمامية:
//   - vue(): تحليل ملفات .vue أحادية الملف.
//   - tailwindcss(): إضافة Tailwind v4 المدمجة مع Vite (بلا postcss.config).
//   - base: './' مسارات نسبية — تعمل سواء خُدمت الملفات من Wails أو فتحت محلياً.
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  base: './',
})
