import { createApp } from 'vue'
import { createPinia } from 'pinia'
import persistedState from 'pinia-plugin-persistedstate'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
// Before the components, like reader-dev's bundle: component styles win ties with it.
import './styles/night-element.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'

import App from './App.vue'
import router from './router'
import { setupServiceWorker } from './utils/serviceWorker'
import './styles/global.css'
import './styles/reader-iconfont.css'

const app = createApp(App)
const pinia = createPinia()
pinia.use(persistedState)

app.use(pinia)
app.use(router)
app.use(ElementPlus, { locale: zhCn })

app.mount('#app')

// Offline shell (PWA contract W1): production builds in secure contexts only.
void setupServiceWorker({ isProduction: import.meta.env.PROD })
