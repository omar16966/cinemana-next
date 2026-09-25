<script setup>
// =============================================================================
// TopBar.vue — الشريط العلوي: الشعار + البحث بالمنتصف مع مرشح النوع.
// (زر الإعدادات انتقل إلى شريط أسفل الصفحة — انظر Footer.vue)
// البحث فوري (debounce في store) مع دعم Enter وزر مسح سريع.
// =============================================================================
import { store, onQueryInput, doSearch, setType, clearSearch } from '../store.js'

const types = [
  { key: 'all', label: 'الكل' },
  { key: 'movie', label: 'أفلام' },
  { key: 'series', label: 'مسلسلات' },
]
</script>

<template>
  <header class="sticky top-0 z-40 border-b border-white/5 bg-ink-950/85 backdrop-blur">
    <div class="flex w-full items-center gap-4 px-4 py-3 md:px-8">
      <!-- الشعار: الاسم الإنجليزي رئيسياً والعربي تحته أصغر -->
      <div class="flex shrink-0 items-center gap-2.5 select-none">
        <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-accent-500 shadow-lg shadow-accent-500/30">
          <svg viewBox="0 0 24 24" class="h-5 w-5 text-white" fill="currentColor">
            <path d="M8 5.14v13.72c0 .83.92 1.33 1.62.89l10.8-6.86a1.05 1.05 0 0 0 0-1.78L9.62 4.25A1.05 1.05 0 0 0 8 5.14Z" />
          </svg>
        </div>
        <div class="flex flex-col leading-none">
          <span class="text-lg font-extrabold tracking-tight text-white" dir="ltr">
            CinemaNa <span class="text-accent-500">Next</span>
          </span>
          <span class="mt-1 text-[11px] font-semibold text-zinc-400">سينمانا نكست</span>
        </div>
      </div>

      <!-- البحث + مرشح النوع: مجموعة واحدة في منتصف الشاشة -->
      <div class="flex flex-1 items-center justify-center gap-3">
        <div class="relative w-full max-w-2xl">
          <svg viewBox="0 0 24 24" class="pointer-events-none absolute start-4 top-1/2 h-5 w-5 -translate-y-1/2 text-zinc-500" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="7" />
            <path d="m20 20-3.5-3.5" stroke-linecap="round" />
          </svg>
          <input
            :value="store.query"
            @input="onQueryInput($event.target.value)"
            @keyup.enter="doSearch(true)"
            type="text"
            placeholder="ابحث عن فيلم أو مسلسل..."
            class="w-full rounded-full border border-white/10 bg-ink-800/90 py-3 ps-12 pe-12 text-sm text-zinc-100 shadow-lg shadow-black/30 placeholder-zinc-500 outline-none transition focus:border-accent-500/60 focus:bg-ink-800 focus:ring-2 focus:ring-accent-500/30"
          />
          <!-- زر مسح سريع يظهر عند وجود نص -->
          <button
            v-if="store.query"
            @click="clearSearch"
            title="مسح البحث"
            class="absolute end-3.5 top-1/2 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-zinc-300 transition hover:bg-white/25 hover:text-white"
          >
            <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.5">
              <path d="M6 6l12 12M18 6 6 18" stroke-linecap="round" />
            </svg>
          </button>
        </div>

        <!-- مرشح النوع: ملاصق لمربع البحث -->
        <div class="flex shrink-0 rounded-full border border-white/10 bg-ink-800 p-1">
          <button
            v-for="t in types"
            :key="t.key"
            @click="setType(t.key)"
            class="rounded-full px-3.5 py-1.5 text-xs font-semibold transition"
            :class="store.type === t.key ? 'bg-accent-500 text-white' : 'text-zinc-400 hover:text-zinc-200'"
          >
            {{ t.label }}
          </button>
        </div>
      </div>

      <!-- موازن عرض الشعار ليبقى البحث في المنتصف تماماً -->
      <div class="hidden w-40 shrink-0 md:block"></div>
    </div>
  </header>
</template>
