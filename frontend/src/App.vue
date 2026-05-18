<template>
  <template v-if="isReader">
    <router-view />
    <GlobalOverlayHost v-if="isLoggedIn" />
  </template>

  <template v-else-if="isLoggedIn">
    <AppLayout>
      <router-view />
    </AppLayout>
    <GlobalOverlayHost />
  </template>

  <router-view v-else />
</template>

<script setup>
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from './layouts/AppLayout.vue'
import GlobalOverlayHost from './components/GlobalOverlayHost.vue'
import { useUserStore } from './stores/user'

const route = useRoute()
const userStore = useUserStore()

const isLoggedIn = computed(() => !!userStore.token)
const isReader = computed(() => route.name === 'reader')

onMounted(() => {
  if (userStore.token && !userStore.profile) {
    userStore.loadMe().catch(() => {})
  }
})
</script>
