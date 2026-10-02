// =============================================================================
// store/utils.js — دوال تنسيق نقية (بلا حالة).
// =============================================================================

// تحويل ثوانٍ إلى مدة مقروءة: "1 س 47 د" أو "55 د".
export function fmtDuration(sec) {
  const n = Math.round(parseFloat(sec) || 0)
  if (n <= 0) return ''
  const total = Math.round(n / 60) // نقرّب الدقائق الكلية أولاً: 3599ث = 60د لا "0س 60د"
  const h = Math.floor(total / 60)
  const m = total % 60
  return h > 0 ? `${h} س ${String(m).padStart(2, '0')} د` : `${m} د`
}
