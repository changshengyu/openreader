<template>
  <div>
    <el-alert
      v-if="reason === 'session'"
      title="登录状态已失效，请重新登录"
      type="warning"
      :closable="false"
      show-icon
      class="auth-alert"
    />

    <el-form @submit.prevent="submit" label-position="top" size="large">
      <el-form-item label="用户名">
        <el-input v-model="username" placeholder="请输入用户名" autocomplete="username" />
      </el-form-item>
      <el-form-item label="密码">
        <el-input
          v-model="password"
          type="password"
          placeholder="请输入密码"
          autocomplete="current-password"
          show-password
        />
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
</template>

<script setup>
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useUserStore } from '../stores/user'

defineProps({
  reason: {
    type: String,
    default: '',
  },
})

const emit = defineEmits(['success'])
const user = useUserStore()
const username = ref('')
const password = ref('')
const mode = ref('login')
const loading = ref(false)

function toggleMode() {
  mode.value = mode.value === 'login' ? 'register' : 'login'
}

async function submit() {
  loading.value = true
  try {
    await user.login(username.value, password.value, mode.value)
    emit('success')
  } catch (err) {
    ElMessage.error(err?.response?.data?.error?.message || err?.response?.data?.error || '请求失败')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.auth-alert {
  margin-bottom: 20px;
}

.auth-btn {
  width: 100%;
}

.auth-switch {
  margin-top: 12px;
  text-align: center;
}
</style>
