<script setup>
// =============================================================================
// HomeView.vue — الصفحة الرئيسية:
//   - بدون بحث: قسم "الأعلى تقييماً" ثم الصفوف الأفقية (أحدث الأفلام
//     الأجنبية/العربية/الأنمي...) — تُجلب الصفوف كسولاً عند اقتراب ظهورها.
//   - أثناء البحث: شبكة نتائج البحث كما كانت.
// =============================================================================
import { store, loadMore, moreBrowse, clearBrowse } from '../store.js'
import { HOME_ROWS } from '../store.js'
import PosterCard from './PosterCard.vue'
import SkeletonCard from './SkeletonCard.vue'
import TopSection from './TopSection.vue'
import CollectionRow from './CollectionRow.vue'
</script>

<template>
  <!-- التمرير الداخلي: هذا العرض يملأ ارتفاع العمود الأوسط -->
  <div class="h-full overflow-y-auto pb-4">
    <!-- ===== وضع البحث: شبكة النتائج ===== -->
    <template v-if="store.searched">
      <div class="mb-4 flex items-center justify-between">
        <h2 class="text-lg font-bold text-zinc-300">
          نتائج البحث
          <span v-if="store.results.length" class="text-sm font-normal text-zinc-500">({{ store.results.length }})</span>
        </h2>
      </div>

      <div class="grid gap-3 md:gap-4 grid-cols-[repeat(auto-fill,minmax(var(--card-w),1fr))]">
        <template v-if="store.loading && !store.results.length">
          <SkeletonCard v-for="i in 12" :key="'sk' + i" />
        </template>
        <PosterCard v-else v-for="item in store.results" :key="item.id" :item="item" />
      </div>

      <div v-if="!store.loading && !store.results.length" class="py-16 text-center text-zinc-500">
        لا توجد نتائج مطابقة. جرّب صياغة أخرى أو غيّر مرشح النوع.
      </div>

      <div v-if="store.hasMore" class="mt-8 text-center">
        <button
          @click="loadMore"
          :disabled="store.loading"
          class="rounded-full border border-white/10 bg-ink-800 px-8 py-2.5 text-sm font-semibold text-zinc-200 transition hover:border-accent-500/50 disabled:opacity-50"
        >
          {{ store.loading ? 'جارٍ التحميل...' : 'المزيد' }}
        </button>
      </div>
    </template>

    <!-- ===== وضع التصفح بالفلاتر (لوحة اليمين) ===== -->
    <template v-else-if="store.browse.active">
      <div class="mb-4 flex items-center justify-between">
        <h2 class="text-lg font-bold text-zinc-300">{{ store.browse.title }}</h2>
        <button
          @click="clearBrowse"
          class="rounded-lg border border-white/15 px-3.5 py-1.5 text-xs text-zinc-400 transition hover:border-white/40 hover:text-white"
        >
          إلغاء التصفح
        </button>
      </div>

      <div class="grid gap-3 md:gap-4 grid-cols-[repeat(auto-fill,minmax(var(--card-w),1fr))]">
        <template v-if="store.browse.loading && !store.browse.items.length">
          <SkeletonCard v-for="i in 12" :key="'bsk' + i" />
        </template>
        <PosterCard v-else v-for="item in store.browse.items" :key="item.id" :item="item" />
      </div>

      <div v-if="!store.browse.loading && !store.browse.items.length" class="py-16 text-center text-zinc-500">
        لا توجد نتائج مطابقة للفلاتر — جرّب توسيع النطاق.
      </div>

      <div v-if="store.browse.hasMore" class="mt-8 text-center">
        <button
          @click="moreBrowse"
          :disabled="store.browse.loading"
          class="rounded-full border border-white/10 bg-ink-800 px-8 py-2.5 text-sm font-semibold text-zinc-200 transition hover:border-accent-500/50 disabled:opacity-50"
        >
          {{ store.browse.loading ? 'جارٍ التحميل...' : 'المزيد' }}
        </button>
      </div>
    </template>

    <!-- ===== الوضع الافتراضي: Top + الصفوف الأفقية ===== -->
    <template v-else>
      <TopSection />
      <CollectionRow v-for="row in HOME_ROWS" :key="row.key" :title="row.title" :row-key="row.key" />
    </template>
  </div>
</template>
