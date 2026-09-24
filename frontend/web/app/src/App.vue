<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import { apiGet, API_BASE } from "./api/client";

const docsUrl = import.meta.env.VITE_DOCS_URL ?? "";

const route = useRoute();

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
        <RouterLink
          :to="{ name: 'regionen' }"
          class="nav-tab"
          :class="{ 'is-active': route.name === 'regionen' }"
          role="tab"
          :aria-selected="route.name === 'regionen'"
        >
          Regionen
        </RouterLink>
        <RouterLink
          :to="{ name: 'karte' }"
          class="nav-tab"
          :class="{ 'is-active': route.name === 'karte' }"
          role="tab"
          :aria-selected="route.name === 'karte'"
        >
          Karte
        </RouterLink>
        <RouterLink
          :to="{ name: 'eigene-region' }"
          class="nav-tab"
          :class="{ 'is-active': route.name === 'eigene-region' }"
          role="tab"
          :aria-selected="route.name === 'eigene-region'"
        >
          Eigene Region
        </RouterLink>
        <RouterLink
          :to="{ name: 'katalog' }"
          class="nav-tab"
          :class="{ 'is-active': route.name === 'katalog' }"
          role="tab"
          :aria-selected="route.name === 'katalog'"
        >
          Datenkatalog
        </RouterLink>
      </nav>
      <a v-if="docsUrl" class="docs-link" :href="docsUrl" target="_blank" rel="noopener">Dokumentation ↗</a>
    </div>
  </header>

  <main id="app-root">
    <p v-if="apiStatus" class="api-status" :class="{ 'error-text': apiStatus.isError }" role="status">
      {{ apiStatus.message }}
    </p>

    <RouterView v-slot="{ Component }">
      <KeepAlive>
        <component :is="Component" />
      </KeepAlive>
    </RouterView>
  </main>

  <footer class="app-footer">
    <p>
      Datenquelle: Go-API über <code>{{ API_BASE }}</code>. Weitere Endpunkte und Hintergrund zur Datenpipeline
      stehen in der Projektdokumentation (Link oben rechts, sofern konfiguriert).
    </p>
  </footer>
</template>
