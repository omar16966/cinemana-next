<script setup>
// =============================================================================
// SettingsModal.vue — نافذة الإعدادات: مسار mpv الخارجي، العنوان الأساسي
// للخدمة، وترويسة User-Agent. تُحفظ في ملف إعدادات النظام عبر الباك-اند
// (settings.go) ويسري مفعولها فوراً دون إعادة تشغيل.
// =============================================================================
import { reactive, ref, onMounted, onBeforeUnmount } from 'vue'
import {
  store,
  closeSettings,
  saveSettings,
  setCardSize,
  CARD_SIZES,
  exportUserData,
  importUserData,
} from '../store.js'

// نسخة عمل محلية: لا نلمس الإعدادات الحية إلا عند الحفظ الفعلي.
const form = reactive({ ...store.settings })
const saving = ref(false)
const formError = ref('')

// تحقق أولي في الواجهة (الباك-اند يعيد التحقق ويرفض غير الصالح).
function validate() {
  const base = (form.base_url || '').trim()
  if (base) {
    try {
      const u = new URL(base.includes('://') ? base : 'https://' + base)
      if (u.protocol !== 'http:' && u.protocol !== 'https:') return 'العنوان الأساسي يجب أن يبدأ بـ http أو https'
    } catch {
      return 'العنوان الأساسي غير صالح'
    }
  }
  if (/[\r\n]/.test(form.user_agent || '')) return 'User-Agent لا يجوز أن يحوي أسطراً جديدة'
  return ''
}

async function save() {
  if (saving.value) return // يمنع النقر المزدوج
  formError.value = validate()
  if (formError.value) return
  // تعطيل التحقق من الشهادة يشمل كل الطلبات (الـ API والبث): تأكيد صريح.
  if (
    form.insecure_tls &&
    !store.settings?.insecure_tls &&
    typeof window.confirm === 'function' &&
    !window.confirm('تخطي التحقق من شهادة TLS يعطّل حماية الاتصال لكل الطلبات (البيانات والفيديو). هل تريد المتابعة؟')
  ) {
    return
  }
  saving.value = true
  try {
    await saveSettings({ ...form })
  } finally {
    saving.value = false
  }
}

function onKey(e) {
  if (e.key === 'Escape') closeSettings()
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <!-- طبقة التعتيم: النقر خارجها يغلقها -->
  <div class="fixed inset-0 z-[70] flex items-center justify-center bg-black/70 p-4" @click.self="closeSettings">
    <div class="w-full max-w-lg rounded-2xl border border-white/10 bg-ink-900 p-6 shadow-2xl">
      <h2 class="text-lg font-bold text-white">الإعدادات</h2>
      <p class="mt-1 text-xs text-zinc-500">تُحفظ في ملف إعدادات التطبيق وتسري فوراً</p>

      <div class="mt-5 space-y-4">
        <!-- تصدير/استيراد المفضلة والقوائم (JSON) -->
        <div>
          <span class="mb-1.5 block text-sm font-semibold text-zinc-300">المفضلة والقوائم — نسخ احتياطي</span>
          <div class="flex gap-2">
            <button
              @click="exportUserData"
              class="flex-1 rounded-lg border border-white/15 py-2.5 text-sm font-semibold text-zinc-200 transition hover:border-white/40 hover:text-white"
            >
              تصدير (JSON)
            </button>
            <button
              @click="importUserData"
              class="flex-1 rounded-lg border border-white/15 py-2.5 text-sm font-semibold text-zinc-200 transition hover:border-white/40 hover:text-white"
            >
              استيراد (JSON)
            </button>
          </div>
          <span class="mt-1 block text-xs text-zinc-500">
            التصدير يحفظ المفضلة والقوائم والمشاهدات في ملف — الاستيراد يدمج ملفاً محفوظاً مع الحالي
          </span>
        </div>

        <!-- حجم بطاقات البوسترات (يُطبق فوراً ويُحفظ محلياً) -->
        <div>
          <span class="mb-1.5 block text-sm font-semibold text-zinc-300">حجم بطاقات البوسترات</span>
          <div class="flex rounded-lg border border-white/10 bg-ink-800 p-1">
            <button
              v-for="s in CARD_SIZES"
              :key="s.key"
              @click="setCardSize(s.key)"
              class="flex-1 rounded-md py-1.5 text-xs font-semibold transition"
              :class="store.cardSize === s.key ? 'bg-accent-500 text-white' : 'text-zinc-400 hover:text-white'"
            >
              {{ s.label }}
            </button>
          </div>
          <span class="mt-1 block text-xs text-zinc-500">يُطبق فوراً على شبكات العرض والصفوف الأفقية</span>
        </div>

        <label class="block">
          <span class="mb-1.5 block text-sm font-semibold text-zinc-300">مسار مشغل mpv (اختياري)</span>
          <input
            v-model="form.mpv_path"
            type="text"
            placeholder="مثال: C:\\tools\\mpv\\mpv.exe — أو اتركه فارغاً للبحث في PATH"
            class="w-full rounded-lg border border-white/10 bg-ink-800 px-3 py-2.5 text-sm text-zinc-200 placeholder-zinc-600 outline-none focus:border-accent-500/60"
          />
          <span class="mt-1 block text-xs text-zinc-500">لتنزيل mpv مجاناً: winget install mpv — يلزم فقط لزر "فتح في MPV"</span>
        </label>

        <label class="block">
          <span class="mb-1.5 block text-sm font-semibold text-zinc-300">العنوان الأساسي للخدمة</span>
          <input
            v-model="form.base_url"
            type="text"
            dir="ltr"
            class="w-full rounded-lg border border-white/10 bg-ink-800 px-3 py-2.5 text-sm text-zinc-200 outline-none focus:border-accent-500/60"
          />
        </label>

        <label class="block">
          <span class="mb-1.5 block text-sm font-semibold text-zinc-300">ترويسة User-Agent (اختياري)</span>
          <input
            v-model="form.user_agent"
            type="text"
            dir="ltr"
            placeholder="فارغة = ترويسة جهاز أندرويد واقعية"
            class="w-full rounded-lg border border-white/10 bg-ink-800 px-3 py-2.5 text-sm text-zinc-200 placeholder-zinc-600 outline-none focus:border-accent-500/60"
          />
        </label>

        <label class="flex items-center gap-2 text-sm text-zinc-300">
          <input v-model="form.insecure_tls" type="checkbox" class="h-4 w-4 rounded accent-red-600" />
          تخطي التحقق من شهادة TLS (للشبكات المحلية فقط)
        </label>
      </div>

      <p v-if="formError" class="mt-4 text-sm text-red-400">{{ formError }}</p>

      <div class="mt-6 flex justify-end gap-3">
        <button @click="closeSettings" class="rounded-lg px-5 py-2.5 text-sm text-zinc-400 transition hover:text-white">
          إلغاء
        </button>
        <button @click="save" :disabled="saving" class="rounded-lg bg-accent-500 px-6 py-2.5 text-sm font-bold text-white transition hover:bg-accent-400 disabled:opacity-50">
          حفظ
        </button>
      </div>
    </div>
  </div>
</template>
