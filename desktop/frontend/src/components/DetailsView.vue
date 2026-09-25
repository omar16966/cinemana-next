<script setup>
// =============================================================================
// DetailsView.vue — صفحة تفاصيل العمل (تخطيط ثابت الأعلى):
//
//   ┌────────────────────────────────────────────┐
//   │ زر العودة                                   │ ← ثابت
//   │ الترويسة (بوستر/عنوان/وصف) — مدمجة للمسلسلات │ ← ثابت
//   │ شريط التشغيل (جودات/ترجمة/أزرار)             │ ← ثابت
//   ├────────────────────────────────────────────┤
//   │ قائمة الحلقات — المنطقة الوحيدة المتحركة     │ ← تمرير خاص
//   └────────────────────────────────────────────┘
//
// بهذا يبقى زر التشغيل واختيار الدقة ظاهرين دائماً مهما كان عدد الحلقات
// (كان القسم مدفوناً أسفل قائمة الحلقات الطويلة).
// =============================================================================
import { computed, ref } from 'vue'
import {
  store,
  closeDetails,
  selectSeason,
  selectEpisode,
  preparePlayback,
  play,
  playMpv,
  fmtDuration,
  isFavorite,
  toggleFavorite,
  addToUserList,
  createUserList,
} from '../store.js'

const details = computed(() => store.details)
const isSeries = computed(() => details.value?.type === 'series')

// نص رأس شريط التشغيل: اسم الحلقة إن كنا في مسلسل.
const playbackHeading = computed(() =>
  store.activeEpisode ? store.activeEpisode.label : 'جودات التشغيل المتاحة',
)

// ===== المفضلة والقوائم =====
const listMenuOpen = ref(false)
const newListMode = ref(false)
const newListName = ref('')

// العنصر بصيغة البطاقة (للمفضلة/القوائم) مشتق من التفاصيل.
const cardItem = computed(() => details.value && ({
  id: details.value.id,
  ar_title: details.value.ar_title,
  en_title: details.value.en_title,
  type: details.value.type,
  year: details.value.year,
  rating: details.value.rating,
  poster_url: details.value.poster_url,
  thumbnail_url: details.value.thumbnail_url,
}))

function addToList(listId) {
  if (cardItem.value) addToUserList(listId, cardItem.value)
  listMenuOpen.value = false
}

function createAndAdd() {
  const list = createUserList(newListName.value)
  if (list && cardItem.value) addToUserList(list.id, cardItem.value)
  newListName.value = ''
  newListMode.value = false
  listMenuOpen.value = false
}
</script>

<template>
  <!-- الجذر: عمود بارتفاع كامل — الحلقات وحدها تتمرر في منطقتها -->
  <div class="flex h-full flex-col overflow-hidden">
    <!-- زر العودة -->
    <div class="shrink-0">
      <button
        @click="closeDetails"
        class="flex items-center gap-2 rounded-full border border-white/10 bg-ink-800 px-4 py-2 text-sm text-zinc-300 transition hover:border-white/30 hover:text-white"
      >
        <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.5">
          <path d="m9 5 7 7-7 7" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        عودة للنتائج
      </button>
    </div>

    <!-- هيكل التحميل -->
    <div v-if="store.detailsLoading || !details" class="flex-1 space-y-4 overflow-y-auto p-1 pt-4">
      <div class="skeleton h-56 w-full rounded-2xl bg-ink-800"></div>
      <div class="skeleton h-6 w-1/3 rounded bg-ink-800"></div>
      <div class="skeleton h-4 w-2/3 rounded bg-ink-800"></div>
    </div>

    <template v-else>
      <!-- ============================================================
           المنطقة الثابتة: الترويسة + شريط التشغيل — لا تتأثر بالتمرير
           (للأفلام تمتد وتتمرر داخلياً إن ضاقت الشاشة لعدم وجود حلقات)
      ============================================================ -->
      <div
        class="shrink-0 px-1 pt-3"
        :class="isSeries ? '' : 'min-h-0 flex-1 overflow-y-auto pb-3'"
      >
        <!-- الترويسة: مدمجة للمسلسلات (توفير مساحة للحلقات) وكاملة للأفلام -->
        <div class="relative overflow-hidden rounded-2xl">
          <img
            v-if="details.poster_url"
            :src="details.poster_url"
            alt=""
            class="absolute inset-0 h-full w-full scale-110 object-cover opacity-25 blur-2xl"
          />
          <div class="absolute inset-0 bg-gradient-to-t from-ink-950 via-ink-950/70 to-ink-950/30"></div>

          <div class="relative flex flex-col gap-4 p-4 sm:flex-row sm:p-5">
            <img
              v-if="details.poster_url"
              :src="details.poster_url"
              :alt="details.title"
              class="shrink-0 self-start rounded-xl shadow-2xl shadow-black/60 ring-1 ring-white/10"
              :class="isSeries ? 'w-24 sm:w-28' : 'w-36 sm:w-48'"
            />
            <div
              v-else
              class="flex shrink-0 items-center justify-center self-start rounded-xl bg-ink-700 text-5xl font-black text-ink-600"
              :class="isSeries ? 'h-40 w-24 sm:w-28' : 'h-56 w-36 sm:w-48'"
            >
              {{ details.title.charAt(0) }}
            </div>

            <div class="min-w-0">
              <h1 class="font-extrabold text-white" :class="isSeries ? 'text-xl md:text-2xl' : 'text-2xl md:text-3xl'">
                {{ details.title }}
              </h1>
              <p v-if="details.ar_title !== details.title || details.en_title !== details.title" class="mt-1 text-sm text-zinc-400">
                {{ details.ar_title }} <span v-if="details.ar_title && details.en_title">·</span> {{ details.en_title }}
              </p>

              <div class="mt-2.5 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-sm text-zinc-300">
                <span v-if="details.year" class="rounded bg-white/10 px-2 py-0.5">{{ details.year }}</span>
                <span v-if="details.type === 'movie' && details.duration_sec" class="text-zinc-400">{{ fmtDuration(details.duration_sec) }}</span>
                <span v-if="details.rating" class="flex items-center gap-1 text-gold-400">
                  <svg viewBox="0 0 24 24" class="h-4 w-4" fill="currentColor">
                    <path d="m12 2 3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2Z" />
                  </svg>
                  {{ details.rating }}
                </span>
                <span
                  class="rounded px-2 py-0.5 text-xs font-semibold"
                  :class="isSeries ? 'bg-sky-500/15 text-sky-300' : 'bg-accent-500/15 text-accent-400'"
                >
                  {{ isSeries ? 'مسلسل' : 'فيلم' }}
                </span>
              </div>

              <div v-if="details.categories?.length" class="mt-2.5 flex flex-wrap gap-1.5">
                <span v-for="c in details.categories" :key="c" class="rounded-full border border-white/10 px-2.5 py-0.5 text-xs text-zinc-400">
                  {{ c }}
                </span>
              </div>

              <div class="mt-3 flex items-center gap-2">
                <button
                  @click="toggleFavorite(cardItem)"
                  class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-semibold transition"
                  :class="isFavorite(details.id) ? 'border-accent-500/60 bg-accent-500/10 text-accent-400' : 'border-white/15 text-zinc-300 hover:border-white/40 hover:text-white'"
                >
                  <svg viewBox="0 0 24 24" class="h-4 w-4" :fill="isFavorite(details.id) ? 'currentColor' : 'none'" stroke="currentColor" stroke-width="2">
                    <path d="M12 21s-7.5-4.9-10-9.3C.4 8.6 2.2 5 5.7 5c2 0 3.4 1.1 4.3 2.6h4C14.9 6.1 16.3 5 18.3 5c3.5 0 5.3 3.6 3.7 6.7C19.5 16.1 12 21 12 21Z" stroke-linejoin="round" />
                  </svg>
                  {{ isFavorite(details.id) ? 'في المفضلة' : 'المفضلة' }}
                </button>

                <div class="relative">
                  <button
                    @click="listMenuOpen = !listMenuOpen"
                    class="flex items-center gap-2 rounded-lg border border-white/15 px-4 py-2 text-sm font-semibold text-zinc-300 transition hover:border-white/40 hover:text-white"
                  >
                    <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.2">
                      <path d="M12 5v14M5 12h14" stroke-linecap="round" />
                    </svg>
                    قائمة
                  </button>
                  <div
                    v-if="listMenuOpen"
                    class="absolute start-0 top-11 z-30 w-56 rounded-xl border border-white/10 bg-ink-800 p-1.5 shadow-2xl"
                  >
                    <p class="px-2.5 py-1 text-xs font-bold text-zinc-500">إضافة إلى...</p>
                    <button
                      v-for="l in store.userLists"
                      :key="l.id"
                      @click="addToList(l.id)"
                      class="flex w-full items-center rounded-lg px-2.5 py-2 text-sm text-zinc-200 transition hover:bg-ink-700"
                    >
                      {{ l.name }}
                    </button>
                    <p v-if="!store.userLists.length" class="px-2.5 py-1.5 text-xs text-zinc-500">لا توجد قوائم بعد</p>
                    <div class="my-1 border-t border-white/10"></div>
                    <button
                      v-if="!newListMode"
                      @click="newListMode = true"
                      class="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-sm font-semibold text-accent-400 transition hover:bg-ink-700"
                    >
                      <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.2">
                        <path d="M12 5v14M5 12h14" stroke-linecap="round" />
                      </svg>
                      قائمة جديدة
                    </button>
                    <div v-else class="p-1.5">
                      <input
                        v-model="newListName"
                        @keyup.enter="createAndAdd"
                        type="text"
                        placeholder="اسم القائمة..."
                        class="w-full rounded-md border border-white/10 bg-ink-900 px-2.5 py-1.5 text-sm outline-none focus:border-accent-500/60"
                      />
                      <div class="mt-2 flex gap-2">
                        <button @click="createAndAdd" class="flex-1 rounded-md bg-accent-500 py-1.5 text-xs font-bold text-white hover:bg-accent-400">
                          إنشاء وإضافة
                        </button>
                        <button @click="newListMode = false" class="flex-1 rounded-md border border-white/10 py-1.5 text-xs text-zinc-400 hover:text-white">
                          إلغاء
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- الوصف: مقتطع بسطرين للمسلسلات لتوفير المساحة -->
              <p class="mt-3 max-w-3xl text-[15px] leading-relaxed text-zinc-300" :class="isSeries ? 'line-clamp-2' : ''">
                {{ details.description_ar || details.description_en || 'لا يوجد وصف متاح.' }}
              </p>
              <p
                v-if="!isSeries && details.description_en && details.description_ar && details.description_ar !== details.description_en"
                class="mt-2 max-w-3xl text-sm leading-relaxed text-zinc-500"
              >
                {{ details.description_en }}
              </p>
            </div>
          </div>
        </div>

        <!-- ============ شريط التشغيل (ضمن المنطقة الثابتة) ============ -->
        <div
          v-if="!isSeries || store.activeEpisode"
          class="mt-3 rounded-xl border border-white/10 bg-ink-900/95 p-3 shadow-xl shadow-black/40"
        >
          <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
            <span class="shrink-0 text-sm font-bold text-white">{{ playbackHeading }}</span>

            <div v-if="store.playbackLoading" class="flex flex-1 flex-wrap items-center gap-1.5">
              <div v-for="i in 5" :key="i" class="skeleton h-8 w-16 rounded-lg bg-ink-800"></div>
              <div class="skeleton h-8 w-24 rounded-lg bg-ink-800"></div>
            </div>

            <template v-else-if="store.playback">
              <div class="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
                <button
                  v-for="(q, i) in store.playback.qualities"
                  :key="q.resolution + i"
                  @click="store.selectedQuality = i"
                  class="rounded-lg px-3 py-1.5 text-xs font-bold transition"
                  :class="store.selectedQuality === i ? 'bg-accent-500 text-white' : 'border border-white/10 bg-ink-800 text-zinc-300 hover:border-white/25'"
                >
                  {{ q.resolution }}
                  <span v-if="q.kind === 'hls' || q.kind === 'hls-variant'" class="ms-1 text-[10px] font-normal opacity-70">HLS</span>
                </button>
              </div>

              <select
                v-model.number="store.selectedSubtitle"
                class="shrink-0 rounded-lg border border-white/10 bg-ink-800 px-2.5 py-2 text-xs text-zinc-200 outline-none focus:border-accent-500/60"
              >
                <option :value="-1">بدون ترجمة</option>
                <option v-for="(s, i) in store.playback.subtitles" :key="i" :value="i">
                  {{ s.language || s.lang_code }} ({{ s.format }})
                </option>
              </select>

              <div class="flex shrink-0 items-center gap-2">
                <button
                  @click="play"
                  class="flex items-center gap-1.5 rounded-lg bg-accent-500 px-5 py-2 text-sm font-extrabold text-white shadow-lg shadow-accent-500/30 transition hover:bg-accent-400 active:bg-accent-600"
                >
                  <svg viewBox="0 0 24 24" class="h-4 w-4 rtl:-scale-x-100" fill="currentColor">
                    <path d="M8 5.14v13.72c0 .83.92 1.33 1.62.89l10.8-6.86a1.05 1.05 0 0 0 0-1.78L9.62 4.25A1.05 1.05 0 0 0 8 5.14Z" />
                  </svg>
                  تشغيل الآن
                </button>
                <button
                  @click="playMpv"
                  title="فتح في مشغل MPV الخارجي"
                  class="flex items-center gap-1.5 rounded-lg border border-white/15 px-3.5 py-2 text-xs font-semibold text-zinc-300 transition hover:border-white/40 hover:text-white"
                >
                  <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" stroke-linecap="round" />
                    <path d="M15 3h6v6M10 14 21 3" stroke-linecap="round" stroke-linejoin="round" />
                  </svg>
                  MPV
                </button>
              </div>

              <span
                v-if="store.playback.signed_urls_expire_on"
                class="hidden w-full text-[10px] text-zinc-600 xl:block"
              >
                الروابط موقعة وتنتهي صلاحيتها {{ store.playback.signed_urls_expire_on }} — تُجلب من جديد عند كل تشغيل
              </span>
            </template>

            <div v-else class="flex flex-1 items-center justify-between gap-3">
              <span class="text-sm text-zinc-400">تعذر جلب معلومات التشغيل — تأكد من اتصالك بشبكة مزود الخدمة.</span>
              <button
                @click="preparePlayback(details.id, store.currentItem?.alts || [])"
                class="shrink-0 rounded-lg bg-accent-500 px-5 py-2 text-sm font-bold text-white transition hover:bg-accent-400"
              >
                إعادة المحاولة
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- ============================================================
           منطقة الحلقات: الوحيدة المتحركة — بتمرير داخلي خاص
      ============================================================ -->
      <div v-if="isSeries" class="flex min-h-0 flex-1 flex-col px-1 pb-2 pt-4">
        <h2 class="mb-2 shrink-0 px-3 text-lg font-bold text-white">الحلقات</h2>

        <div v-if="store.seasonsLoading" class="skeleton mx-3 h-24 rounded-xl bg-ink-800"></div>
        <template v-else>
          <!-- تبويبات المواسم: ثابتة أعلى منطقة الحلقات -->
          <div class="flex shrink-0 gap-2 overflow-x-auto px-3 pb-2">
            <button
              v-for="s in store.seasons"
              :key="s.season"
              @click="selectSeason(s.season)"
              class="shrink-0 rounded-full px-4 py-1.5 text-sm font-semibold transition"
              :class="store.activeSeason === s.season ? 'bg-accent-500 text-white' : 'border border-white/10 bg-ink-800 text-zinc-400 hover:text-white'"
            >
              الموسم {{ s.season }}
            </button>
          </div>

          <!-- قائمة الحلقات: التمرير هنا فقط -->
          <div class="min-h-0 flex-1 overflow-y-auto px-3 pb-2">
            <div
              v-for="s in store.seasons.filter((x) => x.season === store.activeSeason)"
              :key="s.season"
              class="overflow-hidden rounded-xl border border-white/5"
            >
              <button
                v-for="ep in s.episodes"
                :key="ep.id"
                @click="selectEpisode(ep)"
                class="flex w-full items-center gap-4 border-b border-white/5 bg-ink-900 px-4 py-3 text-start transition last:border-0 hover:bg-ink-800"
                :class="store.activeEpisode?.id === ep.id ? 'bg-ink-800' : ''"
              >
                <span
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-sm font-bold"
                  :class="store.activeEpisode?.id === ep.id ? 'bg-accent-500 text-white' : 'bg-ink-700 text-zinc-400'"
                >
                  {{ ep.episode_number }}
                </span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-sm font-semibold text-zinc-100">{{ ep.title }}</span>
                  <span v-if="ep.duration_sec" class="block text-xs text-zinc-500">{{ fmtDuration(ep.duration_sec) }}</span>
                </span>
                <svg
                  v-if="store.activeEpisode?.id === ep.id"
                  viewBox="0 0 24 24" class="h-4 w-4 shrink-0 text-accent-500" fill="currentColor">
                  <path d="M9 16.17 4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17Z" />
                </svg>
              </button>
            </div>
          </div>
        </template>
      </div>
    </template>
  </div>
</template>
