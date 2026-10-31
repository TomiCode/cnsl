<script setup lang="ts">
import { useRoute } from 'vue-router'
import { Construction } from 'lucide-vue-next'

const route = useRoute()
</script>

<template>
  <div class="mx-auto max-w-4xl px-8 py-8">
    <h1 class="mb-6 text-xl font-semibold">{{ route.meta.title }}</h1>
    <div class="flex items-center gap-3 rounded-lg border border-dashed border-zinc-300 bg-white p-6 text-sm text-zinc-500">
      <Construction class="size-5" />
      <span v-if="route.meta.phase">Coming in phase {{ route.meta.phase }}.</span>
      <span v-else>This page does not exist.</span>
    </div>
  </div>
</template>
