// =============================================================================
// store/details.js — شاشة التفاصيل (عمل/مسلسل/حلقات) والتشغيل (جودات، ترجمات،
// مشغل مدمج أو mpv الخارجي). كل جلب محمي برقم تسلسلي فتُهمل الاستجابات القديمة.
// =============================================================================

import * as api from '../services/api.js'
import { store, seq, fail } from './state.js'
import { pushRecent } from './library.js'
import { closeCollection } from './collections.js'

// goHome العودة للرئيسية من أي شاشة (تنظف حالة القائمة والتفاصيل).
export function goHome() {
  closeCollection()
  seq.details++
  seq.playback++
  store.currentItem = null
  store.details = null
  store.playback = null
  store.detailsLoading = false
  store.playbackLoading = false
  store.view = 'home'
}

// =============================================================================
// شاشة التفاصيل
// =============================================================================

export async function openDetails(item) {
  const my = ++seq.details
  seq.playback++
  seq.episodes++
  store.detailsFrom = store.activeCollection && store.view !== 'home' ? 'collection' : 'home'
  store.currentItem = item
  store.view = 'details'
  store.details = null
  store.playback = null
  store.seasons = []
  store.activeEpisode = null
  store.activeSeason = null
  pushRecent(item)

  store.detailsLoading = true
  store.seasonsLoading = false
  store.playbackLoading = false
  try {
    const details = await api.details(item.id)
    if (my !== seq.details) return
    store.details = details
    if (details.type === 'series') {
      store.seasonsLoading = true
      const seasons = await api.episodes(item.id)
      if (my !== seq.details) return
      store.seasons = seasons
      // نفتح الموسم الأول تلقائياً ليظهر المحتوى فوراً.
      store.activeSeason = seasons[0]?.season ?? null
    } else {
      // الأفلام: نجلب معلومات التشغيل فوراً مع تمرير معرفات النسخ
      // المكررة — إن كانت النسخة الأساسية ميتة جُرب البديل تلقائياً.
      await preparePlayback(item.id, item.alts || [])
    }
  } catch (e) {
    if (my === seq.details) fail(e)
  } finally {
    if (my === seq.details) {
      store.detailsLoading = false
      store.seasonsLoading = false
    }
  }
}

export function closeDetails() {
  seq.details++
  seq.playback++
  // العودة: للقائمة التي فُتحت منها التفاصيل، وإلا للرئيسية.
  if (store.detailsFrom === 'collection' && store.activeCollection) {
    store.view = 'collection'
  } else {
    store.view = 'home'
  }
  store.currentItem = null
  store.details = null
  store.playback = null
  store.detailsLoading = false
  store.playbackLoading = false
}

export function selectSeason(num) {
  store.activeSeason = num
}

// اختيار حلقة: نجلب معلومات التشغيل الخاصة بها (لكل حلقة معرّفها الخاص).
// شريط التشغيل ثابت أعلى صفحة التفاصيل فيظهر فور الاختيار دون تمرير.
export async function selectEpisode(ep) {
  store.activeEpisode = { id: ep.id, label: `موسم ${store.activeSeason} — حلقة ${ep.episode_number}` }
  await preparePlayback(ep.id)
}

// =============================================================================
// التشغيل
// =============================================================================

// preparePlayback يجلب الجودات والترجمات (فيلم أو حلقة حسب المعرف).
// alts: معرفات النسخ المكررة لنفس العمل — يجرّبها الباك-اند تلقائياً
// إذا كانت نسخة المعرف الأساسي ميتة (ملفاتها محذوفة من الخدمة).
export async function preparePlayback(nb, alts = []) {
  const my = ++seq.playback
  store.playback = null
  store.playbackLoading = true
  store.selectedQuality = 0
  try {
    const pb = await api.playback(nb, alts)
    if (my !== seq.playback) return // حلقة/عمل آخر اختير أثناء الجلب
    const subs = Array.isArray(pb.subtitles) ? pb.subtitles : []
    store.playback = { ...pb, subtitles: subs, qualities: pb.qualities || [] }
    // افتراضياً: أعلى جودة (القائمة مرتبة تنازلياً من الباك-اند)
    // وأول ترجمة عربية إن وُجدت.
    const arIdx = subs.findIndex((s) => s.lang_code === 'ar')
    store.selectedSubtitle = subs.length ? (arIdx >= 0 ? arIdx : 0) : -1
  } catch (e) {
    if (my === seq.playback) fail(e)
  } finally {
    if (my === seq.playback) store.playbackLoading = false
  }
}

// playbackTitle عنوان كامل للاستخدام في المشغل (فيلم أو حلقة).
export function playbackTitle() {
  if (!store.details) return ''
  if (store.activeEpisode) return `${store.details.title} — ${store.activeEpisode.label}`
  return store.details.title
}

// play يبدأ التشغيل المدمج: مصدر الفيديو عبر وكيل البث المحلي
// (local_url) + ملف الترجمة المحول VTT (local_url أيضاً).
export function play() {
  const p = store.playback
  if (!p || !p.qualities.length) return
  const q = p.qualities[store.selectedQuality]
  const sub = store.selectedSubtitle >= 0 ? p.subtitles[store.selectedSubtitle] : null
  store.player = {
    id: p.id, // معرف الفيلم/الحلقة — مفتاح حفظ موضع المشاهدة
    src: q.local_url,
    sub: sub?.local_url || '',
    subLabel: sub?.language || '',
    subLang: sub?.lang_code || 'ar',
    title: playbackTitle(),
    label: q.resolution,
  }
  store.view = 'player'
}

// playMpv يمرر الرابط الأصلي (remote_url) لمشغل mpv الخارجي — الترويسات
// يضيفها الباك-اند كوسيطات mpv.
export async function playMpv() {
  const p = store.playback
  if (!p || !p.qualities.length) return
  const q = p.qualities[store.selectedQuality]
  const sub = store.selectedSubtitle >= 0 ? p.subtitles[store.selectedSubtitle] : null
  try {
    await api.openInMPV(q.remote_url, sub?.remote_url || '', playbackTitle())
    store.notice = 'تم إرسال الفيديو إلى مشغل mpv'
  } catch (e) {
    fail(e)
  }
}

export function closePlayer() {
  // إزالة الكائن تلغي عنصر <video> (v-if) فيتوقف البث فوراً.
  store.player = null
  store.view = 'details'
}
