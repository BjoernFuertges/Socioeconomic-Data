<script setup lang="ts">
import { computed } from "vue";
import type { DatasetMetadataEntry } from "../types/api";

// Zeigt Herkunft/Lizenz/Beschreibung eines Datensatzes sowie die persistente URI. Lizenz "unbekannt" wird bewusst sichtbar
// dargestellt statt versteckt.
const props = defineProps<{
  metadata: DatasetMetadataEntry;
}>();

const coverage = computed(() => {
  const { min_year, max_year } = props.metadata;
  if (!min_year || !max_year) return null;
  return min_year === max_year ? `${min_year}` : `${min_year}–${max_year}`;
});

const licenseKnown = computed(
  () => props.metadata.license.trim().toLowerCase() !== "unbekannt",
);
</script>

<template>
  <details class="dataset-info">
    <summary>
      Über diesen Datensatz
      <span
        class="license-badge"
        :class="{ 'is-unknown': !licenseKnown }"
        :title="licenseKnown ? metadata.license : 'Lizenz noch nicht recherchiert'"
      >
        Lizenz: {{ metadata.license }}
      </span>
    </summary>

    <dl>
      <template v-if="metadata.description">
        <dt>Beschreibung</dt>
        <dd>{{ metadata.description }}</dd>
      </template>

      <template v-if="metadata.provenance">
        <dt>Herkunft</dt>
        <dd>{{ metadata.provenance }}</dd>
      </template>

      <template v-if="metadata.unit">
        <dt>Einheit</dt>
        <dd>{{ metadata.unit }}</dd>
      </template>

      <template v-if="coverage">
        <dt>Abdeckung</dt>
        <dd>{{ coverage }}<span v-if="metadata.total_records"> · {{ metadata.total_records }} Datensätze</span></dd>
      </template>

      <dt>Lizenz</dt>
      <dd>
        <a v-if="metadata.license_url" :href="metadata.license_url" target="_blank" rel="noopener noreferrer">{{
          metadata.license
        }}</a>
        <span v-else>{{ metadata.license }}</span>
      </dd>

      <dt>Persistente ID</dt>
      <dd><code>{{ metadata.dataset_uri }}</code></dd>
    </dl>
  </details>
</template>
