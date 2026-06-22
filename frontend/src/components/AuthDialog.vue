<template>
  <el-dialog
    v-model="user.authDialogVisible"
    :title="user.authReason === 'session' ? '重新登录' : '登录'"
    width="420px"
    append-to-body
    destroy-on-close
    class="auth-dialog"
  >
    <AuthForm :reason="user.authReason" @success="handleSuccess" />
  </el-dialog>
</template>

<script setup>
import AuthForm from './AuthForm.vue'
import { useUserStore } from '../stores/user'

const user = useUserStore()

function handleSuccess() {
  window.location.reload()
}
</script>

<style>
.auth-dialog {
  max-width: calc(100vw - 28px);
}
</style>
