// =============================================================================
// services/mock.js — بيانات تجريبية لوضع المعاينة في المتصفح
//
// الهدف: تطوير ومراجعة تصميم الواجهة دون تشغيل تطبيق Wails
// (npm run dev ثم فتح http://localhost:5173). نفس أشكال البيانات
// التي تعيدها دوال Go في desktop/app.go حرفياً حتى يبقى الانتقال
// بين الوضعين شفافاً تماماً.
// =============================================================================

// بوسترات بديلة (تدرجات CSS) — PosterCard يعرضها تلقائياً عند فشل الصورة.
const P = '' // فراغ = بطاقة بديلة بتدرج لوني

const RESULTS = [
  { id: '5386', ar_title: 'Fast & Furious', en_title: 'Fast & Furious', type: 'movie', year: '2009', rating: '6.5', poster_url: P, thumbnail_url: P, categories: ['Action', 'Crime'] },
  { id: '5383', ar_title: 'The Fast and the Furious', en_title: 'The Fast and the Furious', type: 'movie', year: '2001', rating: '6.8', poster_url: P, thumbnail_url: P, categories: ['Action'] },
  { id: '5388', ar_title: 'Fast & Furious 6', en_title: 'Fast & Furious 6', type: 'movie', year: '2013', rating: '7', poster_url: P, thumbnail_url: P, categories: ['Action', 'Thriller'] },
  { id: '653', ar_title: 'Game of Thrones', en_title: 'Game of Thrones', type: 'series', year: '2011', rating: '9.2', poster_url: P, thumbnail_url: P, categories: ['Drama', 'Fantasy'] },
  { id: '1242371', ar_title: 'Oppenheimer', en_title: 'Oppenheimer', type: 'movie', year: '2023', rating: '8.3', poster_url: P, thumbnail_url: P, categories: ['Biography', 'Drama'] },
  { id: '99881', ar_title: 'Interstellar', en_title: 'Interstellar', type: 'movie', year: '2014', rating: '8.6', poster_url: P, thumbnail_url: P, categories: ['Sci-Fi'] },
]

const wait = (ms = 250) => new Promise((r) => setTimeout(r, ms))

export async function search(query, mediaType) {
  await wait()
  const q = (query || '').toLowerCase()
  return RESULTS.filter((r) => !q || r.en_title.toLowerCase().includes(q) || r.ar_title.includes(query))
    .filter((r) => mediaType === 'all' || mediaType === undefined || r.type === mediaType)
}

export async function details(nb) {
  await wait()
  const hit = RESULTS.find((r) => r.id === String(nb)) || RESULTS[0]
  return {
    id: hit.id,
    title: hit.en_title,
    ar_title: hit.ar_title,
    en_title: hit.en_title,
    type: hit.type,
    year: hit.year,
    rating: hit.rating,
    poster_url: hit.poster_url,
    thumbnail_url: hit.thumbnail_url,
    description_ar: 'وصف تجريبي: هذه بيانات معاينة فقط تظهر في وضع المتصفح، وفي التطبيق الحقيقي تأتي من نقطة allVideoInfo للخدمة.',
    description_en: 'Mock description used only in browser preview mode.',
    duration_sec: hit.type === 'series' ? '3004' : '6412',
    categories: hit.categories,
    subtitles: [],
    signed_urls_expire_on: '',
  }
}

export async function episodes(nb) {
  await wait()
  const mk = (season, count) => ({
    season: String(season),
    episodes: Array.from({ length: count }, (_, i) => ({
      id: `${nb}${season}${i + 1}`,
      episode_number: String(i + 1),
      title: 'Game of Thrones',
      duration_sec: String(2900 + i * 60),
    })),
  })
  return [mk(1, 10), mk(2, 10)]
}

export async function playback(nb, alts) {
  await wait(400)
  const q = (res) => ({ resolution: res, kind: 'mp4', local_url: 'https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4', remote_url: 'mock://video' })
  // ترجمة تجريبية كـ data URL — لاختبار طبقة عرض الترجمة في وضع المعاينة
  const vtt =
    'WEBVTT\n\n' +
    '1\n00:00:00.500 --> 00:00:02.800\nهذه معاينة للترجمة العربية بخط مخصص\n\n' +
    '2\n00:00:03.000 --> 00:00:30.000\nوما بدأ في 11 سبتمبر {\\an8}\nالحجم والخط واللون قابلان للتغيير من لوحة «الترجمة»\n'
  return {
    id: String(nb),
    title: 'Mock Title',
    qualities: [q('1080p'), q('720p'), q('480p'), q('360p'), q('240p')],
    subtitles: [
      {
        language: 'arabic',
        lang_code: 'ar',
        format: 'vtt',
        local_url: 'data:text/vtt;charset=utf-8,' + encodeURIComponent(vtt),
        remote_url: 'mock://sub',
      },
    ],
    signed_urls_expire_on: '2026-10-01',
  }
}

export async function openInMPV() {
  await wait()
  console.info('[mock] OpenInMPV — في التطبيق الحقيقي سيُفتح المشغل الخارجي')
}

export async function getSettings() {
  return { mpv_path: '', base_url: 'https://cinemana.shabakaty.com', user_agent: '', insecure_tls: false }
}

export async function saveSettings(s) {
  await wait()
  return s
}

// قوائم الاستعراض (الصفوف الأفقية وTop) — نفس الأشكال التي يعيدها GetCollection.
export async function collection(key, limit = 12) {
  await wait(300)
  return RESULTS.slice(0, limit).map((r) => ({ ...r }))
}

// تصفح بفلاتر (وضع المعاينة: نفس نتائج البحث مع فلترة نوع بسيطة).
export async function browse(filters) {
  await wait(350)
  let items = [...RESULTS]
  if (filters?.video_kind === '1') items = items.filter((r) => r.type === 'movie')
  if (filters?.video_kind === '2') items = items.filter((r) => r.type === 'series')
  if (filters?.query) {
    const q = filters.query.toLowerCase()
    items = items.filter((r) => r.en_title.toLowerCase().includes(q) || r.ar_title.includes(filters.query))
  }
  return items
}

// تصنيفات الخدمة.
export async function categories() {
  await wait(150)
  return [
    { nb: '84', title: 'اكشن' }, { nb: '57', title: 'رسوم متحركة' },
    { nb: '62', title: 'دراما' }, { nb: '59', title: 'كوميديا' },
    { nb: '78', title: 'خيال علمي' }, { nb: '56', title: 'مغامرة' },
    { nb: '70', title: 'رعب' }, { nb: '61', title: 'وثائقي' },
  ]
}

// تصدير/استيراد (وضع المعاينة: محاكاة فقط)
export async function exportUserData() {
  await wait()
  return 'C:\mock\cinemana-next-data.json'
}
export async function importUserData() {
  await wait()
  return JSON.stringify({ favorites: RESULTS.slice(0, 2), user_lists: [], recents: [] })
}
