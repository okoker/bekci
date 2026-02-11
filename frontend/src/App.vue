<script setup>
import { useRoute } from 'vue-router'
import { computed } from 'vue'
import DefaultLayout from './components/DefaultLayout.vue'

const route = useRoute()
const isPublic = computed(() => route.meta.public === true)
</script>

<template>
  <component :is="isPublic ? 'div' : DefaultLayout">
    <router-view />
  </component>
</template>
