<script setup lang="ts">
import logoUrl from '@/assets/logo.png'

withDefaults(defineProps<{ size?: number; alt?: string }>(), {
  size: 36,
  alt: 'OWPanel',
})
</script>

<template>
  <img
    :src="logoUrl"
    :alt="alt"
    class="app-logo"
    :width="size"
    :height="size"
    decoding="async"
  />
</template>

<style scoped>
.app-logo {
  display: block;
  object-fit: cover;
  border-radius: 22%;
  flex-shrink: 0;
}
</style>
