<script setup lang="ts">
// Zeigt zu einer API-Abfrage den fertigen curl-Befehl und die direkte URL
// zum Kopieren - damit dieselben Daten, die die Seite anzeigt, ohne Umweg
// über die UI skriptbar sind. Wird sowohl im Download-Dialog (Regionen) als
// auch im Datenkatalog verwendet.
import { ref } from "vue";
import { copyToClipboard, curlCommand, type DownloadFormat } from "../api/download";

const props = defineProps<{
  // URL ohne format/download - das Format läuft im curl-Befehl über Accept.
  apiUrl: string;
  // URL für den Browser-Download (mit format=…&download=1).
  downloadUrl?: string;
  format: DownloadFormat;
  filename: string;
}>();

// null = noch nichts kopiert, false = Zwischenablage nicht verfügbar
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
