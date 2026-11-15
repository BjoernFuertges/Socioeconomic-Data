<script setup lang="ts">
// Shows the ready-made curl command and the direct URL for an API query, to copy,
// so the data the page displays can be scripted without going through the UI.
// Used in both the download dialog (regions) and the dataset catalog.
import { ref } from "vue";
import { copyToClipboard, curlCommand, type DownloadFormat } from "../api/download";

const props = defineProps<{
  // URL without format/download - the format goes via Accept in the curl command.
  apiUrl: string;
  // URL for the browser download (with format=…&download=1).
  downloadUrl?: string;
  format: DownloadFormat;
  filename: string;
}>();

// null = nothing copied yet, false = clipboard unavailable
const copied = ref<Record<string, boolean | null>>({ curl: null, url: null });

async function copy(kind: "curl" | "url", text: string) {
  copied.value = { ...copied.value, [kind]: await copyToClipboard(text) };
  setTimeout(() => (copied.value = { ...copied.value, [kind]: null }), 2500);
}
</script>

<template>
  <div class="api-snippet">
    <div class="api-snippet-row">
      <code>{{ curlCommand(props.apiUrl, props.format, props.filename) }}</code>
      <button
        type="button"
        class="link-button"
        @click="copy('curl', curlCommand(props.apiUrl, props.format, props.filename))"
      >
        {{ copied.curl === true ? "Kopiert ✓" : copied.curl === false ? "Kopieren nicht möglich" : "curl kopieren" }}
      </button>
    </div>
    <div class="api-snippet-row">
      <code>{{ props.downloadUrl ?? props.apiUrl }}</code>
      <button type="button" class="link-button" @click="copy('url', props.downloadUrl ?? props.apiUrl)">
        {{ copied.url === true ? "Kopiert ✓" : copied.url === false ? "Kopieren nicht möglich" : "Link kopieren" }}
      </button>
    </div>
  </div>
</template>
