<script setup>
// =============================================================================
// FilterPanel.vue — لوحة الفلترة والبحث (على يمين النافذة):
//   بحث نصي + النوع (فيلم/مسلسل) + اللغة (عربي/أجنبي) + التصنيف
//   + أدنى تقييم + نطاق السنة — ثم عرض النتائج كشبكة في الرئيسية.
// الفلاتر تعمل عبر نقطة AdvancedSearch (نجم الحد الأدنى/السنة/التصنيف)
// وvideosByCategoryAndLanguage للغة — التفاصيل في core/cinemana/browse.go.
// =============================================================================
import { onMounted, reactive, ref } from 'vue'
import {
  store,
  applyFilters,
  clearBrowse,
  clearSearch,
  ensureCategories,
} from '../store.js'

const open = ref(true)

// الحالة المحلية للفلاتر (تُطبق عند الضغط على "تطبيق" — تجربة أسرع).
const f = reactive({
  query: '',
  video_kind: '',
  language_id: '',
  category_id: '',
  min_star: '',
  year_from: '',
  year_to: '',
})

onMounted(() => ensureCategories())

// السنوات المتاحة في القائمتين المنسدلتين.
const years = Array.from({ length: new Date().getFullYear() - 1969 }, (_, i) => String(new Date().getFullYear() - i))

function apply() {
  // مزامنة البحث النصي مع الشريط العلوي إن كتب هنا.
  store.query = f.query
  applyFilters({ ...f }, true)
}

function clearAll() {
  Object.assign(f, { query: '', video_kind: '', language_id: '', category_id: '', min_star: '', year_from: '', year_to: '' })
  store.query = ''
  clearSearch()
  clearBrowse()
}
</script>

<template>
  <aside class="flex h-full w-60 shrink-0 flex-col border-s border-white/5 bg-ink-900/60">
    <!-- رأس اللوحة مع طي/فتح -->
    <button
      @click="open = !open"
      class="flex items-center justify-between px-4 py-3 text-sm font-bold text-zinc-200"
    >
      <span class="flex items-center gap-2">
        <svg viewBox="0 0 24 24" class="h-4.5 w-4.5" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M4 6h10M4 12h7M4 18h4M17 4v12m0 0-3-3m3 3 3-3" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        فلترة واستعراض
      </span>
      <svg viewBox="0 0 24 24" class="h-4 w-4 transition" :class="open ? 'rotate-180' : ''" fill="none" stroke="currentColor" stroke-width="2.5">
        <path d="m6 9 6 6 6-6" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </button>

    <div v-if="open" class="flex min-h-0 flex-1 flex-col gap-3.5 overflow-y-auto px-4 pb-4">
      <!-- البحث النصي -->
      <label class="block">
        <span class="mb-1 block text-xs font-semibold text-zinc-400">بحث بالعنوان</span>
        <input
          v-model="f.query"
          @keyup.enter="apply"
          type="text"
          placeholder="اسم فيلم أو مسلسل..."
          class="w-full rounded-lg border border-white/10 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-accent-500/60"
        />
      </label>

      <!-- النوع -->
      <div>
        <span class="mb-1.5 block text-xs font-semibold text-zinc-400">النوع</span>
        <div class="flex rounded-lg border border-white/10 bg-ink-800 p-0.5">
          <button
            v-for="t in [{ v: '', l: 'الكل' }, { v: '1', l: 'أفلام' }, { v: '2', l: 'مسلسلات' }]"
            :key="t.v"
            @click="f.video_kind = t.v"
            class="flex-1 rounded-md py-1.5 text-xs font-semibold transition"
            :class="f.video_kind === t.v ? 'bg-accent-500 text-white' : 'text-zinc-400 hover:text-white'"
          >
            {{ t.l }}
          </button>
        </div>
      </div>

      <!-- اللغة -->
      <div>
        <span class="mb-1.5 block text-xs font-semibold text-zinc-400">اللغة</span>
        <div class="flex rounded-lg border border-white/10 bg-ink-800 p-0.5">
          <button
            v-for="t in [{ v: '', l: 'الكل' }, { v: '9', l: 'عربي' }, { v: '7', l: 'أجنبي' }]"
            :key="t.v"
            @click="f.language_id = t.v"
            class="flex-1 rounded-md py-1.5 text-xs font-semibold transition"
            :class="f.language_id === t.v ? 'bg-accent-500 text-white' : 'text-zinc-400 hover:text-white'"
          >
            {{ t.l }}
          </button>
        </div>
      </div>

      <!-- التصنيف -->
      <label class="block">
        <span class="mb-1 block text-xs font-semibold text-zinc-400">التصنيف</span>
        <select
          v-model="f.category_id"
          class="w-full rounded-lg border border-white/10 bg-ink-800 px-2.5 py-2 text-sm outline-none focus:border-accent-500/60"
        >
          <option value="">كل التصنيفات</option>
          <option v-for="c in store.categories" :key="c.nb" :value="c.nb">{{ c.title }}</option>
        </select>
      </label>

      <!-- أدنى تقييم -->
      <label class="block">
        <span class="mb-1 block text-xs font-semibold text-zinc-400">أدنى تقييم</span>
        <select
          v-model="f.min_star"
          class="w-full rounded-lg border border-white/10 bg-ink-800 px-2.5 py-2 text-sm outline-none focus:border-accent-500/60"
        >
          <option value="">أي تقييم</option>
          <option v-for="n in [9, 8, 7, 6, 5, 4, 3, 2, 1]" :key="n" :value="String(n)">{{ n }} وأعلى ★</option>
        </select>
      </label>

      <!-- نطاق السنة -->
      <div>
        <span class="mb-1 block text-xs font-semibold text-zinc-400">سنة الإصدار</span>
        <div class="flex items-center gap-2">
          <select v-model="f.year_from" class="w-full rounded-lg border border-white/10 bg-ink-800 px-2 py-2 text-sm outline-none focus:border-accent-500/60">
            <option value="">من</option>
            <option v-for="y in years" :key="'f' + y" :value="y">{{ y }}</option>
          </select>
          <span class="text-zinc-600">-</span>
          <select v-model="f.year_to" class="w-full rounded-lg border border-white/10 bg-ink-800 px-2 py-2 text-sm outline-none focus:border-accent-500/60">
            <option value="">إلى</option>
            <option v-for="y in years" :key="'t' + y" :value="y">{{ y }}</option>
          </select>
        </div>
      </div>

      <!-- أزرار التنفيذ -->
      <div class="mt-1 flex gap-2">
        <button @click="apply" class="flex-1 rounded-lg bg-accent-500 py-2.5 text-sm font-bold text-white transition hover:bg-accent-400">
          تطبيق
        </button>
        <button @click="clearAll" class="rounded-lg border border-white/15 px-4 py-2.5 text-sm text-zinc-400 transition hover:text-white">
          مسح
        </button>
      </div>
    </div>
  </aside>
</template>
