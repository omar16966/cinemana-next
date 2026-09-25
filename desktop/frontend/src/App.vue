<script setup>
// =============================================================================
// App.vue — هيكل التطبيق (App Shell):
//   النافذة ارتفاعها الثابت (h-screen) والتمرير العمودي يحدث **فقط**
//   في عمود المحتوى الأوسط. الأقسام الثابتة:
//     - الشريط العلوي (الشعار + البحث)
//     - لوحة الفلترة (يمين) والشريط الجانبي (يسار) — بتمرير داخلي خاص
//     - شريط الأسفل (الإعدادات + الحقوق) — ظاهر دائماً دون تمرير
// =============================================================================
import { onMounted } from 'vue'
import { store, applyCardSizeVar } from './store.js'
import TopBar from './components/TopBar.vue'
import Sidebar from './components/Sidebar.vue'
import FilterPanel from './components/FilterPanel.vue'
import HomeView from './components/HomeView.vue'
import DetailsView from './components/DetailsView.vue'
import CollectionView from './components/CollectionView.vue'
import PlayerView from './components/PlayerView.vue'
import ErrorToast from './components/ErrorToast.vue'
import SettingsModal from './components/SettingsModal.vue'
import Footer from './components/Footer.vue'

onMounted(() => applyCardSizeVar())
</script>

<template>
  <!-- الجذر: ارتفاع النافذة بالكامل بلا تمرير — كل تمرير داخلي للأعمدة -->
  <div class="flex h-screen flex-col overflow-hidden bg-ink-950 font-sans text-zinc-200">
    <TopBar v-if="store.view !== 'player'" />

    <!-- الصف الأوسط: الفلترة (يمين) — المحتوى المتحرك — القوائم (يسار) -->
    <div v-if="store.view !== 'player'" class="flex w-full min-h-0 flex-1 items-stretch">
      <FilterPanel v-if="store.view === 'home'" />

      <!-- عمود المحتوى: بلا تمرير هنا — كل شاشة تدير تمريرها الداخلي
           (التفاصيل مثلاً: الحلقات وحدها تتحرك) -->
      <main class="min-w-0 flex-1 overflow-hidden px-4 pt-4 md:px-6">
        <Transition name="fade" mode="out-in">
          <HomeView v-if="store.view === 'home'" key="home" />
          <DetailsView v-else-if="store.view === 'details'" key="details" />
          <CollectionView v-else-if="store.view === 'collection'" key="collection" />
        </Transition>
      </main>

      <Sidebar v-if="store.view !== 'player'" />
    </div>

    <!-- شريط الأسفل: ثابت دائماً أسفل النافذة -->
    <Footer v-if="store.view !== 'player'" />

    <!-- المشغل بملء الشاشة (خارج التخطيط العادي) -->
    <PlayerView v-if="store.view === 'player'" />

    <ErrorToast />
    <SettingsModal v-if="store.settingsOpen" />
  </div>
</template>
