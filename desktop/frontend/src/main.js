import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import { store } from './store.js'

createApp(App).mount('#app')

// وضع المعاينة/التطوير: فضح الحالة للتشخيص من الكونسول (بلا أثر في البناء النهائي وظيفياً)
if (import.meta.env.DEV) {
  window.__store = store
}
