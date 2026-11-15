<script setup lang="ts">
export interface RegionListItem {
  id: string;
  name: string;
  code: string;
  // Optional type badge (e.g. "NUTS-3", "LAU", "Agentur") for the
  // combined region list in RegionBrowser.vue, which shows all three
  // area types at once and distinguishes them via a filter.
  badge?: string;
}

const props = defineProps<{
  items: RegionListItem[];
  selectedId: string | null;
  emptyHint: string;
  maxShown?: number;
}>();

// Returns the whole item (not just the id): the combined list also needs the
// area type at the selection point to dispatch to the right
// select*() function in RegionBrowser.vue.
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
