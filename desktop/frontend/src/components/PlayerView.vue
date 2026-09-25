<script setup>
// =============================================================================
// PlayerView.vue — المشغل المدمج بملء الشاشة.
//
// كيف يعمل؟
//   - مصدر الفيديو هو local_url من الوكيل المحلي (proxy.go) الذي يضيف
//     ترويسات المحاكاة ويمرر Range: البحث والتقديم يعملان طبيعياً.
//   - mp4: عنصر <video> مباشرة. m3u8: hls.js (تحميل ديناميكي).
//
//   الترجمة — لماذا نرسمها بأنفسنا بدل <track>؟
//     1) متصفح Chromium يُعيد قياس خط ::cue نسبةً لارتفاع الفيديو، فيتجاهل
//        البكسلات المحددة — حجم الترجمة لا يستجيب للمنزلق كما يجب.
//     2) نحن نجلب نص VTT أصلاً (عبر الوكيل)؛ نحلله إلى مقاطع ونرسمه
//        كطبقة HTML فوق الفيديو — تحكم كامل بالخط/الحجم/الألوان ببكسل
//        حقيقي، بلا قيود CORS ولا قيود ::cue.
//   - المزامنة: حلقة requestAnimationFrame تقرأ currentTime وتختار
//     المقطع النشط — رخيصة جداً ودقيقة.
//
//   حفظ موضع المشاهدة: كل 5 ثوانٍ في localStorage بمفتاح معرف العمل،
//   ويُستأنف تلقائياً عند العودة (زر "من البداية" يمسحه).
// =============================================================================
import { ref, reactive, computed, onMounted, onBeforeUnmount } from 'vue'
import { store, closePlayer, playMpv } from '../store.js'

const videoEl = ref(null)
const buffering = ref(true)
const videoError = ref('')
const ccOn = ref(true)
const stylePanelOpen = ref(false)
const resumedAt = ref(0)
const cues = ref([])
const activeCue = ref(null)
let hls = null
let rafId = 0
let lastSavedT = -1

// =============================================================================
// إعدادات تنسيق الترجمة — تُطبق على طبقة العرض مباشرة (بكسل حقيقي)
// =============================================================================
const SUB_STYLE_KEY = 'cinemana_substyle_v2'

function loadSubStyle() {
  try {
    return Object.assign(
      { font: 'inherit', size: 30, color: '#ffffff', bg: '#000000', bgOpacity: 45 },
      JSON.parse(localStorage.getItem(SUB_STYLE_KEY) || '{}'),
    )
  } catch {
    return { font: 'inherit', size: 30, color: '#ffffff', bg: '#000000', bgOpacity: 45 }
  }
}

const subStyle = reactive(loadSubStyle())

// حفظ تلقائي عند أي تعديل (بسيط وفعال).
function persistSubStyle() {
  localStorage.setItem(SUB_STYLE_KEY, JSON.stringify({ ...subStyle }))
}

// hexToRgba تحويل لون hex + شفافية إلى rgba (للخلفية).
function hexToRgba(hex, alpha) {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex || '#000000')
  if (!m) return `rgba(0,0,0,${alpha})`
  const n = parseInt(m[1], 16)
  return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},${alpha})`
}

// الخطوط المتاحة: المدمجة محلياً (تعمل بلا إنترنت) + خطوط النظام.
const fonts = [
  { v: 'inherit', l: 'افتراضي' },
  { v: 'Cairo', l: 'Cairo — القاهرة' },
  { v: 'Tajawal', l: 'Tajawal — تجوال' },
  { v: 'Almarai', l: 'Almarai — المراعي' },
  { v: 'Noto Kufi Arabic', l: 'Noto Kufi — كوفي' },
  { v: 'Amiri', l: 'Amiri — أميري (نسخ)' },
  { v: 'Aref Ruqaa', l: 'Aref Ruqaa — رقعة' },
  { v: 'Lalezar', l: 'Lalezar — لالِزار' },
  { v: 'Segoe UI', l: 'Segoe UI' },
  { v: 'Tahoma', l: 'Tahoma' },
  { v: 'Traditional Arabic', l: 'Traditional Arabic' },
  { v: 'Arial', l: 'Arial' },
  { v: 'Courier New', l: 'Courier New' },
]

// نمط طبقة الترجمة (computed يتحدث فوراً مع أي تعديل في اللوحة).
const overlayStyle = computed(() => ({
  color: subStyle.color,
  background: hexToRgba(subStyle.bg, subStyle.bgOpacity / 100),
  fontFamily:
    subStyle.font === 'inherit'
      ? "'Segoe UI', 'Noto Sans Arabic', sans-serif"
      : `'${subStyle.font}', 'Segoe UI', sans-serif`,
  fontSize: subStyle.size + 'px',
}))

// =============================================================================
// تحليل VTT إلى مقاطع مرسومة
// =============================================================================

// toSec تحويل "00:01:02.500" أو "1:02,5" إلى ثوانٍ.
function toSec(s) {
  const p = s.trim().split(':')
  const sec = parseFloat(p[p.length - 1].replace(',', '.')) || 0
  const min = p.length > 1 ? parseInt(p[p.length - 2], 10) || 0 : 0
  const hr = p.length > 2 ? parseInt(p[0], 10) || 0 : 0
  return hr * 3600 + min * 60 + sec
}

// parseVTT محلل مبسط لك blocks VTT التي تنتجها طبقة Go (موحدة التنسيق).
function parseVTT(text) {
  const out = []
  for (const block of text.replace(/\r/g, '').split('\n\n')) {
    const lines = block.split('\n')
    const ti = lines.findIndex((l) => l.includes('-->'))
    if (ti === -1) continue
    const [a, b] = lines[ti].split('-->')
    const start = toSec(a)
    const end = toSec((b || '').trim().split(/\s+/)[0])
    const body = lines
      .slice(ti + 1)
      .join('\n')
      .replace(/\{\\[^}]*\}/g, '') // وسوم ASS إن تسربت
      .trim()
    if (body && end > start) out.push({ start, end, text: body })
  }
  return out
}

// attachSubtitle يجلب نص الترجمة عبر الوكيل ويحلله إلى المقاطع.
async function attachSubtitle() {
  cues.value = []
  activeCue.value = null
  const sub = store.player?.sub
  if (!sub) return
  try {
    const res = await fetch(sub)
    if (res.ok) {
      const text = await res.text()
      if (text.trim().startsWith('WEBVTT')) {
        cues.value = parseVTT(text)
        return
      }
    }
  } catch {
    // بلا ترجمة عند الفشل — لا نص احتياطي لتفادي مسار <track> القديم.
  }
}

// toggleCC إظهار/إخفاء طبقة الترجمة.
function toggleCC() {
  ccOn.value = !ccOn.value
}

// حلقة المزامنة: تختار المقطع النشط من currentTime كل إطار.
function tick() {
  const v = videoEl.value
  if (v && cues.value.length) {
    const t = v.currentTime
    activeCue.value = cues.value.find((c) => t >= c.start && t <= c.end) || null
  }
  rafId = requestAnimationFrame(tick)
}

// =============================================================================
// حفظ/استئناف موضع المشاهدة
// =============================================================================

function posKey() {
  return 'cinemana_pos_' + store.player?.id
}

// onTimeUpdate حفظ الموضع كل 5 ثوانٍ فقط (كتابة متكررة بلا فائدة).
function onTimeUpdate() {
  const v = videoEl.value
  if (!v || !v.duration || isNaN(v.duration)) return
  const t = Math.floor(v.currentTime)
  if (t - lastSavedT >= 5) {
    lastSavedT = t
    try {
      localStorage.setItem(posKey(), JSON.stringify({ t, d: Math.floor(v.duration), ts: Date.now() }))
    } catch { /* التخزين ممتلئ — نتجاهل */ }
  }
}

// resumeIfSaved استئناف تلقائي من آخر موضع محفوظ.
function resumeIfSaved() {
  const v = videoEl.value
  if (!v) return
  try {
    const saved = JSON.parse(localStorage.getItem(posKey()) || 'null')
    if (saved && saved.t > 20 && v.duration && saved.t < v.duration * 0.95) {
      v.currentTime = saved.t
      resumedAt.value = saved.t
      setTimeout(() => (resumedAt.value = 0), 6000)
    }
  } catch { /* لا موضع محفوظ */ }
}

// restart من البداية وحذف الموضع المحفوظ.
function restart() {
  const v = videoEl.value
  if (v) {
    v.currentTime = 0
    v.play().catch(() => {})
  }
  resumedAt.value = 0
  try { localStorage.removeItem(posKey()) } catch {}
}

function fmt(t) {
  const s = Math.max(0, Math.floor(t))
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60
  const mm = String(m).padStart(2, '0'), ss = String(sec).padStart(2, '0')
  return h > 0 ? `${h}:${mm}:${ss}` : `${m}:${ss}`
}

function onVideoError(e) {
  if (e?.target?.tagName === 'TRACK') return
  if (videoEl.value?.src || hls) videoError.value = 'تعذر تشغيل الفيديو — قد تكون الروابط منتهية الصلاحية'
}

function onEnded() {
  try { localStorage.removeItem(posKey()) } catch {}
}

function onKey(e) {
  if (e.key === 'Escape') closePlayer()
}

// =============================================================================
// دورة الحياة
// =============================================================================

async function setup() {
  buffering.value = true
  videoError.value = ''
  resumedAt.value = 0
  lastSavedT = -1
  destroyHls()
  const src = store.player?.src || ''
  const isM3u8 = /\.m3u8(\?|$)/.test(src)
  const nativeHls = videoEl.value?.canPlayType('application/vnd.apple.mpegurl')

  if (isM3u8 && !nativeHls) {
    try {
      const { default: Hls } = await import('hls.js')
      if (Hls.isSupported()) {
        hls = new Hls()
        hls.loadSource(src)
        hls.attachMedia(videoEl.value)
        hls.on(Hls.Events.ERROR, (_, data) => {
          if (data.fatal) videoError.value = 'تعذر تشغيل بث HLS: ' + data.details
        })
        await attachSubtitle()
        return
      }
    } catch {
      videoError.value = 'فشل تحميل مكتبة HLS'
      return
    }
  }
  videoEl.value.src = src
  videoEl.value.play().catch(() => {})
  await attachSubtitle()
}

function destroyHls() {
  if (hls) {
    hls.destroy()
    hls = null
  }
}

onMounted(() => {
  setup()
  rafId = requestAnimationFrame(tick)
  window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  cancelAnimationFrame(rafId)
  destroyHls()
  persistSubStyle()
})

const qualityLabel = computed(() => store.player?.label || '')
</script>

<template>
  <div v-if="store.player" class="fixed inset-0 z-50 flex flex-col bg-black">
    <!-- الشريط العلوي: إغلاق + العنوان + الجودة + الأدوات -->
    <div class="absolute inset-x-0 top-0 z-10 bg-gradient-to-b from-black/80 to-transparent p-4">
      <div class="flex items-center gap-2.5">
        <button
          @click="closePlayer"
          class="flex h-10 w-10 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/20"
          title="إغلاق (Esc)"
        >
          <svg viewBox="0 0 24 24" class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2.5">
            <path d="M6 6l12 12M18 6 6 18" stroke-linecap="round" />
          </svg>
        </button>
        <div class="min-w-0">
          <p class="truncate text-sm font-bold text-white">{{ store.player.title }}</p>
          <p class="text-xs text-zinc-400">الجودة: {{ qualityLabel }}</p>
        </div>

        <div class="ms-auto flex items-center gap-2">
          <button
            @click="stylePanelOpen = !stylePanelOpen"
            title="تنسيق الترجمة"
            class="flex h-10 items-center gap-1.5 rounded-full px-3.5 text-sm font-bold transition"
            :class="stylePanelOpen ? 'bg-accent-500 text-white' : 'bg-white/10 text-white hover:bg-white/20'"
          >
            <svg viewBox="0 0 24 24" class="h-4.5 w-4.5" fill="currentColor">
              <path d="M5 4h5.5v2.5H8v11h2.5V20H5V4Zm8.5 0H19v16h-5.5v-2.5H16v-11h-2.5V4Z" />
            </svg>
            الترجمة
          </button>
          <button
            v-if="store.player.sub"
            @click="toggleCC"
            :title="ccOn ? 'إخفاء الترجمة' : 'إظهار الترجمة'"
            class="flex h-10 w-10 items-center justify-center rounded-full text-sm font-black transition"
            :class="ccOn ? 'bg-accent-500 text-white' : 'bg-white/10 text-zinc-400 hover:text-white'"
          >
            CC
          </button>
          <button
            @click="restart"
            title="مشاهدة من البداية"
            class="flex h-10 w-10 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/20"
          >
            <svg viewBox="0 0 24 24" class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M3 12a9 9 0 1 0 3-6.7" stroke-linecap="round" />
              <path d="M3 4v5h5" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </button>
        </div>
      </div>
    </div>

    <!-- الفيديو -->
    <video
      ref="videoEl"
      class="h-full w-full bg-black"
      controls
      autoplay
      playsinline
      @timeupdate="onTimeUpdate"
      @loadedmetadata="resumeIfSaved"
      @ended="onEnded"
      @waiting="buffering = true"
      @playing="buffering = false"
      @canplay="buffering = false"
      @error="onVideoError"
    ></video>

    <!-- طبقة الترجمة (نرسمها بأنفسنا: حجم حقيقي بلا قيود ::cue) -->
    <div
      v-if="ccOn && activeCue"
      class="pointer-events-none absolute inset-x-0 bottom-[10%] z-10 flex justify-center px-12"
    >
      <div
        class="max-w-[85%] whitespace-pre-line rounded-lg px-4 py-1.5 text-center leading-snug"
        :style="overlayStyle"
      >{{ activeCue.text }}</div>
    </div>

    <!-- شريط الاستئناف -->
    <Transition name="fade">
      <div
        v-if="resumedAt > 0"
        class="absolute bottom-24 start-1/2 z-20 -translate-x-1/2 rtl:translate-x-1/2 rounded-full border border-white/15 bg-black/85 px-5 py-2.5 text-sm text-zinc-100 backdrop-blur"
      >
        أُعيد المشاهدة من {{ fmt(resumedAt) }}
      </div>
    </Transition>

    <!-- لوحة تنسيق الترجمة -->
    <Transition name="fade">
      <div
        v-if="stylePanelOpen"
        class="absolute end-4 top-20 z-20 w-80 rounded-2xl border border-white/10 bg-ink-900/95 p-4 shadow-2xl backdrop-blur"
      >
        <div class="mb-3 flex items-center justify-between">
          <h3 class="text-sm font-bold text-white">تنسيق الترجمة</h3>
          <button @click="stylePanelOpen = false" class="text-zinc-500 transition hover:text-white">
            <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.5">
              <path d="M6 6l12 12M18 6 6 18" stroke-linecap="round" />
            </svg>
          </button>
        </div>

        <!-- معاينة حية بنفس الطبقة -->
        <div class="mb-3 flex h-16 items-center justify-center rounded-lg bg-gradient-to-br from-zinc-700 to-zinc-900">
          <span
            class="rounded px-2 py-0.5 text-center leading-tight"
            :style="{
              color: subStyle.color,
              background: hexToRgba(subStyle.bg, subStyle.bgOpacity / 100),
              fontFamily: subStyle.font === 'inherit' ? 'inherit' : `'${subStyle.font}'`,
              fontSize: Math.min(subStyle.size, 22) + 'px',
            }"
          >
            معاينة الترجمة
          </span>
        </div>

        <div class="space-y-3 text-sm">
          <label class="block">
            <span class="mb-1 block text-xs text-zinc-400">الخط</span>
            <select
              v-model="subStyle.font"
              @change="persistSubStyle"
              class="w-full rounded-lg border border-white/10 bg-ink-800 px-2.5 py-2 outline-none focus:border-accent-500/60"
            >
              <option v-for="ft in fonts" :key="ft.v" :value="ft.v">{{ ft.l }}</option>
            </select>
          </label>

          <label class="block">
            <span class="mb-1 flex items-center justify-between text-xs text-zinc-400">
              الحجم <span class="text-zinc-500">{{ subStyle.size }}px</span>
            </span>
            <input
              v-model.number="subStyle.size"
              @change="persistSubStyle"
              type="range"
              min="16"
              max="72"
              class="w-full accent-red-600"
            />
          </label>

          <div class="flex gap-3">
            <label class="flex-1">
              <span class="mb-1 block text-xs text-zinc-400">لون النص</span>
              <input v-model="subStyle.color" @change="persistSubStyle" type="color" class="h-9 w-full cursor-pointer rounded-lg border border-white/10 bg-ink-800" />
            </label>
            <label class="flex-1">
              <span class="mb-1 block text-xs text-zinc-400">لون الخلفية</span>
              <input v-model="subStyle.bg" @change="persistSubStyle" type="color" class="h-9 w-full cursor-pointer rounded-lg border border-white/10 bg-ink-800" />
            </label>
          </div>

          <label class="block">
            <span class="mb-1 flex items-center justify-between text-xs text-zinc-400">
              شفافية الخلفية <span class="text-zinc-500">{{ subStyle.bgOpacity }}%</span>
            </span>
            <input
              v-model.number="subStyle.bgOpacity"
              @change="persistSubStyle"
              type="range"
              min="0"
              max="95"
              class="w-full accent-red-600"
            />
          </label>
        </div>
      </div>
    </Transition>

    <!-- مؤشر التحميل -->
    <div v-if="buffering && !videoError" class="pointer-events-none absolute inset-0 flex items-center justify-center">
      <div class="h-14 w-14 animate-spin rounded-full border-4 border-white/15 border-t-accent-500"></div>
    </div>

    <!-- لوحة الخطأ البديلة -->
    <div v-if="videoError" class="absolute inset-0 flex flex-col items-center justify-center gap-4 bg-black/90 p-6 text-center">
      <p class="text-lg font-bold text-white">{{ videoError }}</p>
      <p class="max-w-md text-sm text-zinc-400">
        جرّب إعادة المحاولة، أو افتح الفيديو في مشغل mpv الخارجي الذي يستخدم الرابط الموقّع مباشرة.
      </p>
      <div class="flex gap-3">
        <button @click="setup" class="rounded-lg bg-accent-500 px-6 py-2.5 text-sm font-bold text-white transition hover:bg-accent-400">
          إعادة المحاولة
        </button>
        <button @click="playMpv" class="rounded-lg border border-white/20 px-6 py-2.5 text-sm font-semibold text-zinc-200 transition hover:border-white/40">
          فتح في MPV
        </button>
        <button @click="closePlayer" class="rounded-lg border border-white/10 px-6 py-2.5 text-sm text-zinc-400 transition hover:text-white">
          عودة
        </button>
      </div>
    </div>
  </div>
</template>
