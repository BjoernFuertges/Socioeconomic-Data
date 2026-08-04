<script setup lang="ts">
import { onMounted, ref } from "vue";
import { apiGet, API_BASE } from "./api/client";
import NutsBrowser from "./components/NutsBrowser.vue";
import LauBrowser from "./components/LauBrowser.vue";
import MapView from "./components/MapView.vue";

const docsUrl = import.meta.env.VITE_DOCS_URL ?? "";

type ViewKey = "nuts" | "lau" | "karte";
const activeView = ref<ViewKey>("nuts");

const apiStatus = ref<{ message: string; isError: boolean } | null>(null);

onMounted(async () => {
  try {
    await apiGet("/health");
  } catch (err) {
    apiStatus.value = {
      message: `API unter ${API_BASE} nicht erreichbar: ${err instanceof Error ? err.message : String(err)}`,
      isError: true,
    };
  }
});
</script>

<template>
  <header class="app-header">
    <div class="header-inner">
      <h1>Sozioökonomische Daten</h1>
      <nav class="main-nav" role="tablist" aria-label="Ansicht wählen">
        <button
          type="button"
          class="nav-tab"
          :class="{ 'is-active': activeView === 'nuts' }"
          role="tab"
          :aria-selected="activeView === 'nuts'"
          @click="activeView = 'nuts'"
        >
          NUTS-3 Regionen
        </button>
        <button
          type="button"
          class="nav-tab"
          :class="{ 'is-active': activeView === 'lau' }"
          role="tab"
          :aria-selected="activeView === 'lau'"
          @click="activeView = 'lau'"
        >
          Gemeinden (LAU)
        </button>
        <button
          type="button"
          class="nav-tab"
          :class="{ 'is-active': activeView === 'karte' }"
          role="tab"
          :aria-selected="activeView === 'karte'"
          @click="activeView = 'karte'"
        >
          Karte
        </button>
      </nav>
      <a v-if="docsUrl" class="docs-link" :href="docsUrl" target="_blank" rel="noopener">Dokumentation ↗</a>
    </div>
  </header>

  <main id="app-root">
    <p v-if="apiStatus" class="api-status" :class="{ 'error-text': apiStatus.isError }" role="status">
      {{ apiStatus.message }}
    </p>

    <section v-show="activeView === 'nuts'" class="view" :class="{ 'is-active': activeView === 'nuts' }">
      <NutsBrowser />
    </section>

    <section v-show="activeView === 'lau'" class="view" :class="{ 'is-active': activeView === 'lau' }">
      <LauBrowser />
    </section>

    <section v-show="activeView === 'karte'" class="view" :class="{ 'is-active': activeView === 'karte' }">
      <MapView />
    </section>
  </main>

  <footer class="app-footer">
    <p>
      Datenquelle: Go-API über <code>{{ API_BASE }}</code>. Weitere Endpunkte und Hintergrund zur Datenpipeline
      stehen in der Projektdokumentation (Link oben rechts, sofern konfiguriert).
    </p>
  </footer>
</template>
