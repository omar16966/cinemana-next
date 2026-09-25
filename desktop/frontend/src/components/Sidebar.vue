<script setup>
// =============================================================================
// Sidebar.vue — الشريط الجانبي (يُعرض على يسار النافذة):
//   - الرئيسية
//   - المفضلة (♥) — ما علّمه المستخدم من بطاقات/تفاصيل
//   - المشاهدات الأخيرة
//   - قوائمي: القوائم الخاصة التي ينشئها المستخدم + إنشاء قائمة جديدة
// كل القوائم تُحفظ محلياً (services/lists.js) وتفتح في شاشة CollectionView.
// =============================================================================
import { ref } from 'vue'
import {
  store,
  openCollection,
  createUserList,
  deleteUserList,
} from '../store.js'

const newName = ref('')
const creating = ref(false)

function create() {
  const list = createUserList(newName.value)
  if (list) {
    newName.value = ''
    creating.value = false
    openCollection('user', list.id, list.name)
  }
}
</script>

<template>
  <aside
    class="flex h-full w-60 shrink-0 flex-col gap-1 overflow-y-auto border-e border-white/5 bg-ink-900/60 p-3"
  >
    <!-- الرئيسية -->
    <button
      @click="store.view = 'home'"
      class="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-semibold transition"
      :class="store.view === 'home' ? 'bg-ink-700 text-white' : 'text-zinc-400 hover:bg-ink-800 hover:text-white'"
    >
      <svg viewBox="0 0 24 24" class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="1.8">
        <path d="m3 10 9-7 9 7v10a1 1 0 0 1-1 1h-5v-6h-6v6H4a1 1 0 0 1-1-1V10Z" stroke-linejoin="round" />
      </svg>
      الرئيسية
    </button>

    <!-- المفضلة -->
    <button
      @click="openCollection('favorites', null, 'المفضلة')"
      class="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition"
      :class="store.activeCollection?.kind === 'favorites' ? 'bg-ink-700 text-white' : 'text-zinc-300 hover:bg-ink-800 hover:text-white'"
    >
      <svg viewBox="0 0 24 24" class="h-5 w-5" :fill="store.favorites.length ? 'currentColor' : 'none'" stroke="currentColor" stroke-width="1.8">
        <path d="M12 21s-7.5-4.9-10-9.3C.4 8.6 2.2 5 5.7 5c2 0 3.4 1.1 4.3 2.6h4C14.9 6.1 16.3 5 18.3 5c3.5 0 5.3 3.6 3.7 6.7C19.5 16.1 12 21 12 21Z" stroke-linejoin="round" />
      </svg>
      المفضلة
      <span v-if="store.favorites.length" class="ms-auto rounded-full bg-accent-500/15 px-2 py-0.5 text-xs text-accent-400">
        {{ store.favorites.length }}
      </span>
    </button>

    <!-- المشاهدات الأخيرة -->
    <button
      @click="openCollection('recents', null, 'المشاهدات الأخيرة')"
      class="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition"
      :class="store.activeCollection?.kind === 'recents' ? 'bg-ink-700 text-white' : 'text-zinc-300 hover:bg-ink-800 hover:text-white'"
    >
      <svg viewBox="0 0 24 24" class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="1.8">
        <circle cx="12" cy="12" r="9" />
        <path d="M12 7v5l3.5 2" stroke-linecap="round" />
      </svg>
      المشاهدات الأخيرة
      <span v-if="store.recents.length" class="ms-auto rounded-full bg-white/10 px-2 py-0.5 text-xs text-zinc-400">
        {{ store.recents.length }}
      </span>
    </button>

    <!-- قوائمي -->
    <div class="mt-3">
      <div class="flex items-center justify-between px-3 pb-1">
        <span class="text-xs font-bold uppercase tracking-wide text-zinc-500">قوائمي</span>
        <button
          @click="creating = !creating"
          title="إنشاء قائمة جديدة"
          class="flex h-6 w-6 items-center justify-center rounded-md text-zinc-400 transition hover:bg-ink-700 hover:text-white"
        >
          <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.2">
            <path d="M12 5v14M5 12h14" stroke-linecap="round" />
          </svg>
        </button>
      </div>

      <!-- نموذج إنشاء قائمة -->
      <div v-if="creating" class="mx-2 mb-2 rounded-lg border border-white/10 bg-ink-800 p-2">
        <input
          v-model="newName"
          @keyup.enter="create"
          type="text"
          placeholder="اسم القائمة..."
          class="w-full rounded-md border border-white/10 bg-ink-900 px-2.5 py-1.5 text-sm outline-none focus:border-accent-500/60"
        />
        <div class="mt-2 flex gap-2">
          <button @click="create" class="flex-1 rounded-md bg-accent-500 py-1.5 text-xs font-bold text-white hover:bg-accent-400">
            إنشاء
          </button>
          <button @click="creating = false; newName = ''" class="flex-1 rounded-md border border-white/10 py-1.5 text-xs text-zinc-400 hover:text-white">
            إلغاء
          </button>
        </div>
      </div>

      <!-- قوائم المستخدم -->
      <button
        v-for="l in store.userLists"
        :key="l.id"
        @click="openCollection('user', l.id, l.name)"
        class="group flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition"
        :class="store.activeCollection?.kind === 'user' && store.activeCollection?.key === l.id ? 'bg-ink-700 text-white' : 'text-zinc-300 hover:bg-ink-800 hover:text-white'"
      >
        <svg viewBox="0 0 24 24" class="h-5 w-5 shrink-0" fill="none" stroke="currentColor" stroke-width="1.8">
          <path d="M4 6h16M4 12h16M4 18h10" stroke-linecap="round" />
        </svg>
        <span class="min-w-0 truncate">{{ l.name }}</span>
        <span class="ms-auto flex items-center gap-1">
          <span class="rounded-full bg-white/10 px-2 py-0.5 text-xs text-zinc-400">{{ l.items.length }}</span>
          <span
            @click.stop="deleteUserList(l.id)"
            title="حذف القائمة"
            class="hidden h-5 w-5 items-center justify-center rounded text-zinc-500 transition hover:text-red-400 group-hover:flex"
          >
            <svg viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M6 6l12 12M18 6 6 18" stroke-linecap="round" />
            </svg>
          </span>
        </span>
      </button>

      <p v-if="!store.userLists.length && !creating" class="px-3 py-2 text-xs leading-relaxed text-zinc-600">
        لا توجد قوائم بعد — أنشئ قائمة من الزر أعلاه وأضف إليها الأعمال بزر «+ قائمة» في صفحة التفاصيل.
      </p>
    </div>
  </aside>
</template>
