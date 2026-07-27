<template>
  <el-dialog
    v-model="user.authDialogVisible"
    :title="user.authReason === 'session' ? '重新登录' : '登录'"
    width="420px"
    append-to-body
    destroy-on-close
    :close-on-click-modal="user.authReason !== 'session'"
    :close-on-press-escape="user.authReason !== 'session'"
    :show-close="user.authReason !== 'session'"
    class="auth-dialog"
  >
    <AuthForm :reason="user.authReason" @success="handleSuccess" />
  </el-dialog>
</template>

<script setup>
import { useRouter } from 'vue-router'
import AuthForm from './AuthForm.vue'
import { useUserStore } from '../stores/user'

const user = useUserStore()
const router = useRouter()

async function handleSuccess(result = {}) {
  if (!result.sameAuthenticatedScope) {
    await router.replace({ name: 'home' })
  }
  user.completeReauthentication()
}
</script>

<style>
.auth-dialog {
  max-width: calc(100vw - 28px);
}
</style>
