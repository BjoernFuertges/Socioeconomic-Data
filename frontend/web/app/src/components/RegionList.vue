<script setup lang="ts">
export interface RegionListItem {
  id: string;
  name: string;
  code: string;
  // Optionales Typ-Badge (z.B. "NUTS-3", "LAU", "Agentur") - für die
  // kombinierte Regionen-Liste in RegionBrowser.vue, die alle drei
  // Gebietstypen gleichzeitig zeigt und per Filter unterscheidet.
  badge?: string;
}

const props = defineProps<{
  items: RegionListItem[];
  selectedId: string | null;
  emptyHint: string;
  maxShown?: number;
}>();

// Gibt das komplette Item zurück (nicht nur die id) - die kombinierte Liste
// braucht am Auswahlpunkt auch den Gebietstyp, um an die richtige
// select*()-Funktion in RegionBrowser.vue zu dispatchen.
const emit = defineEmits<{ select: [item: RegionListItem] }>();

const MAX_SHOWN_DEFAULT = 300;
</script>

<template>
  <div class="region-list" aria-live="polite">
    <p v-if="!items.length" class="hint">{{ emptyHint }}</p>
    <template v-else>
      <button
        v-for="item in items.slice(0, maxShown ?? MAX_SHOWN_DEFAULT)"
        :key="item.id"
        type="button"
        class="region-item"
        :class="{ 'is-selected': item.id === props.selectedId }"
        @click="emit('select', item)"
      >
        {{ item.name }}<span class="region-code">{{ item.code }}</span>
        <span v-if="item.badge" class="region-badge">{{ item.badge }}</span>
      </button>
      <p v-if="items.length > (maxShown ?? MAX_SHOWN_DEFAULT)" class="hint">
        {{ items.length - (maxShown ?? MAX_SHOWN_DEFAULT) }} weitere Treffer – Suche eingrenzen.
      </p>
    </template>
  </div>
</template>
