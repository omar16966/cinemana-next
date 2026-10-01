<script setup>
// =============================================================================
// ErrorToast.vue — تنبيه عائم سفلي يعرض أخطاء الشبكة/الخدمة (store.error)
// ورسائل النجاح العابرة (store.notice) ويخفي نفسه تلقائياً.
// =============================================================================
import { ref, watch, onBeforeUnmount } from 'vue'
import { store, dismissError } from '../store.js'

const visible = ref(false)
const isNotice = ref(false)
let errTimer = null
let noticeTimer = null

// الخطأ له الأولوية على الرسالة العابرة. لكل قناة مؤقتها الخاص حتى لا يلغي
// وصول خطأ مؤقت الرسالة (فتعاود الظهور بعد إغلاق الخطأ)، ويُعاد ضبط المؤقت
// حتى لو تكررت الرسالة نفسها (watch على النص وحده لا يلتقط التكرار).
function sync() {
  if (store.error) {
    isNotice.value = false
    visible.value = true
  } else if (store.notice) {
    isNotice.value = true
    visible.value = true
  } else {
    visible.value = false
  }
}

watch(
  () => store.error,
  (err) => {
    clearTimeout(errTimer)
    if (err) {
      store.notice = '' // لا رسالة قديمة تظهر بعد إغلاق الخطأ
      errTimer = setTimeout(() => dismissError(), 8000) // الأخطاء تبقى أطول
    }
    sync()
  },
)

watch(
  () => store.notice,
  (notice) => {
    clearTimeout(noticeTimer)
    if (notice) noticeTimer = setTimeout(() => (store.notice = ''), 3000)
    sync()
  },
)

onBeforeUnmount(() => {
  clearTimeout(errTimer)
  clearTimeout(noticeTimer)
})
</script>

<template>
  <Transition name="fade">
    <div
      v-if="visible"
      class="fixed bottom-6 start-1/2 z-[60] w-[92%] max-w-xl -translate-x-1/2 rtl:translate-x-1/2"
    >
      <div
        class="flex items-center gap-3 rounded-xl border p-4 shadow-2xl backdrop-blur"
        :class="isNotice ? 'border-emerald-500/30 bg-emerald-950/90' : 'border-red-500/30 bg-red-950/90'"
      >
        <svg v-if="isNotice" viewBox="0 0 24 24" class="h-5 w-5 shrink-0 text-emerald-400" fill="currentColor">
          <path d="M9 16.17 4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17Z" />
        </svg>
        <svg v-else viewBox="0 0 24 24" class="h-5 w-5 shrink-0 text-red-400" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="9" />
          <path d="M12 8v4M12 16h.01" stroke-linecap="round" />
        </svg>
        <p class="min-w-0 flex-1 text-sm leading-relaxed text-zinc-100">
          {{ isNotice ? store.notice : store.error }}
        </p>
        <button
          v-if="!isNotice"
          @click="dismissError"
          class="shrink-0 rounded-lg px-2 py-1 text-xs text-zinc-400 transition hover:text-white"
        >
          إغلاق
        </button>
      </div>
    </div>
  </Transition>
</template>
