<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { TriangleAlert } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import { unwrap } from '@/stores/devices'

const info = ref<Schemas['SystemInfo'] | null>(null)
const settings = ref<Schemas['Settings'] | null>(null)
const error = ref<string | null>(null)
const saved = ref(false)

onMounted(async () => {
  try {
    const [sys, set] = await Promise.all([api.GET('/system'), api.GET('/settings')])
    info.value = unwrap(sys)
    settings.value = unwrap(set)
  } catch (e) {
    error.value = (e as Error).message
  }
})

async function setLanguage(language: string) {
  error.value = null
  saved.value = false
  try {
    settings.value = unwrap(await api.PUT('/settings', { body: { ...settings.value!, language } }))
    saved.value = true
  } catch (e) {
    error.value = (e as Error).message
  }
}
</script>

<template>
  <div class="mx-auto max-w-3xl space-y-6 px-8 py-8">
    <h1 class="text-xl font-semibold">Settings</h1>
    <p v-if="error" class="text-sm text-red-600">{{ error }}</p>

    <div v-if="info && settings" class="rounded-lg border border-zinc-200 bg-white p-5">
      <div class="mb-1 text-sm font-medium">Screens</div>
      <p class="mb-4 text-xs text-zinc-500">
        Language of day and month names, relative times, weather conditions and built-in component labels. A screen can override it in the
        editor.
      </p>
      <label class="flex items-center gap-3 text-sm">
        <span class="text-zinc-500">Language</span>
        <select
          :value="settings.language"
          class="rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none"
          @change="setLanguage(($event.target as HTMLSelectElement).value)"
        >
          <option v-for="l in info.languages" :key="l.code" :value="l.code">{{ l.name }}</option>
        </select>
        <span v-if="saved" class="text-xs text-emerald-600">Saved — re-rendering screens</span>
      </label>
    </div>

    <div v-if="info" class="rounded-lg border border-zinc-200 bg-white p-5">
      <div class="mb-1 text-sm font-medium">Server</div>
      <p class="mb-4 text-xs text-zinc-500">Configured with CNSL_* environment variables; editing here comes later.</p>
      <dl class="grid grid-cols-[auto_1fr] gap-x-6 gap-y-3 text-sm">
        <dt class="text-zinc-500">Version</dt>
        <dd>{{ info.version }}</dd>
        <dt class="text-zinc-500">Timezone</dt>
        <dd>{{ info.timezone }}</dd>
        <dt class="text-zinc-500">Device URL</dt>
        <dd>
          <span v-if="info.device_public_url" class="font-mono">{{ info.device_public_url }}</span>
          <span v-else class="flex items-center gap-1.5 text-amber-700">
            <TriangleAlert class="size-4" />
            Not set — real devices can't download images. Set CNSL_DEVICE_PUBLIC_URL.
          </span>
          <p class="mt-1 text-xs text-zinc-400">Enter this as the custom server URL in the TRMNL's Wi-Fi setup.</p>
        </dd>
      </dl>
    </div>
  </div>
</template>
