import { createRouter, createWebHistory } from 'vue-router'

const Home = () => import('../views/Home.vue')
const Login = () => import('../views/Login.vue')
const Reader = () => import('../views/Reader.vue')
const LocalStore = () => import('../views/LocalStore.vue')
const Settings = () => import('../views/Settings.vue')

function sourceOverlayIntentFromLegacy(to) {
  if (to.query.panel === 'remote') return 'remote'
  if (['import', 'health', 'debug'].includes(to.query.action)) return to.query.action
  return 'manage'
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: Home },
    { path: '/login', name: 'login', component: Login },
    {
      path: '/search',
      name: 'search',
      redirect: to => ({
        path: '/',
        query: {
          ...to.query,
          workspace: 'search',
        },
      }),
    },
    {
      path: '/discover',
      name: 'discover',
      redirect: to => ({
        path: '/',
        query: {
          ...to.query,
          workspace: 'explore',
        },
      }),
    },
    { path: '/local-store', name: 'local-store', component: LocalStore },
    {
      path: '/sources',
      name: 'sources',
      redirect: to => {
        const { panel, action, ...query } = to.query
        return {
          path: '/',
          query: {
            ...query,
            overlay: 'sources',
            sourceAction: sourceOverlayIntentFromLegacy(to),
          },
        }
      },
    },
    { path: '/settings', name: 'settings', component: Settings },
    {
      path: '/books/:id',
      name: 'book-detail',
      redirect: to => ({
        path: '/',
        query: {
          ...to.query,
          bookInfo: to.params.id,
        },
      }),
    },
    { path: '/books/:id/read', name: 'reader', component: Reader },
  ],
})

router.beforeEach((to) => {
  const token = localStorage.getItem('openreader_token')
  if (!token && to.name !== 'login') {
    return { name: 'login' }
  }
  if (token && to.name === 'login') {
    return { name: 'home' }
  }
  return true
})

export default router
