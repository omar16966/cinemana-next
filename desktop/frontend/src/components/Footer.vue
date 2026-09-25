<script setup>
// =============================================================================
// Footer.vue — شريط أسفل الصفحة:
//   يمين: زر الإعدادات ⚙ — يسار: نص الحقوق بسطرين.
// =============================================================================
import { openSettings } from '../store.js'
</script>

<template>
  <footer class="mt-4 border-t border-white/5 bg-ink-900/40 px-6 py-4">
    <div class="flex items-center justify-between gap-4">
      <!-- الإعدادات (يمين في RTL) -->
      <button
        @click="openSettings"
        class="flex items-center gap-2 rounded-full border border-white/10 bg-ink-800 px-5 py-2.5 text-sm font-semibold text-zinc-300 transition hover:border-accent-500/50 hover:text-white"
      >
        <svg viewBox="0 0 24 24" class="h-4.5 w-4.5" fill="none" stroke="currentColor" stroke-width="1.8">
          <circle cx="12" cy="12" r="3" />
          <path d="M19.4 15a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1 1.55V21a2 2 0 1 1-4 0v-.09a1.7 1.7 0 0 0-1.11-1.55 1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.7 1.7 0 0 0 .34-1.87 1.7 1.7 0 0 0-1.55-1H3a2 2 0 1 1 0-4h.09a1.7 1.7 0 0 0 1.55-1.11 1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.7 1.7 0 0 0 1.87.34h.08a1.7 1.7 0 0 0 1-1.55V3a2 2 0 1 1 4 0v.09a1.7 1.7 0 0 0 1 1.55 1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.7 1.7 0 0 0-.34 1.87v.08a1.7 1.7 0 0 0 1.55 1H21a2 2 0 1 1 0 4h-.09a1.7 1.7 0 0 0-1.55 1Z" />
        </svg>
        الإعدادات
      </button>

      <!-- الحقوق (يسار في RTL) -->
      <div class="text-end text-xs leading-relaxed text-zinc-600">
        <p>@جميع الحقوق محفوظة للمطور</p>
        <p>المطور : <span class="font-semibold text-zinc-500" dir="ltr">Om3r-GLM5.3</span></p>
      </div>
    </div>
  </footer>
</template>
