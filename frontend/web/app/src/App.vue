<script setup lang="ts">
import { onMounted, ref } from "vue";
import { apiGet, API_BASE } from "./api/client";
import RegionBrowser from "./components/RegionBrowser.vue";
import MapView from "./components/MapView.vue";
import DatasetCatalog from "./components/DatasetCatalog.vue";

const docsUrl = import.meta.env.VITE_DOCS_URL ?? "";

type ViewKey = "regionen" | "karte" | "katalog";
const activeView = ref<ViewKey>("regionen");

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
          :class="{ 'is-active': activeView === 'regionen' }"
          role="tab"
          :aria-selected="activeView === 'regionen'"
          @click="activeView = 'regionen'"
        >
          Regionen
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
        <button
          type="button"
          class="nav-tab"
          :class="{ 'is-active': activeView === 'katalog' }"
          role="tab"
          :aria-selected="activeView === 'katalog'"
          @click="activeView = 'katalog'"
        >
          Datenkatalog
        </button>
      </nav>
      <a v-if="docsUrl" class="docs-link" :href="docsUrl" target="_blank" rel="noopener">Dokumentation ↗</a>
    </div>
  </header>

  <main id="app-root">
    <p v-if="apiStatus" class="api-status" :class="{ 'error-text': apiStatus.isError }" role="status">
      {{ apiStatus.message }}
    </p>

    <section v-show="activeView === 'regionen'" class="view" :class="{ 'is-active': activeView === 'regionen' }">
      <RegionBrowser />
    </section>

    <section v-show="activeView === 'karte'" class="view" :class="{ 'is-active': activeView === 'karte' }">
      <MapView />
    </section>

    <section v-show="activeView === 'katalog'" class="view" :class="{ 'is-active': activeView === 'katalog' }">
      <DatasetCatalog />
    </section>
  </main>

  <footer class="app-footer">
    <p>
      Datenquelle: Go-API über <code>{{ API_BASE }}</code>. Weitere Endpunkte und Hintergrund zur Datenpipeline
      stehen in der Projektdokumentation (Link oben rechts, sofern konfiguriert).
    </p>
  </footer>
</template>
