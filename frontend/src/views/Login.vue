<template>
  <div class="auth-page">
    <div class="auth-card">
      <h1 class="auth-title">闲读</h1>
      <p class="auth-sub">继续阅读</p>
      <AuthForm :reason="String(route.query.reason || '')" @success="handleSuccess" />
    </div>
  </div>
</template>

<script setup>
import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AuthForm from '../components/AuthForm.vue'
import { useUserStore } from '../stores/user'
import { safeReturnTo } from '../utils/authNavigation'
import { restoreShellDocumentTitle } from '../utils/documentTitle'

const router = useRouter()
const route = useRoute()
const user = useUserStore()

// The login page shows the shell title (S3), whichever view the tab came from.
onMounted(restoreShellDocumentTitle)

async function handleSuccess(result = {}) {
  if (result.previousScope && !result.sameAuthenticatedScope) {
    await router.replace({ name: 'home' })
    user.completeReauthentication()
    return
  }
  const returnTo = safeReturnTo(route.query.returnTo)
  await router.replace(returnTo)
  user.completeReauthentication()
}
</script>

<style scoped>
/* minmax(0, 1fr): the track follows the page width, so the card's max-width: 100% fits narrow screens. */
.auth-page { min-height: 100vh; display: grid; grid-template-columns: minmax(0, 1fr); place-items: center; background: #faf8f2; padding: 20px; }
.auth-card { background: #fff; border-radius: 12px; padding: 40px 36px; width: 400px; max-width: 100%; box-shadow: 0 2px 12px rgba(0,0,0,0.06); }
.auth-title { font-size: 28px; font-weight: 700; color: #1e293b; margin: 0 0 4px; text-align: center; }
.auth-sub { font-size: 14px; color: #94a3b8; text-align: center; margin: 0 0 28px; }
</style>
