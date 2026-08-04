<script setup lang="ts">
export interface RegionListItem {
  id: string;
  name: string;
  code: string;
}

const props = defineProps<{
  items: RegionListItem[];
  selectedId: string | null;
  emptyHint: string;
  maxShown?: number;
}>();

const emit = defineEmits<{ select: [id: string] }>();

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
        @click="emit('select', item.id)"
      >
        {{ item.name }}<span class="region-code">{{ item.code }}</span>
      </button>
      <p v-if="items.length > (maxShown ?? MAX_SHOWN_DEFAULT)" class="hint">
        {{ items.length - (maxShown ?? MAX_SHOWN_DEFAULT) }} weitere Treffer – Suche eingrenzen.
      </p>
    </template>
  </div>
</template>
