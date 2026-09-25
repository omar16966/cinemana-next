<script setup>
// =============================================================================
// PosterCard.vue — بطاقة بوستر واحدة (شبيهة ببطاقات نتفليكس):
//   - نستخدم البوستر كامل الدقة (poster_url) أولاً — الصور المصغرة كانت
//     سبب "الضبابية" عند تكبير البطاقات — مع الرجوع للمصغرة ثم لتدرج
//     لوني عند فشل التحميل.
//   - عند التمرير ترتفع البطاقة ويظهر زر المفضلة (♥) وزر التشغيل.
//   - النقر يفتح شاشة التفاصيل.
// =============================================================================
import { ref, computed } from 'vue'
import { openDetails, toggleFavorite, isFavorite } from '../store.js'

const props = defineProps({
  item: { type: Object, required: true }, // MediaSummary من الباك-اند
  compact: { type: Boolean, default: false }, // نسخة مصغرة للصفوف الأفقية
})

const imgFailed = ref(false)
// أعلى دقة متاحة أولاً — أصل مشكلة الوضوح.
const imgSrc = computed(() => props.item.poster_url || props.item.thumbnail_url || '')
const title = computed(() => props.item.en_title || props.item.ar_title || 'بدون عنوان')
const typeLabel = computed(() => (props.item.type === 'series' ? 'مسلسل' : 'فيلم'))
const fav = computed(() => isFavorite(props.item.id))
</script>

<template>
  <div class="group relative" :class="compact ? 'w-full' : ''">
    <button
      @click="openDetails(item)"
      class="relative block w-full overflow-hidden rounded-xl bg-ink-800 text-start ring-1 ring-white/5 transition duration-200 hover:z-10 hover:scale-[1.04] hover:ring-accent-500/70 focus:outline-none focus:ring-2 focus:ring-accent-500"
      :class="compact ? 'shadow-md' : 'shadow-lg shadow-black/40'"
    >
      <div class="relative aspect-[2/3] w-full">
        <!-- البوستر: كامل الدقة أولاً مع تراجع تدريجي -->
        <img
          v-if="imgSrc && !imgFailed"
          :src="imgSrc"
          :alt="title"
          loading="lazy"
          decoding="async"
          class="absolute inset-0 h-full w-full object-cover"
          @error="imgFailed = true"
        />
        <!-- بديل بتدرج لوني عند غياب/فشل الصورة -->
        <div
          v-else
          class="absolute inset-0 flex items-center justify-center bg-gradient-to-br from-ink-700 via-ink-800 to-ink-900"
        >
          <span class="text-4xl font-black text-ink-600">{{ title.charAt(0) }}</span>
        </div>

        <!-- تدرج سفلي + التفاصيل الأساسية -->
        <div class="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/95 via-black/60 to-transparent p-2.5 pt-8">
          <p class="line-clamp-2 text-[13px] font-bold leading-snug text-white">{{ title }}</p>
          <div class="mt-1 flex items-center gap-2 text-[11px] text-zinc-400">
            <span v-if="item.year">{{ item.year }}</span>
            <span v-if="item.rating" class="flex items-center gap-0.5 text-gold-400">
              <svg viewBox="0 0 24 24" class="h-3 w-3" fill="currentColor">
                <path d="m12 2 3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2Z" />
              </svg>
              {{ item.rating }}
            </span>
            <span class="ms-auto rounded bg-white/10 px-1.5 py-0.5 text-[10px] text-zinc-300">{{ typeLabel }}</span>
          </div>
        </div>

        <!-- زر تشغيل رمزي عند التمرير -->
        <div class="pointer-events-none absolute inset-0 hidden items-center justify-center bg-black/25 opacity-0 transition group-hover:opacity-100 md:flex">
          <div class="flex h-12 w-12 items-center justify-center rounded-full border-2 border-white/80 bg-black/40">
            <svg viewBox="0 0 24 24" class="h-6 w-6 text-white rtl:-scale-x-100" fill="currentColor">
              <path d="M8 5.14v13.72c0 .83.92 1.33 1.62.89l10.8-6.86a1.05 1.05 0 0 0 0-1.78L9.62 4.25A1.05 1.05 0 0 0 8 5.14Z" />
            </svg>
          </div>
        </div>
      </div>
    </button>

    <!-- زر المفضلة: ظاهر دائماً إذا كان مفضلاً، وإلا عند التمرير -->
    <button
      @click.stop="toggleFavorite(item)"
      :title="fav ? 'إزالة من المفضلة' : 'إضافة إلى المفضلة'"
      class="absolute start-2 top-2 z-10 flex h-8 w-8 items-center justify-center rounded-full bg-black/60 backdrop-blur transition"
      :class="fav ? 'text-accent-500 opacity-100' : 'text-white opacity-0 group-hover:opacity-100 hover:scale-110'"
    >
      <svg viewBox="0 0 24 24" class="h-4.5 w-4.5" :fill="fav ? 'currentColor' : 'none'" stroke="currentColor" stroke-width="2">
        <path d="M12 21s-7.5-4.9-10-9.3C.4 8.6 2.2 5 5.7 5c2 0 3.4 1.1 4.3 2.6h4C14.9 6.1 16.3 5 18.3 5c3.5 0 5.3 3.6 3.7 6.7C19.5 16.1 12 21 12 21Z" stroke-linejoin="round" />
      </svg>
    </button>
  </div>
</template>
