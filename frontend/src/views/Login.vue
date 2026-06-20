<template>
  <div class="auth-page">
    <div class="auth-card">
      <h1 class="auth-title">OpenReader</h1>
      <p class="auth-sub">继续阅读</p>
      <el-alert
        v-if="route.query.reason === 'session'"
        title="登录状态已失效，请重新登录"
        type="warning"
        :closable="false"
        show-icon
        class="auth-alert"
      />

      <el-form @submit.prevent="submit" label-position="top" size="large">
        <el-form-item label="用户名">
          <el-input v-model="username" placeholder="请输入用户名" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="password" type="password" placeholder="请输入密码" show-password />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" native-type="submit" class="auth-btn">
            {{ mode === 'login' ? '登录' : '注册' }}
          </el-button>
        </el-form-item>
      </el-form>

      <div class="auth-switch">
        <el-button link type="primary" @click="toggleMode">
          {{ mode === 'login' ? '创建新账号' : '已有账号，去登录' }}
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useUserStore } from '../stores/user'

const router = useRouter()
const route = useRoute()
const user = useUserStore()
const username = ref('')
const password = ref('')
const mode = ref('login')
const loading = ref(false)

function toggleMode() { mode.value = mode.value === 'login' ? 'register' : 'login' }

async function submit() {
  loading.value = true
  try {
    await user.login(username.value, password.value, mode.value)
    const returnTo = String(route.query.returnTo || '')
    await router.replace(returnTo.startsWith('/') && !returnTo.startsWith('//') ? returnTo : { name: 'home' })
  } catch (err) {
    ElMessage.error(err?.response?.data?.error?.message || '请求失败')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.auth-page { min-height: 100vh; display: grid; place-items: center; background: #faf8f2; padding: 20px; }
.auth-card { background: #fff; border-radius: 12px; padding: 40px 36px; width: 400px; max-width: 100%; box-shadow: 0 2px 12px rgba(0,0,0,0.06); }
.auth-title { font-size: 28px; font-weight: 700; color: #1e293b; margin: 0 0 4px; text-align: center; }
.auth-sub { font-size: 14px; color: #94a3b8; text-align: center; margin: 0 0 28px; }
.auth-alert { margin-bottom: 20px; }
.auth-btn { width: 100%; }
.auth-switch { text-align: center; margin-top: 12px; }
</style>
