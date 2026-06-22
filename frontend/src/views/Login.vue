<template>
  <div class="auth-page">
    <div class="auth-card">
      <h1 class="auth-title">OpenReader</h1>
      <p class="auth-sub">继续阅读</p>
      <AuthForm :reason="String(route.query.reason || '')" @success="handleSuccess" />
    </div>
  </div>
</template>

<script setup>
import { useRoute, useRouter } from 'vue-router'
import AuthForm from '../components/AuthForm.vue'

const router = useRouter()
const route = useRoute()

async function handleSuccess() {
  const returnTo = String(route.query.returnTo || '')
  await router.replace(returnTo.startsWith('/') && !returnTo.startsWith('//') ? returnTo : { name: 'home' })
}
</script>

<style scoped>
.auth-page { min-height: 100vh; display: grid; place-items: center; background: #faf8f2; padding: 20px; }
.auth-card { background: #fff; border-radius: 12px; padding: 40px 36px; width: 400px; max-width: 100%; box-shadow: 0 2px 12px rgba(0,0,0,0.06); }
.auth-title { font-size: 28px; font-weight: 700; color: #1e293b; margin: 0 0 4px; text-align: center; }
.auth-sub { font-size: 14px; color: #94a3b8; text-align: center; margin: 0 0 28px; }
</style>
