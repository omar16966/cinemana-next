<script setup>
// =============================================================================
// CollectionView.vue — شاشة عرض قائمة:
//   - المفضلة / المشاهدات الأخيرة / قائمة خاصة بالمستخدم.
//   - صفحات "المزيد" (kind='row'): شبكة كبيرة مع فرز بالتقييم أو الأحدث.
// =============================================================================
import { computed, ref, watch } from 'vue'
import {
  store,
  closeCollection,
  deleteUserList,
  removeFromUserList,
  setRowSort,
  openRowPage,
} from '../store.js'
import PosterCard from './PosterCard.vue'
import SkeletonCard from './SkeletonCard.vue'

const isUserList = computed(() => store.activeCollection?.kind === 'user')
const isRowPage = computed(() => store.activeCollection?.kind === 'row')

// الفرز: صفحات "المزيد" تتيح الأعلى تقييماً أو الأحدث (ترتيب الخدمة).
// عرض القوائم الكبيرة (مفضلة بالآلاف مثلاً) على دفعات بدل إنشاء كل البطاقات معاً.
const CHUNK = 120
const shown = ref(CHUNK)
watch(() => [store.activeCollection?.kind, store.activeCollection?.key], () => { shown.value = CHUNK })

const sortedItems = computed(() => {
  const items = store.collectionItems
  if (isRowPage.value && store.rowSort === 'rating') {
    return [...items].sort((a, b) => (parseFloat(b.rating) || 0) - (parseFloat(a.rating) || 0))
  }
  return items
})
</script>

<template>
  <div class="h-full overflow-y-auto pb-4">
    <div class="mb-5 flex flex-wrap items-center gap-3">
      <button
        @click="closeCollection"
        class="flex items-center gap-2 rounded-full border border-white/10 bg-ink-800 px-4 py-2 text-sm text-zinc-300 transition hover:border-white/30 hover:text-white"
      >
        <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.5">
          <path d="m9 5 7 7-7 7" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        عودة
      </button>
      <h1 class="text-xl font-extrabold text-white">{{ store.activeCollection?.name }}</h1>
      <span v-if="sortedItems.length" class="rounded-full bg-white/10 px-2.5 py-0.5 text-xs text-zinc-400">
        {{ sortedItems.length }}
      </span>

      <!-- فرز صفحات "المزيد" -->
      <div v-if="isRowPage" class="flex rounded-full border border-white/10 bg-ink-800 p-1">
        <button
          @click="setRowSort('rating')"
          class="rounded-full px-4 py-1.5 text-xs font-semibold transition"
          :class="store.rowSort === 'rating' ? 'bg-accent-500 text-white' : 'text-zinc-400 hover:text-white'"
        >
          الأعلى تقييماً
        </button>
        <button
          @click="setRowSort('latest')"
          class="rounded-full px-4 py-1.5 text-xs font-semibold transition"
          :class="store.rowSort === 'latest' ? 'bg-accent-500 text-white' : 'text-zinc-400 hover:text-white'"
        >
          الأحدث
        </button>
      </div>

      <button
        v-if="isUserList"
        @click="deleteUserList(store.activeCollection.key)"
        class="ms-auto rounded-lg border border-red-500/30 px-4 py-2 text-xs font-semibold text-red-400 transition hover:bg-red-500/10"
      >
        حذف القائمة
      </button>
    </div>

    <!-- هيكل التحميل لصفحات "المزيد" -->
    <div
      v-if="store.collectionLoading"
      class="grid gap-3 md:gap-4 grid-cols-[repeat(auto-fill,minmax(var(--card-w),1fr))]"
    >
      <SkeletonCard v-for="i in 12" :key="'csk' + i" />
    </div>

    <template v-else-if="sortedItems.length">
    <div
      class="grid gap-3 md:gap-4 grid-cols-[repeat(auto-fill,minmax(var(--card-w),1fr))]"
    >
      <div v-for="item in sortedItems.slice(0, shown)" :key="item.id" class="relative">
        <PosterCard :item="item" />
        <!-- إزالة من قائمة مستخدم -->
        <button
          v-if="isUserList"
          @click.stop="removeFromUserList(store.activeCollection.key, item.id)"
          title="إزالة من القائمة"
          class="absolute end-2 top-2 z-10 flex h-7 w-7 items-center justify-center rounded-full bg-black/70 text-zinc-300 backdrop-blur transition hover:text-red-400"
        >
          <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.2">
            <path d="M6 6l12 12M18 6 6 18" stroke-linecap="round" />
          </svg>
        </button>
      </div>
    </div>

    <div v-if="sortedItems.length > shown" class="mt-6 text-center">
      <button
        @click="shown += CHUNK"
        class="rounded-full border border-white/10 bg-ink-800 px-8 py-2.5 text-sm font-semibold text-zinc-200 transition hover:border-accent-500/50"
      >
        عرض المزيد ({{ sortedItems.length - shown }})
      </button>
    </div>
    </template>

    <div v-else-if="store.collectionError" class="py-20 text-center text-zinc-400">
      <p class="text-lg">تعذر تحميل القائمة</p>
      <p class="mt-2 text-sm text-zinc-500">{{ store.collectionError }}</p>
      <button
        @click="openRowPage(store.activeCollection.key, store.activeCollection.name)"
        class="mt-4 rounded-lg bg-accent-500 px-5 py-2 text-sm font-bold text-white transition hover:bg-accent-400"
      >
        إعادة المحاولة
      </button>
    </div>

    <div v-else class="py-20 text-center text-zinc-500">
      <p class="text-lg">هذه القائمة فارغة</p>
      <p class="mt-2 text-sm text-zinc-600">
        أضف أعمالاً عبر زر ♥ على البطاقات أو «+ قائمة» في صفحة التفاصيل
      </p>
    </div>
  </div>
</template>
