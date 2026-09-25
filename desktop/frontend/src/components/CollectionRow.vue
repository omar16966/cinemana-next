<script setup>
// =============================================================================
// CollectionRow.vue — صف أفقي (Netflix-style) لعناصر قائمة استعراض:
//   - التحميل كسول: يجلب عناصره فقط عندما يقترب من مجال الرؤية
//     (IntersectionObserver) — إقلاع سريع بلا 7 طلبات دفعة واحدة.
//   - تمرير أفقي بسهمين (يعملان صح مع اتجاه RTL).
// =============================================================================
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { store, ensureRow, openRowPage } from '../store.js'
import PosterCard from './PosterCard.vue'

const props = defineProps({
  title: { type: String, required: true },
  rowKey: { type: String, required: true },
})

const scroller = ref(null)
const root = ref(null)
let observer = null

// scrollRow تمرير بمقدار عرض مرئي تقريباً؛ الإشارة تتعامل مع RTL.
function scrollRow(dir) {
  const el = scroller.value
  if (!el) return
  el.scrollBy({ left: dir * (el.clientWidth * 0.8), behavior: 'smooth' })
}

onMounted(() => {
  observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((e) => e.isIntersecting)) {
        ensureRow(props.rowKey)
        observer.disconnect()
      }
    },
    { rootMargin: '300px' },
  )
  observer.observe(root.value)
})
onBeforeUnmount(() => observer && observer.disconnect())
</script>

<template>
  <section ref="root" class="mb-7">
    <div class="mb-2.5 flex items-center justify-between">
      <h2 class="text-base font-bold text-zinc-100 md:text-lg">{{ title }}</h2>
      <div class="flex items-center gap-2">
        <!-- المزيد: صفحة كاملة لهذا الصنف -->
        <button
          @click="openRowPage(rowKey, title)"
          class="rounded-full border border-white/10 bg-ink-800 px-4 py-1.5 text-xs font-semibold text-zinc-300 transition hover:border-accent-500/60 hover:text-white"
        >
          المزيد
        </button>
        <button
          @click="scrollRow(1)"
          class="flex h-7 w-7 items-center justify-center rounded-full border border-white/10 bg-ink-800 text-zinc-400 transition hover:border-white/30 hover:text-white"
          title="السابق"
        >
          <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.5">
            <path d="m9 5 7 7-7 7" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>
        <button
          @click="scrollRow(-1)"
          class="flex h-7 w-7 items-center justify-center rounded-full border border-white/10 bg-ink-800 text-zinc-400 transition hover:border-white/30 hover:text-white"
          title="التالي"
        >
          <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.5">
            <path d="M15 19 8 12l7-7" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>
      </div>
    </div>

    <!-- حاوية التمرير الأفقي -->
    <!-- إصلاح استهلاك المعالج: هيكل التحميل يظهر فقط أثناء الجلب الفعلي؛
         الصفوف التي لم تُطلَب بعد (أسفل الشاشة) لا تعرض شيئاً — بدون هذا
         كانت عشرات حركات اللمعان اللانهائية تعمل للأبد وصمتاً -->
    <div ref="scroller" class="flex gap-3 overflow-x-auto pb-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
      <!-- هيكل تحميل — فقط أثناء الجلب الفعلي -->
      <template v-if="store.rows[rowKey]?.loading">
        <div v-for="i in 8" :key="i" class="skeleton aspect-[2/3] w-[var(--card-w)] shrink-0 rounded-xl bg-ink-800"></div>
      </template>

      <!-- خطأ مع إعادة محاولة -->
      <div v-else-if="store.rows[rowKey]?.error" class="flex w-full items-center justify-between rounded-xl border border-red-500/20 bg-red-950/20 px-4 py-3">
        <span class="text-sm text-zinc-400">تعذر تحميل هذا الصف</span>
        <button
          @click="ensureRow(rowKey)"
          class="rounded-lg border border-white/15 px-4 py-1.5 text-xs text-zinc-200 hover:border-white/40"
        >
          إعادة المحاولة
        </button>
      </div>

      <!-- البطاقات (بعد الجلب) -->
      <template v-else-if="store.rows[rowKey]?.loaded">
        <div v-for="item in store.rows[rowKey].items" :key="item.id" class="w-[var(--card-w)] shrink-0">
          <PosterCard :item="item" compact />
        </div>
        <div v-if="!store.rows[rowKey].items.length" class="py-6 text-sm text-zinc-600">
          لا توجد عناصر في هذا الصف حالياً
        </div>
      </template>

      <!-- لم تُطلَب بعد: لا شيء (الصف أسفل مجال الرؤية) -->
    </div>
  </section>
</template>
