<script setup>
// =============================================================================
// TopSection.vue — قسم "الأعلى تقييماً" (Top) في أول الصفحة الرئيسية:
//   تبويبات (أفلام / مسلسلات / أنمي) وشبكة بطاقات مرقمة 1..12 بأسلوب
//   "Top 10" — الرقم كبير خلف البوستر.
// =============================================================================
import { onMounted } from 'vue'
import { store, ensureTop, setTopTab, TOP_TABS } from '../store.js'
import PosterCard from './PosterCard.vue'

onMounted(() => ensureTop(store.topTab))
</script>

<template>
  <section class="mb-8">
    <div class="mb-3 flex items-center gap-3">
      <h2 class="text-lg font-extrabold text-white">
        الأعلى تقييماً
        <span class="text-accent-500">Top</span>
      </h2>
      <!-- تبويبات النوع -->
      <div class="flex rounded-full border border-white/10 bg-ink-800 p-1">
        <button
          v-for="t in TOP_TABS"
          :key="t.key"
          @click="setTopTab(t.key)"
          class="rounded-full px-4 py-1.5 text-xs font-semibold transition"
          :class="store.topTab === t.key ? 'bg-accent-500 text-white' : 'text-zinc-400 hover:text-white'"
        >
          {{ t.label }}
        </button>
      </div>
    </div>

    <!-- شبكة مرقمة -->
    <div class="grid gap-3 grid-cols-[repeat(auto-fill,minmax(var(--card-w),1fr))]">
      <template v-if="store.top[store.topTab]?.loading || !store.top[store.topTab]">
        <div v-for="i in 6" :key="i" class="skeleton aspect-[2/3] rounded-xl bg-ink-800"></div>
      </template>
      <template v-else>
        <div
          v-for="(item, i) in store.top[store.topTab].items.slice(0, 12)"
          :key="item.id"
          class="relative"
        >
          <!-- شارة الترتيب: أعلى البطاقة بلا تغطية للعنوان -->
          <span
            class="pointer-events-none absolute start-2 top-2 z-20 flex h-8 w-8 select-none items-center justify-center rounded-lg bg-black/75 text-lg font-black text-white ring-1 ring-white/20 backdrop-blur"
          >
            {{ i + 1 }}
          </span>
          <PosterCard :item="item" compact />
        </div>
      </template>
    </div>
  </section>
</template>
